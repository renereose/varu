package main

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestTileIDs(t *testing.T) {
	// Fixed points from the PMTiles spec's reference implementation.
	for _, c := range []struct {
		z    uint8
		x, y uint32
		id   uint64
	}{{0, 0, 0, 0}, {1, 0, 0, 1}, {1, 0, 1, 2}, {1, 1, 1, 3}, {1, 1, 0, 4}, {2, 0, 0, 5}} {
		if got := zxyToID(c.z, c.x, c.y); got != c.id {
			t.Errorf("zxyToID(%d,%d,%d) = %d, want %d", c.z, c.x, c.y, got, c.id)
		}
	}
	for _, id := range []uint64{0, 7, 1000, 123456789, zxyToID(15, 18432, 9472)} {
		z, x, y := idToZxy(id)
		if back := zxyToID(z, x, y); back != id {
			t.Errorf("round trip %d -> %d/%d/%d -> %d", id, z, x, y, back)
		}
	}
}

func TestLeafDirectories(t *testing.T) {
	var entries []entry
	for i := uint64(0); i < 60000; i++ {
		entries = append(entries, entry{TileID: i * 3, Offset: i * 977 % 1000003, Length: uint32(i%500 + 1), RunLength: 1})
	}
	root, leaves := buildDirs(entries)
	if len(root) > rootMax-headerLen || len(leaves) == 0 {
		t.Fatalf("root %d bytes, leaves %d bytes", len(root), len(leaves))
	}
	pointers, err := decodeDir(root)
	if err != nil {
		t.Fatal(err)
	}
	var got []entry
	for _, p := range pointers {
		leaf, err := decodeDir(leaves[p.Offset : p.Offset+uint64(p.Length)])
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, leaf...)
	}
	if fmt.Sprint(got) != fmt.Sprint(entries) {
		t.Fatal("entries changed on the way through leaf directories")
	}
}

// writeArchive makes a small source map: every tile of zooms 0-2, plus a
// 6x6 block of street-level tiles around Tartu. Each tile's data is its zxy.
func writeArchive(t *testing.T, path string) map[uint64]string {
	tiles := map[uint64]string{}
	var ids []uint64
	add := func(z uint8, x, y uint32) {
		id := zxyToID(z, x, y)
		tiles[id] = fmt.Sprintf("%d/%d/%d", z, x, y)
	}
	for z := uint8(0); z <= 2; z++ {
		for x := uint32(0); x < 1<<z; x++ {
			for y := uint32(0); y < 1<<z; y++ {
				add(z, x, y)
			}
		}
	}
	x0, y0 := lonToX(26.72, 14), latToY(58.38, 14)
	for x := x0 - 3; x < x0+3; x++ {
		for y := y0 - 3; y < y0+3; y++ {
			add(14, x, y)
		}
	}
	for id := range tiles {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	var data bytes.Buffer
	var entries []entry
	for _, id := range ids {
		entries = append(entries, entry{TileID: id, Offset: uint64(data.Len()), Length: uint32(len(tiles[id])), RunLength: 1})
		data.WriteString(tiles[id])
	}
	root, leaves := buildDirs(entries)
	meta := encodeDir(nil) // any gzip blob will do
	h := pmHeader{InternalComp: gzipComp, TileComp: 1, TileType: 1, MaxZoom: 14}
	h.RootOffset, h.RootLength = headerLen, uint64(len(root))
	h.MetaOffset, h.MetaLength = h.RootOffset+h.RootLength, uint64(len(meta))
	h.LeafOffset, h.LeafLength = h.MetaOffset+h.MetaLength, uint64(len(leaves))
	h.DataOffset, h.DataLength = h.LeafOffset+h.LeafLength, uint64(data.Len())
	out := append(append(append(append(h.bytes(), root...), meta...), leaves...), data.Bytes()...)
	if err := os.WriteFile(path, out, 0o644); err != nil {
		t.Fatal(err)
	}
	return tiles
}

func TestCut(t *testing.T) {
	dir := t.TempDir()
	tiles := writeArchive(t, filepath.Join(dir, "src.pmtiles"))
	srv := httptest.NewServer(http.FileServer(http.Dir(dir)))
	defer srv.Close()

	src, err := openArchive(srv.Client(), srv.URL+"/src.pmtiles")
	if err != nil {
		t.Fatal(err)
	}
	if len(src.Entries) != len(tiles) {
		t.Fatalf("read %d entries, want %d", len(src.Entries), len(tiles))
	}
	// A box one tile wide around the centre of the street-level block.
	cut := src.planCut(areaAround(58.38, 26.72, 1))
	var fetched int64
	if err := cut.write(filepath.Join(dir, "out.pmtiles"), 58.38, 26.72, func(n int64) { fetched += n }); err != nil {
		t.Fatal(err)
	}
	if uint64(fetched) != cut.Download {
		t.Errorf("fetched %d bytes, planned %d", fetched, cut.Download)
	}

	out, err := openArchive(srv.Client(), srv.URL+"/out.pmtiles")
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "out.pmtiles"))
	street := 0
	for _, e := range out.Entries {
		z, _, _ := idToZxy(e.TileID)
		got := string(raw[out.Header.DataOffset+e.Offset:][:e.Length])
		if got != tiles[e.TileID] {
			t.Errorf("tile %d holds %q, want %q", e.TileID, got, tiles[e.TileID])
		}
		if z == 14 {
			street++
		}
	}
	if overview := 1 + 4 + 16; len(out.Entries)-street != overview {
		t.Errorf("kept %d overview tiles, want all %d", len(out.Entries)-street, overview)
	}
	if street == 0 || street >= 36 {
		t.Errorf("kept %d of 36 street tiles; the cut should keep some, not all", street)
	}
}

// With VARU_PMTILES=path/to/real.pmtiles, check the reader against a real
// Protomaps extract: every tile must land inside the file's own bounds.
func TestRealArchive(t *testing.T) {
	path := os.Getenv("VARU_PMTILES")
	if path == "" {
		t.Skip("set VARU_PMTILES to check a real map file")
	}
	a, err := openArchive(httpClient(), "file://"+filepath.ToSlash(path))
	if err != nil {
		t.Fatal(err)
	}
	h := a.Header
	z := h.MaxZoom
	x0, x1 := lonToX(float64(h.MinLonE7)/1e7, z), lonToX(float64(h.MaxLonE7)/1e7, z)
	y0, y1 := latToY(float64(h.MaxLatE7)/1e7, z), latToY(float64(h.MinLatE7)/1e7, z)
	var addressed uint64
	for _, e := range a.Entries {
		addressed += uint64(e.RunLength)
		tz, x, y := idToZxy(e.TileID)
		if tz == z && (x < x0-1 || x > x1+1 || y < y0-1 || y > y1+1) {
			t.Fatalf("tile %d/%d/%d is outside the file's bounds", tz, x, y)
		}
	}
	if addressed != h.Addressed {
		t.Fatalf("addressed %d tiles, header says %d", addressed, h.Addressed)
	}

	// Cut 1 km around the file's centre: fewer street tiles, all overview tiles.
	lat, lon := float64(h.CenterLatE7)/1e7, float64(h.CenterLonE7)/1e7
	out := filepath.Join(t.TempDir(), "cut.pmtiles")
	if keep := os.Getenv("VARU_CUT_OUT"); keep != "" {
		out = keep // to check with `pmtiles verify`
	}
	if err := a.planCut(areaAround(lat, lon, 1)).write(out, lat, lon, func(int64) {}); err != nil {
		t.Fatal(err)
	}
	cut, err := openArchive(httpClient(), "file://"+filepath.ToSlash(out))
	if err != nil {
		t.Fatal(err)
	}
	if len(cut.Entries) == 0 || len(cut.Entries) >= len(a.Entries) {
		t.Fatalf("cut kept %d of %d entries", len(cut.Entries), len(a.Entries))
	}
	t.Logf("1 km cut kept %d of %d entries", len(cut.Entries), len(a.Entries))
}
