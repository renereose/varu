package main

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const placesFile = "varu-places\t1\n" +
	"c\t\n" +
	"c\tPärnu maakond\n" +
	"c\tPärnu linn, Pärnu maakond\n" +
	"c\tTartu linn, Tartu maakond\n" +
	"p\t1\t24.50000\t58.38000\t100\tPärnu linn\n" +
	"s\t2\t24.50100\t58.38500\t10\tRüütli tn\n" +
	"s\t3\t26.72100\t58.38200\t10\tRüütli tn\n" +
	"s\t2\t24.49000\t58.37000\t10\tPärnu mnt\n" +
	"a\t2\t24.50200\t58.38600\t1\tRüütli tn 12\n" +
	"a\t2\t24.49300\t58.38700\t1\tRüütli tn 12a\n" +
	"a\t3\t26.72200\t58.38300\t1\tRüütli tn 12\n" +
	"a\t3\t26.70000\t58.37000\t1\tÕie tn 1\n" +
	"a\t3\t26.70000\t58.37000\t1\tAia tn 225\n" +
	"a\t3\t26.70000\t58.37000\t1\tKastani tn 22\n" +
	"p\t0\t26.72000\t58.38000\t5000\tTartu linn\n" +
	"p\t0\t26.90000\t58.40000\t50\tTartu vald\n"

func gzipped(t *testing.T, s string) []byte {
	t.Helper()
	var b bytes.Buffer
	z := gzip.NewWriter(&b)
	z.Write([]byte(s))
	z.Close()
	return b.Bytes()
}

func TestPlaceSearch(t *testing.T) {
	ix, err := readPlaces(bytes.NewReader(gzipped(t, placesFile)))
	if err != nil {
		t.Fatal(err)
	}
	find := func(q string, near *[2]float64) (out []string) {
		for _, r := range ix.search(q, near) {
			out = append(out, r.Name+" | "+r.Context)
		}
		return out
	}
	tartu := &[2]float64{26.72, 58.38}
	for _, c := range []struct {
		q    string
		near *[2]float64
		want string
	}{
		{"Rüütli 12 Pärnu", nil, "Rüütli tn 12 | Pärnu linn, Pärnu maakond,Rüütli tn 12a | Pärnu linn, Pärnu maakond"},
		{"ruutli tanav 12 parnu", nil, "Rüütli tn 12 | Pärnu linn, Pärnu maakond,Rüütli tn 12a | Pärnu linn, Pärnu maakond"},
		{"rüütli", tartu, "Rüütli tn | Tartu linn, Tartu maakond,Rüütli tn | Pärnu linn, Pärnu maakond," +
			"Rüütli tn 12 | Tartu linn, Tartu maakond,Rüütli tn 12 | Pärnu linn, Pärnu maakond,Rüütli tn 12a | Pärnu linn, Pärnu maakond"},
		{"parnu", nil, "Pärnu linn | Pärnu maakond,Pärnu mnt | Pärnu linn, Pärnu maakond"}, // the context alone is not a match
		{"OIE 1", nil, "Õie tn 1 | Tartu linn, Tartu maakond"},
		{"aia 225/1", nil, ""}, // "225/1" is a number of its own
		{"22", nil, "Kastani tn 22 | Tartu linn, Tartu maakond,Aia tn 225 | Tartu linn, Tartu maakond"}, // typed in full first
		{"tartu", nil, "Tartu linn | ,Tartu vald | "},                                                   // the bigger
		{"tartu", &[2]float64{26.9, 58.4}, "Tartu vald | ,Tartu linn | "},                               // right there
		{"tartu", &[2]float64{24.5, 58.38}, "Tartu linn | ,Tartu vald | "},                              // about as far: the bigger
		{"kastani", nil, "Kastani tn 22 | Tartu linn, Tartu maakond"},
		{"  ", nil, ""},
	} {
		if got := strings.Join(find(c.q, c.near), ","); got != c.want {
			t.Errorf("%q:\n got %s\nwant %s", c.q, got, c.want)
		}
	}

	if _, err := readPlaces(bytes.NewReader(gzipped(t, "varu-places\t2\n"))); err == nil {
		t.Error("a newer format was read")
	}
	if _, err := readPlaces(bytes.NewReader(gzipped(t, "varu-places\t1\na\t5\t1\t2\t1\tX\n"))); err == nil {
		t.Error("an unknown context was accepted")
	}
}

func TestPlacesAPI(t *testing.T) {
	root := newDrive(t)
	h := NewHandler(Config{Root: root})
	if res, _ := get(t, h, "/api/places?q=parnu", nil); res.StatusCode != 404 {
		t.Errorf("without places: %d", res.StatusCode)
	}
	os.WriteFile(filepath.Join(root, "maps", "ee.places"), gzipped(t, placesFile), 0o644)
	os.WriteFile(filepath.Join(root, "manifest.json"), []byte(`{"places":{"file":"maps/ee.places"}}`), 0o644)
	h = NewHandler(Config{Root: root})
	res, body := get(t, h, "/api/places?q=r%C3%BC%C3%BCtli+12&near=58.38,26.72", nil)
	var found []placeResult
	if res.StatusCode != 200 || json.Unmarshal([]byte(body), &found) != nil || len(found) != 3 ||
		found[0].Context != "Tartu linn, Tartu maakond" || found[0].Kind != "address" || found[0].Lon != 26.722 {
		t.Errorf("%d %s", res.StatusCode, body)
	}
	if res, _ := get(t, h, "/api/places?q=x&near=nonsense", nil); res.StatusCode != 400 {
		t.Errorf("bad near: %d", res.StatusCode)
	}
}

// VARU_PLACES=path/to/ee.places go test -run TestRealPlaces -v
func TestRealPlaces(t *testing.T) {
	path := os.Getenv("VARU_PLACES")
	if path == "" {
		t.Skip("set VARU_PLACES to a built places file")
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	start := time.Now()
	ix, err := readPlaces(f)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("loaded %d entries in %v", len(ix.entries), time.Since(start))
	for _, q := range []string{"Rüütli 12 Pärnu", "Otepaa", "tallinn", "Mustamäe tee 5", "k", "pärnu mnt 1"} {
		start = time.Now()
		found := ix.search(q, &[2]float64{24.75, 59.43})
		took := time.Since(start)
		var top []string
		for _, r := range found[:min(3, len(found))] {
			top = append(top, r.Name+" ("+r.Context+")")
		}
		t.Logf("%-18q %6v %d: %s", q, took.Round(time.Millisecond), len(found), strings.Join(top, "; "))
	}
}
