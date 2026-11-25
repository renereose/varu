package main

import (
	"strings"
	"testing"
)

func TestStyle(t *testing.T) {
	for _, s := range append(symbols, symbol{"fallback", 0, fallbackArt}, symbol{"home", 0, homeArt}) {
		want := 15
		if s.id == "home" {
			want = 19
		}
		if s.art.w != want || s.art.h != want || len(s.art.px) != want*want || !strings.Contains(string(s.art.px), string([]byte{ink})) {
			t.Errorf("symbol %s is %d×%d", s.id, s.art.w, s.art.h)
		}
	}
	for i, r := range rules {
		for _, p := range r.area {
			for _, row := range p {
				if p != (pattern{}) && (len(row) != 4 || strings.Trim(row, "#dlo.") != "") {
					t.Errorf("rule %d: pattern row %q", i, row)
				}
			}
		}
		if (r.area == [2]pattern{}) == (len(r.widths) == 0) {
			t.Errorf("rule %d (%s) should fill an area or draw a line", i, r.layer)
		}
	}
}
