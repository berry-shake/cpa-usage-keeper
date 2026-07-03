package test

import (
	"os"
	"testing"
	"time"

	"cpa-usage-keeper/internal/config"
	"cpa-usage-keeper/internal/cpa"
)

var cleanupConfigEnvKeys = []string{
	"APP_PORT", "APP_BASE_PATH", "CPA_PUBLIC_URL", "WORK_DIR", "CPA_BASE_URL", "CPA_MANAGEMENT_KEY",
	"REDIS_QUEUE_ADDR", "REDIS_QUEUE_TLS", "REDIS_QUEUE_BATCH_SIZE", "REDIS_QUEUE_IDLE_INTERVAL",
	"BACKUP_ENABLED", "BACKUP_INTERVAL", "BACKUP_RETENTION_DAYS", "CLEANUP_USAGE_EVENTS_ENABLED",
	"REQUEST_TIMEOUT", "LOG_LEVEL", "LOG_FILE_ENABLED", "LOG_DIR", "LOG_RETENTION_DAYS",
	"AUTH_ENABLED", "LOGIN_PASSWORD", "AUTH_SESSION_TTL", "TZ", "TLS_ENABLED", "TLS_CERT_FILE", "TLS_KEY_FILE",
	"TLS_SKIP_VERIFY", "QUOTA_REFRESH_WORKER_LIMIT",
}

func TestLoadFromEnvDefaultsUsageEventCleanupDisabled(t *testing.T) {
	isolateCleanupConfigEnv(t)
	t.Setenv("CPA_BASE_URL", "http://127.0.0.1:"+cpa.ManagementRedisDefaultPort)
	t.Setenv("CPA_MANAGEMENT_KEY", "secret")

	cfg, err := config.LoadFromEnv()
	if err != nil {
		t.Fatalf("LoadFromEnv returned error: %v", err)
	}

	if cfg.CleanupUsageEventsEnabled {
		t.Fatal("expected usage_events cleanup to be disabled by default")
	}
}

func TestLoadFromEnvReadsUsageEventCleanupFlag(t *testing.T) {
	isolateCleanupConfigEnv(t)
	t.Setenv("CPA_BASE_URL", "http://127.0.0.1:"+cpa.ManagementRedisDefaultPort)
	t.Setenv("CPA_MANAGEMENT_KEY", "secret")
	t.Setenv("CLEANUP_USAGE_EVENTS_ENABLED", "true")

	cfg, err := config.LoadFromEnv()
	if err != nil {
		t.Fatalf("LoadFromEnv returned error: %v", err)
	}

	if !cfg.CleanupUsageEventsEnabled {
		t.Fatal("expected usage_events cleanup to be enabled")
	}
}

func isolateCleanupConfigEnv(t *testing.T) {
	t.Helper()
	previousLocal := time.Local
	previousEnv := make(map[string]string, len(cleanupConfigEnvKeys))
	previousPresent := make(map[string]bool, len(cleanupConfigEnvKeys))
	for _, key := range cleanupConfigEnvKeys {
		previousEnv[key], previousPresent[key] = os.LookupEnv(key)
		if err := os.Unsetenv(key); err != nil {
			t.Fatalf("unset %s: %v", key, err)
		}
	}
	t.Cleanup(func() {
		time.Local = previousLocal
		for _, key := range cleanupConfigEnvKeys {
			if previousPresent[key] {
				if err := os.Setenv(key, previousEnv[key]); err != nil {
					t.Fatalf("restore %s: %v", key, err)
				}
				continue
			}
			if err := os.Unsetenv(key); err != nil {
				t.Fatalf("unset %s: %v", key, err)
			}
		}
	})
}
