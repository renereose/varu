package router

import (
	"os"
	"runtime"
	"testing"
	"time"
)

// TestRealEstonia checks the router on real data. Set VARU_GRAPH to a built
// routing file, or VARU_OSM_PBF to an Estonia extract to build one first.
// It also runs on a Pi as a cross-compiled test binary
// (GOOS=linux GOARCH=arm64 go test -c).
func TestRealEstonia(t *testing.T) {
	var g *Graph
	var err error
	start := time.Now()
	switch {
	case os.Getenv("VARU_GRAPH") != "":
		g, err = Load(os.Getenv("VARU_GRAPH"))
	case os.Getenv("VARU_OSM_PBF") != "":
		g, err = BuildPBF(os.Getenv("VARU_OSM_PBF"), Meta{})
	default:
		t.Skip("set VARU_GRAPH or VARU_OSM_PBF to check real Estonian data")
	}
	if err != nil {
		t.Fatal(err)
	}
	var ms runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&ms)
	v, s, p := g.Stats()
	t.Logf("%d junctions, %d segments, %d shape points; loaded in %v; heap %d MB",
		v, s, p, time.Since(start).Round(time.Millisecond), ms.HeapInuse>>20)

	tallinn := Point{59.4337, 24.7447} // Vabaduse väljak
	tartu := Point{58.3801, 26.7224}   // Raekoja plats
	kuressaare := Point{58.2481, 22.5039}

	for _, c := range []struct {
		name           string
		from, to       Point
		mode           Mode
		minKm, maxKm   float64
		minMin, maxMin float64
		ferry          bool
	}{
		{"Tallinn to Tartu by car", tallinn, tartu, Car, 175, 200, 105, 180, false},
		{"Tartu old town on foot", Point{58.3801, 26.7224}, Point{58.3733, 26.7366}, Foot, 1.0, 2.0, 10, 25, false},
		{"Kuressaare to Tallinn by car", kuressaare, tallinn, Car, 200, 260, 180, 330, true},
	} {
		t0 := time.Now()
		r, err := g.Route(c.from, c.to, c.mode)
		took := time.Since(t0)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		km, min := r.DistanceM/1000, r.DurationS/60
		t.Logf("%s: %.1f km, %.0f min, ferry %v, %d points, snapped %.0f m / %.0f m, in %v",
			c.name, km, min, r.Ferry, len(r.Line), r.SnapFromM, r.SnapToM, took.Round(time.Millisecond))
		if km < c.minKm || km > c.maxKm || min < c.minMin || min > c.maxMin || r.Ferry != c.ferry {
			t.Errorf("%s: %.1f km, %.0f min, ferry %v; want %v-%v km, %v-%v min, ferry %v",
				c.name, km, min, r.Ferry, c.minKm, c.maxKm, c.minMin, c.maxMin, c.ferry)
		}
	}
}
