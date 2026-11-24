package main

import (
	"encoding/json"
	"errors"
	"io/fs"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/renereose/varu/router"
)

// routes answers GET /api/route?from=lat,lon&to=lat,lon&mode=foot|car with
// a GeoJSON Feature, using the road network the manifest lists. The network
// is loaded on the first request and kept.
type routes struct {
	root string
	mu   sync.Mutex
	g    *router.Graph
}

var errNoGraph = errors.New("no road data on this drive")

func (rt *routes) graph() (*router.Graph, error) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.g != nil {
		return rt.g, nil
	}
	var m struct {
		Routing struct {
			File string `json:"file"`
		} `json:"routing"`
	}
	raw, err := os.ReadFile(filepath.Join(rt.root, "manifest.json"))
	if err != nil || json.Unmarshal(raw, &m) != nil || m.Routing.File == "" {
		return nil, errNoGraph
	}
	rel := filepath.FromSlash(m.Routing.File)
	if !filepath.IsLocal(rel) {
		return nil, errNoGraph
	}
	g, err := router.Load(filepath.Join(rt.root, rel))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, errNoGraph
	}
	if err != nil {
		return nil, err
	}
	rt.g = g
	return g, nil
}

func (rt *routes) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	from, okFrom := parsePoint(q.Get("from"))
	to, okTo := parsePoint(q.Get("to"))
	mode, err := router.ParseMode(q.Get("mode"))
	if !okFrom || !okTo || err != nil {
		routeError(w, http.StatusBadRequest, "badRequest")
		return
	}
	g, err := rt.graph()
	if errors.Is(err, errNoGraph) {
		routeError(w, http.StatusNotFound, "noGraph")
		return
	}
	if err != nil {
		routeError(w, http.StatusInternalServerError, "badGraph")
		return
	}
	route, err := g.Route(from, to, mode)
	for code, want := range map[string]error{
		"startTooFar": router.ErrStartTooFar, "endTooFar": router.ErrEndTooFar,
		"notConnected": router.ErrNotConnected, "noRoute": router.ErrNoRoute,
	} {
		if errors.Is(err, want) {
			routeError(w, http.StatusUnprocessableEntity, code)
			return
		}
	}
	if err != nil {
		routeError(w, http.StatusInternalServerError, "noRoute")
		return
	}

	line := make([][2]float64, 0, len(route.Line)+1)
	for _, p := range route.Line {
		line = append(line, [2]float64{round(p.Lon, 6), round(p.Lat, 6)})
	}
	if len(line) == 1 {
		line = append(line, line[0]) // a LineString needs two positions
	}
	steps := route.Steps
	for i := range steps {
		steps[i].AlongM = math.Round(steps[i].AlongM)
	}
	w.Header().Set("Content-Type", "application/geo+json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"type":     "Feature",
		"geometry": map[string]any{"type": "LineString", "coordinates": line},
		"properties": map[string]any{
			"mode": q.Get("mode"), "distance_m": math.Round(route.DistanceM),
			"duration_s": math.Round(route.DurationS), "ferry": route.Ferry,
			"snap_from_m": math.Round(route.SnapFromM), "snap_to_m": math.Round(route.SnapToM),
			"steps": steps,
		},
	})
}

func routeError(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": code})
}

// parsePoint reads "lat,lon".
func parsePoint(s string) (router.Point, bool) {
	a, b, ok := strings.Cut(s, ",")
	lat, err1 := strconv.ParseFloat(a, 64)
	lon, err2 := strconv.ParseFloat(b, 64)
	if !ok || err1 != nil || err2 != nil || !(lat >= -90 && lat <= 90 && lon >= -180 && lon <= 180) {
		return router.Point{}, false
	}
	return router.Point{Lat: lat, Lon: lon}, true
}

func round(v float64, places int) float64 {
	k := math.Pow(10, float64(places))
	return math.Round(v*k) / k
}
