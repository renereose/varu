package router

import (
	"fmt"
	"strings"
	"testing"
)

func at(dy, dx float64) Point { return pt(near(dy, dx)) }

func named(id int64, name string, tags map[string]string, nodes ...int64) Way {
	w := Way{ID: id, Nodes: nodes, Tags: map[string]string{"highway": "residential", "name": name}}
	for k, v := range tags {
		w.Tags[k] = v
	}
	return w
}

// describe writes steps compactly: "depart Pikk | turn right Lai | arrive".
func describe(steps []Step) string {
	var parts []string
	for _, s := range steps {
		p := s.Type
		if s.Modifier != "" {
			p += " " + s.Modifier
		}
		if s.Exit > 0 {
			p += fmt.Sprintf(" exit %d", s.Exit)
		}
		if s.Name != "" {
			p += " " + s.Name
		}
		parts = append(parts, p)
	}
	return strings.Join(parts, " | ")
}

func TestSteps(t *testing.T) {
	// A crossroads of Pikk (south-north) and Lai (west-east) at 2; a
	// roundabout further north, entered from Pikk at 10, with arms east (11),
	// north (12) and west (13). Traffic goes round anticlockwise.
	nodes := map[int64][2]float64{
		1: near(-4, 0), 2: near(0, 0), 3: near(0, 4), 4: near(0, -4), 5: near(4, 0),
		10: near(6, 0), 11: near(7, 1), 12: near(8, 0), 13: near(7, -1),
		21: near(7, 5), 22: near(12, 0), 23: near(7, -5),
	}
	ring := map[string]string{"junction": "roundabout"}
	g, err := Build([]Way{
		named(1, "Pikk", nil, 1, 2, 5, 10),
		named(2, "Lai", nil, 4, 2, 3),
		named(3, "Ring", ring, 10, 11, 12, 13, 10),
		named(4, "Ida", nil, 11, 21),
		named(5, "Põhja", nil, 12, 22),
		named(6, "Lääne", nil, 13, 23),
	}, nodes, Meta{Source: "test"})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		to   Point
		mode Mode
		want string
	}{
		{at(0, 3), Car, "depart Pikk | turn right Lai | arrive"},
		{at(0, -3), Car, "depart Pikk | turn left Lai | arrive"},
		{at(3, 0), Car, "depart Pikk | arrive"},
		{at(7, 4), Car, "depart Pikk | roundabout exit 1 Ida | arrive"},
		{at(11, 0), Car, "depart Pikk | roundabout exit 2 Põhja | arrive"},
		{at(7, -4), Car, "depart Pikk | roundabout exit 3 Lääne | arrive"},
	} {
		r, err := g.Route(at(-3, 0), c.to, c.mode)
		if err != nil {
			t.Fatalf("to %v: %v", c.to, err)
		}
		if got := describe(r.Steps); got != c.want {
			t.Errorf("to %v: got %q, want %q", c.to, got, c.want)
		}
		if last := r.Steps[len(r.Steps)-1]; last.AlongM != r.DistanceM {
			t.Errorf("to %v: arrives at %.0f m of %.0f", c.to, last.AlongM, r.DistanceM)
		}
	}
}

func TestModifier(t *testing.T) {
	for d, want := range map[float64]string{
		0: "straight", 20: "straight", -40: "slight left", 90: "right", -90: "left",
		150: "sharp right", 178: "uturn", 350: "straight", 270: "left",
	} {
		if got := modifier(d); got != want {
			t.Errorf("modifier(%v) = %q, want %q", d, got, want)
		}
	}
}
