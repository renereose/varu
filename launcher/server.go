package main

import (
	"encoding/json"
	"fmt"
	"html"
	"mime"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// Config describes one running instance.
type Config struct {
	Root    string // drive root
	LAN     bool   // accept requests from other hosts
	Version string
	Search  bool // prepare the library's search indexes in the background
}

func init() {
	// Some systems (notably Windows, via the registry) report wrong types
	// for these extensions, and browsers refuse module scripts that are
	// not served as JavaScript. Set them explicitly.
	for ext, typ := range map[string]string{
		".html":    "text/html; charset=utf-8",
		".js":      "text/javascript; charset=utf-8",
		".mjs":     "text/javascript; charset=utf-8",
		".css":     "text/css; charset=utf-8",
		".json":    "application/json",
		".geojson": "application/geo+json",
		".svg":     "image/svg+xml",
		".pbf":     "application/x-protobuf",
		".pmtiles": "application/octet-stream",
		".pdf":     "application/pdf",
	} {
		_ = mime.AddExtensionType(ext, typ)
	}
}

// dataDirs are the drive folders exposed read-only under the same URL prefix.
var dataDirs = []string{"maps", "pois", "docs"}

// NewHandler builds the whole site:
//
//	/                 app/ (the viewer)
//	/maps/ /pois/ /docs/   the matching drive folders (Range requests supported)
//	/manifest.json    what the build put on the drive
//	/api/status       launcher status for the viewer
//	/api/route        directions on the drive's road network
//	/library/  /api/library   the books in library/ (library.go)
func NewHandler(cfg Config) http.Handler {
	mux := http.NewServeMux()

	for _, dir := range dataDirs {
		prefix := "/" + dir + "/"
		fs := http.FileServer(quietFS{http.Dir(filepath.Join(cfg.Root, dir))})
		mux.Handle(prefix, http.StripPrefix(prefix, fs))
	}

	mux.HandleFunc("/manifest.json", func(w http.ResponseWriter, r *http.Request) {
		path := filepath.Join(cfg.Root, "manifest.json")
		if !isFile(path) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte("{}"))
			return
		}
		http.ServeFile(w, r, path)
	})

	mux.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"version": cfg.Version,
			"library": hasBooks(cfg.Root),
			"lan":     cfg.LAN,
		})
	})

	mux.Handle("/api/route", &routes{root: cfg.Root})

	lib := &library{root: cfg.Root, prepare: cfg.Search}
	lib.register(mux)
	if cfg.Search {
		go lib.open() // start on the indexes now, not at the first visit
	}

	mux.Handle("/", http.FileServer(quietFS{http.Dir(filepath.Join(cfg.Root, "app"))}))

	return commonHeaders(hostGuard(cfg.LAN, mux))
}

func messagePage(w http.ResponseWriter, status int, title, body string, refresh bool) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	meta := ""
	if refresh {
		meta = `<meta http-equiv="refresh" content="2">`
	}
	fmt.Fprintf(w, `<!doctype html><html lang="en"><head><meta charset="utf-8">%s
<meta name="viewport" content="width=device-width, initial-scale=1"><title>%s</title>
<style>body{font:16px/1.5 system-ui,sans-serif;max-width:34rem;margin:15vh auto;padding:0 1.5rem;color:#1c2321}a{color:#0b5d4b}</style>
</head><body><h1>%s</h1><p>%s</p><p><a href="/">Back to start</a></p></body></html>`,
		meta, html.EscapeString(title), html.EscapeString(title), html.EscapeString(body))
}

// hostGuard rejects requests whose Host header is not this machine. Without
// it, a hostile web page could use DNS rebinding to read the drive through
// the visitor's browser. In LAN mode any host is accepted by design.
func hostGuard(lan bool, next http.Handler) http.Handler {
	if lan {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		host = strings.Trim(host, "[]")
		if host != "localhost" && host != "127.0.0.1" && host != "::1" {
			http.Error(w, "forbidden host", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func commonHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		// Content only changes when the drive is rebuilt; let the browser
		// revalidate cheaply instead of holding on to stale files.
		w.Header().Set("Cache-Control", "no-cache")
		next.ServeHTTP(w, r)
	})
}

// quietFS hides dotfiles (macOS litters removable drives with ._ files)
// and turns directory listings into 404s.
type quietFS struct{ fs http.FileSystem }

func (q quietFS) Open(name string) (http.File, error) {
	for _, part := range strings.Split(name, "/") {
		if strings.HasPrefix(part, ".") {
			return nil, os.ErrNotExist
		}
	}
	f, err := q.fs.Open(name)
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	if info.IsDir() {
		index, err := q.fs.Open(strings.TrimSuffix(name, "/") + "/index.html")
		if err != nil {
			f.Close()
			return nil, os.ErrNotExist
		}
		index.Close()
	}
	return f, nil
}
