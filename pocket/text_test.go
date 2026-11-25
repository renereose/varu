package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// writeFont makes a font of solid blocks: each glyph 10×16 px of ink.
func writeFont(t *testing.T, ranges map[string][]rune) string {
	dir := t.TempDir()
	for file, runes := range ranges {
		stack := pbBytes(nil, 1, []byte("Test"))
		for _, r := range runes {
			g := pbUint(nil, 1, uint64(r))
			g = pbBytes(g, 2, bytes.Repeat([]byte{255}, (10+6)*(16+6)))
			g = pbUint(g, 3, 10)
			g = pbUint(g, 4, 16)
			g = pbUint(g, 5, uint64(zz(1)))
			g = pbUint(g, 6, uint64(zz(-9)))
			g = pbUint(g, 7, 12)
			stack = pbBytes(stack, 3, g)
		}
		if err := os.WriteFile(filepath.Join(dir, file), pbBytes(nil, 1, stack), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestText(t *testing.T) {
	f := openFont(writeFont(t, map[string][]rune{"0-255.pbf": {'õ'}, "256-511.pbf": {'š'}}))
	inked := func(s stamp) (n int) {
		for _, p := range s.px {
			if p == ink {
				n++
			}
		}
		return n
	}
	flat := f.text("õš", 24, 0)
	if flat.w != 24 || inked(flat) < 2*10*16*8/10 {
		t.Errorf("õš: %d px wide with %d inked, want 24 wide and two blocks", flat.w, inked(flat))
	}
	turned := f.text("õš", 24, 1.5707963)
	if d := turned.w - flat.h; d < -1 || d > 1 || turned.h-flat.w < -1 || turned.h-flat.w > 1 {
		t.Errorf("turned a quarter: %d×%d, flat %d×%d", turned.w, turned.h, flat.w, flat.h)
	}
	missing := f.text("Ж", 24, 0) // no 1024-1279.pbf: a space, no error
	if inked(missing) != 0 || missing.w != 8 {
		t.Errorf("missing glyph drew %d px, %d wide", inked(missing), missing.w)
	}
	if w := f.measure("õšЖ", 12); w != 16 {
		t.Errorf("measure %v, want 16", w)
	}
}
