package pmtiles

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestTileIDs(t *testing.T) {
	// Fixed points from the PMTiles spec's reference implementation.
	for _, c := range []struct {
		z    uint8
		x, y uint32
		id   uint64
	}{{0, 0, 0, 0}, {1, 0, 0, 1}, {1, 0, 1, 2}, {1, 1, 1, 3}, {1, 1, 0, 4}, {2, 0, 0, 5}} {
		if got := ZxyToID(c.z, c.x, c.y); got != c.id {
			t.Errorf("ZxyToID(%d,%d,%d) = %d, want %d", c.z, c.x, c.y, got, c.id)
		}
	}
	for _, id := range []uint64{0, 7, 1000, 123456789, ZxyToID(15, 18432, 9472)} {
		z, x, y := IDToZxy(id)
		if back := ZxyToID(z, x, y); back != id {
			t.Errorf("round trip %d -> %d/%d/%d -> %d", id, z, x, y, back)
		}
	}
}

func TestLeafDirectories(t *testing.T) {
	var entries []Entry
	for i := uint64(0); i < 60000; i++ {
		entries = append(entries, Entry{TileID: i * 3, Offset: i * 977 % 1000003, Length: uint32(i%500 + 1), RunLength: 1})
	}
	root, leaves := BuildDirs(entries)
	if len(root) > RootMax-HeaderLen || len(leaves) == 0 {
		t.Fatalf("root %d bytes, leaves %d bytes", len(root), len(leaves))
	}
	pointers, err := DecodeDir(root)
	if err != nil {
		t.Fatal(err)
	}
	var got []Entry
	for _, p := range pointers {
		leaf, err := DecodeDir(leaves[p.Offset : p.Offset+uint64(p.Length)])
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, leaf...)
	}
	if fmt.Sprint(got) != fmt.Sprint(entries) {
		t.Fatal("entries changed on the way through leaf directories")
	}
}

// TestFileTile reads tiles back from an archive with leaf directories and
// one run of three tiles sharing their data.
func TestFileTile(t *testing.T) {
	var data bytes.Buffer
	var entries []Entry
	zw := gzip.NewWriter(nil)
	add := func(id uint64, run uint32, body string) {
		start := data.Len()
		zw.Reset(&data)
		zw.Write([]byte(body))
		zw.Close()
		entries = append(entries, Entry{TileID: id, Offset: uint64(start), Length: uint32(data.Len() - start), RunLength: run})
	}
	// Uneven gaps between tile IDs, so the directory does not fit the root.
	want := map[uint64]string{}
	id, r := uint64(0), uint64(1)
	for i := 0; i < 60000; i++ {
		want[id] = fmt.Sprint("tile ", id)
		add(id, 1, want[id])
		want[id+1] = "" // a gap
		r = r*6364136223846793005 + 1442695040888963407
		id += 2 + r>>58
	}
	add(id, 3, "sea")
	want[id], want[id+2], want[id+3] = "sea", "sea", ""

	root, leaves := BuildDirs(entries)
	h := Header{InternalComp: Gzip, TileComp: Gzip, TileType: 1}
	h.RootOffset, h.RootLength = HeaderLen, uint64(len(root))
	h.LeafOffset, h.LeafLength = h.RootOffset+h.RootLength, uint64(len(leaves))
	h.DataOffset, h.DataLength = h.LeafOffset+h.LeafLength, uint64(data.Len())
	path := filepath.Join(t.TempDir(), "t.pmtiles")
	if err := os.WriteFile(path, bytes.Join([][]byte{h.Bytes(), root, leaves, data.Bytes()}, nil), 0o644); err != nil {
		t.Fatal(err)
	}
	if len(leaves) == 0 {
		t.Fatal("the test archive should need leaf directories")
	}

	f, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	for id, want := range want {
		got, err := f.Tile(IDToZxy(id))
		if err != nil || string(got) != want {
			t.Fatalf("tile %d: %q %v, want %q", id, got, err, want)
		}
	}
}
