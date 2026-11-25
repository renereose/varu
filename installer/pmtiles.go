package main

// Cutting a map that has the whole country at overview zooms and full
// street detail around home out of the release's country map, with HTTP
// range requests. The file format itself is in ../pmtiles.

import (
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/renereose/varu/pmtiles"
)

const (
	maxChunk     = 8 << 20   // bytes per range request
	maxGap       = 512 << 10 // fetch across gaps smaller than this rather than make another request
	overviewZoom = 12        // whole country down to here: every road and village, ~50 MB for Estonia
)

// Area is the part of the map kept at full detail.
type Area struct{ West, South, East, North float64 }

func areaAround(lat, lon, km float64) Area {
	dLat := km / 111.32
	dLon := km / (111.32 * math.Cos(lat*math.Pi/180))
	return Area{lon - dLon, lat - dLat, lon + dLon, lat + dLat}
}

func lonToX(lon float64, z uint8) uint32 { return tileIndex(pmtiles.MercatorX(lon), z) }

func latToY(lat float64, z uint8) uint32 { return tileIndex(pmtiles.MercatorY(lat), z) }

func tileIndex(f float64, z uint8) uint32 {
	n := float64(uint32(1) << z)
	return uint32(math.Max(0, math.Min(n-1, math.Floor(f*n))))
}

// Archive is a remote PMTiles file whose directories have been read.
type Archive struct {
	URL     string
	Header  pmtiles.Header
	Meta    []byte
	Entries []pmtiles.Entry // tile entries in TileID order
	client  *http.Client
}

func openArchive(client *http.Client, url string) (*Archive, error) {
	a := &Archive{URL: url, client: client}
	head, err := a.read(0, pmtiles.RootMax)
	if err != nil {
		return nil, err
	}
	if a.Header, err = pmtiles.ParseHeader(head); err != nil {
		return nil, err
	}
	h := a.Header
	if h.InternalComp != pmtiles.Gzip || h.RootOffset+h.RootLength > uint64(len(head)) {
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
		entries, err := pmtiles.DecodeDir(dir)
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
	entries  []pmtiles.Entry // output entries, offsets already rewritten
	contents []uint64        // source offsets of the tile data to copy, in order
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
			z, x, y := pmtiles.IDToZxy(id)
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
			c.entries = append(c.entries, pmtiles.Entry{TileID: id, Offset: e.Offset, Length: e.Length, RunLength: 1})
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
	c.Size = size + pmtiles.RootMax + uint64(len(a.Meta)) + uint64(len(c.entries))*4 // directories: a rough upper bound
	return c
}

// write fetches the planned tiles and writes the new archive to dest,
// via dest.part. progress is told about every byte fetched.
func (c *Cut) write(dest string, centerLat, centerLon float64, progress func(int64)) error {
	root, leaves := pmtiles.BuildDirs(c.entries)
	h := c.src.Header
	var dataLen, addressed uint64
	for _, off := range c.contents {
		dataLen += uint64(c.lengths[off])
	}
	for _, e := range c.entries {
		addressed += uint64(e.RunLength)
	}
	h.RootOffset, h.RootLength = pmtiles.HeaderLen, uint64(len(root))
	h.MetaOffset, h.MetaLength = h.RootOffset+h.RootLength, uint64(len(c.src.Meta))
	h.LeafOffset, h.LeafLength = h.MetaOffset+h.MetaLength, uint64(len(leaves))
	h.DataOffset, h.DataLength = h.LeafOffset+h.LeafLength, dataLen
	h.Addressed, h.Entries, h.Contents = addressed, uint64(len(c.entries)), uint64(len(c.contents))
	h.Clustered, h.InternalComp = true, pmtiles.Gzip
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
	for _, b := range [][]byte{h.Bytes(), root, c.src.Meta, leaves} {
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
