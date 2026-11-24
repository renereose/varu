package router

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"strings"
	"sync"
)

// Graph is a road network ready for routing. The arrays listed in arrays()
// are what the file stores; the rest is rebuilt on load.
type Graph struct {
	meta Meta

	vlat, vlon []int32   // vertex positions, 1e-7 degrees
	sfrom, sto []uint32  // segment end vertices
	slen       []float32 // segment length, metres (full geometry)
	skmh       []uint8   // car speed, or the ferry's crossing speed; km/h
	sflags     []uint8   // carFwd, carBwd, footOK, ferry
	sshape     []uint32  // segment s has shape points sshape[s]:sshape[s+1]
	plat, plon []int32   // shape points between a segment's ends
	sname      []uint32  // segment name, an index into names (0: none)
	names      []string  // street names and road numbers, for directions

	first, adj []uint32    // half-edges (seg<<1 | reverse) leaving each vertex
	comp       [2][]uint32 // per mode: connected component of each vertex
	compSize   [2][]uint32 // per mode: vertices in each component
	vmax       [2]float64  // per mode: fastest speed in m/s, for the A* estimate
	grid       grid

	mu     sync.Mutex // one route at a time; the search state below is reused
	dist   []float32
	parent []uint32
	stamp  []uint32
	round  uint32
	heap   []item
}

var magic = [8]byte{'V', 'A', 'R', 'U', 'G', 'R', 'P', 'H'}

// Format 2 adds street names; format 1 files still load, without them.
const formatVersion = 2

// arrays lists the stored arrays in file order; WriteTo and Load both use it.
// Format 1 has all but the last.
func (g *Graph) arrays() []any {
	return []any{g.vlat, g.vlon, g.sfrom, g.sto, g.slen, g.skmh, g.sflags, g.sshape, g.plat, g.plon, g.sname}
}

// Meta says where the graph came from.
func (g *Graph) Meta() Meta { return g.meta }

// Stats counts the graph's vertices (junctions), segments and shape points.
func (g *Graph) Stats() (vertices, segments, shapePoints int) {
	return len(g.vlat), len(g.sfrom), len(g.plat)
}

type fileHead struct {
	Magic                  [8]byte
	Version, V, S, P, Meta uint32
}

func (g *Graph) WriteTo(w io.Writer) (int64, error) {
	meta, err := json.Marshal(g.meta)
	if err != nil {
		return 0, err
	}
	cw := &countWriter{w: bufio.NewWriterSize(w, 1<<20)}
	head := fileHead{magic, formatVersion, uint32(len(g.vlat)), uint32(len(g.sfrom)), uint32(len(g.plat)), uint32(len(meta))}
	if err := binary.Write(cw, binary.LittleEndian, head); err != nil {
		return cw.n, err
	}
	if _, err := cw.Write(meta); err != nil {
		return cw.n, err
	}
	// The names, after the empty first one, separated by NUL.
	names := []byte(strings.Join(g.names[1:], "\x00"))
	if err := binary.Write(cw, binary.LittleEndian, uint32(len(names))); err != nil {
		return cw.n, err
	}
	if _, err := cw.Write(names); err != nil {
		return cw.n, err
	}
	for _, a := range g.arrays() {
		if err := inChunks(a, func(part any) error { return binary.Write(cw, binary.LittleEndian, part) }); err != nil {
			return cw.n, err
		}
	}
	return cw.n, cw.w.(*bufio.Writer).Flush()
}

// Load reads a graph written by WriteTo and checks it is whole and sound.
func Load(path string) (*Graph, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	r := bufio.NewReaderSize(f, 1<<20)
	var h fileHead
	if err := binary.Read(r, binary.LittleEndian, &h); err != nil || h.Magic != magic {
		return nil, errors.New("not a Varu routing file")
	}
	if h.Version != 1 && h.Version != formatVersion {
		return nil, fmt.Errorf("routing data format %d; this Varu reads format %d", h.Version, formatVersion)
	}
	if h.Meta > 1<<16 {
		return nil, errors.New("routing data is damaged")
	}
	g := &Graph{names: []string{""}}
	meta := make([]byte, h.Meta)
	if _, err := io.ReadFull(r, meta); err != nil || json.Unmarshal(meta, &g.meta) != nil {
		return nil, errors.New("routing data is damaged")
	}
	want := int64(binary.Size(h)) + int64(h.Meta) + 8*int64(h.V) + 14*int64(h.S) + 4*(int64(h.S)+1) + 8*int64(h.P)
	arrays := 10
	if h.Version >= 2 {
		var n uint32
		if binary.Read(r, binary.LittleEndian, &n) != nil || int64(n) > info.Size() {
			return nil, errors.New("routing data is damaged or incomplete")
		}
		names := make([]byte, n)
		if _, err := io.ReadFull(r, names); err != nil {
			return nil, errors.New("routing data is damaged or incomplete")
		}
		if n > 0 {
			g.names = append(g.names, strings.Split(string(names), "\x00")...)
		}
		want += 4 + int64(n) + 4*int64(h.S)
		arrays = 11
	}
	if info.Size() != want {
		return nil, errors.New("routing data is damaged or incomplete")
	}
	g.vlat, g.vlon = make([]int32, h.V), make([]int32, h.V)
	g.sfrom, g.sto, g.slen = make([]uint32, h.S), make([]uint32, h.S), make([]float32, h.S)
	g.skmh, g.sflags, g.sshape = make([]uint8, h.S), make([]uint8, h.S), make([]uint32, h.S+1)
	g.plat, g.plon = make([]int32, h.P), make([]int32, h.P)
	g.sname = make([]uint32, h.S)
	for _, a := range g.arrays()[:arrays] {
		if err := inChunks(a, func(part any) error { return binary.Read(r, binary.LittleEndian, part) }); err != nil {
			return nil, errors.New("routing data is damaged or incomplete")
		}
	}
	for s := range g.sfrom {
		if g.sfrom[s] >= h.V || g.sto[s] >= h.V || g.sshape[s] > g.sshape[s+1] || int(g.sname[s]) >= len(g.names) {
			return nil, errors.New("routing data is damaged")
		}
	}
	if g.sshape[0] != 0 || g.sshape[h.S] != h.P {
		return nil, errors.New("routing data is damaged")
	}
	g.index()
	return g, nil
}

// inChunks hands a stored array to fn 64k elements at a time, so reading
// and writing never hold a second copy of a large array.
func inChunks(a any, fn func(part any) error) error {
	const step = 1 << 16
	var n int
	var part func(i, j int) any
	switch s := a.(type) {
	case []int32:
		n, part = len(s), func(i, j int) any { return s[i:j] }
	case []uint32:
		n, part = len(s), func(i, j int) any { return s[i:j] }
	case []float32:
		n, part = len(s), func(i, j int) any { return s[i:j] }
	case []uint8:
		n, part = len(s), func(i, j int) any { return s[i:j] }
	default:
		panic("router: unexpected array type")
	}
	for i := 0; i < n; i += step {
		if err := fn(part(i, min(i+step, n))); err != nil {
			return err
		}
	}
	return nil
}

type countWriter struct {
	w io.Writer
	n int64
}

func (c *countWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.n += int64(n)
	return n, err
}

// usable says whether mode m may travel segment s in that direction.
func (g *Graph) usable(s uint32, reverse bool, m Mode) bool {
	f := g.sflags[s]
	if m == Foot {
		return f&footOK != 0
	}
	if reverse {
		return f&carBwd != 0
	}
	return f&carFwd != 0
}

func (g *Graph) usableAny(s uint32, m Mode) bool {
	return g.usable(s, false, m) || g.usable(s, true, m)
}

// speed is how fast mode m travels segment s, in m/s.
func (g *Graph) speed(s uint32, m Mode) float64 {
	if m == Foot && g.sflags[s]&ferry == 0 {
		return footKmh / 3.6
	}
	return float64(g.skmh[s]) / 3.6
}

// index rebuilds what the file does not store: adjacency, per-mode
// components, the A* speed bound and the snapping grid.
func (g *Graph) index() {
	nv, ns := len(g.vlat), len(g.sfrom)
	g.first = make([]uint32, nv+1)
	for s := 0; s < ns; s++ {
		g.first[g.sfrom[s]+1]++
		g.first[g.sto[s]+1]++
	}
	for v := 1; v <= nv; v++ {
		g.first[v] += g.first[v-1]
	}
	g.adj = make([]uint32, 2*ns)
	fill := make([]uint32, nv)
	copy(fill, g.first[:nv])
	for s := 0; s < ns; s++ {
		a, b := g.sfrom[s], g.sto[s]
		g.adj[fill[a]] = uint32(s) << 1
		fill[a]++
		g.adj[fill[b]] = uint32(s)<<1 | 1
		fill[b]++
	}

	for _, m := range []Mode{Foot, Car} {
		comp := make([]uint32, nv)
		for i := range comp {
			comp[i] = math.MaxUint32
		}
		var sizes []uint32
		stack := make([]uint32, 0, 1024)
		for start := 0; start < nv; start++ {
			if comp[start] != math.MaxUint32 {
				continue
			}
			id, size := uint32(len(sizes)), uint32(0)
			comp[start] = id
			stack = append(stack[:0], uint32(start))
			for len(stack) > 0 {
				u := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				size++
				for _, he := range g.adj[g.first[u]:g.first[u+1]] {
					s := he >> 1
					if !g.usableAny(s, m) {
						continue
					}
					v := g.sto[s]
					if he&1 == 1 {
						v = g.sfrom[s]
					}
					if comp[v] == math.MaxUint32 {
						comp[v] = id
						stack = append(stack, v)
					}
				}
			}
			sizes = append(sizes, size)
		}
		g.comp[m], g.compSize[m] = comp, sizes

		g.vmax[m] = footKmh / 3.6
		for s := 0; s < ns; s++ {
			if g.usableAny(uint32(s), m) {
				g.vmax[m] = max(g.vmax[m], g.speed(uint32(s), m))
			}
		}
	}
	g.grid = newGrid(g)
}

// grid finds the segments near a point: cells of about 1.1 km.
type grid struct {
	lat0, lon0 float64
	rows, cols int
	start      []uint32 // cell c holds segs[start[c]:start[c+1]]
	segs       []uint32
}

const cellLat, cellLon = 0.01, 0.02

func newGrid(g *Graph) grid {
	minLat, minLon, maxLat, maxLon := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
	for v := range g.vlat {
		minLat, maxLat = min(minLat, deg(g.vlat[v])), max(maxLat, deg(g.vlat[v]))
		minLon, maxLon = min(minLon, deg(g.vlon[v])), max(maxLon, deg(g.vlon[v]))
	}
	for p := range g.plat {
		minLat, maxLat = min(minLat, deg(g.plat[p])), max(maxLat, deg(g.plat[p]))
		minLon, maxLon = min(minLon, deg(g.plon[p])), max(maxLon, deg(g.plon[p]))
	}
	gr := grid{lat0: minLat, lon0: minLon}
	gr.rows = int((maxLat-minLat)/cellLat) + 1
	gr.cols = int((maxLon-minLon)/cellLon) + 1

	// Each segment goes into every cell its bounding box touches; ferries
	// never, as nobody starts or ends a route on the water.
	cells := func(s uint32, visit func(c int)) {
		lo, hi := Point{math.Inf(1), math.Inf(1)}, Point{math.Inf(-1), math.Inf(-1)}
		for i, n := 0, g.npts(s); i < n; i++ {
			p := g.pt(s, i)
			lo = Point{min(lo.Lat, p.Lat), min(lo.Lon, p.Lon)}
			hi = Point{max(hi.Lat, p.Lat), max(hi.Lon, p.Lon)}
		}
		r0, c0 := gr.cell(lo)
		r1, c1 := gr.cell(hi)
		for r := r0; r <= r1; r++ {
			for c := c0; c <= c1; c++ {
				visit(r*gr.cols + c)
			}
		}
	}
	gr.start = make([]uint32, gr.rows*gr.cols+1)
	for s := range g.sfrom {
		if g.sflags[s]&ferry == 0 {
			cells(uint32(s), func(c int) { gr.start[c+1]++ })
		}
	}
	for c := 1; c < len(gr.start); c++ {
		gr.start[c] += gr.start[c-1]
	}
	gr.segs = make([]uint32, gr.start[len(gr.start)-1])
	fill := make([]uint32, len(gr.start)-1)
	copy(fill, gr.start)
	for s := range g.sfrom {
		if g.sflags[s]&ferry == 0 {
			cells(uint32(s), func(c int) { gr.segs[fill[c]] = uint32(s); fill[c]++ })
		}
	}
	return gr
}

// cell returns the row and column of p, clamped to the grid.
func (gr grid) cell(p Point) (int, int) {
	r := int(math.Floor((p.Lat - gr.lat0) / cellLat))
	c := int(math.Floor((p.Lon - gr.lon0) / cellLon))
	return max(0, min(gr.rows-1, r)), max(0, min(gr.cols-1, c))
}

// npts and pt walk a segment's geometry: start vertex, shape points, end vertex.
func (g *Graph) npts(s uint32) int { return int(g.sshape[s+1]-g.sshape[s]) + 2 }

func (g *Graph) pt(s uint32, i int) Point {
	switch {
	case i == 0:
		return g.vpt(g.sfrom[s])
	case i == g.npts(s)-1:
		return g.vpt(g.sto[s])
	}
	j := g.sshape[s] + uint32(i-1)
	return Point{deg(g.plat[j]), deg(g.plon[j])}
}

func (g *Graph) vpt(v uint32) Point { return Point{deg(g.vlat[v]), deg(g.vlon[v])} }
