package main

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"math/bits"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"unicode"
)

// places answers GET /api/places?q=text[&near=lat,lon] with up to ten
// towns, streets and addresses from the file the manifest lists
// (builder/places.py writes it). Loaded on the first search and kept.
type places struct {
	root string
	mu   sync.Mutex
	ix   *placeIndex
}

// placeIndex keeps names in two long strings (as written and folded for
// matching), so the entries hold no pointers and cost the collector nothing.
type placeIndex struct {
	contexts []string // "Pärnu linn, Pärnu maakond"
	folded   []string // the same, folded
	names    string
	folds    string
	entries  []place
}

type place struct {
	lon, lat     float32
	ctx          uint32
	addresses    uint32 // how many it has: 1 for an address
	name, fold   uint32 // offsets into names and folds
	nameN, foldN uint8
	kind         byte // 'p' area, 's' street, 'a' address
}

type placeResult struct {
	Name    string  `json:"name"`
	Context string  `json:"context"`
	Kind    string  `json:"kind"` // area, street or address
	Lon     float64 `json:"lon"`
	Lat     float64 `json:"lat"`
}

var errNoPlaces = errors.New("no places on this drive")

const placeLimit = 10

func (p *places) index() (*placeIndex, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.ix != nil {
		return p.ix, nil
	}
	var m struct {
		Places struct {
			File string `json:"file"`
		} `json:"places"`
	}
	raw, err := os.ReadFile(filepath.Join(p.root, "manifest.json"))
	if err != nil || json.Unmarshal(raw, &m) != nil || m.Places.File == "" {
		return nil, errNoPlaces
	}
	rel := filepath.FromSlash(m.Places.File)
	if !filepath.IsLocal(rel) {
		return nil, errNoPlaces
	}
	f, err := os.Open(filepath.Join(p.root, rel))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, errNoPlaces
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	ix, err := readPlaces(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", rel, err)
	}
	p.ix = ix
	return ix, nil
}

func readPlaces(r io.Reader) (*placeIndex, error) {
	z, err := gzip.NewReader(r)
	if err != nil {
		return nil, err
	}
	sc := bufio.NewScanner(z)
	sc.Buffer(make([]byte, 64<<10), 64<<10)
	if !sc.Scan() || sc.Text() != "varu-places\t1" {
		return nil, errors.New("not a Varu places file, or a newer format")
	}
	ix := &placeIndex{}
	var names, folds strings.Builder
	for sc.Scan() {
		f := strings.Split(sc.Text(), "\t")
		if len(f) == 2 && f[0] == "c" {
			ix.contexts = append(ix.contexts, f[1])
			ix.folded = append(ix.folded, fold(f[1]))
			continue
		}
		if len(f) != 6 || len(f[0]) != 1 || !strings.Contains("psa", f[0]) {
			return nil, fmt.Errorf("bad line %q", sc.Text())
		}
		ctx, err1 := strconv.ParseUint(f[1], 10, 32)
		lon, err2 := strconv.ParseFloat(f[2], 32)
		lat, err3 := strconv.ParseFloat(f[3], 32)
		count, err4 := strconv.ParseUint(f[4], 10, 32)
		name := f[5]
		folded := fold(name)
		if err1 != nil || err2 != nil || err3 != nil || err4 != nil || count == 0 || int(ctx) >= len(ix.contexts) {
			return nil, fmt.Errorf("bad line %q", sc.Text())
		}
		if len(name) > 255 || len(folded) > 255 {
			continue // ponytail: no real name is this long
		}
		ix.entries = append(ix.entries, place{
			lon: float32(lon), lat: float32(lat), ctx: uint32(ctx), addresses: uint32(count), kind: f[0][0],
			name: uint32(names.Len()), nameN: uint8(len(name)),
			fold: uint32(folds.Len()), foldN: uint8(len(folded)),
		})
		names.WriteString(name)
		folds.WriteString(folded)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	ix.names, ix.folds = names.String(), folds.String()
	return ix, nil
}

// fold lowercases, drops accents ("Pärnu" → "parnu") and turns everything
// but letters, digits and "/" into single spaces. Street types also match in
// full: "tn" is "tänav", so "Rüütli tänav" finds "Rüütli tn".
func fold(s string) string {
	var b strings.Builder
	space := true
	for _, r := range strings.ToLower(s) {
		if plain, ok := unaccent[r]; ok {
			r = plain
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '/' { // "21/1" is one house number
			b.WriteRune(r)
			space = false
		} else if !space {
			b.WriteByte(' ')
			space = true
		}
	}
	out := strings.TrimSuffix(b.String(), " ")
	for short, long := range streetTypes {
		if strings.HasPrefix(out, short+" ") || strings.Contains(out, " "+short+" ") || strings.HasSuffix(out, " "+short) {
			out += " " + long
		}
	}
	return out
}

var streetTypes = map[string]string{"tn": "tanav", "mnt": "maantee", "pst": "puiestee"}

// ponytail: Latin letters with accents only; names in other scripts match as written.
var unaccent = func() map[rune]rune {
	m := map[rune]rune{}
	for plain, accented := range map[rune]string{
		'a': "àáâãäåāăą", 'c': "çćčĉċ", 'd': "ďđ", 'e': "èéêëēĕėęě", 'g': "ğĝġģ", 'i': "ìíîïīĭįı",
		'k': "ķ", 'l': "ĺļľŀł", 'n': "ñńņňŉ", 'o': "òóôõöøōŏő", 'r': "ŕŗř", 's': "śŝşšș",
		't': "ţťŧț", 'u': "ùúûüũūŭůűų", 'y': "ýÿŷ", 'z': "źżž",
	} {
		for _, r := range accented {
			m[r] = plain
		}
	}
	return m
}()

// search: every word typed must start a word of the name or of where it
// lies, and at least one must be in the name. Areas come first, then
// streets, then addresses; within each, the most words found in the name,
// the fewest typed only in part, names that start with the first word,
// and then distance to near divided by size.
//
// ponytail: a scan of every entry, 20-60 ms for Estonia's 776,000 on a laptop.
// A sorted word index if a bigger region makes it slow.
func (ix *placeIndex) search(q string, near *[2]float64) []placeResult {
	words := strings.Fields(fold(q))
	if len(words) == 0 {
		return nil
	}
	if len(words) > 8 {
		words = words[:8]
	}
	all := uint8(1)<<len(words) - 1
	spaced := make([]string, len(words))
	for i, w := range words {
		spaced[i] = " " + w
	}
	starts := func(s string, i int) bool { return strings.HasPrefix(s, words[i]) || strings.Contains(s, spaced[i]) }
	// A word typed in full ("22") ranks above one that only starts a word ("227").
	whole := func(s string, i int) bool {
		for rest := " " + s + " "; ; {
			at := strings.Index(rest, spaced[i])
			if at < 0 {
				return false
			}
			if rest[at+len(spaced[i])] == ' ' {
				return true
			}
			rest = rest[at+1:]
		}
	}
	inContext := make([]uint8, len(ix.folded))
	wholeInContext := make([]uint8, len(ix.folded))
	for c, s := range ix.folded {
		for i := range words {
			if starts(s, i) {
				inContext[c] |= 1 << i
				if whole(s, i) {
					wholeInContext[c] |= 1 << i
				}
			}
		}
	}

	type hit struct {
		e     *place
		score [5]float64
	}
	var best []hit
	for k := range ix.entries {
		e := &ix.entries[k]
		name := ix.folds[e.fold : e.fold+uint32(e.foldN)]
		var mask uint8
		for i := range words {
			if starts(name, i) {
				mask |= 1 << i
			}
		}
		if mask == 0 || mask|inContext[e.ctx] != all {
			continue
		}
		exact := wholeInContext[e.ctx]
		for i := range words {
			if mask&(1<<i) != 0 && whole(name, i) {
				exact |= 1 << i
			}
		}
		score := [5]float64{float64(strings.IndexByte("psa", e.kind)), -float64(bits.OnesCount8(mask)),
			float64(bits.OnesCount8(all &^ exact)), 1, 0}
		if strings.HasPrefix(name, words[0]) {
			score[3] = 0
		}
		// Bigger and nearer first: Tartu linn before Tartu vald, and of two
		// villages of one name, the nearer.
		score[4] = -float64(e.addresses)
		if near != nil {
			dx := (float64(e.lon) - near[0]) * math.Cos(near[1]*math.Pi/180)
			dy := float64(e.lat) - near[1]
			score[4] = (dx*dx + dy*dy) / float64(e.addresses)
		}
		if len(best) == placeLimit && !less(score, best[placeLimit-1].score) {
			continue
		}
		i := len(best)
		if i < placeLimit {
			best = append(best, hit{})
		} else {
			i--
		}
		for ; i > 0 && less(score, best[i-1].score); i-- {
			best[i] = best[i-1]
		}
		best[i] = hit{e, score}
	}

	out := make([]placeResult, len(best))
	for i, h := range best {
		out[i] = placeResult{
			Name:    ix.names[h.e.name : h.e.name+uint32(h.e.nameN)],
			Context: ix.contexts[h.e.ctx],
			Kind:    map[byte]string{'p': "area", 's': "street", 'a': "address"}[h.e.kind],
			Lon:     round(float64(h.e.lon), 5),
			Lat:     round(float64(h.e.lat), 5),
		}
	}
	return out
}

func less(a, b [5]float64) bool {
	for i := range a {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return false
}

func (p *places) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	var near *[2]float64
	if s := q.Get("near"); s != "" {
		pt, ok := parsePoint(s)
		if !ok {
			routeError(w, http.StatusBadRequest, "badRequest")
			return
		}
		near = &[2]float64{pt.Lon, pt.Lat}
	}
	ix, err := p.index()
	if errors.Is(err, errNoPlaces) {
		routeError(w, http.StatusNotFound, "noPlaces")
		return
	}
	if err != nil {
		routeError(w, http.StatusInternalServerError, "badPlaces")
		return
	}
	found := ix.search(q.Get("q"), near)
	if found == nil {
		found = []placeResult{}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(found)
}
