package main

import (
	"bytes"
	"compress/gzip"
	"image"
	"os"
	"path/filepath"
	"testing"

	"github.com/renereose/varu/pmtiles"
)

func TestViewTiles(t *testing.T) {
	// The whole world at zoom 1 is 1024 px: four tiles.
	tiles := viewTiles(view{zoom: 1, w: 1024, h: 1024}, 14)
	if len(tiles) != 4 || tiles[0].rect != image.Rect(0, 0, 512, 512) || tiles[3].rect != image.Rect(512, 512, 1024, 1024) {
		t.Errorf("zoom 1: %v", tiles)
	}
	// Beyond the file's deepest zoom, its tiles are drawn bigger.
	tiles = viewTiles(view{lat: 58.38, lon: 26.72, zoom: 16, w: 400, h: 300}, 15)
	if len(tiles) == 0 || len(tiles) > 4 || tiles[0].z != 15 || tiles[0].rect.Dx() != 1024 {
		t.Errorf("zoom 16 from a zoom 15 map: %v", tiles)
	}
}

func TestScaleBar(t *testing.T) {
	for _, c := range []struct {
		lat  float64
		zoom int
		text string
	}{{59.44, 12, "500 m"}, {58.38, 12, "1 km"}, {59.44, 10, "2 km"}, {59.44, 14, "200 m"}, {0, 0, "5000 km"}} {
		px, text := scaleLength(c.lat, c.zoom, 100)
		if text != c.text || px < 40 || px > 100 {
			t.Errorf("lat %v zoom %d: %d px %q, want %q", c.lat, c.zoom, px, text, c.text)
		}
	}
}

// A map with only the zoom 0 tile still draws at zoom 3, from that tile.
func TestParentTile(t *testing.T) {
	world := []pt{{0, 0}, {4096, 0}, {4096, 4096}, {0, 4096}}
	var tile bytes.Buffer
	zw := gzip.NewWriter(&tile)
	zw.Write(testTile(map[string][]testFeature{"water": {{kind: 3, props: map[string]string{"kind": "ocean"}, geom: geometry(true, world)}}}))
	zw.Close()
	root, _ := pmtiles.BuildDirs([]pmtiles.Entry{{TileID: 0, Length: uint32(tile.Len()), RunLength: 1}})
	h := pmtiles.Header{InternalComp: pmtiles.Gzip, TileComp: pmtiles.Gzip, TileType: 1, MaxZoom: 14}
	h.RootOffset, h.RootLength = pmtiles.HeaderLen, uint64(len(root))
	h.DataOffset, h.DataLength = h.RootOffset+h.RootLength, uint64(tile.Len())
	path := filepath.Join(t.TempDir(), "world.pmtiles")
	if err := os.WriteFile(path, bytes.Join([][]byte{h.Bytes(), root, tile.Bytes()}, nil), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := pmtiles.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	d := &drive{tiles: f, maxZoom: 14, font: openFont(t.TempDir())}
	img, err := render(d, view{lat: 58.38, lon: 26.72, zoom: 3, w: 64, h: 48})
	if err != nil {
		t.Fatal(err)
	}
	// Water hatching: one black row in four, everywhere outside the scale bar.
	if img.GrayAt(10, 0).Y != black || img.GrayAt(10, 1).Y != white || img.GrayAt(60, 4).Y != black {
		t.Errorf("rows %d %d %d", img.GrayAt(10, 0).Y, img.GrayAt(10, 1).Y, img.GrayAt(60, 4).Y)
	}
}

// With VARU_DRIVE=path/to/drive, draw a screen from a real drive.
func TestRealRender(t *testing.T) {
	dir := os.Getenv("VARU_DRIVE")
	if dir == "" {
		t.Skip("set VARU_DRIVE to draw from a real drive")
	}
	d, _, mid, err := openDrive(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer d.tiles.Close()
	for _, zoom := range []int{10, 12, 14} {
		img, err := render(d, view{lat: mid.y, lon: mid.x, zoom: zoom, w: 400, h: 300})
		if err != nil {
			t.Fatal(err)
		}
		inked := 0
		for _, p := range img.Pix {
			if p == black {
				inked++
			}
		}
		if inked < 400*300/100 || inked > 400*300*2/3 {
			t.Errorf("zoom %d: %d black pixels of %d", zoom, inked, 400*300)
		}
	}
}
