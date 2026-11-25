package main

// Mapbox Vector Tiles (the map's tiles) and the font files share the
// protobuf wire format; this is just enough of it to read both.
// Spec: https://github.com/mapbox/vector-tile-spec/tree/master/2.1

import (
	"encoding/binary"
	"errors"
	"math"
)

var errDamaged = errors.New("damaged tile")

// pbf walks the fields of one protobuf message.
type pbf struct {
	b    []byte
	tag  int
	wire int
	num  uint64 // a varint or fixed field
	data []byte // a length-delimited field
	err  error
}

func (p *pbf) next() bool {
	if len(p.b) == 0 || p.err != nil {
		return false
	}
	key := p.varint()
	p.tag, p.wire = int(key>>3), int(key&7)
	switch p.wire {
	case 0:
		p.num = p.varint()
	case 1:
		p.num = p.fixed(8)
	case 5:
		p.num = p.fixed(4)
	case 2:
		n := p.varint()
		if n > uint64(len(p.b)) {
			p.err = errDamaged
			return false
		}
		p.data, p.b = p.b[:n], p.b[n:]
	default:
		p.err = errDamaged
	}
	return p.err == nil
}

func (p *pbf) varint() uint64 {
	v, n := binary.Uvarint(p.b)
	if n <= 0 {
		p.err, p.b = errDamaged, nil
		return 0
	}
	p.b = p.b[n:]
	return v
}

func (p *pbf) fixed(n int) uint64 {
	if len(p.b) < n {
		p.err, p.b = errDamaged, nil
		return 0
	}
	var v uint64
	if n == 8 {
		v = binary.LittleEndian.Uint64(p.b)
	} else {
		v = uint64(binary.LittleEndian.Uint32(p.b))
	}
	p.b = p.b[n:]
	return v
}

// packed reads a packed repeated varint field.
func packed(b []byte) ([]uint32, error) {
	var out []uint32
	for len(b) > 0 {
		v, n := binary.Uvarint(b)
		if n <= 0 {
			return nil, errDamaged
		}
		out = append(out, uint32(v))
		b = b[n:]
	}
	return out, nil
}

func zigzag(v uint32) int32 { return int32(v>>1) ^ -int32(v&1) }

type layer struct {
	name     string
	extent   float64
	features []feature
}

type feature struct {
	kind  int // 1 point, 2 line, 3 polygon
	props map[string]any
	geom  [][]pt // points, lines or polygon rings, in tile units
}

type pt struct{ x, y float64 }

func (f feature) str(key string) string { s, _ := f.props[key].(string); return s }

func (f feature) num(key string) float64 {
	switch v := f.props[key].(type) {
	case float64:
		return v
	case int64:
		return float64(v)
	}
	return math.NaN()
}

func decodeTile(b []byte) (map[string]*layer, error) {
	layers := map[string]*layer{}
	p := pbf{b: b}
	for p.next() {
		if p.tag != 3 || p.wire != 2 {
			continue
		}
		l, err := decodeLayer(p.data)
		if err != nil {
			return nil, err
		}
		layers[l.name] = l
	}
	return layers, p.err
}

func decodeLayer(b []byte) (*layer, error) {
	l := &layer{extent: 4096}
	var keys []string
	var values []any
	var raw [][]byte
	p := pbf{b: b}
	for p.next() {
		switch {
		case p.tag == 1 && p.wire == 2:
			l.name = string(p.data)
		case p.tag == 2 && p.wire == 2:
			raw = append(raw, p.data)
		case p.tag == 3 && p.wire == 2:
			keys = append(keys, string(p.data))
		case p.tag == 4 && p.wire == 2:
			values = append(values, decodeValue(p.data))
		case p.tag == 5 && p.wire == 0:
			l.extent = float64(p.num)
		}
	}
	if p.err != nil {
		return nil, p.err
	}
	for _, r := range raw {
		f, err := decodeFeature(r, keys, values)
		if err != nil {
			return nil, err
		}
		l.features = append(l.features, f)
	}
	return l, nil
}

func decodeValue(b []byte) any {
	p := pbf{b: b}
	for p.next() {
		switch p.tag {
		case 1:
			return string(p.data)
		case 2:
			return float64(math.Float32frombits(uint32(p.num)))
		case 3:
			return math.Float64frombits(p.num)
		case 4, 5:
			return int64(p.num)
		case 6:
			return int64(p.num>>1) ^ -int64(p.num&1)
		case 7:
			return p.num != 0
		}
	}
	return nil
}

func decodeFeature(b []byte, keys []string, values []any) (feature, error) {
	f := feature{props: map[string]any{}}
	var tags, geom []uint32
	var err error
	p := pbf{b: b}
	for p.next() {
		switch {
		case p.tag == 2 && p.wire == 2:
			tags, err = packed(p.data)
		case p.tag == 3 && p.wire == 0:
			f.kind = int(p.num)
		case p.tag == 4 && p.wire == 2:
			geom, err = packed(p.data)
		}
		if err != nil {
			return f, err
		}
	}
	if p.err != nil || len(tags)%2 != 0 {
		return f, errDamaged
	}
	for i := 0; i < len(tags); i += 2 {
		if int(tags[i]) >= len(keys) || int(tags[i+1]) >= len(values) {
			return f, errDamaged
		}
		f.props[keys[tags[i]]] = values[tags[i+1]]
	}
	f.geom, err = decodeGeometry(geom, f.kind)
	return f, err
}

// decodeGeometry follows the MoveTo, LineTo and ClosePath commands.
func decodeGeometry(g []uint32, kind int) ([][]pt, error) {
	var out [][]pt
	var x, y int32
	for i := 0; i < len(g); {
		cmd, count := g[i]&7, int(g[i]>>3)
		i++
		switch cmd {
		case 1, 2:
			if i+2*count > len(g) {
				return nil, errDamaged
			}
			for j := 0; j < count; j++ {
				x += zigzag(g[i])
				y += zigzag(g[i+1])
				i += 2
				p := pt{float64(x), float64(y)}
				if cmd == 1 || len(out) == 0 || kind == 1 {
					out = append(out, []pt{p})
				} else {
					out[len(out)-1] = append(out[len(out)-1], p)
				}
			}
		case 7:
			if n := len(out); n > 0 {
				out[n-1] = append(out[n-1], out[n-1][0])
			}
		default:
			return nil, errDamaged
		}
	}
	return out, nil
}
