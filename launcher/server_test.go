package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/renereose/varu/router"
)

// newDrive lays out a miniature drive in a temp folder.
func newDrive(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"app/index.html":        "<h1>home</h1>",
		"app/vendor/lib.mjs":    "export default 1;",
		"app/assets/readme.txt": "no index here",
		"app/.secret":           "hidden",
		"maps/ee.pmtiles":       "0123456789",
		"maps/._ee.pmtiles":     "resource fork",
		"pois/hospital.geojson": `{"type":"FeatureCollection","features":[]}`,
		"docs/guide.pdf":        "%PDF-1.4",
		"manifest.json":         `{"schema":1}`,
		"library/notes.txt":     "not served directly",
	}
	for name, body := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func get(t *testing.T, h http.Handler, target string, mod func(*http.Request)) (*http.Response, string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	req.Host = "localhost:8480"
	if mod != nil {
		mod(req)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	res := rec.Result()
	body, _ := io.ReadAll(res.Body)
	return res, string(body)
}

func TestServesAppAndData(t *testing.T) {
	h := NewHandler(Config{Root: newDrive(t), Version: "test"})

	cases := []struct {
		path, wantBody, wantType string
	}{
		{"/", "<h1>home</h1>", "text/html"},
		{"/vendor/lib.mjs", "export default 1;", "text/javascript"},
		{"/pois/hospital.geojson", "FeatureCollection", "application/geo+json"},
		{"/docs/guide.pdf", "%PDF", "application/pdf"},
		{"/manifest.json", `"schema":1`, "application/json"},
		{"/api/status", `"library":false`, "application/json"},
	}
	for _, c := range cases {
		res, body := get(t, h, c.path, nil)
		if res.StatusCode != http.StatusOK {
			t.Errorf("%s: status %d", c.path, res.StatusCode)
		}
		if !strings.Contains(body, c.wantBody) {
			t.Errorf("%s: body %q does not contain %q", c.path, body, c.wantBody)
		}
		if ct := res.Header.Get("Content-Type"); !strings.HasPrefix(ct, c.wantType) {
			t.Errorf("%s: content type %q, want %q", c.path, ct, c.wantType)
		}
	}
}

// PMTiles are read with HTTP Range requests; this is the launcher's core job.
func TestRangeRequests(t *testing.T) {
	h := NewHandler(Config{Root: newDrive(t)})
	res, body := get(t, h, "/maps/ee.pmtiles", func(r *http.Request) {
		r.Header.Set("Range", "bytes=2-5")
	})
	if res.StatusCode != http.StatusPartialContent {
		t.Fatalf("status %d, want 206", res.StatusCode)
	}
	if body != "2345" {
		t.Fatalf("body %q, want 2345", body)
	}
	if got := res.Header.Get("Content-Range"); got != "bytes 2-5/10" {
		t.Fatalf("Content-Range %q", got)
	}
}

func TestHidesDotfilesListingsAndOtherFolders(t *testing.T) {
	h := NewHandler(Config{Root: newDrive(t)})
	// Dotfiles, directory listings, folders without an index page, library/
	// (only reachable through kiwix), path escapes and bin/ must never be
	// answered with a 200.
	for _, path := range []string{
		"/.secret",
		"/maps/._ee.pmtiles",
		"/maps/",
		"/assets/",
		"/library/notes.txt",
		"/maps/../manifest.json",
		"/bin/linux-amd64/varu",
	} {
		res, body := get(t, h, path, nil)
		if res.StatusCode == http.StatusOK {
			t.Errorf("%s: unexpectedly served %q", path, body)
		}
		if strings.Contains(body, "hidden") || strings.Contains(body, "resource fork") || strings.Contains(body, "not served directly") {
			t.Errorf("%s: leaked protected content", path)
		}
	}
}

func TestHostGuard(t *testing.T) {
	root := newDrive(t)
	evil := func(r *http.Request) { r.Host = "evil.example:8480" }

	res, _ := get(t, NewHandler(Config{Root: root}), "/", evil)
	if res.StatusCode != http.StatusForbidden {
		t.Errorf("local mode: foreign host got %d, want 403", res.StatusCode)
	}
	for _, host := range []string{"localhost:8480", "127.0.0.1:8480", "[::1]:8480", "localhost"} {
		res, _ := get(t, NewHandler(Config{Root: root}), "/", func(r *http.Request) { r.Host = host })
		if res.StatusCode != http.StatusOK {
			t.Errorf("local mode: host %s got %d, want 200", host, res.StatusCode)
		}
	}
	res, _ = get(t, NewHandler(Config{Root: root, LAN: true}), "/", func(r *http.Request) { r.Host = "192.168.1.20:8480" })
	if res.StatusCode != http.StatusOK {
		t.Errorf("LAN mode: got %d, want 200", res.StatusCode)
	}
}

func TestLibraryWithoutKiwix(t *testing.T) {
	h := NewHandler(Config{Root: newDrive(t)})
	res, body := get(t, h, "/library/", nil)
	if res.StatusCode != http.StatusNotFound || !strings.Contains(body, "No library") {
		t.Errorf("got %d %q", res.StatusCode, body)
	}
}

func TestLibraryProxy(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "kiwix saw "+r.URL.Path)
	}))
	defer backend.Close()
	u, _ := url.Parse(backend.URL)

	h := NewHandler(Config{Root: newDrive(t), KiwixURL: u})
	res, body := get(t, h, "/library/content/wikipedia_et_all/A/Tallinn", nil)
	if res.StatusCode != http.StatusOK || body != "kiwix saw /library/content/wikipedia_et_all/A/Tallinn" {
		t.Errorf("got %d %q", res.StatusCode, body)
	}

	_, status := get(t, h, "/api/status", nil)
	if !strings.Contains(status, `"library":true`) {
		t.Errorf("status %q", status)
	}
}

func TestLibraryStartingPage(t *testing.T) {
	// A backend address nobody listens on: kiwix-serve still opening archives.
	u, _ := url.Parse("http://127.0.0.1:1")
	h := NewHandler(Config{Root: newDrive(t), KiwixURL: u})
	res, body := get(t, h, "/library/", nil)
	if res.StatusCode != http.StatusServiceUnavailable || !strings.Contains(body, "starting") {
		t.Errorf("got %d %q", res.StatusCode, body)
	}
}

func TestResolveRoot(t *testing.T) {
	root := newDrive(t)
	got, err := resolveRoot(root)
	if err != nil || got != root {
		t.Errorf("explicit root: %q %v", got, err)
	}
	if _, err := resolveRoot(filepath.Join(root, "maps")); err == nil {
		t.Error("a folder without app/ must be rejected")
	}
}

func TestRouteAPI(t *testing.T) {
	root := newDrive(t)
	h := NewHandler(Config{Root: root})
	if res, body := get(t, h, "/api/route?from=58.38,26.72&to=58.381,26.72&mode=foot", nil); res.StatusCode != 404 ||
		!strings.Contains(body, "noGraph") {
		t.Fatalf("without road data: %d %s", res.StatusCode, body)
	}

	// One street in Tartu, 111 m north to south.
	g, err := router.Build([]router.Way{{ID: 1, Nodes: []int64{1, 2}, Tags: map[string]string{"highway": "residential"}}},
		map[int64][2]float64{1: {58.380, 26.72}, 2: {58.381, 26.72}}, router.Meta{})
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(filepath.Join(root, "maps", "ee.graph"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := g.WriteTo(f); err != nil {
		t.Fatal(err)
	}
	f.Close()
	if err := os.WriteFile(filepath.Join(root, "manifest.json"), []byte(`{"routing":{"file":"maps/ee.graph"}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	h = NewHandler(Config{Root: root})
	for _, c := range []struct {
		query, want string
		status      int
	}{
		{"from=58.380,26.72&to=58.381,26.72&mode=foot", `"distance_m":111`, 200},
		{"from=58.380,26.72&to=58.381,26.72&mode=bike", "badRequest", 400},
		{"from=58.380&to=58.381,26.72&mode=car", "badRequest", 400},
		{"from=59.4,24.7&to=58.381,26.72&mode=car", "startTooFar", 422},
	} {
		res, body := get(t, h, "/api/route?"+c.query, nil)
		if res.StatusCode != c.status || !strings.Contains(body, c.want) {
			t.Errorf("%s: %d %s; want %d with %s", c.query, res.StatusCode, body, c.status, c.want)
		}
	}
}
