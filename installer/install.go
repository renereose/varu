package main

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Index is regions.json from the release (written by builder/release.py).
type Index struct {
	Schema  int      `json:"schema"`
	Regions []Region `json:"regions"`
	Desktop []Asset  `json:"desktop,omitempty"` // the Varu app for each system, for sticks
}

type Region struct {
	ID          string            `json:"id"`
	Name        map[string]string `json:"name"`
	CountryCode string            `json:"country_code"`
	Languages   []string          `json:"languages"`
	Bounds      []float64         `json:"bounds"` // west, south, east, north
	Center      []float64         `json:"center"` // lon, lat
	Bundle      Asset             `json:"bundle"`
	Map         *Asset            `json:"map,omitempty"`
	Routing     *Asset            `json:"routing,omitempty"` // road network for directions, whole country
	Pois        []PoiLayer        `json:"pois"`
	Library     []Book            `json:"library"`
}

// PoiLayer is a place category (water, shelters, hospitals ...). The
// bundle holds the whole country; the installer keeps what was chosen.
type PoiLayer struct {
	ID      string            `json:"id"`
	Label   map[string]string `json:"label"`
	Color   string            `json:"color"`
	Default bool              `json:"default"`
	Count   int               `json:"count"`
}

// Asset is a file attached to the release.
type Asset struct {
	Asset    string `json:"asset"`
	Size     int64  `json:"size"`
	SHA256   string `json:"sha256"`
	Path     string `json:"path,omitempty"`     // where it goes on the drive
	Unpacked int64  `json:"unpacked,omitempty"` // bundle only: size once extracted
}

// Book is a library archive, downloaded straight from the Kiwix mirrors.
type Book struct {
	Name     string `json:"name"`
	Title    string `json:"title"`
	Language string `json:"language"`
	File     string `json:"file"`
	URL      string `json:"url"`
	Size     int64  `json:"size"`
	SHA256   string `json:"sha256"`
}

// Item is one download in a plan.
type Item struct {
	Kind   string `json:"kind"` // app, map, routing or book
	Title  string `json:"title"`
	Size   int64  `json:"size"`
	Have   int64  `json:"have"` // bytes already on disk from an earlier run
	Cut    bool   `json:"cut"`  // map cut around home rather than downloaded whole
	url    string
	dest   string
	sha256 string
	cut    *Cut // map only: cut around home instead of downloading the whole file
}

// Problem stops an install; Warning does not. Codes are translated by the page.
type Problem struct {
	Code   string `json:"code"`
	Detail string `json:"detail,omitempty"`
}

type Plan struct {
	Items    []Item    `json:"items"`
	Needed   int64     `json:"needed"` // bytes still to be written to the target
	Free     int64     `json:"free"`
	FS       string    `json:"fs"`
	USB      bool      `json:"usb"`
	Problems []Problem `json:"problems"`
	Warnings []Problem `json:"warnings"`
	region   *Region
	req      Request
}

const fat32Max = 1<<32 - 1

// makePlan works out what an install needs. open reads a remote map's
// directories, which a radius cut needs to know its size.
func makePlan(index *Index, req Request, volumeRoot bool, open func(string) (*Archive, error)) Plan {
	plan := Plan{USB: volumeRoot, Problems: []Problem{}, Warnings: []Problem{}, Items: []Item{}, req: req}
	problem := func(code, detail string) { plan.Problems = append(plan.Problems, Problem{code, detail}) }

	if index.Schema != 1 {
		problem("tooOld", "")
		return plan
	}
	for i := range index.Regions {
		if index.Regions[i].ID == req.Region {
			plan.region = &index.Regions[i]
		}
	}
	if plan.region == nil {
		problem("region", req.Region)
		return plan
	}
	region := plan.region

	if info, err := os.Stat(req.Target); err == nil {
		if !info.IsDir() {
			problem("notFolder", req.Target)
		} else if !volumeRoot && hasForeignFiles(req.Target) {
			// A folder full of someone's files is not a place to unpack into.
			problem("notEmpty", req.Target)
		}
	}

	bundleName := "varu-" + region.Bundle.SHA256
	if len(bundleName) > 21 {
		bundleName = bundleName[:21]
	}
	plan.Items = append(plan.Items, Item{
		Kind: "app", Title: region.Bundle.Asset, Size: region.Bundle.Size,
		url: base(region.Bundle.Asset), sha256: region.Bundle.SHA256,
		dest: filepath.Join(os.TempDir(), bundleName+".zip"),
	})
	if req.RadiusKm > 0 && req.Home == nil {
		problem("home", "")
	}
	if h := req.Home; h != nil && len(region.Bounds) == 4 &&
		(h.Lon < region.Bounds[0] || h.Lat < region.Bounds[1] || h.Lon > region.Bounds[2] || h.Lat > region.Bounds[3]) {
		plan.Warnings = append(plan.Warnings, Problem{"outside", ""})
	}
	if m := region.Map; m != nil {
		if !safeRel(m.Path) {
			problem("bad", m.Path)
		}
		item := assetItem("map", m, req.Target)
		if req.RadiusKm > 0 && req.Home != nil {
			src, err := open(m.Asset)
			if err != nil {
				problem("index", err.Error())
			} else {
				item.cut = src.planCut(areaAround(req.Home.Lat, req.Home.Lon, req.RadiusKm))
				item.Size, item.Cut = int64(item.cut.Download), true
			}
		}
		plan.Items = append(plan.Items, item)
	}
	// ponytail: the road network is not cut to the radius; ~35 MB for the whole country.
	if r := region.Routing; r != nil {
		if !safeRel(r.Path) {
			problem("bad", r.Path)
		}
		plan.Items = append(plan.Items, assetItem("routing", r, req.Target))
	}
	// A stick carries the Varu app for every system, so it opens on any computer.
	if volumeRoot {
		for i := range index.Desktop {
			if d := &index.Desktop[i]; safeRel(d.Path) {
				plan.Items = append(plan.Items, assetItem("desktop", d, req.Target))
			} else {
				problem("bad", d.Path)
			}
		}
	}
	for _, name := range req.Library {
		for _, b := range region.Library {
			if b.Name != name {
				continue
			}
			if !safeRel(b.File) || strings.ContainsAny(b.File, `/\`) {
				problem("bad", b.File)
				continue
			}
			plan.Items = append(plan.Items, Item{
				Kind: "book", Title: b.Title, Size: b.Size, url: b.URL, sha256: b.SHA256,
				dest: filepath.Join(req.Target, "library", b.File),
			})
		}
	}

	free, fs, err := diskInfo(req.Target)
	if err != nil {
		problem("path", err.Error())
	}
	plan.Free, plan.FS = free, fs
	for i := range plan.Items {
		it := &plan.Items[i]
		it.Have = existing(it.dest, it.Size)
		if it.Kind == "app" {
			plan.Needed += region.Bundle.Unpacked // extracted every time, over the old copy
			continue
		}
		if it.cut != nil {
			it.Have = 0 // a cut is made fresh every time
			plan.Needed += int64(it.cut.Size)
			continue
		}
		plan.Needed += it.Size - it.Have
		if isFAT(fs) && it.Size > fat32Max {
			problem("fat32", it.Title)
		}
	}
	if err == nil && plan.Needed > plan.Free {
		problem("space", "")
	}
	// A stick must work on every computer, whichever made it.
	if volumeRoot && !portable(fs) {
		problem("portable", fs)
	}
	return plan
}

// assetItem downloads a release file to its place on the drive.
func assetItem(kind string, a *Asset, target string) Item {
	return Item{Kind: kind, Title: a.Asset, Size: a.Size, url: base(a.Asset), sha256: a.SHA256,
		dest: filepath.Join(target, filepath.FromSlash(a.Path))}
}

// base marks an asset name as relative to the release; Job resolves it.
func base(asset string) string { return "release:" + asset }

func safeRel(p string) bool { return p != "" && filepath.IsLocal(filepath.FromSlash(p)) }

// existing reports how much of a download is already on disk.
func existing(dest string, size int64) int64 {
	if info, err := os.Stat(dest); err == nil && info.Size() == size {
		return size
	}
	if info, err := os.Stat(dest + ".part"); err == nil {
		return min(info.Size(), size)
	}
	return 0
}

// hasForeignFiles reports whether dir holds anything a Varu drive (or an
// interrupted install of one) would not.
func hasForeignFiles(dir string) bool {
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		name := e.Name()
		if !slices.Contains(varuNames, name) && !strings.HasPrefix(name, "Start-") && !strings.HasPrefix(name, "._") {
			return true
		}
	}
	return false
}

var varuNames = []string{"app", "bin", "maps", "pois", "docs", "library", "licenses",
	"manifest.json", "build-info.json", "README.txt", ".DS_Store"}

// portable says whether Windows, macOS and Linux all read and write fs.
func portable(fs string) bool { return isFAT(fs) || strings.EqualFold(fs, "exfat") }

func isFAT(fs string) bool {
	switch strings.ToLower(fs) {
	case "msdos", "fat", "fat16", "fat32", "vfat":
		return true
	}
	return false
}

func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

// Job runs one plan. Downloads go one at a time, to be kind to the mirrors.
type Job struct {
	Target string
	Plan   Plan
	Source *url.URL
	Client *http.Client
	Phone  bool // leave out the desktop launchers, which a phone cannot run

	done     atomic.Int64
	mu       sync.Mutex
	current  *Item
	errors   []string
	finished bool
}

type Progress struct {
	Target   string   `json:"target"`
	USB      bool     `json:"usb"`
	Current  *Item    `json:"current,omitempty"`
	Done     int64    `json:"done"`
	Total    int64    `json:"total"`
	Errors   []string `json:"errors"`
	Finished bool     `json:"finished"`
}

func (j *Job) snapshot() Progress {
	j.mu.Lock()
	defer j.mu.Unlock()
	var total int64
	for _, it := range j.Plan.Items {
		total += it.Size
	}
	return Progress{
		Target: j.Target, USB: j.Plan.USB, Current: j.current, Done: j.done.Load(), Total: total,
		Errors: append([]string{}, j.errors...), Finished: j.finished,
	}
}

func (j *Job) fail(err error) {
	j.mu.Lock()
	j.errors = append(j.errors, err.Error())
	j.mu.Unlock()
}

// Run installs everything it can. Like the builder, a failed part does not
// stop the others, and the manifest lists only what actually arrived.
func (j *Job) Run() {
	defer func() {
		j.mu.Lock()
		j.current, j.finished = nil, true
		j.mu.Unlock()
	}()
	if err := os.MkdirAll(j.Target, 0o755); err != nil {
		j.fail(err)
		return
	}
	for i := range j.Plan.Items {
		it := &j.Plan.Items[i]
		j.mu.Lock()
		j.current = it
		j.mu.Unlock()
		var err error
		if it.cut != nil {
			h := j.Plan.req.Home
			err = it.cut.write(it.dest, h.Lat, h.Lon, func(n int64) { j.done.Add(n) })
		} else {
			err = j.fetch(it)
		}
		if err == nil && it.Kind == "app" {
			// Unpacking writes hundreds of files: give a drive that drops out a
			// moment to come back, as downloads do.
			for tries := 1; ; tries++ {
				if err = extract(it.dest, j.Target, j.Phone); err == nil || tries == 3 {
					break
				}
				time.Sleep(diskPause)
			}
		}
		if err != nil {
			j.fail(fmt.Errorf("%s: %w", it.Title, err))
			if it.Kind == "app" {
				return // without the app there is no drive to put the rest on
			}
		}
	}
	if err := keepPlaces(j.Target, j.Plan.region, j.Plan.req); err != nil {
		j.fail(err)
	}
	if err := writeManifest(j.Target, j.Plan.region, j.Plan.req); err != nil {
		j.fail(err)
	}
}

func (j *Job) resolve(u string) string {
	if name, ok := strings.CutPrefix(u, "release:"); ok {
		return j.Source.JoinPath(name).String()
	}
	return u
}

// fetch downloads it.url to it.dest, resuming from dest.part and checking
// size and SHA-256 before the file takes its final name.
func (j *Job) fetch(it *Item) error {
	if existing(it.dest, it.Size) == it.Size && isFile(it.dest) {
		j.done.Add(it.Size)
		return nil
	}
	for tries := 1; ; tries++ {
		mark := j.done.Load()
		err := j.download(it)
		var disk diskError
		if !errors.As(err, &disk) {
			return err
		}
		if tries == 4 {
			return stickTrouble
		}
		// The drive went away for a moment (a loose plug, a tired stick) and
		// took the open file with it: start again from what reached it.
		j.done.Store(mark)
		time.Sleep(diskPause)
	}
}

// stickTrouble is what a drive that keeps failing writes looks like to people.
var stickTrouble = errors.New("the drive stopped responding while Varu was writing to it. " +
	"Check that the stick is plugged in firmly (or try another port), then run the installer again: it continues where it stopped")

// diskPause is how long a drive that failed a write gets to come back.
var diskPause = 5 * time.Second

// diskError marks a failure of the drive being written, as opposed to the network.
type diskError struct{ err error }

func (e diskError) Error() string { return e.err.Error() }
func (e diskError) Unwrap() error { return e.err }

func onDisk(err error) error {
	if err == nil {
		return nil
	}
	return diskError{err}
}

type diskWriter struct{ f *os.File }

func (d diskWriter) Write(p []byte) (int, error) {
	n, err := d.f.Write(p)
	return n, onDisk(err)
}

// download is one go at fetch, with a freshly opened partial file.
func (j *Job) download(it *Item) error {
	if err := os.MkdirAll(filepath.Dir(it.dest), 0o755); err != nil {
		return onDisk(err)
	}
	part := it.dest + ".part"
	f, err := os.OpenFile(part, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return onDisk(err)
	}
	defer f.Close()

	// Re-hash what an earlier run left, so the checksum covers the whole file.
	sum := sha256.New()
	have, err := io.Copy(io.MultiWriter(sum, counter{&j.done}), f)
	if err != nil {
		return onDisk(err)
	}
	for attempt := 1; it.Size == 0 || have < it.Size; attempt++ {
		err = j.get(j.resolve(it.url), f, sum, &have)
		if err == nil {
			break
		}
		var final permanent
		var disk diskError
		if errors.As(err, &final) || errors.As(err, &disk) || attempt == 5 {
			return err
		}
		time.Sleep(time.Duration(attempt) * 3 * time.Second)
	}
	if it.Size > 0 && have != it.Size {
		return fmt.Errorf("got %d of %d bytes", have, it.Size)
	}
	if it.sha256 != "" && hex.EncodeToString(sum.Sum(nil)) != strings.ToLower(it.sha256) {
		f.Close()
		os.Remove(part)
		j.done.Add(-have)
		return errors.New("download is damaged (checksum mismatch); run the installer again")
	}
	if err := f.Close(); err != nil {
		return onDisk(err)
	}
	return onDisk(os.Rename(part, it.dest))
}

type permanent struct{ error }

// get appends the rest of url (from byte *have) to f.
func (j *Job) get(u string, f *os.File, sum hash.Hash, have *int64) error {
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return permanent{err}
	}
	req.Header.Set("User-Agent", "varu-installer/"+version)
	if *have > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", *have))
	}
	resp, err := j.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusPartialContent && *have > 0:
	case resp.StatusCode == http.StatusOK:
		if *have > 0 { // the server ignored Range: start over
			if err := f.Truncate(0); err != nil {
				return onDisk(err)
			}
			if _, err := f.Seek(0, io.SeekStart); err != nil {
				return onDisk(err)
			}
			sum.Reset()
			j.done.Add(-*have)
			*have = 0
		}
	case resp.StatusCode == http.StatusRequestedRangeNotSatisfiable:
		return permanent{errors.New("the file on the server changed; delete " + f.Name() + " and run again")}
	case resp.StatusCode >= 500 || resp.StatusCode == http.StatusTooManyRequests:
		return fmt.Errorf("%s: %s", u, resp.Status)
	default:
		return permanent{fmt.Errorf("%s: %s", u, resp.Status)}
	}
	n, err := io.Copy(io.MultiWriter(diskWriter{f}, sum, counter{&j.done}), resp.Body)
	*have += n
	return err
}

type counter struct{ n *atomic.Int64 }

func (c counter) Write(p []byte) (int, error) { c.n.Add(int64(len(p))); return len(p), nil }

// extract unpacks the bundle over target. Entry names are checked so a
// damaged or hostile archive cannot write outside the target.
func extract(archive, target string, phone bool) error {
	r, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}
	defer r.Close()
	for _, zf := range r.File {
		name := filepath.FromSlash(zf.Name)
		if !filepath.IsLocal(name) {
			return fmt.Errorf("unsafe path in archive: %q", zf.Name)
		}
		if phone && (strings.HasPrefix(zf.Name, "bin/") || strings.HasPrefix(zf.Name, "Start-") || zf.Name == "README.txt") {
			continue
		}
		dest := filepath.Join(target, name)
		if zf.FileInfo().IsDir() {
			if err := os.MkdirAll(dest, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := writeEntry(zf, dest); err != nil {
			return err
		}
	}
	return nil
}

func writeEntry(zf *zip.File, dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	src, err := zf.Open()
	if err != nil {
		return err
	}
	defer src.Close()
	// A new file renamed over the old one: macOS kills a program whose file
	// was rewritten in place, as it remembers signatures per file.
	tmp := dest + ".new"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, src)
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err == nil && zf.Mode()&0o111 != 0 {
		_ = os.Chmod(tmp, 0o755) // exFAT has no modes; macOS and Linux treat it all as executable
	}
	if err == nil {
		err = os.Rename(tmp, dest)
	}
	if err != nil {
		os.Remove(tmp)
	}
	return err
}

// writeManifest updates the bundle's manifest.json to list what is really
// on the drive: the viewer shows only what the manifest lists.
func writeManifest(target string, region *Region, req Request) error {
	path := filepath.Join(target, "manifest.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var manifest map[string]any
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return fmt.Errorf("manifest.json: %w", err)
	}
	if region.Map != nil && !isFile(filepath.Join(target, filepath.FromSlash(region.Map.Path))) {
		delete(manifest, "map")
	}
	if region.Routing == nil || !isFile(filepath.Join(target, filepath.FromSlash(region.Routing.Path))) {
		delete(manifest, "routing")
	}
	library := []map[string]any{}
	for _, b := range region.Library {
		info, err := os.Stat(filepath.Join(target, "library", b.File))
		if err != nil || !safeRel(b.File) {
			continue
		}
		library = append(library, map[string]any{
			"file": "library/" + b.File, "name": b.Name, "title": b.Title,
			"language": b.Language, "size": info.Size(),
		})
	}
	manifest["library"] = library

	pois := []any{}
	list, _ := manifest["pois"].([]any)
	for _, p := range list {
		layer, _ := p.(map[string]any)
		id, _ := layer["id"].(string)
		if !slices.Contains(req.Pois, id) {
			continue
		}
		if n, err := countFeatures(filepath.Join(target, "pois", id+".geojson")); err == nil {
			layer["count"] = n
			pois = append(pois, layer)
		}
	}
	manifest["pois"] = pois

	if h := req.Home; h != nil {
		home := map[string]any{"lon": h.Lon, "lat": h.Lat, "name": h.Name}
		if req.RadiusKm > 0 {
			home["radius_km"] = req.RadiusKm
		}
		manifest["home"] = home
		if m, ok := manifest["map"].(map[string]any); ok {
			m["center"] = []float64{h.Lon, h.Lat}
			m["zoom"] = 11
		}
	}
	out, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(out, '\n'), 0o644)
}

// keepPlaces removes the place layers that were not chosen and, for a
// radius install, the places farther than the radius from home.
func keepPlaces(target string, region *Region, req Request) error {
	for _, layer := range region.Pois {
		path := filepath.Join(target, "pois", layer.ID+".geojson")
		if !slices.Contains(req.Pois, layer.ID) {
			_ = os.Remove(path)
			continue
		}
		if req.RadiusKm <= 0 || req.Home == nil {
			continue
		}
		var fc struct {
			Type     string            `json:"type"`
			Features []json.RawMessage `json:"features"`
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			continue // a layer that failed to build is simply absent
		}
		if err := json.Unmarshal(raw, &fc); err != nil {
			return fmt.Errorf("%s: %w", layer.ID, err)
		}
		kept := fc.Features[:0]
		for _, f := range fc.Features {
			var point struct {
				Geometry struct {
					Coordinates []float64 `json:"coordinates"`
				} `json:"geometry"`
			}
			if json.Unmarshal(f, &point) == nil && len(point.Geometry.Coordinates) == 2 &&
				distanceKm(req.Home.Lat, req.Home.Lon, point.Geometry.Coordinates[1], point.Geometry.Coordinates[0]) <= req.RadiusKm {
				kept = append(kept, f)
			}
		}
		fc.Features = kept
		out, err := json.Marshal(fc)
		if err != nil {
			return err
		}
		if err := os.WriteFile(path, out, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func countFeatures(path string) (int, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	var fc struct {
		Features []json.RawMessage `json:"features"`
	}
	err = json.Unmarshal(raw, &fc)
	return len(fc.Features), err
}

// distanceKm is the great-circle distance between two points.
func distanceKm(lat1, lon1, lat2, lon2 float64) float64 {
	const r = 6371.0
	rad := math.Pi / 180
	dLat, dLon := (lat2-lat1)*rad, (lon2-lon1)*rad
	a := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(lat1*rad)*math.Cos(lat2*rad)*math.Sin(dLon/2)*math.Sin(dLon/2)
	return 2 * r * math.Asin(math.Sqrt(a))
}
