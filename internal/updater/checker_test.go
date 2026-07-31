package updater

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		left  string
		right string
		want  int
	}{
		{"v1.0.8", "1.0.7", 1},
		{"17.9.11", "17.9.11", 0},
		{"1.0.0-alpha", "1.0.0", -1},
		{"1.0.0-alpha.2", "1.0.0-alpha.10", -1},
		{"1.0.0-beta", "1.0.0-alpha", 1},
		{"1.0.0+build.2", "1.0.0+build.1", 0},
		{"999999999999999999999.0.0", "2.0.0", 1},
	}
	for _, test := range tests {
		t.Run(test.left+"_"+test.right, func(t *testing.T) {
			got, err := CompareVersions(test.left, test.right)
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("CompareVersions(%q, %q) = %d, want %d", test.left, test.right, got, test.want)
			}
		})
	}
}

func TestCompareVersionsRejectsInvalidInput(t *testing.T) {
	for _, value := range []string{"", "1.2", "1.02.3", "1.2.3-01", "1.2.3+bad!", "release-1.2.3"} {
		if _, err := CompareVersions(value, "1.0.0"); err == nil {
			t.Fatalf("CompareVersions accepted %q", value)
		}
	}
}

func TestCheckApplication(t *testing.T) {
	published := "2026-07-30T12:34:56Z"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/"+ApplicationRepository+"/releases/latest" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if got := r.Header.Get("User-Agent"); got != defaultUserAgent {
			t.Errorf("User-Agent = %q", got)
		}
		if got := r.Header.Get("Accept"); got != "application/vnd.github+json" {
			t.Errorf("Accept = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"tag_name":"v1.0.8","html_url":"https://github.com/2218164692/Frida-GUI-Helper/releases/tag/v1.0.8","published_at":%q}`, published)
	}))
	defer server.Close()

	checker := NewCheckerWithClient(server.Client())
	checker.apiBase = server.URL
	checker.now = func() time.Time { return time.Date(2026, 8, 1, 1, 2, 3, 0, time.UTC) }
	status, err := checker.CheckApplication(context.Background(), "1.0.7")
	if err != nil {
		t.Fatal(err)
	}
	if status.Repository != ApplicationRepository || status.Component != ComponentApplication {
		t.Fatalf("unexpected component status: %#v", status)
	}
	if status.CurrentVersion != "1.0.7" || status.LatestVersion != "1.0.8" || !status.UpdateAvailable {
		t.Fatalf("unexpected version status: %#v", status)
	}
	if status.ReleaseURL == "" || status.PublishedAt != published {
		t.Fatalf("release metadata missing: %#v", status)
	}
	if status.CheckedAt != checker.now().Format(time.RFC3339) {
		t.Fatalf("CheckedAt = %v", status.CheckedAt)
	}
}

func TestCheckFridaWithoutInstalledVersionReportsUpdate(t *testing.T) {
	server := releaseServer(t, FridaRepository, "17.9.11")
	defer server.Close()
	checker := NewCheckerWithClient(server.Client())
	checker.apiBase = server.URL

	status, err := checker.CheckFrida(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if !status.UpdateAvailable || status.LatestVersion != "17.9.11" {
		t.Fatalf("unexpected status: %#v", status)
	}
}

func TestCheckRejectsOversizedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(strings.Repeat("x", maxResponseBytes+1)))
	}))
	defer server.Close()
	checker := NewCheckerWithClient(server.Client())
	checker.apiBase = server.URL

	status, err := checker.CheckApplication(context.Background(), "1.0.0")
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("error = %v, want response size error", err)
	}
	if status.Error == "" {
		t.Fatal("status did not retain the check error")
	}
}

func TestCheckRejectsHTTPFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()
	checker := NewCheckerWithClient(server.Client())
	checker.apiBase = server.URL

	if _, err := checker.CheckFrida(context.Background(), "17.9.11"); err == nil || !strings.Contains(err.Error(), "HTTP 403") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewCheckerAppliesTimeoutWithoutMutatingClient(t *testing.T) {
	client := &http.Client{}
	checker := NewCheckerWithClient(client)
	if checker.client.Timeout != requestTimeout {
		t.Fatalf("checker timeout = %v, want %v", checker.client.Timeout, requestTimeout)
	}
	if client.Timeout != 0 {
		t.Fatalf("caller's client was mutated: %v", client.Timeout)
	}
}

func TestCheckRejectsUnknownComponent(t *testing.T) {
	checker := NewChecker()
	status, err := checker.Check(context.Background(), Component("other"), "1.0.0")
	if err == nil {
		t.Fatal("unknown component was accepted")
	}
	if status.Error == "" {
		t.Fatal("status did not retain the component error")
	}
}

func releaseServer(t *testing.T, repository, version string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/"+repository+"/releases/latest" {
			t.Errorf("path = %q", r.URL.Path)
		}
		fmt.Fprintf(w, `{"tag_name":%q,"html_url":"https://github.com/%s/releases/tag/%s","published_at":"2026-07-30T00:00:00Z"}`, version, repository, version)
	}))
}
