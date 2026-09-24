package test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"cpa-usage-keeper/internal/updatecheck"
)

func TestCheckerComparesForkVersionWithLatestRelease(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/Willxup/cpa-usage-keeper/releases/latest" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name":"v1.2.4"}`))
	}))
	defer server.Close()

	checker := updatecheck.NewChecker("v1.2.3-fork.1", updatecheck.WithBaseURL(server.URL), updatecheck.WithHTTPClient(server.Client()))
	result, err := checker.Check(context.Background())
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}

	if result.CurrentVersion != "v1.2.3-fork.1" {
		t.Fatalf("CurrentVersion = %q, want v1.2.3-fork.1", result.CurrentVersion)
	}
	if result.LatestVersion != "v1.2.4" {
		t.Fatalf("LatestVersion = %q, want v1.2.4", result.LatestVersion)
	}
	if !result.CanCompare {
		t.Fatalf("CanCompare = false, want true")
	}
	if !result.UpdateAvailable {
		t.Fatalf("UpdateAvailable = false, want true")
	}
}
