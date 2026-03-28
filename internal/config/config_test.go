package config

import (
	"testing"
	"time"
)

func TestDefaults(t *testing.T) {
	// t.Setenv restores the original value automatically after the test.
	t.Setenv("PRS_PORT", "")
	t.Setenv("PRS_DATA_DIR", "")
	t.Setenv("PRS_MAX_UPLOAD_BYTES", "")
	t.Setenv("PRS_WORKERS", "")
	t.Setenv("PRS_RETENTION_DAYS", "")
	t.Setenv("PRS_CLEANUP_INTERVAL", "")
	t.Setenv("PRS_DISK_EXPANSION_FACTOR", "")
	t.Setenv("PRS_BASE_URL", "")
	t.Setenv("PRS_LOG_LEVEL", "")
	t.Setenv("PRS_LOG_FORMAT", "")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Port != "3912" {
		t.Errorf("Port: want 3912, got %s", cfg.Port)
	}
	if cfg.DataDir != "./data" {
		t.Errorf("DataDir: want ./data, got %s", cfg.DataDir)
	}
	if cfg.MaxUploadBytes != 2684354560 {
		t.Errorf("MaxUploadBytes: want 2684354560, got %d", cfg.MaxUploadBytes)
	}
	if cfg.Workers != 2 {
		t.Errorf("Workers: want 2, got %d", cfg.Workers)
	}
	if cfg.RetentionDays != 5 {
		t.Errorf("RetentionDays: want 5, got %d", cfg.RetentionDays)
	}
	if cfg.CleanupInterval != time.Hour {
		t.Errorf("CleanupInterval: want 1h, got %v", cfg.CleanupInterval)
	}
	if cfg.DiskExpansionFactor != 1.5 {
		t.Errorf("DiskExpansionFactor: want 1.5, got %v", cfg.DiskExpansionFactor)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel: want info, got %s", cfg.LogLevel)
	}
	if cfg.LogFormat != "text" {
		t.Errorf("LogFormat: want text, got %s", cfg.LogFormat)
	}
}

func TestEnvVarOverrides(t *testing.T) {
	t.Setenv("PRS_PORT", "9000")
	t.Setenv("PRS_WORKERS", "4")
	t.Setenv("PRS_RETENTION_DAYS", "10")
	t.Setenv("PRS_CLEANUP_INTERVAL", "30m")
	t.Setenv("PRS_DISK_EXPANSION_FACTOR", "2.0")
	t.Setenv("PRS_LOG_LEVEL", "debug")
	t.Setenv("PRS_LOG_FORMAT", "json")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Port != "9000" {
		t.Errorf("Port: want 9000, got %s", cfg.Port)
	}
	if cfg.Workers != 4 {
		t.Errorf("Workers: want 4, got %d", cfg.Workers)
	}
	if cfg.RetentionDays != 10 {
		t.Errorf("RetentionDays: want 10, got %d", cfg.RetentionDays)
	}
	if cfg.CleanupInterval != 30*time.Minute {
		t.Errorf("CleanupInterval: want 30m, got %v", cfg.CleanupInterval)
	}
	if cfg.DiskExpansionFactor != 2.0 {
		t.Errorf("DiskExpansionFactor: want 2.0, got %v", cfg.DiskExpansionFactor)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel: want debug, got %s", cfg.LogLevel)
	}
	if cfg.LogFormat != "json" {
		t.Errorf("LogFormat: want json, got %s", cfg.LogFormat)
	}
}

func TestInvalidValues(t *testing.T) {
	cases := []struct {
		env string
		val string
	}{
		{"PRS_MAX_UPLOAD_BYTES", "not-a-number"},
		{"PRS_MAX_UPLOAD_BYTES", "-1"},
		{"PRS_WORKERS", "0"},
		{"PRS_WORKERS", "abc"},
		{"PRS_RETENTION_DAYS", "-1"},
		{"PRS_CLEANUP_INTERVAL", "bad-duration"},
		{"PRS_DISK_EXPANSION_FACTOR", "0"},
		{"PRS_DISK_EXPANSION_FACTOR", "abc"},
	}

	for _, c := range cases {
		t.Run(c.env+"="+c.val, func(t *testing.T) {
			t.Setenv(c.env, c.val)
			_, err := Load()
			if err == nil {
				t.Errorf("expected error for %s=%s, got nil", c.env, c.val)
			}
		})
	}
}
