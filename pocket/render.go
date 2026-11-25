package main

// One screen of map: the tiles under the view drawn rule by rule, then
// the scale bar, home, place symbols and names on top.

import (
	"fmt"
	"image"
	"math"
	"slices"
	"sort"

	"github.com/renereose/varu/pmtiles"
)

const tilePx = 512 // MapLibre's zoom scale: the world is 512·2^zoom px wide

type drive struct {
	tiles   *pmtiles.File
	maxZoom int
	font    *font
	pois    map[string][]pt // longitude, latitude
	labels  map[string]string
	order   []string // layer ids as the drive lists them
	home    *pt
}

type view struct {
	lat, lon float64
	zoom     int
	w, h     int
	grey     bool
}

// world gives the view's top left corner in world px and px per world unit.
func (v view) world() (left, top, size float64) {
	size = tilePx * math.Exp2(float64(v.zoom))
	return pmtiles.MercatorX(v.lon)*size - float64(v.w)/2, pmtiles.MercatorY(v.lat)*size - float64(v.h)/2, size
}

func (v view) project(lon, lat float64) pt {
	left, top, size := v.world()
	return pt{pmtiles.MercatorX(lon)*size - left, pmtiles.MercatorY(lat)*size - top}
}

type tileRef struct {
	z    int
	x, y uint32
	rect image.Rectangle // where it lands on the screen
}

// viewTiles lists the tiles under the view, at the zoom asked for or the
// file's deepest, each drawn bigger when the view zooms in further.
func viewTiles(v view, maxZoom int) []tileRef {
	z := min(v.zoom, maxZoom)
	px := tilePx * math.Exp2(float64(v.zoom-z))
	left, top, size := v.world()
	n := size / px
	var out []tileRef
	for ty := math.Floor(top / px); ty*px < top+float64(v.h); ty++ {
		for tx := math.Floor(left / px); tx*px < left+float64(v.w); tx++ {
			if tx < 0 || ty < 0 || tx >= n || ty >= n {
				continue
			}
			x0, y0 := int(math.Round(tx*px-left)), int(math.Round(ty*px-top))
			out = append(out, tileRef{z, uint32(tx), uint32(ty), image.Rect(x0, y0, x0+int(px), y0+int(px))})
		}
	}
	return out
}

// piece is a tile's content placed on the screen: px = o + unit·k, drawn
// only inside clip.
type piece struct {
	layers map[string]*layer
	clip   image.Rectangle
	ox, oy float64
	k      float64
}

func (p piece) screen(rings [][]pt) [][]pt {
	out := make([][]pt, len(rings))
	for i, r := range rings {
		out[i] = make([]pt, len(r))
		for j, q := range r {
			out[i][j] = pt{p.ox + q.x*p.k, p.oy + q.y*p.k}
		}
	}
	return out
}

// load reads each tile, or the nearest coarser tile that covers it when
// the file does not have it (a drive keeps street detail only near home).
func (d *drive) load(v view) ([]piece, error) {
	var out []piece
	screen := image.Rect(0, 0, v.w, v.h)
	for _, t := range viewTiles(v, d.maxZoom) {
		for up := 0; up <= 6 && up <= t.z; up++ {
			b, err := d.tiles.Tile(uint8(t.z-up), t.x>>up, t.y>>up)
			if err != nil {
				return nil, err
			}
			if b == nil {
				continue
			}
			layers, err := decodeTile(b)
			if err != nil {
				return nil, fmt.Errorf("tile %d/%d/%d: %w", t.z-up, t.x>>up, t.y>>up, err)
			}
			span := float64(t.rect.Dx() << up) // px of the tile found
			p := piece{layers: layers, clip: t.rect.Intersect(screen)}
			p.ox = float64(t.rect.Min.X) - float64(t.x&(1<<up-1))*float64(t.rect.Dx())
			p.oy = float64(t.rect.Min.Y) - float64(t.y&(1<<up-1))*float64(t.rect.Dy())
			for _, l := range layers {
				p.k = span / l.extent
				break
			}
			out = append(out, p)
			break
		}
	}
	return out, nil
}

func render(d *drive, v view) (*image.Gray, error) {
	img := image.NewGray(image.Rect(0, 0, v.w, v.h))
	for i := range img.Pix {
		img.Pix[i] = white
	}
	pieces, err := d.load(v)
	if err != nil {
		return nil, err
	}
	mode := 0
	if v.grey {
		mode = 1
	}
	for _, r := range rules {
		for _, p := range pieces {
			l := p.layers[r.layer]
			if l == nil {
				continue
			}
			sub := img.SubImage(p.clip).(*image.Gray)
			for _, f := range l.features {
				if r.matches(f, v.zoom) {
					drawFeature(sub, r, f, p.screen(f.geom), v.zoom, mode)
				}
			}
		}
	}

	g := newGrid(v.w, v.h)
	scale := scaleBar(d.font, v)
	g.take(scale, 6, v.h-6-scale.h)
	drawStamp(img, scale, 6, v.h-6-scale.h)
	if d.home != nil {
		home := homeArt.halo(1)
		at := v.project(d.home.x, d.home.y)
		x, y := int(at.x)-home.w/2, int(at.y)-home.h/2
		g.take(home, x, y)
		drawStamp(img, home, x, y)
	}
	// Town names before symbols, so a town is never lost under its own
	// pharmacies; village and quarter names after.
	names := placeNames(pieces, v)
	big := 0
	for big < len(names) && names[big].size >= 15 {
		big++
	}
	placeLabels(img, g, d.font, names[:big])
	for _, s := range d.symbolsInOrder() {
		if v.zoom < s.minZoom {
			continue
		}
		st := s.art.halo(1)
		for _, q := range d.pois[s.id] {
			g.put(img, st, v.project(q.x, q.y))
		}
	}
	placeLabels(img, g, d.font, names[big:])
	placeLabels(img, g, d.font, streetLabels(d.font, streets(pieces, v), 11))
	return img, nil
}

func drawFeature(img *image.Gray, r rule, f feature, geom [][]pt, z, mode int) {
	if r.area[mode] != (pattern{}) {
		if f.kind == 3 {
			fill(img, geom, r.area[mode])
		}
		return
	}
	w := r.width(z)
	if w == 0 || f.kind == 1 {
		return
	}
	colour := black
	switch {
	case r.cased > 0 && z >= r.cased:
		if r.inner {
			w, colour = w-2, white
		}
	case r.inner:
		return // not cased yet at this zoom: one plain line
	case r.white:
		colour = white
	case mode == 1:
		colour = r.grey
	}
	dash := r.dash
	if r.dotted && mode == 0 {
		dash = [2]float64{2, 2 + w} // the round ends fill all but 2 px of each gap
	}
	for _, line := range geom {
		if dash[0] > 0 {
			for _, d := range dashes(line, dash[0], dash[1]) {
				stroke(img, d, w, colour)
			}
		} else {
			stroke(img, line, w, colour)
		}
	}
}

// symbolsInOrder lists the drive's place layers by symbol priority.
func (d *drive) symbolsInOrder() []symbol {
	var out []symbol
	for _, s := range symbols {
		if _, ok := d.pois[s.id]; ok {
			out = append(out, s)
		}
	}
	for _, id := range d.order {
		if !slices.ContainsFunc(symbols, func(s symbol) bool { return s.id == id }) {
			out = append(out, symbol{id, 12, fallbackArt})
		}
	}
	return out
}

// placeNames lists town and village names, biggest first.
func placeNames(pieces []piece, v view) []label {
	type place struct {
		label
		rank float64
	}
	seen := map[string]bool{}
	var places []place
	for _, p := range pieces {
		l := p.layers["places"]
		if l == nil {
			continue
		}
		for _, f := range l.features {
			name, size := f.str("name"), placeSize(f, v.zoom)
			if name == "" || size == 0 || seen[name] || len(f.geom) == 0 {
				continue
			}
			seen[name] = true
			at := p.screen(f.geom[:1])[0][0]
			places = append(places, place{label{name, size, at, 0}, f.num("population_rank")})
		}
	}
	sort.SliceStable(places, func(i, j int) bool {
		if places[i].size != places[j].size {
			return places[i].size > places[j].size
		}
		return places[i].rank > places[j].rank
	})
	out := make([]label, len(places))
	for i, p := range places {
		out[i] = p.label
	}
	return out
}

// streets lists the named roads, cut to the parts on the screen.
func streets(pieces []piece, v view) []street {
	var out []street
	inside := func(q pt) bool { return q.x >= 0 && q.y >= 0 && q.x < float64(v.w) && q.y < float64(v.h) }
	for _, p := range pieces {
		l := p.layers["roads"]
		if l == nil {
			continue
		}
		for _, f := range l.features {
			name := f.str("name")
			if name == "" || f.kind != 2 || streetSize(f, v.zoom) == 0 {
				continue
			}
			rank := 2
			switch f.str("kind") {
			case "highway":
				rank = 0
			case "major_road":
				rank = 1
			}
			for _, line := range p.screen(f.geom) {
				var part []pt
				for _, q := range append(line, pt{-1, -1}) {
					if inside(q) {
						part = append(part, q)
						continue
					}
					if len(part) > 1 {
						out = append(out, street{name, rank, part})
					}
					part = nil
				}
			}
		}
	}
	return out
}

// scaleBar is a bar of a round distance, at most 100 px long, with its
// length written above.
func scaleBar(f *font, v view) stamp {
	px, text := scaleLength(v.lat, v.zoom, 100)
	words := f.text(text, 11, 0)
	s := newStamp(max(px+1, words.w), words.h+5)
	for y := 0; y < words.h; y++ {
		copy(s.px[y*s.w:y*s.w+words.w], words.px[y*words.w:(y+1)*words.w])
	}
	for x := 0; x <= px; x++ {
		s.px[(s.h-2)*s.w+x], s.px[(s.h-1)*s.w+x] = ink, ink
	}
	for y := words.h; y < s.h; y++ {
		s.px[y*s.w], s.px[y*s.w+px] = ink, ink
	}
	return s.halo(2)
}

// scaleLength picks the longest 1, 2 or 5 times a power of ten metres
// that fits in maxPx at this latitude and zoom.
func scaleLength(lat float64, zoom, maxPx int) (px int, text string) {
	perPx := 2 * math.Pi * 6378137 * math.Cos(lat*math.Pi/180) / (tilePx * math.Exp2(float64(zoom)))
	most := perPx * float64(maxPx)
	step := math.Pow(10, math.Floor(math.Log10(most)))
	m := step
	for _, k := range []float64{2, 5} {
		if k*step <= most {
			m = k * step
		}
	}
	text = fmt.Sprintf("%.0f m", m)
	if m >= 1000 {
		text = fmt.Sprintf("%.0f km", m/1000)
	}
	return int(math.Round(m / perPx)), text
}
