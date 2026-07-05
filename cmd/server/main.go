package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/lib/pq"
	"go.uber.org/zap"

	"wardis-server/internal/accesscontrol"
	"wardis-server/internal/audit"
	"wardis-server/internal/auth"
	"wardis-server/internal/config"
	"wardis-server/internal/database"
	"wardis-server/internal/events"
	"wardis-server/internal/health"
	"wardis-server/internal/intrusion"
	"wardis-server/internal/logger"
	"wardis-server/internal/ratelimit"
	"wardis-server/internal/video"
	"wardis-server/internal/layout"
)

const serverVersion = "0.0.1"

func main() {
	// Command-line flags
	createUser := flag.Bool("create-user", false, "Create a user and exit")
	userName := flag.String("username", "", "Username of the user")
	userPass := flag.String("password", "", "Password of the user")
	userRole := flag.String("role", "admin", "Role of the user (admin/user)")
	flag.Parse()

	if *createUser {
		if *userName == "" || *userPass == "" {
			fmt.Println("Error: username and password are required to create a user.")
			os.Exit(1)
		}

		// 1. Load configuration
		cfg, err := config.Load()
		if err != nil {
			panic("failed to load configuration: " + err.Error())
		}

		// 2. Initialize logger
		log, err := logger.Init(cfg.Env)
		if err != nil {
			panic("failed to initialize logger: " + err.Error())
		}
		defer log.Sync()

		ctx := context.Background()
		dbPool, err := database.Init(ctx, cfg.DatabaseURL, log)
		if err != nil {
			log.Fatal("failed to initialize database connection pool", zap.Error(err))
		}
		defer dbPool.Close()

		authRepo := auth.NewRepository(dbPool)
		authService := auth.NewService(authRepo, cfg.JWTSecret, cfg.JWTExpiry)

		hashedPassword, err := authService.HashPassword(*userPass)
		if err != nil {
			log.Fatal("failed to hash password", zap.Error(err))
		}

		userObj, err := authRepo.CreateUser(ctx, *userName, hashedPassword, *userRole)
		if err != nil {
			log.Fatal("failed to create user", zap.Error(err))
		}

		fmt.Printf("Successfully created user: ID=%s Username=%s Role=%s\n", userObj.ID, userObj.Email, userObj.Role)
		os.Exit(0)
	}

	// 1. Load configuration
	cfg, err := config.Load()
	if err != nil {
		panic("failed to load configuration: " + err.Error())
	}

	// 2. Initialize logger
	log, err := logger.Init(cfg.Env)
	if err != nil {
		panic("failed to initialize logger: " + err.Error())
	}
	defer log.Sync()

	log.Info("Starting Wardis Server...", zap.String("env", cfg.Env), zap.String("port", cfg.Port), zap.String("version", serverVersion))

	// 3. Run database migrations
	log.Info("Running database migrations...")
	if err := runMigrations(cfg.DatabaseURL); err != nil {
		log.Fatal("database migration failed", zap.Error(err))
	}
	log.Info("Database migrations completed successfully")

	// 4. Initialize Database connection pool
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dbPool, err := database.Init(ctx, cfg.DatabaseURL, log)
	if err != nil {
		log.Fatal("failed to initialize database connection pool", zap.Error(err))
	}
	defer dbPool.Close()

	// 4b. Initialize NATS connection for Access Control
	log.Info("Connecting to NATS...", zap.String("url", cfg.NatsURL))
	natsPub, err := accesscontrol.NewNatsPublisher(cfg.NatsURL)
	if err != nil {
		log.Fatal("failed to initialize NATS publisher", zap.Error(err))
	}
	defer natsPub.Close()

	// 4c. Initialize NATS connection for Intrusion
	log.Info("Connecting to NATS for Intrusion...", zap.String("url", cfg.NatsURL))
	intrusionNatsPub, err := intrusion.NewNatsPublisher(cfg.NatsURL)
	if err != nil {
		log.Fatal("failed to initialize Intrusion NATS publisher", zap.Error(err))
	}
	defer intrusionNatsPub.Close()

	// 4g. Initialize NATS connection for Video
	log.Info("Connecting to NATS for Video...", zap.String("url", cfg.NatsURL))
	videoNatsPub, err := video.NewNatsPublisher(cfg.NatsURL)
	if err != nil {
		log.Fatal("failed to initialize Video NATS publisher", zap.Error(err))
	}
	defer videoNatsPub.Close()

	// 4d. Initialize Audit Logger
	auditLogger := audit.New(dbPool, log)

	// 4e. Initialize Rate Limiter
	loginRateLimiter := ratelimit.New(cfg.LoginRateLimitRate, cfg.LoginRateLimitBurst)
	defer loginRateLimiter.Close()

	// 4f. Initialize Health Handler
	healthHandler := health.NewHandler(dbPool, cfg.NatsURL)

	// 5. Initialize Auth Components
	authRepo := auth.NewRepository(dbPool)
	authService := auth.NewService(authRepo, cfg.JWTSecret, cfg.JWTExpiry)
	authHandler := auth.NewHandler(authService, log, cfg.CookieSecure, auditLogger)

	// 5b. Initialize Access Control Components
	acRepo := accesscontrol.NewRepository(dbPool)
	acService := accesscontrol.NewService(acRepo, natsPub, log)
	acHandler := accesscontrol.NewHandler(acService, log, auditLogger)

	// 5c. Initialize Video Components
	videoRepo := video.NewRepository(dbPool)
	videoMtxClient := video.NewMediaMtxClient(cfg.MediaMtxAPIURL)
	videoService := video.NewService(videoRepo, videoMtxClient, videoNatsPub, cfg.JWTSecret, log, cfg)
	videoHandler := video.NewHandler(videoService, log, auditLogger)

	// 5d. Initialize Intrusion Components
	intrusionRepo := intrusion.NewRepository(dbPool)
	intrusionService := intrusion.NewService(intrusionRepo, intrusionNatsPub, log)
	intrusionHandler := intrusion.NewHandler(intrusionService, log, auditLogger)

	// 5e. Initialize Events Components
	eventsRepo := events.NewRepository(dbPool)
	eventsService := events.NewService(eventsRepo, cfg.NatsURL, log)
	eventsHandler := events.NewHandler(eventsService, log)

	// 5f. Initialize Layout Components
	layoutRepo := layout.NewRepository(dbPool)
	layoutService := layout.NewService(layoutRepo, log)
	layoutHandler := layout.NewHandler(layoutService, log, auditLogger)

	// Start NATS subscriber for Events
	if err := eventsService.Start(ctx); err != nil {
		log.Error("Failed to start NATS event subscriber", zap.Error(err))
	}
	defer eventsService.Close()

	// Start Video Service Background Jobs (Tiering + NATS subscriber)
	if err := videoService.Start(ctx); err != nil {
		log.Error("Failed to start Video Service background tasks", zap.Error(err))
	}
	defer videoService.Close()

	// 6. Seed default Admin User if it doesn't exist
	seedDefaultAdmin(ctx, authService, authRepo, log)

	// 6b. Sync active cameras with MediaMTX
	if err := videoService.SyncWithMediaMTX(ctx); err != nil {
		log.Error("Failed to sync cameras with MediaMTX on startup", zap.Error(err))
	}

	// 7. Setup Router
	r := chi.NewRouter()

	// Global Middlewares
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)

	// CORS Middleware
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Credentials", "true")
			} else {
				w.Header().Set("Access-Control-Allow-Origin", "*")
			}
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding, Authorization, X-CSRF-Token")

			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	})

	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)
			log.Info("HTTP Request",
				zap.String("method", r.Method),
				zap.String("path", r.URL.Path),
				zap.Int("status", ww.Status()),
				zap.Duration("duration", time.Since(start)),
				zap.String("ip", r.RemoteAddr),
			)
		})
	})

	// Health & Readiness Routes
	r.Get("/health", healthHandler.Health)
	r.Get("/ready", healthHandler.Ready)

	// Public Routes (Rate limited)
	r.Group(func(r chi.Router) {
		r.Use(loginRateLimiter.Limit)
		r.Post("/login", authHandler.Login)
		r.Post("/login/mfa", authHandler.LoginMfa)
		r.Post("/logout", authHandler.Logout)
		r.Post("/cameras/auth", videoHandler.MediaMtxAuth)
	})

	// Protected Routes
	// Protected Routes
	r.Group(func(r chi.Router) {
		r.Use(auth.AuthMiddleware(authService))
		r.Get("/me", authHandler.Me)
		r.Put("/api/auth/me/password", authHandler.UpdatePassword)

		// MFA Routes
		r.Post("/api/auth/mfa/setup", authHandler.SetupMfa)
		r.Post("/api/auth/mfa/enable", authHandler.EnableMfa)
		r.Post("/api/auth/mfa/disable", authHandler.DisableMfa)

		// My Sessions Routes
		r.Get("/api/auth/me/sessions", authHandler.ListMySessions)
		r.Delete("/api/auth/me/sessions/{id}", authHandler.RevokeSession)

		// Access Control Routes
		r.Get("/doors", acHandler.ListDoors)
		r.Get("/doors/{id}", acHandler.GetDoorByID)
		r.Post("/doors/{id}/swipe", acHandler.SwipeBadge)
		r.Get("/sites", acHandler.ListSites)
		r.Get("/sites/{id}", acHandler.GetSiteByID)
		r.Get("/cardholders/{id}", acHandler.GetCardholderByID)

		// Video Routes
		r.Get("/api/v1/video/export", videoHandler.ExportVideo)
		r.Get("/cameras", videoHandler.ListCameras)
		r.Get("/cameras/{id}", videoHandler.GetCameraByID)
		r.Post("/cameras/{id}/token", videoHandler.GenerateStreamToken)
		r.Post("/cameras/{id}/whep", videoHandler.ConfigureWHEPStream)
		r.Post("/cameras/{id}/ptz", videoHandler.SendPTZCommand)
		r.Get("/cameras/active-streams", videoHandler.ListActiveStreams)
		r.Post("/cameras/{id}/sync", videoHandler.SyncRecording)
		r.Get("/cameras/{id}/recordings", videoHandler.GetRecordingsForTimeline)
		r.Post("/cameras/{id}/motion", videoHandler.TriggerMotion)
		r.Post("/cameras/{id}/status", videoHandler.ToggleCameraStatus)

		// Bookmarks Routes
		r.Get("/cameras/{id}/bookmarks", videoHandler.ListBookmarks)
		r.Post("/cameras/{id}/bookmarks", videoHandler.CreateBookmark)
		r.Get("/bookmarks/{id}", videoHandler.GetBookmarkByID)
		r.Put("/bookmarks/{id}", videoHandler.UpdateBookmark)
		r.Delete("/bookmarks/{id}", videoHandler.DeleteBookmark)

		// Intrusion Routes
		r.Get("/zones", intrusionHandler.ListZones)
		r.Get("/zones/{id}", intrusionHandler.GetZoneByID)
		r.Post("/zones/{id}/arm", intrusionHandler.ArmZone)
		r.Post("/zones/{id}/disarm", intrusionHandler.DisarmZone)
		r.Get("/capteurs", intrusionHandler.ListSensors)
		r.Get("/capteurs/{id}", intrusionHandler.GetSensorByID)
		r.Post("/capteurs/{id}/trigger", intrusionHandler.TriggerSensor)
		r.Get("/alarmes", intrusionHandler.ListAllAlarms)
		r.Get("/alarmes/active", intrusionHandler.ListActiveAlarms)
		r.Post("/alarmes/{id}/acquit", intrusionHandler.AcknowledgeAlarm)
		r.Post("/alarmes/{id}/transfer", intrusionHandler.TransferAlarm)
		r.Post("/alarmes/{id}/delay", intrusionHandler.SnoozeAlarm)

		// Events Routes
		r.Get("/events", eventsHandler.ListEvents)
		r.Get("/events/stream", eventsHandler.StreamEvents)
		r.Get("/events/ws", eventsHandler.StreamEventsWS)

		// Saved Views / Layouts Routes
		r.Get("/api/layouts", layoutHandler.List)
		r.Get("/api/layouts/{id}", layoutHandler.GetByID)
		r.Post("/api/layouts", layoutHandler.Create)
		r.Put("/api/layouts/{id}", layoutHandler.Update)
		r.Delete("/api/layouts/{id}", layoutHandler.Delete)

		// Admin only routes
		r.Group(func(r chi.Router) {
			r.Use(auth.RequireRole("admin"))
			r.Post("/badges/assign", acHandler.AssignBadge)
			r.Post("/doors/{id}/open", acHandler.OpenDoor)
			r.Post("/doors/{id}/close", acHandler.CloseDoor)
			r.Get("/access-logs", acHandler.ListAccessLogs)
			r.Get("/cardholders", acHandler.ListCardholders)
			r.Post("/cardholders", acHandler.CreateCardholder)
			r.Put("/cardholders/{id}", acHandler.UpdateCardholder)
			r.Delete("/cardholders/{id}", acHandler.DeleteCardholder)

			// Doors admin CRUD
			r.Post("/doors", acHandler.CreateDoor)
			r.Put("/doors/{id}", acHandler.UpdateDoor)
			r.Delete("/doors/{id}", acHandler.DeleteDoor)

			// Sites admin CRUD
			r.Post("/sites", acHandler.CreateSite)
			r.Put("/sites/{id}", acHandler.UpdateSite)
			r.Delete("/sites/{id}", acHandler.DeleteSite)

			// Intrusion zones admin CRUD
			r.Post("/zones", intrusionHandler.CreateZone)
			r.Put("/zones/{id}", intrusionHandler.UpdateZone)
			r.Delete("/zones/{id}", intrusionHandler.DeleteZone)

			// Intrusion sensors admin CRUD
			r.Post("/capteurs", intrusionHandler.CreateSensor)
			r.Put("/capteurs/{id}", intrusionHandler.UpdateSensor)
			r.Delete("/capteurs/{id}", intrusionHandler.DeleteSensor)

			// User Management Routes
			r.Get("/api/users", authHandler.ListUsers)
			r.Post("/api/users", authHandler.CreateUser)
			r.Delete("/api/users/{id}", authHandler.DeleteUser)
			r.Get("/api/roles", authHandler.ListRoles)
			r.Get("/api/permissions", authHandler.ListPermissions)
			r.Get("/api/users/{id}/permissions", authHandler.GetEntityPermissionsForUser)
			r.Post("/api/users/{id}/permissions", authHandler.SaveEntityPermissions)

			// Roles CRUD Admin Routes
			r.Post("/api/roles", authHandler.CreateRole)
			r.Put("/api/roles/{id}", authHandler.UpdateRole)
			r.Delete("/api/roles/{id}", authHandler.DeleteRole)

			// Sessions Admin Routes
			r.Get("/api/sessions", authHandler.ListActiveSessions)
			r.Delete("/api/sessions/{id}", authHandler.RevokeSession)

			// Audit Logs Route
			r.Get("/api/audit-logs", auditLogger.ListHandler)

			// Video Admin Routes
			r.Post("/cameras", videoHandler.CreateCamera)
			r.Put("/cameras/{id}", videoHandler.UpdateCamera)
			r.Delete("/cameras/{id}", videoHandler.DeleteCamera)
			r.Post("/cameras/discover", videoHandler.DiscoverCameras)
		})
	})

	// 8. Start HTTP Server with Graceful Shutdown
	server := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: r,
	}

	serverCtx, serverStopCtx := context.WithCancel(context.Background())

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	go func() {
		<-sig

		log.Info("Shutdown signal received. Shutting down server...")

		// Shutdown context with 30s timeout
		shutdownCtx, shutdownCancel := context.WithTimeout(serverCtx, 30*time.Second)
		defer shutdownCancel()

		go func() {
			<-shutdownCtx.Done()
			if shutdownCtx.Err() == context.DeadlineExceeded {
				log.Fatal("Graceful shutdown timed out.. forcing exit.")
			}
		}()

		// Trigger server shutdown
		err := server.Shutdown(shutdownCtx)
		if err != nil {
			log.Fatal("server shutdown error", zap.Error(err))
		}
		serverStopCtx()
	}()

	log.Info("Server listening", zap.String("addr", server.Addr))
	err = server.ListenAndServe()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal("HTTP server ListenAndServe error", zap.Error(err))
	}

	<-serverCtx.Done()
	log.Info("Server stopped gracefully")
}

func runMigrations(databaseURL string) error {
	m, err := migrate.New("file://migrations", databaseURL)
	if err != nil {
		return err
	}
	defer m.Close()

	err = m.Up()
	if err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return err
	}

	return nil
}

func seedDefaultAdmin(ctx context.Context, authService auth.Service, repo auth.Repository, log *zap.Logger) {
	adminEmail := "root"
	adminPassword := "root"

	_, err := repo.GetUserByEmail(ctx, adminEmail)
	if err == nil {
		log.Info("Default admin user already exists")
		return
	}

	if !errors.Is(err, auth.ErrUserNotFound) {
		log.Error("Failed to check if default admin exists", zap.Error(err))
		return
	}

	// Hash password
	hashedPassword, err := authService.HashPassword(adminPassword)
	if err != nil {
		log.Error("Failed to hash admin password", zap.Error(err))
		return
	}

	// Create user with 'admin' role
	adminUser, err := repo.CreateUser(ctx, adminEmail, hashedPassword, "admin")
	if err != nil {
		log.Error("Failed to seed default admin user", zap.Error(err))
		return
	}

	log.Info("Successfully seeded default admin user", zap.String("id", adminUser.ID), zap.String("email", adminEmail))
}
