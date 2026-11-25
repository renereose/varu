package main

import (
	"image"
	"testing"
)

func blank(w, h int) *image.Gray {
	img := image.NewGray(image.Rect(0, 0, w, h))
	for i := range img.Pix {
		img.Pix[i] = white
	}
	return img
}

func TestFillAndStroke(t *testing.T) {
	img := blank(40, 40)
	square := []pt{{5, 5}, {35, 5}, {35, 35}, {5, 35}, {5, 5}}
	hole := []pt{{15, 15}, {15, 25}, {25, 25}, {25, 15}, {15, 15}}
	fill(img, [][]pt{square, hole}, solid)
	for _, c := range []struct {
		x, y int
		want uint8
	}{{10, 10, black}, {20, 20, white}, {2, 2, white}, {5, 5, black}, {35, 35, white}} {
		if got := img.GrayAt(c.x, c.y).Y; got != c.want {
			t.Errorf("fill: pixel %d,%d is %d, want %d", c.x, c.y, got, c.want)
		}
	}

	img = blank(40, 40)
	stroke(img.SubImage(image.Rect(0, 0, 20, 40)).(*image.Gray), []pt{{5, 20.5}, {35, 20.5}}, 3, black)
	if img.GrayAt(10, 21).Y != black || img.GrayAt(10, 18).Y != white || img.GrayAt(25, 20).Y != white {
		t.Error("a 3 px line, clipped at x 20, drew in the wrong places")
	}

	if got := dashes([]pt{{0, 0}, {10, 0}, {20, 0}}, 4, 4); len(got) != 3 || got[1][0] != (pt{8, 0}) || got[2][len(got[2])-1] != (pt{20, 0}) {
		t.Errorf("dashes %v", got)
	}
}

func TestStamps(t *testing.T) {
	s := art(
		"#o",
		"..").halo(1)
	if s.w != 4 || s.h != 4 || s.px[1*4+1] != ink || s.px[1*4+2] != paper || s.px[0*4+1] != paper || s.px[3*4+3] != clear {
		t.Errorf("halo: %v", s.px)
	}
	img := blank(3, 3)
	drawStamp(img, s, -1, -1) // partly off the image
	if img.GrayAt(0, 0).Y != black || img.GrayAt(1, 0).Y != white {
		t.Errorf("drawStamp: %v", img.Pix)
	}
}
