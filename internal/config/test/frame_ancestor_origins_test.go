package test

import (
	"reflect"
	"testing"

	"cpa-usage-keeper/internal/config"
	"cpa-usage-keeper/internal/cpa"
)

func TestLoadFromEnvParsesFrameAncestorOrigins(t *testing.T) {
	isolateConfigEnv(t)
	t.Setenv("CPA_BASE_URL", "http://127.0.0.1:"+cpa.ManagementRedisDefaultPort)
	t.Setenv("CPA_MANAGEMENT_KEY", "secret")
	t.Setenv("FRAME_ANCESTOR_ORIGINS", " https://cpamc.example.com , http://10.0.0.2:18317/manager/ ,https://cpamc.example.com")

	cfg, err := config.LoadFromEnv()
	if err != nil {
		t.Fatalf("LoadFromEnv returned error: %v", err)
	}

	want := []string{"https://cpamc.example.com", "http://10.0.0.2:18317"}
	if !reflect.DeepEqual(cfg.FrameAncestorOrigins, want) {
		t.Fatalf("expected frame ancestor origins %#v, got %#v", want, cfg.FrameAncestorOrigins)
	}
}

func TestLoadFromEnvDefaultsFrameAncestorOriginsEmpty(t *testing.T) {
	isolateConfigEnv(t)
	t.Setenv("CPA_BASE_URL", "http://127.0.0.1:"+cpa.ManagementRedisDefaultPort)
	t.Setenv("CPA_MANAGEMENT_KEY", "secret")

	cfg, err := config.LoadFromEnv()
	if err != nil {
		t.Fatalf("LoadFromEnv returned error: %v", err)
	}

	if len(cfg.FrameAncestorOrigins) != 0 {
		t.Fatalf("expected no frame ancestor origins by default, got %#v", cfg.FrameAncestorOrigins)
	}
}

func TestLoadFromEnvRejectsInvalidFrameAncestorOrigins(t *testing.T) {
	for _, invalid := range []string{"cpamc.example.com", "/manager/", "ftp://cpamc.example.com", "//cpamc.example.com"} {
		t.Run("origin "+invalid, func(t *testing.T) {
			isolateConfigEnv(t)
			t.Setenv("CPA_BASE_URL", "http://127.0.0.1:"+cpa.ManagementRedisDefaultPort)
			t.Setenv("CPA_MANAGEMENT_KEY", "secret")
			t.Setenv("FRAME_ANCESTOR_ORIGINS", invalid)

			if _, err := config.LoadFromEnv(); err == nil {
				t.Fatalf("expected LoadFromEnv to reject FRAME_ANCESTOR_ORIGINS %q", invalid)
			}
		})
	}
}
