package router

import (
	"bytes"
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestWayRules(t *testing.T) {
	const both, fwd, bwd = carFwd | carBwd | footOK, carFwd | footOK, carBwd | footOK
	for _, c := range []struct {
		tags     map[string]string
		ok       bool
		kmh      uint8
		flags    uint8
		ferrySec uint32
	}{
		{map[string]string{"highway": "residential"}, true, 25, both, 0},
		{map[string]string{"highway": "motorway"}, true, 100, carFwd, 0},
		{map[string]string{"highway": "primary", "maxspeed": "90"}, true, 90, both, 0},
		{map[string]string{"highway": "residential", "maxspeed": "EE:urban"}, true, 50, both, 0},
		{map[string]string{"highway": "primary", "maxspeed": "60 mph"}, true, 75, both, 0},
		{map[string]string{"highway": "residential", "oneway": "-1"}, true, 25, bwd, 0},
		{map[string]string{"highway": "tertiary", "junction": "roundabout"}, true, 55, fwd | roundabout, 0},
		{map[string]string{"highway": "motorway", "oneway": "no"}, true, 100, carFwd | carBwd, 0},
		{map[string]string{"highway": "service", "oneway": "reversible"}, true, 0, footOK, 0},
		{map[string]string{"highway": "footway"}, true, 0, footOK, 0},
		{map[string]string{"highway": "footway", "access": "no"}, false, 0, 0, 0},
		{map[string]string{"highway": "service", "access": "private"}, false, 0, 0, 0},
		{map[string]string{"highway": "service", "access": "private", "foot": "yes"}, true, 0, footOK, 0},
		{map[string]string{"highway": "track"}, true, 15, both, 0},
		{map[string]string{"highway": "track", "motor_vehicle": "forestry"}, true, 0, footOK, 0},
		{map[string]string{"highway": "residential", "access": "no", "motor_vehicle": "yes"}, true, 25, carFwd | carBwd, 0},
		{map[string]string{"route": "ferry", "duration": "01:30"}, true, 0, ferry | both, 5400},
		{map[string]string{"route": "ferry", "duration": "25"}, true, 0, ferry | both, 1500},
		{map[string]string{"route": "ferry", "motor_vehicle": "no"}, true, 0, ferry | footOK, 0},
		{map[string]string{"highway": "construction"}, false, 0, 0, 0},
		{map[string]string{"building": "yes"}, false, 0, 0, 0},
	} {
		r, ok := wayRule(func(k string) string { return c.tags[k] })
		if ok != c.ok || (ok && (r.kmh != c.kmh || r.flags != c.flags || r.ferrySec != c.ferrySec)) {
			t.Errorf("%v: got ok=%v kmh=%d flags=%04b ferry=%d; want ok=%v kmh=%d flags=%04b ferry=%d",
				c.tags, ok, r.kmh, r.flags, r.ferrySec, c.ok, c.kmh, c.flags, c.ferrySec)
		}
	}
}

// Positions near Tartu; dy and dx in thousandths of a degree.
func near(dy, dx float64) [2]float64 { return [2]float64{58 + dy/1000, 26 + dx/1000} }

func road(id int64, kind string, nodes ...int64) Way {
	return Way{ID: id, Nodes: nodes, Tags: map[string]string{"highway": kind}}
}

func TestBuildGraph(t *testing.T) {
	nodes := map[int64][2]float64{1: near(0, 0), 2: near(1, 0), 3: near(2, 0), 4: near(3, 0), 5: near(2, 2),
		6: near(5, 0), 8: near(5, 2), 9: near(5, 4)} // 7 is missing from the extract
	named := road(1, "residential", 1, 2, 3, 4)
	named.Tags["name"] = "Rüütli"
	numbered := road(2, "residential", 3, 5)
	numbered.Tags["ref"] = "92"
	g, err := Build([]Way{
		named,
		numbered,
		road(3, "residential", 6, 7, 8, 9),
		road(4, "building_passage", 1, 5), // not routable: ignored
	}, nodes, Meta{Source: "test"})
	if err != nil {
		t.Fatal(err)
	}
	v, s, p := g.Stats()
	// Vertices 1, 3, 4, 5 and the piece after the gap: 8, 9. Node 2 is a shape point.
	if v != 6 || s != 4 || p != 1 {
		t.Fatalf("stats %d/%d/%d, want 6 vertices, 4 segments, 1 shape point", v, s, p)
	}
	want := distM(58, 26, 58.002, 26) // node 1 to node 3 through node 2
	if math.Abs(float64(g.slen[0])-want) > 0.01 || g.npts(0) != 3 {
		t.Errorf("first segment %.2f m with %d points, want %.2f m with 3", g.slen[0], g.npts(0), want)
	}

	path := filepath.Join(t.TempDir(), "g.graph")
	var buf bytes.Buffer
	if _, err := g.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(path, buf.Bytes(), 0o644)
	back, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(back.arrays(), g.arrays()) || back.Meta() != g.Meta() || !reflect.DeepEqual(back.names, g.names) {
		t.Error("graph changed on the way through a file")
	}
	if got := []string{back.names[back.sname[0]], back.names[back.sname[2]], back.names[back.sname[3]]}; !reflect.DeepEqual(got, []string{"Rüütli", "92", ""}) {
		t.Errorf("names %q, want Rüütli, 92 and none", got)
	}

	// A format 1 file (no names) still loads.
	head := binary.Size(fileHead{})
	meta := int(binary.LittleEndian.Uint32(buf.Bytes()[head-4:]))
	names := int(binary.LittleEndian.Uint32(buf.Bytes()[head+meta:]))
	v1 := append([]byte{}, buf.Bytes()[:head+meta]...)
	binary.LittleEndian.PutUint32(v1[8:], 1)
	v1 = append(v1, buf.Bytes()[head+meta+4+names:buf.Len()-4*s]...)
	os.WriteFile(path, v1, 0o644)
	if old, err := Load(path); err != nil || !reflect.DeepEqual(old.arrays()[:10], g.arrays()[:10]) || old.names[old.sname[0]] != "" {
		t.Errorf("format 1 file: %v", err)
	}

	bad := map[string][]byte{
		"magic":     append([]byte("NOTAVARU"), buf.Bytes()[8:]...),
		"truncated": buf.Bytes()[:buf.Len()-1],
		"version":   binary.LittleEndian.AppendUint32(append([]byte{}, buf.Bytes()[:8]...), 3),
	}
	bad["version"] = append(bad["version"], buf.Bytes()[12:]...)
	for name, b := range bad {
		os.WriteFile(path, b, 0o644)
		if _, err := Load(path); err == nil {
			t.Errorf("%s: loaded", name)
		}
	}
}
