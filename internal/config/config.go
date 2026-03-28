package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds all runtime configuration for PRS.
type Config struct {
	Port                string
	DataDir             string
	MaxUploadBytes      int64
	Workers             int
	RetentionDays       int
	CleanupInterval     time.Duration
	DiskExpansionFactor float64
	BaseURL             string
	LogLevel            string // "debug", "info", "error" — default "info"
	LogFormat           string // "text", "json" — default "text"
}

// Load reads configuration from environment variables, applying defaults.
func Load() (*Config, error) {
	c := &Config{
		Port:                getEnv("PRS_PORT", "3912"),
		DataDir:             getEnv("PRS_DATA_DIR", "./data"),
		MaxUploadBytes:      2684354560, // 2.5 GB
		Workers:             2,
		RetentionDays:       5,
		CleanupInterval:     time.Hour,
		DiskExpansionFactor: 1.5,
		BaseURL:             getEnv("PRS_BASE_URL", "http://localhost:3912"),
		LogLevel:            getEnv("PRS_LOG_LEVEL", "info"),
		LogFormat:           getEnv("PRS_LOG_FORMAT", "text"),
	}

	if v := os.Getenv("PRS_MAX_UPLOAD_BYTES"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n <= 0 {
			return nil, fmt.Errorf("PRS_MAX_UPLOAD_BYTES: invalid value %q", v)
		}
		c.MaxUploadBytes = n
	}

	if v := os.Getenv("PRS_WORKERS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return nil, fmt.Errorf("PRS_WORKERS: invalid value %q", v)
		}
		c.Workers = n
	}

	if v := os.Getenv("PRS_RETENTION_DAYS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return nil, fmt.Errorf("PRS_RETENTION_DAYS: invalid value %q", v)
		}
		c.RetentionDays = n
	}

	if v := os.Getenv("PRS_CLEANUP_INTERVAL"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d <= 0 {
			return nil, fmt.Errorf("PRS_CLEANUP_INTERVAL: invalid value %q", v)
		}
		c.CleanupInterval = d
	}

	if v := os.Getenv("PRS_DISK_EXPANSION_FACTOR"); v != "" {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil || f <= 0 {
			return nil, fmt.Errorf("PRS_DISK_EXPANSION_FACTOR: invalid value %q", v)
		}
		c.DiskExpansionFactor = f
	}

	return c, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
