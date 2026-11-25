package main

// Labels drawn from the drive's own map font: MapLibre glyph files, one per
// 256 code points, each glyph a signed distance field at 24 px with a 3 px
// border. Sampling the field at any size and angle and keeping what is
// inside the outline gives sharp 1-bit text without a font rasteriser.

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
)

const (
	glyphSize   = 24 // px the distance fields were made for
	glyphBorder = 3  // px of field around each glyph
	// A glyph's top is -top px below the line's origin, where the baseline
	// is 25 px down; accents reach up to 2 px and descenders down to 31.
	lineTop      = 2
	lineBottom   = 31
	inkThreshold = 192 // field value at the outline (MapLibre's 0.75)
)

type glyph struct {
	w, h, left, top, advance int
	sdf                      []byte // (w+6) × (h+6)
}

type font struct {
	dir    string
	ranges map[rune]map[rune]glyph // by first code point of the range; empty if the file is missing
}

func openFont(dir string) *font { return &font{dir: dir, ranges: map[rune]map[rune]glyph{}} }

func (f *font) glyph(r rune) (glyph, bool) {
	start := r &^ 255
	gs, ok := f.ranges[start]
	if !ok {
		gs = map[rune]glyph{}
		if b, err := os.ReadFile(filepath.Join(f.dir, fmt.Sprintf("%d-%d.pbf", start, start+255))); err == nil {
			gs, _ = decodeGlyphs(b)
		}
		f.ranges[start] = gs
	}
	g, ok := gs[r]
	return g, ok
}

func decodeGlyphs(b []byte) (map[rune]glyph, error) {
	out := map[rune]glyph{}
	stacks := pbf{b: b}
	for stacks.next() {
		if stacks.tag != 1 || stacks.wire != 2 {
			continue
		}
		stack := pbf{b: stacks.data}
		for stack.next() {
			if stack.tag != 3 || stack.wire != 2 {
				continue
			}
			var g glyph
			var id rune
			p := pbf{b: stack.data}
			for p.next() {
				switch p.tag {
				case 1:
					id = rune(p.num)
				case 2:
					g.sdf = p.data
				case 3:
					g.w = int(p.num)
				case 4:
					g.h = int(p.num)
				case 5:
					g.left = int(zigzag(uint32(p.num)))
				case 6:
					g.top = int(zigzag(uint32(p.num)))
				case 7:
					g.advance = int(p.num)
				}
			}
			if p.err != nil {
				return out, p.err
			}
			if len(g.sdf) != (g.w+2*glyphBorder)*(g.h+2*glyphBorder) {
				g.sdf, g.w, g.h = nil, 0, 0 // a space, or damaged: draw nothing
			}
			out[id] = g
		}
		if stack.err != nil {
			return out, stack.err
		}
	}
	return out, stacks.err
}

// text draws s size px tall, turned clockwise by angle (radians) about its
// centre. Characters missing from the font leave a space.
func (f *font) text(s string, size, angle float64) stamp {
	type placed struct {
		g    glyph
		x, y float64 // top left of the field, at 24 px
	}
	var line []placed
	pen := 0.0
	for _, r := range s {
		g, ok := f.glyph(r)
		if !ok {
			pen += glyphSize / 3
			continue
		}
		if g.sdf != nil {
			line = append(line, placed{g, pen + float64(g.left-glyphBorder), float64(-g.top - glyphBorder - lineTop)})
		}
		pen += float64(g.advance)
	}
	scale := size / glyphSize
	w, h := pen*scale, (lineBottom-lineTop)*scale // the unturned line box, in px
	sin, cos := math.Sin(angle), math.Cos(angle)
	bw := int(math.Ceil(math.Abs(w*cos) + math.Abs(h*sin)))
	bh := int(math.Ceil(math.Abs(w*sin) + math.Abs(h*cos)))
	out := newStamp(bw, bh)
	for py := 0; py < bh; py++ {
		for px := 0; px < bw; px++ {
			// Back from the turned box to the line, then to glyph units.
			dx, dy := float64(px)+0.5-float64(bw)/2, float64(py)+0.5-float64(bh)/2
			u := (dx*cos+dy*sin)/scale + pen/2
			v := (-dx*sin+dy*cos)/scale + (lineBottom-lineTop)/2.0
			for _, p := range line {
				if field(p.g, u-p.x, v-p.y) >= inkThreshold {
					out.px[py*bw+px] = ink
					break
				}
			}
		}
	}
	return out
}

// measure gives the width of s at size px.
func (f *font) measure(s string, size float64) float64 {
	pen := 0.0
	for _, r := range s {
		if g, ok := f.glyph(r); ok {
			pen += float64(g.advance)
		} else {
			pen += glyphSize / 3
		}
	}
	return pen * size / glyphSize
}

// field samples a glyph's distance field between its pixels.
func field(g glyph, x, y float64) float64 {
	fw, fh := g.w+2*glyphBorder, g.h+2*glyphBorder
	x, y = x-0.5, y-0.5
	if x < 0 || y < 0 || x >= float64(fw-1) || y >= float64(fh-1) {
		return 0
	}
	ix, iy := int(x), int(y)
	fx, fy := x-float64(ix), y-float64(iy)
	at := func(x, y int) float64 { return float64(g.sdf[y*fw+x]) }
	top := at(ix, iy)*(1-fx) + at(ix+1, iy)*fx
	bottom := at(ix, iy+1)*(1-fx) + at(ix+1, iy+1)*fx
	return top*(1-fy) + bottom*fy
}
