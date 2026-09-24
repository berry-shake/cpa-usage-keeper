package test

import (
	"slices"
	"testing"
	_ "unsafe"

	_ "cpa-usage-keeper/internal/app"
	"cpa-usage-keeper/internal/config"
)

//go:linkname frameAncestorOrigins cpa-usage-keeper/internal/app.frameAncestorOrigins
func frameAncestorOrigins(config.Config) []string

func TestFrameAncestorsUsesCPAPublicURLOrigin(t *testing.T) {
	cases := []struct {
		name      string
		publicURL string
		want      []string
	}{
		{
			name:      "absolute public URL",
			publicURL: "https://my-cliproxy.zeabur.app/cpa/",
			want:      []string{"https://my-cliproxy.zeabur.app"},
		},
		{
			name:      "http LAN public URL with port",
			publicURL: "http://10.34.44.12:8317/cpa/management.html",
			want:      []string{"http://10.34.44.12:8317"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.Config{CPABaseURL: "https://private-cpa.internal", CPAPublicURL: tc.publicURL}

			origins := frameAncestorOrigins(cfg)
			if !slices.Equal(origins, tc.want) {
				t.Fatalf("expected frame ancestor origins %#v, got %#v", tc.want, origins)
			}
		})
	}
}

func TestFrameAncestorsMergesConfiguredOrigins(t *testing.T) {
	cases := []struct {
		name      string
		publicURL string
		extra     []string
		want      []string
	}{
		{
			name:      "extra origins only",
			publicURL: "",
			extra:     []string{"https://cpamc.example.com", "http://10.0.0.2:18317"},
			want:      []string{"https://cpamc.example.com", "http://10.0.0.2:18317"},
		},
		{
			name:      "public URL origin first then extras",
			publicURL: "https://cpa.example.com/cpa/",
			extra:     []string{"https://cpamc.example.com"},
			want:      []string{"https://cpa.example.com", "https://cpamc.example.com"},
		},
		{
			name:      "duplicate of public URL origin removed",
			publicURL: "https://cpa.example.com/cpa/",
			extra:     []string{"https://cpa.example.com", "https://cpamc.example.com"},
			want:      []string{"https://cpa.example.com", "https://cpamc.example.com"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.Config{
				CPABaseURL:           "https://private-cpa.internal",
				CPAPublicURL:         tc.publicURL,
				FrameAncestorOrigins: tc.extra,
			}

			origins := frameAncestorOrigins(cfg)
			if !slices.Equal(origins, tc.want) {
				t.Fatalf("expected frame ancestor origins %#v, got %#v", tc.want, origins)
			}
		})
	}
}

func TestFrameAncestorsNeverFallsBackToCPABaseURL(t *testing.T) {
	for _, publicURL := range []string{"", "/cpa/", "ftp://cpa.example.com", "cpa.example.com:8443/", "//cpa.example.com"} {
		t.Run("public URL "+publicURL, func(t *testing.T) {
			cfg := config.Config{CPABaseURL: "https://private-cpa.internal", CPAPublicURL: publicURL}

			origins := frameAncestorOrigins(cfg)
			if len(origins) != 0 {
				t.Fatalf("expected no extra frame ancestor origins, got %#v", origins)
			}
		})
	}
}
