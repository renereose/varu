// Package pmtiles reads and writes the parts of the PMTiles v3 format Varu
// uses: the header, directories and tile IDs, and tiles from a local file.
// Spec: https://github.com/protomaps/PMTiles/blob/main/spec/v3/spec.md
package pmtiles

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"errors"
	"io"
	"math"
)

const (
	HeaderLen = 127
	RootMax   = 16384 // header and root directory always fit in the first 16 KiB
	Gzip      = 2     // compression type
)

type Header struct {
	RootOffset, RootLength, MetaOffset, MetaLength uint64
	LeafOffset, LeafLength, DataOffset, DataLength uint64
	Addressed, Entries, Contents                   uint64
	Clustered                                      bool
	InternalComp, TileComp, TileType               uint8
	MinZoom, MaxZoom                               uint8
	MinLonE7, MinLatE7, MaxLonE7, MaxLatE7         int32
	CenterZoom                                     uint8
	CenterLonE7, CenterLatE7                       int32
}

func ParseHeader(b []byte) (Header, error) {
	var h Header
	if len(b) < HeaderLen || string(b[:7]) != "PMTiles" || b[7] != 3 {
		return h, errors.New("not a PMTiles v3 file")
	}
	u := func(i int) uint64 { return binary.LittleEndian.Uint64(b[i:]) }
	s := func(i int) int32 { return int32(binary.LittleEndian.Uint32(b[i:])) }
	h = Header{
		RootOffset: u(8), RootLength: u(16), MetaOffset: u(24), MetaLength: u(32),
		LeafOffset: u(40), LeafLength: u(48), DataOffset: u(56), DataLength: u(64),
		Addressed: u(72), Entries: u(80), Contents: u(88),
		Clustered: b[96] == 1, InternalComp: b[97], TileComp: b[98], TileType: b[99],
		MinZoom: b[100], MaxZoom: b[101],
		MinLonE7: s(102), MinLatE7: s(106), MaxLonE7: s(110), MaxLatE7: s(114),
		CenterZoom: b[118], CenterLonE7: s(119), CenterLatE7: s(123),
	}
	return h, nil
}

func (h Header) Bytes() []byte {
	b := make([]byte, HeaderLen)
	copy(b, "PMTiles")
	b[7] = 3
	for i, v := range []uint64{h.RootOffset, h.RootLength, h.MetaOffset, h.MetaLength, h.LeafOffset,
		h.LeafLength, h.DataOffset, h.DataLength, h.Addressed, h.Entries, h.Contents} {
		binary.LittleEndian.PutUint64(b[8+8*i:], v)
	}
	if h.Clustered {
		b[96] = 1
	}
	b[97], b[98], b[99], b[100], b[101] = h.InternalComp, h.TileComp, h.TileType, h.MinZoom, h.MaxZoom
	for i, v := range []int32{h.MinLonE7, h.MinLatE7, h.MaxLonE7, h.MaxLatE7} {
		binary.LittleEndian.PutUint32(b[102+4*i:], uint32(v))
	}
	b[118] = h.CenterZoom
	binary.LittleEndian.PutUint32(b[119:], uint32(h.CenterLonE7))
	binary.LittleEndian.PutUint32(b[123:], uint32(h.CenterLatE7))
	return b
}

type Entry struct {
	TileID    uint64
	Offset    uint64
	Length    uint32
	RunLength uint32 // 0 marks a pointer to a leaf directory
}

func DecodeDir(compressed []byte) ([]Entry, error) {
	zr, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return nil, err
	}
	raw, err := io.ReadAll(zr)
	if err != nil {
		return nil, err
	}
	r := bytes.NewReader(raw)
	next := func() uint64 {
		v, e := binary.ReadUvarint(r)
		if e != nil && err == nil {
			err = e
		}
		return v
	}
	n := next()
	if err != nil || n > uint64(len(raw)) {
		return nil, errors.New("damaged directory")
	}
	entries := make([]Entry, n)
	var id uint64
	for i := range entries {
		id += next()
		entries[i].TileID = id
	}
	for i := range entries {
		entries[i].RunLength = uint32(next())
	}
	for i := range entries {
		entries[i].Length = uint32(next())
	}
	for i := range entries {
		v := next()
		if v == 0 && i > 0 {
			entries[i].Offset = entries[i-1].Offset + uint64(entries[i-1].Length)
		} else {
			entries[i].Offset = v - 1
		}
	}
	return entries, err
}

func EncodeDir(entries []Entry) []byte {
	var raw []byte
	raw = binary.AppendUvarint(raw, uint64(len(entries)))
	var last uint64
	for _, e := range entries {
		raw = binary.AppendUvarint(raw, e.TileID-last)
		last = e.TileID
	}
	for _, e := range entries {
		raw = binary.AppendUvarint(raw, uint64(e.RunLength))
	}
	for _, e := range entries {
		raw = binary.AppendUvarint(raw, uint64(e.Length))
	}
	for i, e := range entries {
		if i > 0 && e.Offset == entries[i-1].Offset+uint64(entries[i-1].Length) {
			raw = binary.AppendUvarint(raw, 0)
		} else {
			raw = binary.AppendUvarint(raw, e.Offset+1)
		}
	}
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	_, _ = zw.Write(raw)
	_ = zw.Close()
	return buf.Bytes()
}

// BuildDirs makes the root directory, splitting entries into leaf
// directories when they do not fit in the first 16 KiB.
func BuildDirs(entries []Entry) (root, leaves []byte) {
	if root = EncodeDir(entries); len(root) <= RootMax-HeaderLen {
		return root, nil
	}
	for size := 4096; ; size *= 2 {
		var pointers []Entry
		leaves = leaves[:0]
		for i := 0; i < len(entries); i += size {
			leaf := EncodeDir(entries[i:min(i+size, len(entries))])
			pointers = append(pointers, Entry{TileID: entries[i].TileID, Offset: uint64(len(leaves)), Length: uint32(len(leaf))})
			leaves = append(leaves, leaf...)
		}
		if root = EncodeDir(pointers); len(root) <= RootMax-HeaderLen {
			return root, leaves
		}
	}
}

// Tile IDs number tiles zoom by zoom along a Hilbert curve.

func ZxyToID(z uint8, x, y uint32) uint64 {
	acc := (uint64(1)<<(2*uint64(z)) - 1) / 3
	var d uint64
	for s := uint32(1) << z >> 1; s > 0; s >>= 1 {
		var rx, ry uint32
		if x&s != 0 {
			rx = 1
		}
		if y&s != 0 {
			ry = 1
		}
		d += uint64(s) * uint64(s) * uint64((3*rx)^ry)
		x, y = rotate(s, x, y, rx, ry)
	}
	return acc + d
}

func IDToZxy(id uint64) (uint8, uint32, uint32) {
	var z uint8
	var acc uint64
	for ; ; z++ {
		count := uint64(1) << (2 * uint64(z))
		if id < acc+count {
			break
		}
		acc += count
	}
	t := id - acc
	var x, y uint32
	for s := uint32(1); s < uint32(1)<<z; s <<= 1 {
		rx := uint32(1 & (t / 2))
		ry := uint32(1 & (t ^ uint64(rx)))
		x, y = rotate(s, x, y, rx, ry)
		x += s * rx
		y += s * ry
		t /= 4
	}
	return z, x, y
}

func rotate(n, x, y, rx, ry uint32) (uint32, uint32) {
	if ry == 0 {
		if rx == 1 {
			x, y = n-1-x, n-1-y
		}
		return y, x
	}
	return x, y
}

// MercatorX and MercatorY place a point on the Web Mercator world, 0 to 1
// from west to east and from north to south.
func MercatorX(lon float64) float64 { return (lon + 180) / 360 }

func MercatorY(lat float64) float64 {
	r := lat * math.Pi / 180
	return (1 - math.Log(math.Tan(r)+1/math.Cos(r))/math.Pi) / 2
}
