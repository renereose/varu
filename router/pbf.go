package router

// A minimal reader for OpenStreetMap PBF extracts (the format Geofabrik
// publishes): just nodes' positions and ways with their tags.
// Spec: https://wiki.openstreetmap.org/wiki/PBF_Format

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// ---------- protobuf wire format (no schema) ----------
// Unexported here; when the e-ink device needs the same reader,
// move this section to an exported router/wire package.

type wire struct {
	b   []byte
	err error
}

// next reads a field key; ok is false at the end or on an error.
func (w *wire) next() (field, typ int, ok bool) {
	if w.err != nil || len(w.b) == 0 {
		return 0, 0, false
	}
	key := w.uvarint()
	return int(key >> 3), int(key & 7), w.err == nil
}

func (w *wire) uvarint() uint64 {
	v, n := binary.Uvarint(w.b)
	if n <= 0 {
		w.fail()
		return 0
	}
	w.b = w.b[n:]
	return v
}

func (w *wire) bytes() []byte {
	n := w.uvarint()
	if w.err != nil || n > uint64(len(w.b)) {
		w.fail()
		return nil
	}
	v := w.b[:n]
	w.b = w.b[n:]
	return v
}

func (w *wire) skip(typ int) {
	switch typ {
	case 0:
		w.uvarint()
	case 1, 5:
		n := 8
		if typ == 5 {
			n = 4
		}
		if len(w.b) < n {
			w.fail()
			return
		}
		w.b = w.b[n:]
	case 2:
		w.bytes()
	default:
		w.fail()
	}
}

func (w *wire) fail() {
	if w.err == nil {
		w.err = errors.New("damaged protobuf data")
	}
	w.b = nil
}

func zigzag(v uint64) int64 { return int64(v>>1) ^ -int64(v&1) }

// varints appends a repeated varint field, packed (typ 2) or not (typ 0).
func (w *wire) varints(typ int, dst []uint64) []uint64 {
	if typ == 0 {
		return append(dst, w.uvarint())
	}
	p := wire{b: w.bytes()}
	for len(p.b) > 0 && p.err == nil {
		dst = append(dst, p.uvarint())
	}
	if p.err != nil {
		w.fail()
	}
	return dst
}

// ---------- OSM PBF ----------

// pbfHandler receives what a pass needs; a nil function skips that element
// type. Coordinates are in 1e-7 degrees. tag looks up one of the way's tags.
type pbfHandler struct {
	node func(id int64, lat, lon int32)
	way  func(id int64, refs func() []int64, tag func(key string) string)
}

const (
	maxHeader = 64 << 10
	maxBlob   = 32 << 20
)

// readPBF calls h for every node and way in r and returns the extract's
// replication time (RFC 3339) when the file records one.
func readPBF(r io.Reader, h pbfHandler) (osmTime string, err error) {
	var size [4]byte
	for {
		if _, err := io.ReadFull(r, size[:]); err == io.EOF {
			return osmTime, nil
		} else if err != nil {
			return osmTime, fmt.Errorf("osm extract: %w", err)
		}
		n := binary.BigEndian.Uint32(size[:])
		if n > maxHeader {
			return osmTime, errors.New("osm extract: damaged block header")
		}
		head := make([]byte, n)
		if _, err := io.ReadFull(r, head); err != nil {
			return osmTime, fmt.Errorf("osm extract: %w", err)
		}
		var kind string
		var dataSize uint64
		hw := wire{b: head}
		for f, t, ok := hw.next(); ok; f, t, ok = hw.next() {
			switch {
			case f == 1 && t == 2:
				kind = string(hw.bytes())
			case f == 3 && t == 0:
				dataSize = hw.uvarint()
			default:
				hw.skip(t)
			}
		}
		if hw.err != nil || dataSize > maxBlob {
			return osmTime, errors.New("osm extract: damaged block header")
		}
		blob := make([]byte, dataSize)
		if _, err := io.ReadFull(r, blob); err != nil {
			return osmTime, fmt.Errorf("osm extract: %w", err)
		}
		data, err := unpackBlob(blob)
		if err != nil {
			return osmTime, err
		}
		switch kind {
		case "OSMHeader":
			if osmTime, err = readHeaderBlock(data); err != nil {
				return osmTime, err
			}
		case "OSMData":
			if err := readPrimitiveBlock(data, h); err != nil {
				return osmTime, err
			}
		}
	}
}

func unpackBlob(blob []byte) ([]byte, error) {
	w := wire{b: blob}
	var raw, packed []byte
	var rawSize uint64
	for f, t, ok := w.next(); ok; f, t, ok = w.next() {
		switch {
		case f == 1 && t == 2:
			raw = w.bytes()
		case f == 2 && t == 0:
			rawSize = w.uvarint()
		case f == 3 && t == 2:
			packed = w.bytes()
		case f >= 4 && f <= 7:
			// Only zlib, which is what Geofabrik and osmium write.
			return nil, errors.New("osm extract: unsupported compression (only zlib is read)")
		default:
			w.skip(t)
		}
	}
	if w.err != nil {
		return nil, errors.New("osm extract: damaged block")
	}
	if packed == nil {
		return raw, nil
	}
	if rawSize > maxBlob {
		return nil, errors.New("osm extract: block too large")
	}
	zr, err := zlib.NewReader(bytes.NewReader(packed))
	if err != nil {
		return nil, fmt.Errorf("osm extract: %w", err)
	}
	out := make([]byte, 0, rawSize)
	buf := bytes.NewBuffer(out)
	if _, err := io.Copy(buf, io.LimitReader(zr, maxBlob+1)); err != nil {
		return nil, fmt.Errorf("osm extract: %w", err)
	}
	if buf.Len() > maxBlob {
		return nil, errors.New("osm extract: block too large")
	}
	return buf.Bytes(), nil
}

var knownFeatures = map[string]bool{"OsmSchema-V0.6": true, "DenseNodes": true}

func readHeaderBlock(data []byte) (osmTime string, err error) {
	w := wire{b: data}
	for f, t, ok := w.next(); ok; f, t, ok = w.next() {
		switch {
		case f == 4 && t == 2:
			if feature := string(w.bytes()); !knownFeatures[feature] {
				return "", fmt.Errorf("osm extract needs %q, which this reader does not support", feature)
			}
		case f == 32 && t == 0:
			osmTime = unixUTC(int64(w.uvarint()))
		default:
			w.skip(t)
		}
	}
	if w.err != nil {
		return "", errors.New("osm extract: damaged header")
	}
	return osmTime, nil
}

func readPrimitiveBlock(data []byte, h pbfHandler) error {
	var strs [][]byte
	var groups [][]byte
	gran, latOff, lonOff := int64(100), int64(0), int64(0)
	w := wire{b: data}
	for f, t, ok := w.next(); ok; f, t, ok = w.next() {
		switch {
		case f == 1 && t == 2:
			st := wire{b: w.bytes()}
			for sf, stt, sok := st.next(); sok; sf, stt, sok = st.next() {
				if sf == 1 && stt == 2 {
					strs = append(strs, st.bytes())
				} else {
					st.skip(stt)
				}
			}
			if st.err != nil {
				return errors.New("osm extract: damaged string table")
			}
		case f == 2 && t == 2:
			groups = append(groups, w.bytes())
		case f == 17 && t == 0:
			gran = int64(w.uvarint())
		case f == 19 && t == 0:
			latOff = int64(w.uvarint())
		case f == 20 && t == 0:
			lonOff = int64(w.uvarint())
		default:
			w.skip(t)
		}
	}
	if w.err != nil {
		return errors.New("osm extract: damaged data block")
	}
	// Positions are lat_offset + granularity*value in 1e-9 degrees.
	coord := func(off, v int64) int32 { return int32((off + gran*v) / 100) }

	for _, g := range groups {
		gw := wire{b: g}
		for f, t, ok := gw.next(); ok; f, t, ok = gw.next() {
			switch {
			case f == 1 && t == 2:
				return errors.New("osm extract: plain nodes are not supported (only dense nodes)")
			case f == 2 && t == 2 && h.node != nil:
				if err := readDense(gw.bytes(), latOff, lonOff, coord, h.node); err != nil {
					return err
				}
			case f == 3 && t == 2 && h.way != nil:
				if err := readWay(gw.bytes(), strs, h.way); err != nil {
					return err
				}
			default:
				gw.skip(t)
			}
		}
		if gw.err != nil {
			return errors.New("osm extract: damaged group")
		}
	}
	return nil
}

func readDense(data []byte, latOff, lonOff int64, coord func(off, v int64) int32, node func(int64, int32, int32)) error {
	var ids, lats, lons []uint64
	w := wire{b: data}
	for f, t, ok := w.next(); ok; f, t, ok = w.next() {
		switch f {
		case 1:
			ids = w.varints(t, ids)
		case 8:
			lats = w.varints(t, lats)
		case 9:
			lons = w.varints(t, lons)
		default:
			w.skip(t)
		}
	}
	if w.err != nil || len(lats) != len(ids) || len(lons) != len(ids) {
		return errors.New("osm extract: damaged dense nodes")
	}
	var id, lat, lon int64
	for i := range ids {
		id += zigzag(ids[i])
		lat += zigzag(lats[i])
		lon += zigzag(lons[i])
		node(id, coord(latOff, lat), coord(lonOff, lon))
	}
	return nil
}

func readWay(data []byte, strs [][]byte, way func(int64, func() []int64, func(string) string)) error {
	var id uint64
	var keys, vals, deltas []uint64
	w := wire{b: data}
	for f, t, ok := w.next(); ok; f, t, ok = w.next() {
		switch f {
		case 1:
			id = w.uvarint()
		case 2:
			keys = w.varints(t, keys)
		case 3:
			vals = w.varints(t, vals)
		case 8:
			deltas = w.varints(t, deltas)
		default:
			w.skip(t)
		}
	}
	if w.err != nil || len(keys) != len(vals) {
		return errors.New("osm extract: damaged way")
	}
	for i := range keys {
		if keys[i] >= uint64(len(strs)) || vals[i] >= uint64(len(strs)) {
			return errors.New("osm extract: damaged way tags")
		}
	}
	tag := func(key string) string {
		for i, k := range keys {
			if string(strs[k]) == key {
				return string(strs[vals[i]])
			}
		}
		return ""
	}
	// Decoded only for the ways a pass keeps: most ways are buildings and fields.
	refs := func() []int64 {
		out := make([]int64, len(deltas))
		var ref int64
		for i, d := range deltas {
			ref += zigzag(d)
			out[i] = ref
		}
		return out
	}
	way(int64(id), refs, tag)
	return nil
}
