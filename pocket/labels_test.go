package main

import (
	"math"
	"testing"
)

func TestLabels(t *testing.T) {
	// An L: 100 px east, then 30 px south.
	a, b, n := longestRun([]pt{{0, 0}, {50, 1}, {100, 0}, {100, 30}}, 0.3)
	if a != (pt{0, 0}) || b != (pt{100, 0}) || math.Abs(n-100) > 0.1 {
		t.Errorf("longest run %v-%v %v", a, b, n)
	}

	f := openFont(writeFont(t, map[string][]rune{"0-255.pbf": []rune("Tamme")}))
	labels := streetLabels(f, []street{
		{"Tamme", 2, []pt{{10, 10}, {110, 10}}},
		{"Tamme", 2, []pt{{10, 50}, {200, 50}}},  // longer: this one is used
		{"Tamme", 2, []pt{{150, 90}, {10, 90}}},  // drawn west, still reads left to right
		{"Tamme", 2, []pt{{10, 120}, {20, 120}}}, // too short for the name
	}, 12)
	if len(labels) != 1 || labels[0].at != (pt{105, 50}) || labels[0].angle != 0 {
		t.Errorf("street labels %+v", labels)
	}
	if l := streetLabels(f, []street{{"Tamme", 2, []pt{{150, 90}, {10, 90}}}}, 12); len(l) != 1 || l[0].angle != 0 {
		t.Errorf("a street drawn westwards is labelled upside down: %+v", l)
	}

	g := newGrid(100, 100)
	img := blank(100, 100)
	box := art("###", "###", "###")
	if !g.put(img, box, pt{50, 50}) || g.put(img, box, pt{51, 51}) || !g.put(img, box, pt{60, 50}) {
		t.Error("overlapping stamps should be left out, others placed")
	}
	if g.put(img, box, pt{0, 50}) {
		t.Error("a stamp off the screen's edge was placed")
	}
}
