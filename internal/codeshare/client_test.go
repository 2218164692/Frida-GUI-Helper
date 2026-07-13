package codeshare

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseBrowseHTML(t *testing.T) {
	body := []byte(`<!doctype html><html><body>
<article>
  <h2><a href="https://codeshare.frida.re/@alice/demo-hook/">Demo Hook</a></h2>
  <h3>12 | 3K</h3>
  <h4>Uploaded by: <a href="/@alice/">@alice</a></h4>
  <p>Demo description</p>
</article>
<a href="?page=7">7</a>
</body></html>`)

	items, totalPages, err := parseBrowseHTML(body, defaultBaseURL)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Ref != "alice/demo-hook" {
		t.Fatalf("unexpected items: %#v", items)
	}
	if items[0].Likes != 12 || items[0].Views != "3K" || totalPages != 7 {
		t.Fatalf("unexpected stats/pages: %#v pages=%d", items[0], totalPages)
	}
}

func TestParseBrowseHTMLAllowsEmptySearch(t *testing.T) {
	body := []byte(`<html><body><article><h2>No results found for "missing"</h2><p>Try a different search term.</p></article></body></html>`)
	items, totalPages, err := parseBrowseHTML(body, defaultBaseURL)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 || totalPages != 1 {
		t.Fatalf("unexpected empty result: items=%#v pages=%d", items, totalPages)
	}
}

func TestSearchFallsBackToCache(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<article><h2><a href="/@alice/demo/">Demo</a></h2><h3>1 | 2K</h3><p>cached</p></article>`)
	}))
	cacheDir := t.TempDir()
	client := newClient(server.URL, cacheDir, filepath.Join(t.TempDir(), "trust.json"))

	first, err := client.Search(context.Background(), "demo", 1)
	if err != nil || first.Source != "online" {
		t.Fatalf("first search failed: result=%#v err=%v", first, err)
	}
	server.Close()

	second, err := client.Search(context.Background(), "demo", 1)
	if err != nil {
		t.Fatal(err)
	}
	if second.Source != "cache" || second.CachedAt == "" || !strings.Contains(second.Warning, "已回退") {
		t.Fatalf("expected cache fallback, got %#v", second)
	}
}

func TestProjectTrustDetectsSourceChange(t *testing.T) {
	source := "console.log('one');"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"id":"1","project_name":"Demo","description":"test","owner":"alice","slug":"demo","frida_version":"16.0.0","likes":2,"source":%q}`, source)
	}))
	defer server.Close()

	client := newClient(server.URL, t.TempDir(), filepath.Join(t.TempDir(), "trust.json"))
	project, err := client.GetProject(context.Background(), "alice/demo")
	if err != nil {
		t.Fatal(err)
	}
	if project.TrustState != "new" {
		t.Fatalf("expected new trust state, got %q", project.TrustState)
	}
	if err := client.Trust(project.Ref, project.Fingerprint); err != nil {
		t.Fatal(err)
	}

	project, err = client.GetProject(context.Background(), "alice/demo")
	if err != nil || project.TrustState != "trusted" {
		t.Fatalf("expected trusted state: project=%#v err=%v", project, err)
	}

	source = "console.log('two');"
	project, err = client.GetProject(context.Background(), "alice/demo")
	if err != nil {
		t.Fatal(err)
	}
	if project.TrustState != "changed" || !strings.Contains(project.Warning, "不同") {
		t.Fatalf("expected changed state, got %#v", project)
	}
}

func TestSearchReportsRateLimitWithoutCache(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()
	client := newClient(server.URL, t.TempDir(), filepath.Join(t.TempDir(), "trust.json"))

	_, err := client.Search(context.Background(), "demo", 1)
	if err == nil || !strings.Contains(err.Error(), "HTTP 429") || !strings.Contains(err.Error(), "没有可用缓存") {
		t.Fatalf("unexpected error: %v", err)
	}
}
