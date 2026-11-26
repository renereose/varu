package main

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"mime"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Config describes one running instance.
type Config struct {
	Root    string // drive root
	Version string
	Search  bool // prepare the library's search indexes in the background

	ShareHost string // address the sharing listener binds; 0.0.0.0 when empty
	SharePort int    // preferred port for the sharing listener
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
//	/api/places       search for towns, streets and addresses (places.go)
//	/library/  /api/library   the books in library/ (library.go)
func NewHandler(cfg Config) http.Handler {
	return newSite(cfg).local
}

// site is the handler for the person's own browser plus the optional
// Wi-Fi sharing server that serves the same pages to other devices.
type site struct {
	local http.Handler // 127.0.0.1 only, behind the host guard
	share *sharer
}

type remoteKey struct{}

// isRemote reports whether the request arrived on the sharing server.
func isRemote(r *http.Request) bool {
	v, _ := r.Context().Value(remoteKey{}).(bool)
	return v
}

func newSite(cfg Config) *site {
	mux := http.NewServeMux()
	sh := &sharer{host: cfg.ShareHost, port: cfg.SharePort, addrs: lanAddresses}
	status := func(r *http.Request) map[string]any {
		on, addrs := sh.state()
		return map[string]any{
			"version":   cfg.Version,
			"library":   hasBooks(cfg.Root),
			"lan":       on,
			"addresses": addrs,
			"remote":    isRemote(r),
		}
	}

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
		_ = json.NewEncoder(w).Encode(status(r))
	})

	// Only the person at this computer may turn sharing on or off: not
	// from the sharing server, and not from a web page elsewhere (the
	// custom header forces a preflight, which is never answered).
	mux.HandleFunc("/api/share", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if isRemote(r) || r.Header.Get("X-Varu") != "1" {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		var req struct {
			On bool `json:"on"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if err := sh.set(req.On); err != nil {
			http.Error(w, "Could not share on the network: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(status(r))
	})

	mux.Handle("/api/route", &routes{root: cfg.Root})
	mux.Handle("/api/places", &places{root: cfg.Root})

	lib := &library{root: cfg.Root, prepare: cfg.Search}
	lib.register(mux)
	if cfg.Search {
		go lib.open() // start on the indexes now, not at the first visit
	}

	mux.Handle("/", http.FileServer(quietFS{http.Dir(filepath.Join(cfg.Root, "app"))}))

	local := func(h http.Handler, remote bool) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), remoteKey{}, remote)))
		})
	}
	sh.handler = commonHeaders(local(mux, true))
	return &site{local: commonHeaders(local(hostGuard(mux), false)), share: sh}
}

// sharer owns the optional listener on the local network. It is bound
// only while sharing is on, so people who never share never trigger the
// firewall prompt a wildcard bind causes.
type sharer struct {
	host    string
	port    int
	addrs   func() []string
	handler http.Handler

	mu   sync.Mutex
	srv  *http.Server
	bind int // port in use while on
}

func (s *sharer) set(on bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !on {
		if s.srv != nil {
			_ = s.srv.Close()
			s.srv = nil
		}
		return nil
	}
	if s.srv != nil {
		return nil
	}
	host := s.host
	if host == "" {
		host = "0.0.0.0"
	}
	ln, err := listen(host, s.port)
	if err != nil {
		return err
	}
	srv := &http.Server{Handler: s.handler, ReadHeaderTimeout: 10 * time.Second}
	s.srv = srv
	s.bind = ln.Addr().(*net.TCPAddr).Port
	go func() { _ = srv.Serve(ln) }()
	return nil
}

// state reports whether sharing is on and the addresses other devices can
// open. The addresses are read each time so a network that appears later
// (a phone hotspot) shows up.
func (s *sharer) state() (bool, []string) {
	s.mu.Lock()
	on, port := s.srv != nil, s.bind
	s.mu.Unlock()
	out := []string{}
	if on {
		for _, ip := range s.addrs() {
			out = append(out, fmt.Sprintf("http://%s/", net.JoinHostPort(ip, fmt.Sprint(port))))
		}
	}
	return on, out
}

func (s *sharer) close() { _ = s.set(false) }

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
// the visitor's browser. The sharing server does not use it by design.
func hostGuard(next http.Handler) http.Handler {
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
