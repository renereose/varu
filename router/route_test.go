package router

import (
	"errors"
	"math"
	"math/rand"
	"testing"
)

func pt(p [2]float64) Point { return Point{p[0], p[1]} }

// same: within a centimetre (positions are stored to 1e-7 degrees).
func same(p, q Point) bool { return distM(p.Lat, p.Lon, q.Lat, q.Lon) < 0.01 }

func build(t *testing.T, ways []Way, nodes map[int64][2]float64) *Graph {
	t.Helper()
	g, err := Build(ways, nodes, Meta{})
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func tagged(id int64, tags map[string]string, nodes ...int64) Way {
	return Way{ID: id, Nodes: nodes, Tags: tags}
}

func TestRoute(t *testing.T) {
	// A short slow street from A (1) to B (4), and a longer fast road round it
	// through C (2) and D (3). An island (5-6) reached only by ferry (6-7)
	// from a quay (7) on the fast road, and an islet (8-9) not reached at all.
	nodes := map[int64][2]float64{1: near(0, 0), 2: near(2, 0), 3: near(2, 10), 4: near(0, 10),
		5: near(-20, 0), 6: near(-15, 0), 7: near(2, 5), 8: near(-40, 0), 9: near(-41, 0)}
	short := map[string]string{"highway": "residential"}
	ways := func(shortTags map[string]string) []Way {
		return []Way{
			tagged(1, shortTags, 1, 4),
			road(2, "primary", 1, 2, 7, 3, 4),
			road(3, "residential", 5, 6),
			tagged(4, map[string]string{"route": "ferry", "duration": "00:10"}, 7, 6),
			road(5, "residential", 8, 9),
		}
	}
	a, b := pt(nodes[1]), pt(nodes[4])
	direct := distM(58, 26, 58, 26.010)
	around := distM(58, 26, 58.002, 26)*2 + direct

	g := build(t, ways(short), nodes)
	car, err := g.Route(a, b, Car)
	if err != nil || math.Abs(car.DistanceM-around) > 1 || math.Abs(car.DurationS-around/(75/3.6)) > 0.5 {
		t.Errorf("car took %.0f m in %.1f s (%v); want the fast road, %.0f m", car.DistanceM, car.DurationS, err, around)
	}
	foot, err := g.Route(a, b, Foot)
	if err != nil || math.Abs(foot.DistanceM-direct) > 1 || math.Abs(foot.DurationS-direct/(5/3.6)) > 0.5 {
		t.Errorf("walk took %.0f m (%v); want the short street, %.0f m", foot.DistanceM, err, direct)
	}
	if !same(foot.Line[0], a) || !same(foot.Line[len(foot.Line)-1], b) {
		t.Errorf("line runs %v to %v", foot.Line[0], foot.Line[len(foot.Line)-1])
	}

	// A one-way short street (B to A only) sends cars round, not walkers.
	oneway := build(t, ways(map[string]string{"highway": "residential", "oneway": "-1"}), nodes)
	if r, _ := oneway.Route(a, b, Car); math.Abs(r.DistanceM-around) > 1 {
		t.Errorf("one way: car took %.0f m, want %.0f", r.DistanceM, around)
	}
	if r, _ := oneway.Route(b, a, Car); math.Abs(r.DistanceM-direct) > 1 {
		t.Errorf("one way, right direction: car took %.0f m, want %.0f", r.DistanceM, direct)
	}
	if r, _ := oneway.Route(a, b, Foot); math.Abs(r.DistanceM-direct) > 1 {
		t.Errorf("one way: walk took %.0f m, want %.0f", r.DistanceM, direct)
	}

	// No cars on the short street: cars go round.
	private := build(t, ways(map[string]string{"highway": "residential", "motor_vehicle": "private"}), nodes)
	if r, _ := private.Route(a, b, Car); math.Abs(r.DistanceM-around) > 1 {
		t.Errorf("private: car took %.0f m, want %.0f", r.DistanceM, around)
	}

	// The island by ferry: flagged, with the boarding allowance in the time.
	island, err := g.Route(a, pt(nodes[5]), Car)
	if err != nil || !island.Ferry || island.DurationS < ferryWaitS {
		t.Errorf("island: ferry %v, %.0f s, %v", island.Ferry, island.DurationS, err)
	}
	// The islet: not connected.
	if _, err := g.Route(a, pt(nodes[8]), Foot); !errors.Is(err, ErrNotConnected) {
		t.Errorf("islet: %v, want not connected", err)
	}

	// Along one segment, both ways, and against a one-way street.
	quarter, three := pt(near(0, 2.5)), pt(near(0, 7.5))
	if r, _ := g.Route(quarter, three, Car); math.Abs(r.DistanceM-direct/2) > 1 {
		t.Errorf("same segment: %.0f m, want %.0f", r.DistanceM, direct/2)
	}
	if r, _ := g.Route(three, quarter, Car); math.Abs(r.DistanceM-direct/2) > 1 {
		t.Errorf("same segment backwards: %.0f m, want %.0f", r.DistanceM, direct/2)
	}
	if r, _ := oneway.Route(quarter, three, Car); r.DistanceM < around {
		t.Errorf("against the one-way: %.0f m, want the way round", r.DistanceM)
	}
	if r, err := g.Route(a, a, Foot); err != nil || r.DistanceM != 0 {
		t.Errorf("from A to A: %.0f m, %v", r.DistanceM, err)
	}
	if _, err := g.Route(pt(near(200, 0)), b, Foot); !errors.Is(err, ErrStartTooFar) {
		t.Errorf("22 km away: %v, want start too far", err)
	}
}

// gridTown is an n×n street grid with random road classes, 111 m × 117 m blocks.
func gridTown(n int, rnd *rand.Rand) ([]Way, map[int64][2]float64) {
	kinds := []string{"residential", "tertiary", "secondary", "primary", "track"}
	nodes := map[int64][2]float64{}
	id := func(r, c int) int64 { return int64(r*n + c + 1) }
	var ways []Way
	for r := 0; r < n; r++ {
		for c := 0; c < n; c++ {
			nodes[id(r, c)] = near(float64(r), float64(c)*2)
			if c+1 < n {
				ways = append(ways, road(int64(len(ways)+1), kinds[rnd.Intn(len(kinds))], id(r, c), id(r, c+1)))
			}
			if r+1 < n {
				ways = append(ways, road(int64(len(ways)+1), kinds[rnd.Intn(len(kinds))], id(r, c), id(r+1, c)))
			}
		}
	}
	return ways, nodes
}

func TestAStarMatchesDijkstra(t *testing.T) {
	rnd := rand.New(rand.NewSource(1))
	ways, nodes := gridTown(30, rnd)
	g := build(t, ways, nodes)
	for i := 0; i < 50; i++ {
		from, to := pt(nodes[int64(rnd.Intn(900)+1)]), pt(nodes[int64(rnd.Intn(900)+1)])
		astar, err := g.Route(from, to, Car)
		if err != nil {
			t.Fatal(err)
		}
		vmax := g.vmax[Car]
		g.vmax[Car] = math.Inf(1) // no estimate: plain Dijkstra
		dijkstra, err := g.Route(from, to, Car)
		g.vmax[Car] = vmax
		if err != nil {
			t.Fatal(err)
		}
		if math.Abs(astar.DurationS-dijkstra.DurationS) > 1e-3*dijkstra.DurationS+0.01 {
			t.Fatalf("pair %d: A* %.3f s, Dijkstra %.3f s", i, astar.DurationS, dijkstra.DurationS)
		}
	}
}

func TestSnap(t *testing.T) {
	ways, nodes := gridTown(23, rand.New(rand.NewSource(2))) // 529 junctions: a main network
	// A lone car park 200 m west of the grid, a footway closer still, an islet
	// 3 km away and a ferry line passing the query point.
	nodes[9001], nodes[9002] = near(5, -3), near(6, -3)
	nodes[9003], nodes[9004] = near(5, -1), near(6, -1)
	nodes[9005], nodes[9006] = near(-30, 0), near(-30, 1)
	nodes[9007], nodes[9008] = near(5.5, -5), near(5.5, -10)
	ways = append(ways,
		road(9001, "service", 9001, 9002),
		road(9002, "footway", 9003, 9004),
		road(9003, "residential", 9005, 9006),
		tagged(9004, map[string]string{"route": "ferry"}, 9007, 9008),
	)
	g := build(t, ways, nodes)
	seg := func(r snapResult) (Point, Point) { return g.vpt(g.sfrom[r.seg]), g.vpt(g.sto[r.seg]) }

	p := pt(near(5.5, -4)) // next to the car park and on the ferry line
	r, ok := g.snap(p, Car)
	if a, _ := seg(r); !ok || a.Lon < 25.99999 {
		t.Errorf("car snapped to %v; want the main grid, not the car park or the ferry", a)
	}
	r, ok = g.snap(p, Foot)
	if a, _ := seg(r); !ok || a.Lon < 25.99999 {
		t.Errorf("foot snapped to %v; want the main grid (the footway is a fragment)", a)
	}
	r, ok = g.snap(pt(near(-29, 0.5)), Car)
	if a, _ := seg(r); !ok || !same(a, pt(nodes[9005])) || r.off > 120 {
		t.Errorf("islet snapped to %v (%.0f m); want the islet's own road", a, r.off)
	}
	if _, ok := g.snap(pt(near(-80, 0)), Car); ok {
		t.Error("8.9 km away snapped")
	}
	r, _ = g.snap(pt(near(0.5, 0)), Car) // half-way along the first street north
	if math.Abs(r.along-distM(58, 26, 58.0005, 26)) > 0.5 || r.off > 0.5 {
		t.Errorf("mid-segment: along %.1f m, off %.1f m", r.along, r.off)
	}
}
