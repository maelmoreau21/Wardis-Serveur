package main

import (
	"context"
	"errors"
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
	"wardis-server/internal/auth"
	"wardis-server/internal/config"
	"wardis-server/internal/database"
	"wardis-server/internal/intrusion"
	"wardis-server/internal/logger"
	"wardis-server/internal/video"
	"wardis-server/internal/events"
)

func main() {
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

	log.Info("Starting Wardis Server...", zap.String("env", cfg.Env), zap.String("port", cfg.Port))

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

	// 5. Initialize Auth Components
	authRepo := auth.NewRepository(dbPool)
	authService := auth.NewService(authRepo, cfg.JWTSecret, cfg.JWTExpiry)
	authHandler := auth.NewHandler(authService, log, cfg.CookieSecure)

	// 5b. Initialize Access Control Components
	acRepo := accesscontrol.NewRepository(dbPool)
	acService := accesscontrol.NewService(acRepo, natsPub, log)
	acHandler := accesscontrol.NewHandler(acService, log)

	// 5c. Initialize Video Components
	videoRepo := video.NewRepository(dbPool)
	videoMtxClient := video.NewMediaMtxClient(cfg.MediaMtxAPIURL)
	videoService := video.NewService(videoRepo, videoMtxClient, cfg.JWTSecret, log)
	videoHandler := video.NewHandler(videoService, log)

	// 5d. Initialize Intrusion Components
	intrusionRepo := intrusion.NewRepository(dbPool)
	intrusionService := intrusion.NewService(intrusionRepo, intrusionNatsPub, log)
	intrusionHandler := intrusion.NewHandler(intrusionService, log)

	// 5e. Initialize Events Components
	eventsRepo := events.NewRepository(dbPool)
	eventsService := events.NewService(eventsRepo, cfg.NatsURL, log)
	eventsHandler := events.NewHandler(eventsService, log)

	// Start NATS subscriber for Events
	if err := eventsService.Start(ctx); err != nil {
		log.Error("Failed to start NATS event subscriber", zap.Error(err))
	}
	defer eventsService.Close()

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

	// Public Routes
	r.Post("/login", authHandler.Login)
	r.Post("/logout", authHandler.Logout)
	r.Post("/cameras/auth", videoHandler.MediaMtxAuth)

	// Protected Routes
	r.Group(func(r chi.Router) {
		r.Use(auth.AuthMiddleware(authService))
		r.Get("/me", authHandler.Me)

		// Access Control Routes
		r.Get("/doors", acHandler.ListDoors)
		r.Post("/doors/{id}/swipe", acHandler.SwipeBadge)

		// Video Routes
		r.Get("/cameras", videoHandler.ListCameras)
		r.Get("/cameras/{id}", videoHandler.GetCameraByID)
		r.Post("/cameras/{id}/token", videoHandler.GenerateStreamToken)
		r.Get("/cameras/active-streams", videoHandler.ListActiveStreams)

		// Intrusion Routes
		r.Get("/zones", intrusionHandler.ListZones)
		r.Post("/zones/{id}/arm", intrusionHandler.ArmZone)
		r.Post("/zones/{id}/disarm", intrusionHandler.DisarmZone)
		r.Get("/capteurs", intrusionHandler.ListSensors)
		r.Post("/capteurs/{id}/trigger", intrusionHandler.TriggerSensor)
		r.Get("/alarmes/active", intrusionHandler.ListActiveAlarms)

		// Events Routes
		r.Get("/events", eventsHandler.ListEvents)
		r.Get("/events/stream", eventsHandler.StreamEvents)

		// Admin only access control routes
		r.Group(func(r chi.Router) {
			r.Use(auth.RequireRole("admin"))
			r.Post("/badges/assign", acHandler.AssignBadge)
			r.Post("/doors/{id}/open", acHandler.OpenDoor)
			r.Post("/doors/{id}/close", acHandler.CloseDoor)
			r.Get("/access-logs", acHandler.ListAccessLogs)

			// Video Admin Routes
			r.Post("/cameras", videoHandler.CreateCamera)
			r.Put("/cameras/{id}", videoHandler.UpdateCamera)
			r.Delete("/cameras/{id}", videoHandler.DeleteCamera)
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
	adminEmail := "admin@wardis.com"
	adminPassword := "password"

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
