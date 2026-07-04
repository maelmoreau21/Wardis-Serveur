package config

import (
	"strings"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	Env                 string        `mapstructure:"ENV"`
	Port                string        `mapstructure:"PORT"`
	DatabaseURL         string        `mapstructure:"DATABASE_URL"`
	JWTSecret           string        `mapstructure:"JWT_SECRET"`
	JWTExpiry           time.Duration `mapstructure:"JWT_EXPIRY"`
	CookieSecure        bool          `mapstructure:"COOKIE_SECURE"`
	NatsURL             string        `mapstructure:"NATS_URL"`
	MediaMtxAPIURL      string        `mapstructure:"MEDIAMTX_API_URL"`
	LoginRateLimitRate  float64       `mapstructure:"LOGIN_RATE_LIMIT_RATE"`
	LoginRateLimitBurst float64       `mapstructure:"LOGIN_RATE_LIMIT_BURST"`

	// MinIO & Video Tiering configuration
	MinioEndpoint        string        `mapstructure:"MINIO_ENDPOINT"`
	MinioAccessKey       string        `mapstructure:"MINIO_ACCESS_KEY"`
	MinioSecretKey       string        `mapstructure:"MINIO_SECRET_KEY"`
	MinioUseSSL          bool          `mapstructure:"MINIO_USE_SSL"`
	MinioBucket          string        `mapstructure:"MINIO_BUCKET"`
	VideoTieringInterval time.Duration `mapstructure:"VIDEO_TIERING_INTERVAL"`
	VideoTieringAgeDays  int           `mapstructure:"VIDEO_TIERING_AGE_DAYS"`
	RecordingsLocalDir   string        `mapstructure:"RECORDINGS_LOCAL_DIR"`
}

func Load() (*Config, error) {
	viper.SetDefault("ENV", "development")
	viper.SetDefault("PORT", "8080")
	viper.SetDefault("DATABASE_URL", "postgres://wardis_user:wardis_password@localhost:5432/wardis_db?sslmode=disable")
	viper.SetDefault("JWT_SECRET", "super-secret-key-replace-in-production")
	viper.SetDefault("JWT_EXPIRY", 24*time.Hour)
	viper.SetDefault("COOKIE_SECURE", false)
	viper.SetDefault("NATS_URL", "nats://localhost:4222")
	viper.SetDefault("MEDIAMTX_API_URL", "http://localhost:9997")
	viper.SetDefault("LOGIN_RATE_LIMIT_RATE", 0.1)
	viper.SetDefault("LOGIN_RATE_LIMIT_BURST", 5.0)

	viper.SetDefault("MINIO_ENDPOINT", "localhost:9000")
	viper.SetDefault("MINIO_ACCESS_KEY", "minioadmin")
	viper.SetDefault("MINIO_SECRET_KEY", "minioadminpassword")
	viper.SetDefault("MINIO_USE_SSL", false)
	viper.SetDefault("MINIO_BUCKET", "wardis-video-recordings")
	viper.SetDefault("VIDEO_TIERING_INTERVAL", 1*time.Hour)
	viper.SetDefault("VIDEO_TIERING_AGE_DAYS", 7)
	viper.SetDefault("RECORDINGS_LOCAL_DIR", "./data/recordings")

	// Read environment variables
	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	// Also support reading from a .env file if it exists
	viper.AddConfigPath(".")
	viper.AddConfigPath("./deploy")
	viper.SetConfigName(".env")
	viper.SetConfigType("env")

	if err := viper.ReadInConfig(); err != nil {
		// Ignore ConfigFileNotFoundError, just use env vars or defaults
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, err
		}
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}
