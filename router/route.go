package router

import (
	"errors"
	"math"
)

// Point is a position in degrees.
type Point struct{ Lat, Lon float64 }

// Route is the answer to one query.
type Route struct {
	Line      []Point
	DistanceM float64
	DurationS float64
	Ferry     bool    // the route crosses water by ferry
	SnapFromM float64 // how far the start was from the road it was moved to
	SnapToM   float64
	Steps     []Step // turn-by-turn directions
}

var (
	ErrStartTooFar  = errors.New("the start is too far from a road or path")
	ErrEndTooFar    = errors.New("the destination is too far from a road or path")
	ErrNotConnected = errors.New("the start and destination are not connected")
	ErrNoRoute      = errors.New("no route found")
)

const (
	// A flat boarding allowance per ferry; no timetables.
	ferryWaitS = 20 * 60
	// Fixed snapping limits. Pieces of the network with fewer than
	// mainNetwork junctions (car parks, odd footpaths) are only used when
	// nothing in the main network is within snapNear.
	snapNear    = 1000.0
	snapFar     = 5000.0
	mainNetwork = 500
)

type snapResult struct {
	seg   uint32
	at    Point   // the point on the segment
	along float64 // metres from the segment's start
	off   float64 // metres from the asked point
}

// How the best route reaches the destination.
const (
	viaNone = iota
	viaSameSegment
	viaEndFwd // into the destination segment from its start
	viaEndBwd // into the destination segment from its end
)

// Parent markers for the vertices next to the start.
const (
	seedFwd = math.MaxUint32
	seedBwd = math.MaxUint32 - 1
)

// Route finds the quickest route for mode m. One route is computed at a time.
func (g *Graph) Route(from, to Point, m Mode) (Route, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	a, ok := g.snap(from, m)
	if !ok {
		return Route{}, ErrStartTooFar
	}
	b, ok := g.snap(to, m)
	if !ok {
		return Route{}, ErrEndTooFar
	}
	if g.comp[m][g.sfrom[a.seg]] != g.comp[m][g.sfrom[b.seg]] {
		return Route{}, ErrNotConnected
	}
	g.prepare()

	// A*: the estimate is the straight-line distance at the fastest speed
	// in the graph, slightly shortened so it never overestimates.
	est := func(v uint32) float64 {
		return 0.99 * distM(deg(g.vlat[v]), deg(g.vlon[v]), b.at.Lat, b.at.Lon) / g.vmax[m]
	}
	la, lb := float64(g.slen[a.seg]), float64(g.slen[b.seg])
	va, vb := g.speed(a.seg, m), g.speed(b.seg, m)

	best, how, end := math.Inf(1), viaNone, uint32(0)
	if a.seg == b.seg {
		if g.usable(a.seg, false, m) && b.along >= a.along {
			best, how = (b.along-a.along)/va, viaSameSegment
		}
		if g.usable(a.seg, true, m) && b.along <= a.along && (a.along-b.along)/va < best {
			best, how = (a.along-b.along)/va, viaSameSegment
		}
	}
	if g.usable(a.seg, false, m) {
		g.relax(g.sto[a.seg], (la-a.along)/va, seedFwd, est)
	}
	if g.usable(a.seg, true, m) {
		g.relax(g.sfrom[a.seg], a.along/va, seedBwd, est)
	}

	for len(g.heap) > 0 {
		it := g.pop()
		if float64(it.f) >= best {
			break
		}
		u := it.v
		if it.g > g.dist[u] {
			continue // a stale entry: u was reached more cheaply since
		}
		du := float64(g.dist[u])
		if u == g.sfrom[b.seg] && g.usable(b.seg, false, m) && du+b.along/vb < best {
			best, how, end = du+b.along/vb, viaEndFwd, u
		}
		if u == g.sto[b.seg] && g.usable(b.seg, true, m) && du+(lb-b.along)/vb < best {
			best, how, end = du+(lb-b.along)/vb, viaEndBwd, u
		}
		for _, he := range g.adj[g.first[u]:g.first[u+1]] {
			s, rev := he>>1, he&1 == 1
			if !g.usable(s, rev, m) {
				continue
			}
			v := g.sto[s]
			if rev {
				v = g.sfrom[s]
			}
			cost := float64(g.slen[s]) / g.speed(s, m)
			if g.sflags[s]&ferry != 0 {
				cost += ferryWaitS
			}
			g.relax(v, du+cost, he, est)
		}
	}
	if how == viaNone {
		return Route{}, ErrNoRoute
	}
	return g.path(a, b, best, how, end, m), nil
}

// path rebuilds the route's geometry and length from the search.
func (g *Graph) path(a, b snapResult, cost float64, how int, end uint32, m Mode) (r Route) {
	r = Route{DurationS: cost, SnapFromM: a.off, SnapToM: b.off}
	legs := []leg{{seg: a.seg}}
	enter := func(s, v uint32) { legs = append(legs, leg{seg: s, v: v, at: len(r.Line) - 1, along: r.DistanceM}) }
	defer func() { r.Steps = g.steps(legs, r.Line, r.DistanceM, m) }()
	add := func(pts []Point) {
		for _, p := range pts {
			if n := len(r.Line); n == 0 || r.Line[n-1] != p {
				r.Line = append(r.Line, p)
			}
		}
	}
	add([]Point{a.at})
	if how == viaSameSegment {
		add(g.cut(a.seg, a.along, b.along))
		add([]Point{b.at})
		r.DistanceM = math.Abs(b.along - a.along)
		return r
	}

	var hes []uint32 // half-edges from the end back to the start
	v := end
	for i := 0; g.parent[v] != seedFwd && g.parent[v] != seedBwd; i++ {
		if i > len(g.vlat) {
			panic("router: broken search state") // unreachable: costs are positive
		}
		he := g.parent[v]
		hes = append(hes, he)
		if s := he >> 1; he&1 == 1 {
			v = g.sto[s]
		} else {
			v = g.sfrom[s]
		}
	}
	if la := float64(g.slen[a.seg]); g.parent[v] == seedFwd {
		add(g.cut(a.seg, a.along, la))
		r.DistanceM += la - a.along
	} else {
		add(g.cut(a.seg, a.along, 0))
		r.DistanceM += a.along
	}
	for i := len(hes) - 1; i >= 0; i-- {
		s := hes[i] >> 1
		l := float64(g.slen[s])
		if hes[i]&1 == 0 {
			enter(s, g.sfrom[s])
		} else {
			enter(s, g.sto[s])
		}
		if hes[i]&1 == 0 {
			add(g.cut(s, 0, l))
		} else {
			add(g.cut(s, l, 0))
		}
		r.DistanceM += l
		r.Ferry = r.Ferry || g.sflags[s]&ferry != 0
	}
	if lb := float64(g.slen[b.seg]); how == viaEndFwd {
		enter(b.seg, g.sfrom[b.seg])
		add(g.cut(b.seg, 0, b.along))
		r.DistanceM += b.along
	} else {
		enter(b.seg, g.sto[b.seg])
		add(g.cut(b.seg, lb, b.along))
		r.DistanceM += lb - b.along
	}
	add([]Point{b.at})
	return r
}

// cut returns segment s's geometry between two distances along it, in the
// order asked (from > to walks it backwards).
func (g *Graph) cut(s uint32, from, to float64) []Point {
	lo, hi := min(from, to), max(from, to)
	var out []Point
	pos, n := 0.0, g.npts(s)
	for i := 1; i < n; i++ {
		p, q := g.pt(s, i-1), g.pt(s, i)
		step := distM(p.Lat, p.Lon, q.Lat, q.Lon)
		next := pos + step
		if len(out) == 0 && lo <= next {
			out = append(out, lerp(p, q, (lo-pos)/step))
		}
		if len(out) > 0 && hi <= next {
			out = append(out, lerp(p, q, (hi-pos)/step))
			break
		}
		if len(out) > 0 {
			out = append(out, q)
		}
		pos = next
	}
	if len(out) == 0 { // rounding put both ends past the last point
		out = []Point{g.pt(s, n-1)}
	}
	if from > to {
		for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
			out[i], out[j] = out[j], out[i]
		}
	}
	return out
}

func lerp(p, q Point, t float64) Point {
	if math.IsNaN(t) || t < 0 {
		t = 0
	}
	t = min(t, 1)
	return Point{p.Lat + (q.Lat-p.Lat)*t, p.Lon + (q.Lon-p.Lon)*t}
}

// snap moves p onto the nearest road or path mode m may use: in the main
// network if one is within snapNear, else any within snapFar.
func (g *Graph) snap(p Point, m Mode) (snapResult, bool) {
	best, main := snapResult{off: math.Inf(1)}, snapResult{off: math.Inf(1)}
	cos := math.Cos(p.Lat * math.Pi / 180)
	dr := int(math.Ceil(snapFar / (cellLat * math.Pi / 180 * earthR)))
	dc := int(math.Ceil(snapFar / (cellLon * math.Pi / 180 * earthR * cos)))
	gr := g.grid
	r0 := int(math.Floor((p.Lat - gr.lat0) / cellLat))
	c0 := int(math.Floor((p.Lon - gr.lon0) / cellLon))
	for r := max(0, r0-dr); r <= min(gr.rows-1, r0+dr); r++ {
		for c := max(0, c0-dc); c <= min(gr.cols-1, c0+dc); c++ {
			cell := r*gr.cols + c
			for _, s := range gr.segs[gr.start[cell]:gr.start[cell+1]] {
				if !g.usableAny(s, m) {
					continue
				}
				cand := g.project(s, p)
				if cand.off < best.off {
					best = cand
				}
				if cand.off < main.off && g.compSize[m][g.comp[m][g.sfrom[s]]] >= mainNetwork {
					main = cand
				}
			}
		}
	}
	if main.off <= snapNear {
		return main, true
	}
	return best, best.off <= snapFar
}

// project finds the point of segment s nearest to p.
func (g *Graph) project(s uint32, p Point) snapResult {
	cos := math.Cos(p.Lat * math.Pi / 180)
	k := math.Pi / 180 * earthR
	xy := func(q Point) (float64, float64) { return (q.Lon - p.Lon) * cos * k, (q.Lat - p.Lat) * k }
	res := snapResult{seg: s, off: math.Inf(1)}
	pos := 0.0
	for i, n := 1, g.npts(s); i < n; i++ {
		a, b := g.pt(s, i-1), g.pt(s, i)
		ax, ay := xy(a)
		bx, by := xy(b)
		dx, dy := bx-ax, by-ay
		t := 0.0
		if l2 := dx*dx + dy*dy; l2 > 0 {
			t = max(0, min(1, -(ax*dx+ay*dy)/l2))
		}
		step := distM(a.Lat, a.Lon, b.Lat, b.Lon)
		if d := math.Hypot(ax+t*dx, ay+t*dy); d < res.off {
			res.off, res.at, res.along = d, lerp(a, b, t), pos+t*step
		}
		pos += step
	}
	res.along = min(res.along, float64(g.slen[s]))
	return res
}

// ---------- search state ----------

type item struct {
	f, g float32 // estimated total, cost so far
	v    uint32
}

// prepare readies the reused search arrays; stamp marks what this search
// has touched, so nothing needs clearing between routes.
func (g *Graph) prepare() {
	if len(g.dist) != len(g.vlat) {
		g.dist = make([]float32, len(g.vlat))
		g.parent = make([]uint32, len(g.vlat))
		g.stamp = make([]uint32, len(g.vlat))
	}
	g.round++
	if g.round == 0 {
		clear(g.stamp)
		g.round = 1
	}
	g.heap = g.heap[:0]
}

func (g *Graph) relax(v uint32, d float64, via uint32, est func(uint32) float64) {
	if g.stamp[v] == g.round && float64(g.dist[v]) <= d {
		return
	}
	g.stamp[v], g.dist[v], g.parent[v] = g.round, float32(d), via
	g.push(item{f: float32(d + est(v)), g: float32(d), v: v})
}

func (g *Graph) push(it item) {
	h := append(g.heap, it)
	for i := len(h) - 1; i > 0; {
		p := (i - 1) / 2
		if h[p].f <= h[i].f {
			break
		}
		h[p], h[i] = h[i], h[p]
		i = p
	}
	g.heap = h
}

func (g *Graph) pop() item {
	h := g.heap
	top := h[0]
	last := len(h) - 1
	h[0] = h[last]
	h = h[:last]
	for i := 0; ; {
		l, small := 2*i+1, i
		if l < len(h) && h[l].f < h[small].f {
			small = l
		}
		if r := l + 1; r < len(h) && h[r].f < h[small].f {
			small = r
		}
		if small == i {
			break
		}
		h[i], h[small] = h[small], h[i]
		i = small
	}
	g.heap = h
	return top
}
