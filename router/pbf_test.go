package router

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"strings"
	"testing"
)

// pbw writes protobuf wire format, to build small extracts for tests.
type pbw struct{ b []byte }

func (w *pbw) key(f, t int)           { w.b = binary.AppendUvarint(w.b, uint64(f<<3|t)) }
func (w *pbw) varint(f int, v uint64) { w.key(f, 0); w.b = binary.AppendUvarint(w.b, v) }
func (w *pbw) bytes(f int, v []byte) {
	w.key(f, 2)
	w.b = binary.AppendUvarint(w.b, uint64(len(v)))
	w.b = append(w.b, v...)
}
func (w *pbw) packed(f int, vs []uint64) {
	var p pbw
	for _, v := range vs {
		p.b = binary.AppendUvarint(p.b, v)
	}
	w.bytes(f, p.b)
}

func zz(v int64) uint64 { return uint64(v<<1) ^ uint64(v>>63) }

func deltas(vs []int64) []uint64 {
	out, prev := make([]uint64, len(vs)), int64(0)
	for i, v := range vs {
		out[i], prev = zz(v-prev), v
	}
	return out
}

func pbfBlob(kind string, data []byte, compress bool) []byte {
	var blob pbw
	if compress {
		var z bytes.Buffer
		zw := zlib.NewWriter(&z)
		zw.Write(data)
		zw.Close()
		blob.varint(2, uint64(len(data)))
		blob.bytes(3, z.Bytes())
	} else {
		blob.bytes(1, data)
	}
	var head pbw
	head.bytes(1, []byte(kind))
	head.varint(3, uint64(len(blob.b)))
	out := binary.BigEndian.AppendUint32(nil, uint32(len(head.b)))
	return append(append(out, head.b...), blob.b...)
}

func headerBlock(features ...string) []byte {
	var h pbw
	for _, f := range features {
		h.bytes(4, []byte(f))
	}
	h.varint(32, 1795400000) // replication timestamp
	return h.b
}

// testExtract: one compressed block of dense nodes with a lat/lon offset,
// one raw block with a way.
func testExtract(t *testing.T) []byte {
	t.Helper()
	var latOff, lonOff int64 = 1000, -2000 // nanodegrees
	ids := []int64{10, 11, 13}
	lat7 := []int64{583801000, 583802000, 583803000}
	lon7 := []int64{267224000, 267225000, 267226000}
	var lats, lons []int64
	for i := range ids {
		lats = append(lats, (lat7[i]*100-latOff)/100)
		lons = append(lons, (lon7[i]*100-lonOff)/100)
	}
	var dense pbw
	dense.packed(1, deltas(ids))
	dense.packed(8, deltas(lats))
	dense.packed(9, deltas(lons))
	var group pbw
	group.bytes(2, dense.b)
	var nodes pbw
	nodes.bytes(1, []byte{}) // empty string table
	nodes.bytes(2, group.b)
	nodes.varint(19, uint64(latOff))
	nodes.varint(20, uint64(lonOff))

	var st pbw
	for _, s := range []string{"", "highway", "residential", "name", "Riia"} {
		st.bytes(1, []byte(s))
	}
	var way pbw
	way.varint(1, 77)
	way.packed(2, []uint64{1, 3})
	way.packed(3, []uint64{2, 4})
	way.packed(8, deltas([]int64{10, 11, 13}))
	var wgroup pbw
	wgroup.bytes(3, way.b)
	var ways pbw
	ways.bytes(1, st.b)
	ways.bytes(2, wgroup.b)

	out := pbfBlob("OSMHeader", headerBlock("OsmSchema-V0.6", "DenseNodes"), true)
	out = append(out, pbfBlob("OSMData", nodes.b, true)...)
	return append(out, pbfBlob("OSMData", ways.b, false)...)
}

func TestReadPBF(t *testing.T) {
	data := testExtract(t)
	type node struct{ lat, lon int32 }
	nodes := map[int64]node{}
	var wayRefs []int64
	var tags [2]string
	osmTime, err := readPBF(bytes.NewReader(data), pbfHandler{
		node: func(id int64, lat, lon int32) { nodes[id] = node{lat, lon} },
		way: func(id int64, refs func() []int64, tag func(string) string) {
			wayRefs = refs()
			tags = [2]string{tag("highway"), tag("name")}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if osmTime != "2026-11-23T02:13:20Z" {
		t.Errorf("osm time %q", osmTime)
	}
	if got := nodes[13]; got != (node{583803000, 267226000}) || len(nodes) != 3 {
		t.Errorf("nodes %v", nodes)
	}
	if len(wayRefs) != 3 || wayRefs[2] != 13 || tags != [2]string{"residential", "Riia"} {
		t.Errorf("way refs %v tags %v", wayRefs, tags)
	}

	for name, bad := range map[string][]byte{
		"unknown feature": pbfBlob("OSMHeader", headerBlock("OsmSchema-V0.6", "HistoricalInformation"), false),
		"truncated":       data[:len(data)-5],
		"plain nodes": func() []byte {
			var g, b pbw
			g.bytes(1, []byte{8, 1})
			b.bytes(2, g.b)
			return pbfBlob("OSMData", b.b, false)
		}(),
	} {
		_, err := readPBF(bytes.NewReader(bad), pbfHandler{node: func(int64, int32, int32) {}})
		if err == nil {
			t.Errorf("%s: no error", name)
		} else if name == "unknown feature" && !strings.Contains(err.Error(), "HistoricalInformation") {
			t.Errorf("%s: %v", name, err)
		}
	}
}
