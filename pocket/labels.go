package main

// Placing symbols and names so none covers another: every pixel drawn is
// marked in a grid, and anything that would land on a marked pixel is
// left out. Earlier calls win, so callers go in order of importance.

import (
	"image"
	"math"
	"sort"
)

type grid struct {
	w, h int
	used []bool
}

func newGrid(w, h int) *grid { return &grid{w, h, make([]bool, w*h)} }

// fits reports whether the stamp at x, y is on the screen and on free pixels.
func (g *grid) fits(s stamp, x, y int) bool {
	if x < 0 || y < 0 || x+s.w > g.w || y+s.h > g.h {
		return false
	}
	for sy := 0; sy < s.h; sy++ {
		for sx := 0; sx < s.w; sx++ {
			if s.px[sy*s.w+sx] != clear && g.used[(y+sy)*g.w+x+sx] {
				return false
			}
		}
	}
	return true
}

func (g *grid) take(s stamp, x, y int) {
	for sy := 0; sy < s.h; sy++ {
		for sx := 0; sx < s.w; sx++ {
			if px, py := x+sx, y+sy; s.px[sy*s.w+sx] != clear && px >= 0 && py >= 0 && px < g.w && py < g.h {
				g.used[py*g.w+px] = true
			}
		}
	}
}

// put draws the stamp centred on p if its place is free.
func (g *grid) put(img *image.Gray, s stamp, p pt) bool {
	x, y := int(math.Round(p.x-float64(s.w)/2)), int(math.Round(p.y-float64(s.h)/2))
	if !g.fits(s, x, y) {
		return false
	}
	g.take(s, x, y)
	drawStamp(img, s, x, y)
	return true
}

type label struct {
	text  string
	size  float64
	at    pt      // centre
	angle float64 // clockwise, radians
}

// placeLabels draws, in order, each label whose text and halo are free.
func placeLabels(img *image.Gray, g *grid, f *font, labels []label) {
	for _, l := range labels {
		g.put(img, f.text(l.text, l.size, l.angle).halo(2), l.at)
	}
}

// longestRun finds the longest stretch of a line that keeps within
// maxTurn radians of one direction.
func longestRun(line []pt, maxTurn float64) (a, b pt, length float64) {
	for i := 1; i < len(line); i++ {
		dir := math.Atan2(line[i].y-line[i-1].y, line[i].x-line[i-1].x)
		run := 0.0
		j := i
		for ; j < len(line); j++ {
			d := math.Atan2(line[j].y-line[j-1].y, line[j].x-line[j-1].x)
			if math.Abs(math.Remainder(d-dir, 2*math.Pi)) > maxTurn {
				break
			}
			run += math.Hypot(line[j].x-line[j-1].x, line[j].y-line[j-1].y)
		}
		if run > length {
			a, b, length = line[i-1], line[j-1], run
		}
	}
	return a, b, length
}

type street struct {
	name string
	rank int // 0 for the most important roads
	line []pt
}

// streetLabels picks, for each street name, the longest straight stretch
// long enough for the name, turned to read left to right. Main roads come
// first, so they keep their names when space is short.
func streetLabels(f *font, streets []street, size float64) []label {
	type pick struct {
		label
		rank   int
		length float64
	}
	best := map[string]pick{}
	for _, s := range streets {
		a, b, n := longestRun(s.line, 0.3)
		old, seen := best[s.name]
		if n < f.measure(s.name, size)+8 || seen && (old.rank < s.rank || old.rank == s.rank && old.length >= n) {
			continue
		}
		angle := math.Atan2(b.y-a.y, b.x-a.x)
		if angle > math.Pi/2 {
			angle -= math.Pi
		} else if angle <= -math.Pi/2 {
			angle += math.Pi
		}
		best[s.name] = pick{label{s.name, size, pt{(a.x + b.x) / 2, (a.y + b.y) / 2}, angle}, s.rank, n}
	}
	picks := make([]pick, 0, len(best))
	for _, p := range best {
		picks = append(picks, p)
	}
	sort.Slice(picks, func(i, j int) bool {
		if picks[i].rank != picks[j].rank {
			return picks[i].rank < picks[j].rank
		}
		return picks[i].length > picks[j].length
	})
	out := make([]label, len(picks))
	for i, p := range picks {
		out[i] = p.label
	}
	return out
}
