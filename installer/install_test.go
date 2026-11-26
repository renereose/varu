package main

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

func sum(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

func writeZip(t *testing.T, path string, files map[string]string) {
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for name, body := range files {
		hdr := &zip.FileHeader{Name: name, Method: zip.Deflate}
		hdr.SetMode(0o644)
		if strings.HasPrefix(name, "bin/") {
			hdr.SetMode(0o755)
		}
		w, err := zw.CreateHeader(hdr)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(body))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	f.Close()
}

// TestInstall runs a whole install against a fake release: the bundle,
// a map cut around home, a library book that resumes from an earlier
// run, and the place layers trimmed to the radius.
func TestInstall(t *testing.T) {
	src, target := t.TempDir(), filepath.Join(t.TempDir(), "Varu")
	manifest := `{"schema":1,"region":"ee","map":{"file":"maps/ee.pmtiles","center":[25,58.6],"zoom":6},"routing":{"file":"maps/ee.graph"},
		"places":{"file":"maps/ee.places"},"pois":[{"id":"water","file":"pois/water.geojson","count":2},{"id":"fuel","file":"pois/fuel.geojson","count":1}]}`
	water := `{"type":"FeatureCollection","features":[
		{"type":"Feature","geometry":{"type":"Point","coordinates":[26.73,58.38]},"properties":{}},
		{"type":"Feature","geometry":{"type":"Point","coordinates":[24.75,59.43]},"properties":{}}]}`
	writeZip(t, filepath.Join(src, "varu-ee.zip"), map[string]string{
		"manifest.json": manifest, "app/index.html": "<h1>", "bin/darwin-arm64/varu": "#!",
		"pois/water.geojson": water, "pois/fuel.geojson": `{"features":[]}`,
	})
	writeArchive(t, filepath.Join(src, "ee.pmtiles"))
	book := []byte(strings.Repeat("wikipedia ", 5000))
	roads := []byte("VARUGRPH roads")
	search := []byte("VARUIDX1 index")
	addresses := []byte("varu-places 1")
	for name, body := range map[string][]byte{"book.zim": book, "ee.graph": roads, "wp_2026-11.search": search, "ee.places": addresses} {
		if err := os.WriteFile(filepath.Join(src, name), body, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	bundle, _ := os.ReadFile(filepath.Join(src, "varu-ee.zip"))
	mapFile, _ := os.ReadFile(filepath.Join(src, "ee.pmtiles"))

	srv := httptest.NewServer(http.FileServer(http.Dir(src)))
	defer srv.Close()
	index := &Index{Schema: 1, Regions: []Region{{
		ID: "ee", CountryCode: "EE", Bounds: []float64{21.3, 57.45, 28.3, 59.9},
		Bundle:  Asset{Asset: "varu-ee.zip", Size: int64(len(bundle)), SHA256: sum(bundle), Unpacked: 1000},
		Map:     &Asset{Asset: "ee.pmtiles", Path: "maps/ee.pmtiles", Size: int64(len(mapFile)), SHA256: sum(mapFile)},
		Routing: &Asset{Asset: "ee.graph", Path: "maps/ee.graph", Size: int64(len(roads)), SHA256: sum(roads)},
		Places:  &Asset{Asset: "ee.places", Path: "maps/ee.places", Size: int64(len(addresses)), SHA256: sum(addresses)},
		Pois:    []PoiLayer{{ID: "water"}, {ID: "fuel"}},
		Library: []Book{{Name: "wp", Title: "Wikipedia", File: "wp_2026-11.zim", URL: srv.URL + "/book.zim",
			Size: int64(len(book)), SHA256: sum(book),
			Search: &Asset{Asset: "wp_2026-11.search", Path: "library/wp_2026-11.search", Size: int64(len(search)), SHA256: sum(search)}}},
	}}}

	// Half of the book is already there from an interrupted run.
	if err := os.MkdirAll(filepath.Join(target, "library"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "library", "wp_2026-11.zim.part"), book[:20000], 0o644); err != nil {
		t.Fatal(err)
	}

	// An earlier edition from a previous update.
	for _, old := range []string{"wp_2026-05.zim", "wp_2026-05.search"} {
		if err := os.WriteFile(filepath.Join(target, "library", old), []byte("old"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	base, _ := url.Parse(srv.URL + "/")
	app := &App{Source: base, Client: srv.Client(), index: index}
	req := Request{Target: target, Region: "ee", Home: &Home{Lat: 58.38, Lon: 26.72, Name: "Tartu"},
		RadiusKm: 10, Pois: []string{"water"}, Library: []string{"wp"}}
	plan := app.plan(req)
	if len(plan.Problems) > 0 {
		t.Fatalf("problems: %+v", plan.Problems)
	}
	if plan.Items[2].Kind != "routing" || plan.Items[3].Kind != "places" || plan.Items[4].Have != 20000 {
		t.Errorf("plan should count the partial book, have=%d", plan.Items[4].Have)
	}
	job := &Job{Target: target, Plan: plan, Source: base, Client: srv.Client()}
	job.Run()
	if p := job.snapshot(); len(p.Errors) > 0 || !p.Finished {
		t.Fatalf("install: %+v", p)
	}

	if got, _ := os.ReadFile(filepath.Join(target, "library", "wp_2026-11.zim")); string(got) != string(book) {
		t.Error("resumed book differs from the original")
	}
	if got, _ := os.ReadFile(filepath.Join(target, "library", "wp_2026-11.search")); string(got) != string(search) {
		t.Error("the book's search index, built with the release, was not installed")
	}
	for _, old := range []string{"wp_2026-05.zim", "wp_2026-05.search"} {
		if _, err := os.Stat(filepath.Join(target, "library", old)); !os.IsNotExist(err) {
			t.Errorf("%s survived the newer edition", old)
		}
	}
	if got, _ := os.ReadFile(filepath.Join(target, "maps", "ee.graph")); string(got) != string(roads) {
		t.Error("road data missing or different")
	}
	if got, _ := os.ReadFile(filepath.Join(target, "maps", "ee.places")); string(got) != string(addresses) {
		t.Error("addresses for search missing or different")
	}
	if info, err := os.Stat(filepath.Join(target, "bin/darwin-arm64/varu")); err != nil || info.Mode()&0o100 == 0 {
		t.Errorf("launcher should be executable: %v %v", info, err)
	}
	if _, err := os.Stat(filepath.Join(target, "pois/fuel.geojson")); !os.IsNotExist(err) {
		t.Error("an unchosen place layer was kept")
	}
	if _, err := openArchive(httpClient(), "file://"+filepath.ToSlash(filepath.Join(target, "maps/ee.pmtiles"))); err != nil {
		t.Errorf("cut map does not open: %v", err)
	}

	var m map[string]any
	raw, _ := os.ReadFile(filepath.Join(target, "manifest.json"))
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	pois := m["pois"].([]any)
	if len(pois) != 1 || pois[0].(map[string]any)["count"].(float64) != 1 {
		t.Errorf("want only water with the 1 place within 10 km, got %v", pois)
	}
	if m["routing"] == nil || m["places"] == nil {
		t.Error("manifest lost the road data or the addresses")
	}
	if lib := m["library"].([]any); len(lib) != 1 {
		t.Errorf("library = %v", lib)
	}
	if home := m["home"].(map[string]any); home["name"] != "Tartu" || home["radius_km"].(float64) != 10 {
		t.Errorf("home = %v", home)
	}
	if center := m["map"].(map[string]any)["center"].([]any); center[0].(float64) != 26.72 {
		t.Errorf("map should open at home, center = %v", center)
	}
	if m["release"] != version {
		t.Errorf("release = %v, want %q, so the apps can tell old data", m["release"], version)
	}

	// An update starts from what was chosen.
	got := installedChoices(target)
	want := &Earlier{Request{Target: target, Region: "ee", Home: &Home{Lat: 58.38, Lon: 26.72, Name: "Tartu"},
		RadiusKm: 10, Pois: []string{"water"}, Library: []string{"wp"}}, []string{"water", "fuel"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("earlier choices %+v, want %+v", got, want)
	}
	if installedChoices(t.TempDir()) != nil {
		t.Error("an empty folder has no earlier choices")
	}
}

func TestExtractRefusesEscapes(t *testing.T) {
	dir := t.TempDir()
	writeZip(t, filepath.Join(dir, "bad.zip"), map[string]string{"../evil.txt": "x"})
	if err := extract(filepath.Join(dir, "bad.zip"), filepath.Join(dir, "out"), false); err == nil {
		t.Fatal("an entry outside the target was accepted")
	}
	if _, err := os.Stat(filepath.Join(dir, "evil.txt")); !os.IsNotExist(err) {
		t.Fatal("file written outside the target")
	}
}

func TestAPIGuards(t *testing.T) {
	app := &App{Token: "secret", index: &Index{Schema: 1}}
	h := app.Handler("127.0.0.1:8470")
	for _, c := range []struct {
		host, token string
		want        int
	}{
		{"127.0.0.1:8470", "secret", 200},
		{"127.0.0.1:8470", "", 403},          // another website's page has no token
		{"evil.example:8470", "secret", 403}, // DNS rebinding
	} {
		req := httptest.NewRequest("GET", "/api/state", nil)
		req.Host = c.host
		req.Header.Set("X-Token", c.token)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != c.want {
			t.Errorf("host %s token %q: %d, want %d", c.host, c.token, rec.Code, c.want)
		}
	}
}

// A failed road data download must not leave the viewer offering
// directions, nor a failed address download a search box.
func TestManifestDropsMissingRouting(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(`{"routing":{"file":"maps/ee.graph"},"places":{"file":"maps/ee.places"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeManifest(dir, &Region{Routing: &Asset{Path: "maps/ee.graph"}, Places: &Asset{Path: "maps/ee.places"}}, Request{}); err != nil {
		t.Fatal(err)
	}
	if raw, _ := os.ReadFile(filepath.Join(dir, "manifest.json")); strings.Contains(string(raw), "routing") || strings.Contains(string(raw), "places") {
		t.Errorf("manifest still offers routing or search: %s", raw)
	}
}

func TestStickGetsDesktopApps(t *testing.T) {
	index := &Index{Schema: 1, Regions: []Region{{ID: "ee", Bundle: Asset{Asset: "varu-ee.zip"}}},
		Desktop: []Asset{{Asset: "Varu-macOS.dmg", Path: "Varu-macOS.dmg", Size: 5}}}
	req := Request{Target: t.TempDir(), Region: "ee"}
	if folder := makePlan(index, req, false, nil); len(folder.Items) != 1 {
		t.Errorf("a folder on this computer got %+v", folder.Items)
	}
	stick := makePlan(index, req, true, nil)
	if len(stick.Items) != 2 || stick.Items[1].Kind != "desktop" || stick.Items[1].dest != filepath.Join(req.Target, "Varu-macOS.dmg") {
		t.Errorf("a stick got %+v", stick.Items)
	}
	// The temp folder is APFS, ext4 or NTFS: as a stick, not every computer could read it.
	if !slices.ContainsFunc(stick.Problems, func(p Problem) bool { return p.Code == "portable" }) {
		t.Errorf("a stick only some computers read was accepted: %+v", stick.Problems)
	}
}

// Updating must not rewrite a program in place (macOS then kills it).
func TestExtractReplacesFiles(t *testing.T) {
	dir := t.TempDir()
	archive, out := filepath.Join(dir, "a.zip"), filepath.Join(dir, "out")
	writeZip(t, archive, map[string]string{"bin/varu": "new"})
	if err := os.MkdirAll(filepath.Join(out, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(out, "bin/varu"), []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	before, _ := os.Stat(filepath.Join(out, "bin/varu"))
	if err := extract(archive, out, false); err != nil {
		t.Fatal(err)
	}
	after, _ := os.Stat(filepath.Join(out, "bin/varu"))
	if got, _ := os.ReadFile(filepath.Join(out, "bin/varu")); string(got) != "new" || os.SameFile(before, after) {
		t.Errorf("want a new file with the new content, got %q (same file: %v)", got, os.SameFile(before, after))
	}
}

// A drive that drops out for a moment (writes fail) must not fail the
// download when it comes back.
func TestDownloadSurvivesDriveDropout(t *testing.T) {
	body := []byte(strings.Repeat("road data ", 1000))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeContent(w, r, "ee.graph", time.Time{}, bytes.NewReader(body))
	}))
	defer srv.Close()
	stick := filepath.Join(t.TempDir(), "stick")
	if err := os.Mkdir(stick, 0o500); err != nil { // "gone": nothing can be written
		t.Fatal(err)
	}
	defer os.Chmod(stick, 0o755)
	saved := diskPause
	diskPause = 50 * time.Millisecond
	defer func() { diskPause = saved }()
	go func() { time.Sleep(20 * time.Millisecond); os.Chmod(stick, 0o755) }() // back again

	j := &Job{Client: srv.Client()}
	it := &Item{Kind: "routing", Title: "ee.graph", Size: int64(len(body)), url: srv.URL + "/ee.graph",
		sha256: sum(body), dest: filepath.Join(stick, "maps", "ee.graph")}
	if err := j.fetch(it); err != nil {
		t.Fatalf("after the drive came back: %v", err)
	}
	if got, _ := os.ReadFile(it.dest); !bytes.Equal(got, body) || j.done.Load() != int64(len(body)) {
		t.Errorf("file %d bytes, progress %d, want %d", len(got), j.done.Load(), len(body))
	}
}

// A stick shows Varu for each system and the README; the rest is hidden.
func TestPrepareStick(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"app/", "maps/", "manifest.json", "Start-Windows.bat", "README.txt", "Varu-Windows.exe", "Varu-macOS.dmg"} {
		if strings.HasSuffix(name, "/") {
			os.Mkdir(filepath.Join(root, name), 0o755)
		} else {
			os.WriteFile(filepath.Join(root, name), []byte("x"), 0o644)
		}
	}
	prepareStick(root)
	list, _ := os.ReadFile(filepath.Join(root, ".hidden"))
	hidden := strings.Fields(string(list))
	for _, want := range []string{"app", "maps", "manifest.json", "Start-Windows.bat", "autorun.inf", "autorun.sh", "varu.ico", ".VolumeIcon.icns", ".hidden"} {
		if !slices.Contains(hidden, want) {
			t.Errorf("%s is not hidden: %v", want, hidden)
		}
	}
	for _, shown := range stickShown {
		if slices.Contains(hidden, shown) {
			t.Errorf("%s is hidden", shown)
		}
	}
	if inf, _ := os.ReadFile(filepath.Join(root, "autorun.inf")); !strings.Contains(string(inf), "icon=varu.ico") {
		t.Errorf("autorun.inf: %q", inf)
	}
}

func TestRemoveOlderEditions(t *testing.T) {
	dir := t.TempDir()
	gone := []string{"wp_2026-03.zim", "wp_2026-03.search", "wp_2025-11.zim.part", "wp_2026-03.search.part"}
	kept := []string{"wp_2026-11.zim", "wp_2026-11.search", "wp_2026-11.search.part", "wp_2027-01.zim",
		"wp_extra_2026-03.zim", "undated.zim", "notes.txt"}
	for _, name := range append(slices.Clone(gone), kept...) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	removeOlderEditions(dir, "undated.zim")
	for _, name := range gone {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("an undated book removed %s", name)
		}
	}
	removeOlderEditions(dir, "wp_2026-11.zim")
	for _, name := range gone {
		if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Errorf("%s should be removed", name)
		}
	}
	for _, name := range kept {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("%s should stay: %v", name, err)
		}
	}
}
