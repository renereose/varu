package main

// A minimal PMTiles v3 reader and writer: enough to cut a map that has the
// whole country at overview zooms and full street detail around home out
// of the release's country map, with HTTP range requests.
// Spec: https://github.com/protomaps/PMTiles/blob/main/spec/v3/spec.md

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"time"
)

const (
	headerLen    = 127
	rootMax      = 16384 // header and root directory always fit in the first 16 KiB
	gzipComp     = 2
	maxChunk     = 8 << 20   // bytes per range request
	maxGap       = 512 << 10 // fetch across gaps smaller than this rather than make another request
	overviewZoom = 12        // whole country down to here: every road and village, ~50 MB for Estonia
)

type pmHeader struct {
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

func parseHeader(b []byte) (pmHeader, error) {
	var h pmHeader
	if len(b) < headerLen || string(b[:7]) != "PMTiles" || b[7] != 3 {
		return h, errors.New("not a PMTiles v3 file")
	}
	u := func(i int) uint64 { return binary.LittleEndian.Uint64(b[i:]) }
	s := func(i int) int32 { return int32(binary.LittleEndian.Uint32(b[i:])) }
	h = pmHeader{
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

func (h pmHeader) bytes() []byte {
	b := make([]byte, headerLen)
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

type entry struct {
	TileID    uint64
	Offset    uint64
	Length    uint32
	RunLength uint32 // 0 marks a pointer to a leaf directory
}

func decodeDir(compressed []byte) ([]entry, error) {
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
	entries := make([]entry, n)
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

func encodeDir(entries []entry) []byte {
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

// buildDirs makes the root directory, splitting entries into leaf
// directories when they do not fit in the first 16 KiB.
func buildDirs(entries []entry) (root, leaves []byte) {
	if root = encodeDir(entries); len(root) <= rootMax-headerLen {
		return root, nil
	}
	for size := 4096; ; size *= 2 {
		var pointers []entry
		leaves = leaves[:0]
		for i := 0; i < len(entries); i += size {
			leaf := encodeDir(entries[i:min(i+size, len(entries))])
			pointers = append(pointers, entry{TileID: entries[i].TileID, Offset: uint64(len(leaves)), Length: uint32(len(leaf))})
			leaves = append(leaves, leaf...)
		}
		if root = encodeDir(pointers); len(root) <= rootMax-headerLen {
			return root, leaves
		}
	}
}

// Tile IDs number tiles zoom by zoom along a Hilbert curve.

func zxyToID(z uint8, x, y uint32) uint64 {
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

func idToZxy(id uint64) (uint8, uint32, uint32) {
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

// Area is the part of the map kept at full detail.
type Area struct{ West, South, East, North float64 }

func areaAround(lat, lon, km float64) Area {
	dLat := km / 111.32
	dLon := km / (111.32 * math.Cos(lat*math.Pi/180))
	return Area{lon - dLon, lat - dLat, lon + dLon, lat + dLat}
}

func lonToX(lon float64, z uint8) uint32 {
	n := float64(uint32(1) << z)
	return uint32(math.Max(0, math.Min(n-1, math.Floor((lon+180)/360*n))))
}

func latToY(lat float64, z uint8) uint32 {
	n := float64(uint32(1) << z)
	r := lat * math.Pi / 180
	y := (1 - math.Log(math.Tan(r)+1/math.Cos(r))/math.Pi) / 2 * n
	return uint32(math.Max(0, math.Min(n-1, math.Floor(y))))
}

// Archive is a remote PMTiles file whose directories have been read.
type Archive struct {
	URL     string
	Header  pmHeader
	Meta    []byte
	Entries []entry // tile entries in TileID order
	client  *http.Client
}

func openArchive(client *http.Client, url string) (*Archive, error) {
	a := &Archive{URL: url, client: client}
	head, err := a.read(0, rootMax)
	if err != nil {
		return nil, err
	}
	if a.Header, err = parseHeader(head); err != nil {
		return nil, err
	}
	h := a.Header
	if h.InternalComp != gzipComp || h.RootOffset+h.RootLength > uint64(len(head)) {
		return nil, errors.New("unsupported map file layout")
	}
	if a.Meta, err = a.read(h.MetaOffset, h.MetaLength); err != nil {
		return nil, err
	}
	var leaves []byte
	if h.LeafLength > 0 {
		if leaves, err = a.read(h.LeafOffset, h.LeafLength); err != nil {
			return nil, err
		}
	}
	var walk func(dir []byte, depth int) error
	walk = func(dir []byte, depth int) error {
		entries, err := decodeDir(dir)
		if err != nil || depth > 3 {
			return errors.Join(err, errors.New("damaged map directory"))
		}
		for _, e := range entries {
			if e.RunLength > 0 {
				a.Entries = append(a.Entries, e)
				continue
			}
			if e.Offset+uint64(e.Length) > uint64(len(leaves)) {
				return errors.New("damaged map directory")
			}
			if err := walk(leaves[e.Offset:e.Offset+uint64(e.Length)], depth+1); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(head[h.RootOffset:h.RootOffset+h.RootLength], 0); err != nil {
		return nil, err
	}
	return a, nil
}

// read fetches length bytes at offset (or fewer, at the end of the file).
func (a *Archive) read(offset, length uint64) ([]byte, error) {
	var lastErr error
	for attempt := 1; attempt <= 5; attempt++ {
		req, _ := http.NewRequest("GET", a.URL, nil)
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", offset, offset+length-1))
		req.Header.Set("User-Agent", "varu-installer/"+version)
		resp, err := a.client.Do(req)
		if err == nil {
			body, readErr := io.ReadAll(resp.Body)
			resp.Body.Close()
			switch {
			case resp.StatusCode == http.StatusPartialContent && readErr == nil:
				return body, nil
			case resp.StatusCode == http.StatusOK && readErr == nil && offset == 0:
				return body[:min(uint64(len(body)), length)], nil // small file served whole
			case readErr != nil:
				err = readErr
			case resp.StatusCode < 500 && resp.StatusCode != http.StatusTooManyRequests:
				return nil, fmt.Errorf("%s: %s", a.URL, resp.Status)
			default:
				err = fmt.Errorf("%s: %s", a.URL, resp.Status)
			}
		}
		lastErr = err
		time.Sleep(time.Duration(attempt) * 2 * time.Second)
	}
	return nil, lastErr
}

// Cut is a plan for a smaller map: which tiles to keep and which byte
// ranges of the source to fetch for them.
type Cut struct {
	src      *Archive
	entries  []entry  // output entries, offsets already rewritten
	contents []uint64 // source offsets of the tile data to copy, in order
	lengths  map[uint64]uint32
	chunks   [][2]uint64 // source data ranges to request: offset, length
	Download uint64      // bytes to fetch
	Size     uint64      // bytes of the finished file
	minZoom  uint8
	maxZoom  uint8
}

// planCut keeps every tile at overview zooms and the tiles touching area
// at all zooms.
func (a *Archive) planCut(area Area) *Cut {
	type span struct{ x0, x1, y0, y1 uint32 }
	spans := map[uint8]span{}
	inside := func(z uint8, x, y uint32) bool {
		if z <= overviewZoom {
			return true
		}
		s, ok := spans[z]
		if !ok {
			s = span{lonToX(area.West, z), lonToX(area.East, z), latToY(area.North, z), latToY(area.South, z)}
			spans[z] = s
		}
		return x >= s.x0 && x <= s.x1 && y >= s.y0 && y <= s.y1
	}

	c := &Cut{src: a, lengths: map[uint64]uint32{}, minZoom: 255}
	for _, e := range a.Entries {
		for i := uint64(0); i < uint64(e.RunLength); i++ {
			id := e.TileID + i
			z, x, y := idToZxy(id)
			if !inside(z, x, y) {
				continue
			}
			c.minZoom, c.maxZoom = min(c.minZoom, z), max(c.maxZoom, z)
			if n := len(c.entries); n > 0 {
				last := &c.entries[n-1]
				if last.Offset == e.Offset && last.TileID+uint64(last.RunLength) == id {
					last.RunLength++
					continue
				}
			}
			c.entries = append(c.entries, entry{TileID: id, Offset: e.Offset, Length: e.Length, RunLength: 1})
			c.lengths[e.Offset] = e.Length
		}
	}

	for offset := range c.lengths {
		c.contents = append(c.contents, offset)
	}
	sort.Slice(c.contents, func(i, j int) bool { return c.contents[i] < c.contents[j] })
	newOffset := make(map[uint64]uint64, len(c.contents))
	var size uint64
	for _, off := range c.contents {
		newOffset[off] = size
		size += uint64(c.lengths[off])
		end := off + uint64(c.lengths[off])
		if n := len(c.chunks); n > 0 {
			last := &c.chunks[n-1]
			if off-(last[0]+last[1]) <= maxGap && end-last[0] <= maxChunk {
				last[1] = end - last[0]
				continue
			}
		}
		c.chunks = append(c.chunks, [2]uint64{off, end - off})
	}
	for i := range c.entries {
		c.entries[i].Offset = newOffset[c.entries[i].Offset]
	}
	for _, ch := range c.chunks {
		c.Download += ch[1]
	}
	c.Size = size + rootMax + uint64(len(a.Meta)) + uint64(len(c.entries))*4 // directories: a rough upper bound
	return c
}

// write fetches the planned tiles and writes the new archive to dest,
// via dest.part. progress is told about every byte fetched.
func (c *Cut) write(dest string, centerLat, centerLon float64, progress func(int64)) error {
	root, leaves := buildDirs(c.entries)
	h := c.src.Header
	var dataLen, addressed uint64
	for _, off := range c.contents {
		dataLen += uint64(c.lengths[off])
	}
	for _, e := range c.entries {
		addressed += uint64(e.RunLength)
	}
	h.RootOffset, h.RootLength = headerLen, uint64(len(root))
	h.MetaOffset, h.MetaLength = h.RootOffset+h.RootLength, uint64(len(c.src.Meta))
	h.LeafOffset, h.LeafLength = h.MetaOffset+h.MetaLength, uint64(len(leaves))
	h.DataOffset, h.DataLength = h.LeafOffset+h.LeafLength, dataLen
	h.Addressed, h.Entries, h.Contents = addressed, uint64(len(c.entries)), uint64(len(c.contents))
	h.Clustered, h.InternalComp = true, gzipComp
	h.MinZoom, h.MaxZoom = c.minZoom, c.maxZoom
	h.CenterZoom = 11
	h.CenterLonE7, h.CenterLatE7 = int32(centerLon*1e7), int32(centerLat*1e7)

	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	part := dest + ".part"
	f, err := os.Create(part)
	if err != nil {
		return err
	}
	defer f.Close()
	for _, b := range [][]byte{h.bytes(), root, c.src.Meta, leaves} {
		if _, err := f.Write(b); err != nil {
			return err
		}
	}
	// ponytail: one range request at a time, and a failed run starts the
	// map again; fetch chunks in parallel if this proves slow.
	next := 0
	for _, ch := range c.chunks {
		data, err := c.src.read(c.src.Header.DataOffset+ch[0], ch[1])
		if err != nil {
			return err
		}
		if uint64(len(data)) != ch[1] {
			return errors.New("map download was cut short")
		}
		progress(int64(len(data)))
		for ; next < len(c.contents) && c.contents[next] < ch[0]+ch[1]; next++ {
			off := c.contents[next] - ch[0]
			if _, err := f.Write(data[off : off+uint64(c.lengths[c.contents[next]])]); err != nil {
				return err
			}
		}
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(part, dest)
}
