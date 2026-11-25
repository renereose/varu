package pmtiles

import (
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"os"
	"sort"
)

// File reads tiles from a PMTiles archive on disk.
type File struct {
	Header Header
	f      *os.File
	root   []Entry
	leaves map[uint64][]Entry // by offset in the leaf section
}

func Open(path string) (*File, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	p := &File{f: f, leaves: map[uint64][]Entry{}}
	head := make([]byte, RootMax)
	n, err := f.ReadAt(head, 0)
	if err == io.EOF {
		err = nil
	}
	if err == nil {
		p.Header, err = ParseHeader(head[:n])
	}
	if err == nil {
		h := p.Header
		if h.InternalComp != Gzip || h.RootOffset+h.RootLength > uint64(n) {
			err = errors.New("unsupported map file layout")
		} else {
			p.root, err = DecodeDir(head[h.RootOffset : h.RootOffset+h.RootLength])
		}
	}
	if err != nil {
		f.Close()
		return nil, err
	}
	return p, nil
}

func (p *File) Close() error { return p.f.Close() }

// Tile returns the tile's data, uncompressed, or nil if the archive does
// not have it.
func (p *File) Tile(z uint8, x, y uint32) ([]byte, error) {
	id := ZxyToID(z, x, y)
	dir := p.root
	for depth := 0; depth < 4; depth++ {
		e, ok := findEntry(dir, id)
		if !ok {
			return nil, nil
		}
		if e.RunLength > 0 {
			data, err := p.read(p.Header.DataOffset+e.Offset, uint64(e.Length))
			if err != nil || p.Header.TileComp != Gzip {
				return data, err
			}
			zr, err := gzip.NewReader(bytes.NewReader(data))
			if err != nil {
				return nil, err
			}
			return io.ReadAll(zr)
		}
		leaf, ok := p.leaves[e.Offset]
		if !ok {
			if e.Offset+uint64(e.Length) > p.Header.LeafLength {
				return nil, errors.New("damaged map directory")
			}
			raw, err := p.read(p.Header.LeafOffset+e.Offset, uint64(e.Length))
			if err != nil {
				return nil, err
			}
			if leaf, err = DecodeDir(raw); err != nil {
				return nil, err
			}
			p.leaves[e.Offset] = leaf
		}
		dir = leaf
	}
	return nil, errors.New("damaged map directory")
}

func (p *File) read(offset, length uint64) ([]byte, error) {
	b := make([]byte, length)
	_, err := p.f.ReadAt(b, int64(offset))
	return b, err
}

// findEntry finds the entry for id: a run of tiles that includes it, or
// the leaf directory that may.
func findEntry(dir []Entry, id uint64) (Entry, bool) {
	i := sort.Search(len(dir), func(i int) bool { return dir[i].TileID > id }) - 1
	if i < 0 {
		return Entry{}, false
	}
	e := dir[i]
	if e.RunLength == 0 || id < e.TileID+uint64(e.RunLength) {
		return e, true
	}
	return Entry{}, false
}
