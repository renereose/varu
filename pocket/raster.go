package main

// Drawing on a grey image without anti-aliasing: e-ink panels show black,
// white and at most two greys, so every pixel is one of those. Drawing
// stays inside img.Rect, so a SubImage clips.

import (
	"image"
	"math"
	"sort"
)

// Greys on a 4-level panel; a 1-bit panel uses black and white only.
const (
	black uint8 = 0
	dark  uint8 = 85
	light uint8 = 170
	white uint8 = 255
)

// pattern is a 4×4 tile repeated from the screen's corner: '#' black,
// 'd' dark grey, 'l' light grey, 'o' white, '.' leaves the pixel as it was.
type pattern [4]string

var solid = pattern{"####", "####", "####", "####"}

func (p pattern) at(x, y int) (uint8, bool) {
	switch p[y&3][x&3] {
	case '#':
		return black, true
	case 'd':
		return dark, true
	case 'l':
		return light, true
	case 'o':
		return white, true
	}
	return 0, false
}

func set(img *image.Gray, x, y int, v uint8) {
	if image.Pt(x, y).In(img.Rect) {
		img.Pix[img.PixOffset(x, y)] = v
	}
}

// fill paints the inside of rings (even-odd, so holes stay open).
func fill(img *image.Gray, rings [][]pt, p pattern) {
	r := img.Rect
	ymin, ymax := math.Inf(1), math.Inf(-1)
	for _, ring := range rings {
		for _, q := range ring {
			ymin, ymax = math.Min(ymin, q.y), math.Max(ymax, q.y)
		}
	}
	var xs []float64
	for y := max(r.Min.Y, int(math.Floor(ymin))); y < min(r.Max.Y, int(math.Ceil(ymax))+1); y++ {
		cy := float64(y) + 0.5
		xs = xs[:0]
		for _, ring := range rings {
			for i := 1; i < len(ring); i++ {
				a, b := ring[i-1], ring[i]
				if (a.y <= cy) != (b.y <= cy) {
					xs = append(xs, a.x+(cy-a.y)/(b.y-a.y)*(b.x-a.x))
				}
			}
		}
		sort.Float64s(xs)
		for i := 0; i+1 < len(xs); i += 2 {
			for x := max(r.Min.X, int(math.Ceil(xs[i]-0.5))); x < min(r.Max.X, int(math.Ceil(xs[i+1]-0.5))); x++ {
				if v, ok := p.at(x, y); ok {
					img.Pix[img.PixOffset(x, y)] = v
				}
			}
		}
	}
}

// stroke draws a line w px wide with round joins and ends.
func stroke(img *image.Gray, line []pt, w float64, v uint8) {
	if w <= 1 {
		for i := 1; i < len(line); i++ {
			thin(img, line[i-1], line[i], v)
		}
		return
	}
	rad := w / 2
	for i := 1; i < len(line); i++ {
		a, b := line[i-1], line[i]
		box := image.Rect(int(math.Floor(math.Min(a.x, b.x)-rad)), int(math.Floor(math.Min(a.y, b.y)-rad)),
			int(math.Ceil(math.Max(a.x, b.x)+rad))+1, int(math.Ceil(math.Max(a.y, b.y)+rad))+1).Intersect(img.Rect)
		dx, dy := b.x-a.x, b.y-a.y
		len2 := dx*dx + dy*dy
		for y := box.Min.Y; y < box.Max.Y; y++ {
			for x := box.Min.X; x < box.Max.X; x++ {
				px, py := float64(x)+0.5-a.x, float64(y)+0.5-a.y
				f := 0.0
				if len2 > 0 {
					f = math.Max(0, math.Min(1, (px*dx+py*dy)/len2))
				}
				if math.Hypot(px-f*dx, py-f*dy) <= rad {
					img.Pix[img.PixOffset(x, y)] = v
				}
			}
		}
	}
	if len(line) == 1 {
		stroke(img, []pt{line[0], line[0]}, w, v)
	}
}

// thin draws a one-pixel line, each step to one of the eight neighbours.
func thin(img *image.Gray, a, b pt, v uint8) {
	n := int(math.Ceil(math.Max(math.Abs(b.x-a.x), math.Abs(b.y-a.y))))
	for i := 0; i <= n; i++ {
		f := 0.0
		if n > 0 {
			f = float64(i) / float64(n)
		}
		set(img, int(math.Floor(a.x+f*(b.x-a.x))), int(math.Floor(a.y+f*(b.y-a.y))), v)
	}
}

// dashes cuts a line into pieces on px long with gaps off px long.
func dashes(line []pt, on, off float64) [][]pt {
	var out [][]pt
	var cur []pt
	drawing, left := true, on
	for i := 1; i < len(line); i++ {
		a, b := line[i-1], line[i]
		seg := math.Hypot(b.x-a.x, b.y-a.y)
		for t := 0.0; t < seg; {
			step := math.Min(left, seg-t)
			p := pt{a.x + (b.x-a.x)*(t/seg), a.y + (b.y-a.y)*(t/seg)}
			q := pt{a.x + (b.x-a.x)*((t+step)/seg), a.y + (b.y-a.y)*((t+step)/seg)}
			if drawing {
				if len(cur) == 0 {
					cur = append(cur, p)
				}
				cur = append(cur, q)
			}
			t += step
			if left -= step; left <= 1e-9 {
				if drawing && len(cur) > 1 {
					out = append(out, cur)
				}
				cur, drawing, left = nil, !drawing, off
				if drawing {
					left = on
				}
			}
		}
	}
	if len(cur) > 1 {
		out = append(out, cur)
	}
	return out
}

// A stamp is a small picture (a symbol or a label) placed whole.
const (
	clear uint8 = iota
	ink
	paper
)

type stamp struct {
	w, h int
	px   []uint8
}

func newStamp(w, h int) stamp { return stamp{w, h, make([]uint8, w*h)} }

// art reads a stamp from rows of '#' (ink), 'o' (paper) and '.' (clear).
func art(rows ...string) stamp {
	s := newStamp(len(rows[0]), len(rows))
	for y, row := range rows {
		for x, c := range row {
			switch c {
			case '#':
				s.px[y*s.w+x] = ink
			case 'o':
				s.px[y*s.w+x] = paper
			}
		}
	}
	return s
}

// halo grows the stamp by r px on every side, as paper, so it stands out
// from whatever is under it.
func (s stamp) halo(r int) stamp {
	out := newStamp(s.w+2*r, s.h+2*r)
	for y := 0; y < s.h; y++ {
		for x := 0; x < s.w; x++ {
			if s.px[y*s.w+x] == clear {
				continue
			}
			for dy := -r; dy <= r; dy++ {
				for dx := -r; dx <= r; dx++ {
					if dx*dx+dy*dy <= r*r+r/2 {
						i := (y+r+dy)*out.w + x + r + dx
						if out.px[i] == clear {
							out.px[i] = paper
						}
					}
				}
			}
		}
	}
	for y := 0; y < s.h; y++ {
		for x := 0; x < s.w; x++ {
			if v := s.px[y*s.w+x]; v != clear {
				out.px[(y+r)*out.w+x+r] = v
			}
		}
	}
	return out
}

// drawStamp puts the stamp's top left corner at x, y.
func drawStamp(img *image.Gray, s stamp, x, y int) {
	for sy := 0; sy < s.h; sy++ {
		for sx := 0; sx < s.w; sx++ {
			switch s.px[sy*s.w+sx] {
			case ink:
				set(img, x+sx, y+sy, black)
			case paper:
				set(img, x+sx, y+sy, white)
			}
		}
	}
}
