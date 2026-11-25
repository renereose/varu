package main

import (
	"encoding/binary"
	"fmt"
	"testing"
)

// A little protobuf writer, enough to make tiles and font files for tests.

func pbUint(b []byte, tag int, v uint64) []byte {
	return binary.AppendUvarint(binary.AppendUvarint(b, uint64(tag<<3)), v)
}

func pbBytes(b []byte, tag int, data []byte) []byte {
	b = binary.AppendUvarint(b, uint64(tag<<3|2))
	return append(binary.AppendUvarint(b, uint64(len(data))), data...)
}

func pbPacked(b []byte, tag int, vs ...uint32) []byte {
	var data []byte
	for _, v := range vs {
		data = binary.AppendUvarint(data, uint64(v))
	}
	return pbBytes(b, tag, data)
}

func zz(n int32) uint32 { return uint32(n<<1) ^ uint32(n>>31) }

// geometry encodes lines (or rings, closed with ClosePath) as commands.
func geometry(closed bool, lines ...[]pt) []uint32 {
	var g []uint32
	var x, y int32
	for _, l := range lines {
		for i, p := range l {
			if i == 0 {
				g = append(g, 1|1<<3)
			} else if i == 1 {
				g = append(g, 2|uint32(len(l)-1)<<3)
			}
			g = append(g, zz(int32(p.x)-x), zz(int32(p.y)-y))
			x, y = int32(p.x), int32(p.y)
		}
		if closed {
			g = append(g, 7|1<<3)
		}
	}
	return g
}

type testFeature struct {
	kind  int
	props map[string]string
	geom  []uint32
}

// testTile makes a tile from layers of features with string properties.
func testTile(layers map[string][]testFeature) []byte {
	var tile []byte
	for name, features := range layers {
		l := pbBytes(nil, 1, []byte(name))
		keys, values := map[string]uint32{}, map[string]uint32{}
		var kv []byte
		for _, f := range features {
			var tags []uint32
			for k, v := range f.props {
				if _, ok := keys[k]; !ok {
					keys[k] = uint32(len(keys))
					kv = pbBytes(kv, 3, []byte(k))
				}
				if _, ok := values[v]; !ok {
					values[v] = uint32(len(values))
					kv = pbBytes(kv, 4, pbBytes(nil, 1, []byte(v)))
				}
				tags = append(tags, keys[k], values[v])
			}
			feat := pbPacked(nil, 2, tags...)
			feat = pbUint(feat, 3, uint64(f.kind))
			l = pbBytes(l, 2, pbPacked(feat, 4, f.geom...))
		}
		l = append(l, kv...)
		l = pbUint(l, 5, 4096)
		tile = pbBytes(tile, 3, l)
	}
	return tile
}

func TestDecodeTile(t *testing.T) {
	b := testTile(map[string][]testFeature{
		"roads": {{kind: 2, props: map[string]string{"kind": "minor_road", "name": "Rüütli"},
			geom: geometry(false, []pt{{10, 10}, {20, 10}, {20, 30}})}},
		"water": {{kind: 3, props: map[string]string{"kind": "water"},
			geom: geometry(true, []pt{{0, 0}, {100, 0}, {100, 100}, {0, 100}}, []pt{{40, 40}, {40, 60}, {60, 60}, {60, 40}})}},
	})
	layers, err := decodeTile(b)
	if err != nil {
		t.Fatal(err)
	}
	road := layers["roads"].features[0]
	if road.str("name") != "Rüütli" || road.str("kind") != "minor_road" || road.kind != 2 ||
		fmt.Sprint(road.geom) != "[[{10 10} {20 10} {20 30}]]" {
		t.Errorf("road %+v", road)
	}
	lake := layers["water"].features[0]
	if len(lake.geom) != 2 || len(lake.geom[1]) != 5 || lake.geom[1][0] != lake.geom[1][4] || lake.geom[1][2] != (pt{60, 60}) {
		t.Errorf("lake with an island: %v", lake.geom)
	}
	if layers["water"].extent != 4096 {
		t.Errorf("extent %v", layers["water"].extent)
	}
	for _, n := range []int{1, 7, len(b) / 2, len(b) - 3} {
		if _, err := decodeTile(b[:len(b)-n]); err == nil {
			t.Errorf("a tile cut %d bytes short was read without an error", n)
		}
	}
}
