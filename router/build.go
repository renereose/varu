package router

import (
	"bufio"
	"errors"
	"fmt"
	"math"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Mode is how a route is travelled.
type Mode uint8

const (
	Foot Mode = iota
	Car
)

func ParseMode(s string) (Mode, error) {
	switch s {
	case "foot":
		return Foot, nil
	case "car":
		return Car, nil
	}
	return 0, fmt.Errorf("unknown mode %q (use foot or car)", s)
}

// Segment flags.
const (
	carFwd = 1 << iota
	carBwd
	footOK
	ferry
	roundabout
)

const footKmh = 5

// rule is what one way allows.
type rule struct {
	kmh      uint8 // car speed, or the crossing speed of a ferry; 0 = no cars
	flags    uint8
	ferrySec uint32 // crossing time from the duration tag; 0 = unknown
	name     string // for directions: the street's name, else its road number
}

var carKmh = map[string]uint8{
	"motorway": 100, "trunk": 85, "primary": 75, "secondary": 65, "tertiary": 55,
	"unclassified": 45, "residential": 25, "living_street": 10, "service": 15,
	"track": 15, "road": 30,
	"motorway_link": 45, "trunk_link": 45, "primary_link": 45, "secondary_link": 45, "tertiary_link": 45,
}

var footOnly = map[string]bool{
	"footway": true, "path": true, "pedestrian": true, "steps": true, "cycleway": true, "bridleway": true,
}

var (
	denied  = map[string]bool{"no": true, "private": true, "agricultural": true, "forestry": true, "military": true}
	allowed = map[string]bool{"yes": true, "designated": true, "permissive": true, "destination": true, "customers": true, "delivery": true}
)

// firstTag returns the value of the first of keys the way has: the most
// specific access tag wins.
func firstTag(tag func(string) string, keys ...string) string {
	for _, k := range keys {
		if v := tag(k); v != "" {
			return v
		}
	}
	return ""
}

// wayRule says whether cars and walkers may use a way, and how fast cars go.
// ponytail: no turn restrictions, barriers, or junction and signal delays;
// walking is a flat 5 km/h in both directions; maxspeed is taken as the
// travel speed. Times are estimates, which the viewer says.
func wayRule(tag func(string) string) (rule, bool) {
	var r rule
	highway := tag("highway")
	switch {
	case tag("route") == "ferry":
		r.flags = ferry | carFwd | carBwd | footOK
		r.ferrySec, _ = parseDuration(tag("duration"))
	case carKmh[highway] > 0:
		r.kmh = carKmh[highway]
		r.flags = carFwd | carBwd | footOK
		if highway == "motorway" || highway == "motorway_link" {
			r.flags &^= footOK
		}
		if v, ok := parseMaxspeed(tag("maxspeed")); ok {
			r.kmh = uint8(v)
		}
	case footOnly[highway]:
		r.flags = footOK
	default:
		return r, false
	}

	if denied[firstTag(tag, "motorcar", "motor_vehicle", "vehicle", "access")] {
		r.flags &^= carFwd | carBwd
	}
	switch foot := firstTag(tag, "foot", "access"); {
	case denied[foot]:
		r.flags &^= footOK
	case allowed[foot] && tag("foot") != "":
		r.flags |= footOK
	}

	r.name = firstTag(tag, "name", "ref")
	if r.flags&ferry == 0 {
		oneway, junction := tag("oneway"), tag("junction")
		if junction == "roundabout" || junction == "circular" {
			r.flags |= roundabout
		}
		implied := highway == "motorway" || highway == "motorway_link" || junction == "roundabout" || junction == "circular"
		switch {
		case oneway == "-1" || oneway == "reverse":
			r.flags &^= carFwd
		case oneway == "reversible" || oneway == "alternating":
			r.flags &^= carFwd | carBwd
		case oneway == "yes" || oneway == "true" || oneway == "1" || (implied && oneway != "no"):
			r.flags &^= carBwd
		}
	}
	if r.flags&(carFwd|carBwd|footOK) == 0 {
		return r, false
	}
	if r.flags&(carFwd|carBwd) == 0 && r.flags&ferry == 0 {
		r.kmh = 0
	}
	return r, true
}

// parseMaxspeed reads "50", "50 km/h", "EE:urban" or "EE:rural".
func parseMaxspeed(v string) (int, bool) {
	switch v {
	case "EE:urban":
		return 50, true
	case "EE:rural":
		return 90, true
	}
	end := 0
	for end < len(v) && v[end] >= '0' && v[end] <= '9' {
		end++
	}
	n, err := strconv.Atoi(v[:end])
	if err != nil || n <= 0 || n > 130 || strings.Contains(v[end:], "mph") {
		return 0, false
	}
	return n, true
}

// parseDuration reads a ferry's "MM", "HH:MM" or "HH:MM:SS" into seconds.
func parseDuration(v string) (uint32, bool) {
	parts := strings.Split(v, ":")
	if v == "" || len(parts) > 3 {
		return 0, false
	}
	total := 0
	for _, p := range parts {
		n, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil || n < 0 {
			return 0, false
		}
		total = total*60 + n
	}
	if len(parts) < 3 { // minutes so far
		total *= 60
	}
	return uint32(total), total > 0
}

// Way is an OpenStreetMap way, for building a graph without an extract.
type Way struct {
	ID    int64
	Nodes []int64
	Tags  map[string]string
}

// Meta records where a graph came from.
type Meta struct {
	Source  string `json:"source"`
	OSMTime string `json:"osm_timestamp,omitempty"`
	Built   string `json:"built"`
}

// Build makes a graph from ways and node positions (latitude, longitude).
func Build(ways []Way, nodes map[int64][2]float64, meta Meta) (*Graph, error) {
	var kept keptWays
	for _, w := range ways {
		if r, ok := wayRule(func(k string) string { return w.Tags[k] }); ok {
			kept.add(w.Nodes, r)
		}
	}
	return kept.assemble(meta, func(ids []int64, lat, lon []int32, have []bool) error {
		for i, id := range ids {
			if p, ok := nodes[id]; ok {
				lat[i], lon[i], have[i] = int32(math.Round(p[0]*1e7)), int32(math.Round(p[1]*1e7)), true
			}
		}
		return nil
	})
}

// BuildPBF makes a graph from an OpenStreetMap extract, reading it twice:
// first the ways, then the positions of the nodes those ways use.
func BuildPBF(path string, meta Meta) (*Graph, error) {
	pass := func(h pbfHandler) (string, error) {
		f, err := os.Open(path)
		if err != nil {
			return "", err
		}
		defer f.Close()
		return readPBF(bufio.NewReaderSize(f, 1<<20), h)
	}
	var kept keptWays
	osmTime, err := pass(pbfHandler{way: func(_ int64, refs func() []int64, tag func(string) string) {
		if r, ok := wayRule(tag); ok {
			kept.add(refs(), r)
		}
	}})
	if err != nil {
		return nil, err
	}
	if meta.OSMTime == "" {
		meta.OSMTime = osmTime
	}
	return kept.assemble(meta, func(ids []int64, lat, lon []int32, have []bool) error {
		_, err := pass(pbfHandler{node: func(id int64, la, lo int32) {
			if i, ok := slices.BinarySearch(ids, id); ok {
				lat[i], lon[i], have[i] = la, lo, true
			}
		}})
		return err
	})
}

// keptWays holds the routable ways compactly: way i uses
// refs[starts[i]:starts[i+1]].
type keptWays struct {
	refs   []int64
	starts []uint32
	rules  []rule
}

func (k *keptWays) add(refs []int64, r rule) {
	if len(refs) < 2 {
		return
	}
	if len(k.starts) == 0 {
		k.starts = append(k.starts, 0)
	}
	k.refs = append(k.refs, refs...)
	k.starts = append(k.starts, uint32(len(k.refs)))
	k.rules = append(k.rules, r)
}

// assemble turns the kept ways into a graph. Junctions (nodes used more than
// once, and way ends) become vertices; the nodes between them are kept as
// shape points so routes follow the road.
func (k *keptWays) assemble(meta Meta, locate func(ids []int64, lat, lon []int32, have []bool) error) (*Graph, error) {
	if len(k.rules) == 0 {
		return nil, errors.New("no roads or paths in the data")
	}
	ids := slices.Clone(k.refs)
	slices.Sort(ids)
	junction := make([]bool, 0, len(ids)/2)
	n := 0
	for _, id := range ids {
		if n > 0 && id == ids[n-1] {
			junction[n-1] = true
			continue
		}
		ids[n] = id
		n++
		junction = append(junction, false)
	}
	ids = slices.Clip(ids[:n])
	index := func(id int64) int { i, _ := slices.BinarySearch(ids, id); return i }
	for w := range k.rules {
		junction[index(k.refs[k.starts[w]])] = true
		junction[index(k.refs[k.starts[w+1]-1])] = true
	}

	lat, lon, have := make([]int32, n), make([]int32, n), make([]bool, n)
	if err := locate(ids, lat, lon, have); err != nil {
		return nil, err
	}

	g := &Graph{meta: meta, sshape: []uint32{0}, names: []string{""}}
	nameOf := map[string]uint32{"": 0}
	if g.meta.Built == "" {
		g.meta.Built = time.Now().UTC().Format(time.RFC3339)
	}
	vertexOf := make([]int32, n)
	for i := range vertexOf {
		vertexOf[i] = -1
	}
	vertex := func(i int) uint32 {
		if vertexOf[i] < 0 {
			vertexOf[i] = int32(len(g.vlat))
			g.vlat, g.vlon = append(g.vlat, lat[i]), append(g.vlon, lon[i])
		}
		return uint32(vertexOf[i])
	}
	step := func(i, j int) float64 { return distM(deg(lat[i]), deg(lon[i]), deg(lat[j]), deg(lon[j])) }

	for w, r := range k.rules {
		refs := k.refs[k.starts[w]:k.starts[w+1]]
		name, ok := nameOf[r.name]
		if !ok {
			name = uint32(len(g.names))
			nameOf[r.name] = name
			g.names = append(g.names, r.name)
		}
		kmh := r.kmh
		if r.flags&ferry != 0 {
			kmh = ferryKmh(r.ferrySec, refs, index, have, step)
		}
		start, prev, length := -1, -1, 0.0 // start: the node the piece in progress began at
		for _, id := range refs {
			i := index(id)
			if i == prev {
				continue // the same node twice in a row
			}
			if !have[i] {
				// ponytail: a node missing from the extract splits the way; the piece in progress is dropped.
				start, prev = -1, -1
				g.plat, g.plon = g.plat[:g.sshape[len(g.sshape)-1]], g.plon[:g.sshape[len(g.sshape)-1]]
				continue
			}
			if start < 0 {
				start, prev, length = i, i, 0
				continue
			}
			length += step(prev, i)
			prev = i
			if !junction[i] {
				g.plat, g.plon = append(g.plat, lat[i]), append(g.plon, lon[i])
				continue
			}
			if i == start && len(g.plat) == int(g.sshape[len(g.sshape)-1]) {
				length = 0
				continue // back where it began with nothing between: no segment
			}
			g.sfrom, g.sto = append(g.sfrom, vertex(start)), append(g.sto, vertex(i))
			g.slen, g.skmh, g.sflags = append(g.slen, float32(length)), append(g.skmh, kmh), append(g.sflags, r.flags)
			g.sname = append(g.sname, name)
			g.sshape = append(g.sshape, uint32(len(g.plat)))
			start, length = i, 0
		}
		// Shape points after the last junction (a way ending at a gap) are dropped.
		g.plat, g.plon = g.plat[:g.sshape[len(g.sshape)-1]], g.plon[:g.sshape[len(g.sshape)-1]]
	}
	if len(g.sfrom) == 0 {
		return nil, errors.New("no usable road segments")
	}
	g.index()
	return g, nil
}

// ferryKmh is a ferry way's crossing speed: its length over its duration,
// or 20 km/h when the duration is not tagged.
func ferryKmh(sec uint32, refs []int64, index func(int64) int, have []bool, step func(i, j int) float64) uint8 {
	if sec == 0 {
		return 20
	}
	total, prev := 0.0, -1
	for _, id := range refs {
		i := index(id)
		if !have[i] {
			prev = -1
			continue
		}
		if prev >= 0 {
			total += step(prev, i)
		}
		prev = i
	}
	return uint8(max(1, min(255, math.Round(total/float64(sec)*3.6))))
}

const earthR = 6371008.8

// distM is the distance in metres between two points; equirectangular,
// accurate to well under 0.5% at the distances a country's roads span.
func distM(lat1, lon1, lat2, lon2 float64) float64 {
	k := math.Pi / 180 * earthR
	x := (lon2 - lon1) * math.Cos((lat1+lat2)/2*math.Pi/180) * k
	y := (lat2 - lat1) * k
	return math.Hypot(x, y)
}

func deg(v int32) float64 { return float64(v) * 1e-7 }

func unixUTC(s int64) string { return time.Unix(s, 0).UTC().Format(time.RFC3339) }
