// Varu installer: downloads a region's drive from the GitHub release and
// writes it to a USB stick or a folder on this computer. The wizard is a
// local web page, like the launcher, so there is no GUI toolkit to ship.
package main

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// version is set at release time with -ldflags "-X main.version=v1.2.3"; it
// also picks which release the content comes from.
var version = "dev"

const repo = "https://github.com/renereose/varu"

// ponytail: an idle wizard exits after this long without the page asking
// for anything. Browsers slow or pause timers in background tabs, so this
// is long; the cost is a small process lingering after the tab is closed.
const idleExit = 30 * time.Minute

//go:embed wizard.html
var wizard []byte

func main() {
	source := flag.String("source", "", "where releases files come from: a URL or a local folder (default: this version's GitHub release)")
	port := flag.Int("port", 8470, "preferred port")
	noBrowser := flag.Bool("no-browser", false, "do not open the browser")
	// For the Android app, which knows its storage places and the DNS servers.
	flag.Var(&fixedVolumes, "volume", "offer only these targets, as label=path (repeatable)")
	phone := flag.Bool("phone", false, "running inside the Android app: no desktop launchers")
	dns := flag.String("dns", "", "comma-separated DNS servers to use instead of the system's")
	flag.Parse()
	useDNS(*dns)
	log.SetFlags(0)

	base, err := sourceURL(*source)
	if err != nil {
		log.Fatal(err)
	}
	app := &App{Source: base, Token: newToken(), Client: httpClient(), Phone: *phone}

	ln, err := listen(*port)
	if err != nil {
		log.Fatal(err)
	}
	addr := ln.Addr().String()
	page := fmt.Sprintf("http://%s/#%s", addr, app.Token)
	fmt.Println("Varu installer is open in your browser:", page)

	go app.exitWhenIdle()
	if !*noBrowser {
		if err := openPath(page); err != nil {
			log.Printf("open %s in your browser (%v)", page, err)
		}
	}
	srv := &http.Server{Handler: app.Handler(addr), ReadHeaderTimeout: 10 * time.Second}
	log.Fatal(srv.Serve(ln))
}

func sourceURL(flagValue string) (*url.URL, error) {
	switch {
	case flagValue == "" && version == "dev":
		return url.Parse(repo + "/releases/latest/download/")
	case flagValue == "":
		return url.Parse(repo + "/releases/download/" + version + "/")
	case strings.Contains(flagValue, "://"):
		return url.Parse(strings.TrimSuffix(flagValue, "/") + "/")
	default:
		abs, err := filepath.Abs(flagValue)
		if err != nil {
			return nil, err
		}
		return &url.URL{Scheme: "file", Path: filepath.ToSlash(abs) + "/"}, nil
	}
}

func httpClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	// file:// sources make local testing use the same code path as a release.
	transport.RegisterProtocol("file", http.NewFileTransport(http.Dir("/")))
	return &http.Client{Transport: transport}
}

func newToken() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

func listen(preferred int) (net.Listener, error) {
	for p := preferred; p < preferred+10; p++ {
		if ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", p)); err == nil {
			return ln, nil
		}
	}
	return net.Listen("tcp", "127.0.0.1:0")
}

// App is the running wizard: the release index, the one install job and
// the API the page talks to.
type App struct {
	Source *url.URL
	Token  string
	Client *http.Client
	Phone  bool

	mu       sync.Mutex
	index    *Index
	job      *Job
	archives map[string]*Archive
	lastSeen atomic.Int64

	geocodeMu   sync.Mutex
	lastGeocode time.Time
}

// archive reads a release map's directories once, for planning radius cuts.
func (a *App) archive(asset string) (*Archive, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if arch, ok := a.archives[asset]; ok {
		return arch, nil
	}
	arch, err := openArchive(a.Client, a.Source.JoinPath(asset).String())
	if err != nil {
		return nil, fmt.Errorf("could not read the map: %w", err)
	}
	if a.archives == nil {
		a.archives = map[string]*Archive{}
	}
	a.archives[asset] = arch
	return arch, nil
}

// geocode finds places by name with OpenStreetMap's Nominatim, at most one
// request a second as its usage policy asks.
func (a *App) geocode(query, country string) (any, error) {
	if strings.TrimSpace(query) == "" {
		return []any{}, nil
	}
	a.geocodeMu.Lock()
	defer a.geocodeMu.Unlock()
	time.Sleep(time.Until(a.lastGeocode.Add(time.Second)))
	a.lastGeocode = time.Now()

	q := url.Values{"q": {query}, "format": {"jsonv2"}, "limit": {"6"}, "countrycodes": {strings.ToLower(country)}}
	req, _ := http.NewRequest("GET", "https://nominatim.openstreetmap.org/search?"+q.Encode(), nil)
	req.Header.Set("User-Agent", "varu-installer/"+version+" (+"+repo+")")
	resp, err := a.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("place search failed: %w", err)
	}
	defer resp.Body.Close()
	var found []struct {
		Name string `json:"display_name"`
		Lat  string `json:"lat"`
		Lon  string `json:"lon"`
	}
	if resp.StatusCode != http.StatusOK || json.NewDecoder(resp.Body).Decode(&found) != nil {
		return nil, fmt.Errorf("place search failed: %s", resp.Status)
	}
	out := []Home{}
	for _, f := range found {
		lat, err1 := strconv.ParseFloat(f.Lat, 64)
		lon, err2 := strconv.ParseFloat(f.Lon, 64)
		if err1 == nil && err2 == nil {
			out = append(out, Home{Lat: lat, Lon: lon, Name: f.Name})
		}
	}
	return out, nil
}

func (a *App) exitWhenIdle() {
	a.lastSeen.Store(time.Now().UnixNano())
	for range time.Tick(10 * time.Second) {
		idle := time.Since(time.Unix(0, a.lastSeen.Load()))
		a.mu.Lock()
		busy := a.job != nil && !a.job.snapshot().Finished
		a.mu.Unlock()
		if idle > idleExit && !busy {
			os.Exit(0)
		}
	}
}

// Handler serves the wizard page and its API. Every API call must carry
// the token from the page's URL, and the Host must be this machine, so
// other websites open in the browser cannot drive the installer.
func (a *App) Handler(addr string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(wizard)
	})
	api := func(method, path string, fn func(*http.Request) (any, error)) {
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			if r.Method != method {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Token")), []byte(a.Token)) != 1 {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			a.lastSeen.Store(time.Now().UnixNano())
			out, err := fn(r)
			w.Header().Set("Content-Type", "application/json")
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				out = map[string]string{"error": err.Error()}
			}
			_ = json.NewEncoder(w).Encode(out)
		})
	}

	api("GET", "/api/state", func(r *http.Request) (any, error) { return a.state(), nil })
	api("GET", "/api/geocode", func(r *http.Request) (any, error) {
		return a.geocode(r.URL.Query().Get("q"), r.URL.Query().Get("country"))
	})
	api("POST", "/api/plan", func(r *http.Request) (any, error) {
		req, err := a.decode(r)
		if err != nil {
			return nil, err
		}
		return a.plan(req), nil
	})
	api("POST", "/api/install", func(r *http.Request) (any, error) {
		req, err := a.decode(r)
		if err != nil {
			return nil, err
		}
		return nil, a.start(req)
	})
	api("POST", "/api/open", func(r *http.Request) (any, error) {
		var body struct{ What string }
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			return nil, err
		}
		a.mu.Lock()
		job := a.job
		a.mu.Unlock()
		if job == nil {
			return nil, errors.New("nothing installed yet")
		}
		if body.What == "varu" {
			return nil, startVaru(job.Target)
		}
		return nil, openPath(job.Target)
	})
	api("POST", "/api/quit", func(r *http.Request) (any, error) {
		go func() { time.Sleep(200 * time.Millisecond); os.Exit(0) }()
		return nil, nil
	})

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != addr && r.Host != strings.Replace(addr, "127.0.0.1", "localhost", 1) {
			http.Error(w, "forbidden host", http.StatusForbidden)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		mux.ServeHTTP(w, r)
	})
}

// State is everything the page shows.
type State struct {
	Version  string    `json:"version"`
	Regions  []Region  `json:"regions"`
	Error    string    `json:"error,omitempty"`
	Volumes  []Volume  `json:"volumes"`
	Computer string    `json:"computer"` // suggested folder on this computer; "" offers volumes only
	Phone    bool      `json:"phone"`
	Job      *Progress `json:"job,omitempty"`
}

func (a *App) state() State {
	s := State{Version: version, Volumes: allVolumes(), Computer: defaultFolder(), Phone: a.Phone}
	if len(fixedVolumes) > 0 {
		s.Computer = ""
	}
	if s.Volumes == nil {
		s.Volumes = []Volume{}
	}
	index, err := a.loadIndex()
	if err != nil {
		s.Error = err.Error()
	} else {
		s.Regions = index.Regions
	}
	a.mu.Lock()
	if a.job != nil {
		p := a.job.snapshot()
		s.Job = &p
	}
	a.mu.Unlock()
	return s
}

// loadIndex fetches regions.json once; a failure (offline) is retried on
// the next call.
func (a *App) loadIndex() (*Index, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.index != nil {
		return a.index, nil
	}
	resp, err := a.Client.Get(a.Source.JoinPath("regions.json").String())
	if err != nil {
		return nil, fmt.Errorf("could not reach the download server: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download server answered %s for regions.json", resp.Status)
	}
	var index Index
	if err := json.NewDecoder(resp.Body).Decode(&index); err != nil {
		return nil, fmt.Errorf("regions.json: %w", err)
	}
	a.index = &index
	return a.index, nil
}

// Request is what the page asks to install.
type Request struct {
	Target   string   `json:"target"`
	Region   string   `json:"region"`
	Home     *Home    `json:"home"`      // where the family lives; nil for a whole-country install
	RadiusKm float64  `json:"radius_km"` // street-level map and places within this; 0 = whole country
	Pois     []string `json:"pois"`      // place layers wanted: water, shelter, hospital ...
	Library  []string `json:"library"`   // names of the library books wanted
}

type Home struct {
	Lat  float64 `json:"lat"`
	Lon  float64 `json:"lon"`
	Name string  `json:"name"`
}

func (a *App) decode(r *http.Request) (Request, error) {
	var req Request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return req, err
	}
	req.Target = expandHome(strings.TrimSpace(req.Target))
	if !filepath.IsAbs(req.Target) {
		return req, errors.New("choose a full folder path, such as " + defaultFolder())
	}
	req.Target = filepath.Clean(req.Target)
	return req, nil
}

func (a *App) plan(req Request) Plan {
	index, err := a.loadIndex()
	if err != nil {
		return Plan{Problems: []Problem{{Code: "index", Detail: err.Error()}}, Warnings: []Problem{}, Items: []Item{}}
	}
	// Phone storage is not a stick to carry to other computers.
	return makePlan(index, req, isVolumeRoot(req.Target) && !a.Phone, a.archive)
}

func (a *App) start(req Request) error {
	plan := a.plan(req)
	if len(plan.Problems) > 0 {
		return fmt.Errorf("cannot install: %s %s", plan.Problems[0].Code, plan.Problems[0].Detail)
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.job != nil && !a.job.snapshot().Finished {
		return errors.New("an installation is already running")
	}
	a.job = &Job{Target: req.Target, Plan: plan, Source: a.Source, Client: a.Client, Phone: a.Phone}
	go a.job.Run()
	return nil
}

// fixedVolumes, when given, replace the system's list (the Android app
// passes its own storage folders).
var fixedVolumes volumeFlags

type volumeFlags []Volume

func (v *volumeFlags) String() string { return fmt.Sprint(*v) }
func (v *volumeFlags) Set(s string) error {
	label, path, ok := strings.Cut(s, "=")
	if !ok || !filepath.IsAbs(path) {
		return errors.New("want label=/absolute/path")
	}
	*v = append(*v, Volume{Label: label, Path: filepath.Clean(path)})
	return nil
}

func allVolumes() []Volume {
	if len(fixedVolumes) == 0 {
		return volumes()
	}
	out := make([]Volume, len(fixedVolumes))
	for i, v := range fixedVolumes {
		v.Free, v.FS, _ = diskInfo(v.Path)
		out[i] = v
	}
	return out
}

// useDNS points Go's resolver at the given servers. Android has no
// /etc/resolv.conf, so without this a pure-Go binary cannot resolve names.
func useDNS(servers string) {
	if servers == "" {
		return
	}
	list := strings.Split(servers, ",")
	var next atomic.Uint32
	net.DefaultResolver = &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
		server := strings.TrimSpace(list[int(next.Add(1))%len(list)])
		var d net.Dialer
		return d.DialContext(ctx, network, net.JoinHostPort(server, "53"))
	}}
}

func isVolumeRoot(path string) bool {
	for _, v := range allVolumes() {
		if filepath.Clean(v.Path) == path {
			return true
		}
	}
	return false
}

func defaultFolder() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, "Varu")
}

func expandHome(path string) string {
	if path == "~" || strings.HasPrefix(path, "~/") || strings.HasPrefix(path, `~\`) {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[1:])
		}
	}
	return path
}
