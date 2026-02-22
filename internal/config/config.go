package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds all runtime configuration loaded from environment variables.
// Every field has a sensible default; only DATABASE_URL is required.
type Config struct {
	// Server
	HTTPPort        string
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	ShutdownTimeout time.Duration

	// Database
	DatabaseURL string
	DBMaxConns  int32
	DBMinConns  int32

	// External provider
	ProviderBaseURL string
	ProviderTimeout time.Duration

	// Worker counts (one worker pool is shared across all channel types)
	SMSWorkers   int
	EmailWorkers int
	PushWorkers  int

	// Rate limiting: maximum requests per second per channel
	RateLimit int

	// Retry backoff durations: index 0 = first retry delay, etc.
	RetryBackoff []time.Duration

	// Background worker poll intervals
	SchedulerInterval time.Duration
	RetryInterval     time.Duration
}

func Load() (*Config, error) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}

	return &Config{
		HTTPPort:        getEnv("HTTP_PORT", "8080"),
		ReadTimeout:     getDuration("READ_TIMEOUT", 5*time.Second),
		WriteTimeout:    getDuration("WRITE_TIMEOUT", 10*time.Second),
		ShutdownTimeout: getDuration("SHUTDOWN_TIMEOUT", 30*time.Second),

		DatabaseURL: dbURL,
		DBMaxConns:  int32(getInt("DB_MAX_CONNS", 25)),
		DBMinConns:  int32(getInt("DB_MIN_CONNS", 5)),

		ProviderBaseURL: getEnv("PROVIDER_BASE_URL", "https://webhook.site/your-uuid-here"),
		ProviderTimeout: getDuration("PROVIDER_TIMEOUT", 10*time.Second),

		SMSWorkers:   getInt("SMS_WORKERS", 5),
		EmailWorkers: getInt("EMAIL_WORKERS", 5),
		PushWorkers:  getInt("PUSH_WORKERS", 5),

		RateLimit: getInt("RATE_LIMIT_PER_CHANNEL", 100),

		RetryBackoff: []time.Duration{
			getDuration("RETRY_BACKOFF_1", 5*time.Second),
			getDuration("RETRY_BACKOFF_2", 30*time.Second),
			getDuration("RETRY_BACKOFF_3", 120*time.Second),
		},

		SchedulerInterval: getDuration("SCHEDULER_INTERVAL", 5*time.Second),
		RetryInterval:     getDuration("RETRY_INTERVAL", 10*time.Second),
	}, nil
}

func getEnv(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func getInt(key string, defaultVal int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return defaultVal
}

func getDuration(key string, defaultVal time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return defaultVal
}
