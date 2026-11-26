package router

import (
	"math"
	"unicode"
)

// Step is one instruction of turn-by-turn directions.
type Step struct {
	AlongM   float64 `json:"along_m"`            // metres from the start of the route
	Type     string  `json:"type"`               // depart, turn, continue, roundabout, arrive
	Modifier string  `json:"modifier,omitempty"` // left, slight right, sharp left, uturn, straight, ...
	Name     string  `json:"name,omitempty"`     // the road to take
	Exit     int     `json:"exit,omitempty"`     // roundabout exit number

	in float64 // bearing arriving at the step, for merging
}

// leg is one road segment of a route: entered at vertex v (none for the
// first), starting at line[at], along metres from the start.
type leg struct {
	seg   uint32
	v     uint32
	at    int
	along float64
}

// Turns gentler than this are "straight"; the rest grow slight, plain,
// sharp and finally a U-turn.
const (
	straightDeg = 25
	slightDeg   = 50
	turnDeg     = 135
	sharpDeg    = 165
	// bearings are measured over this much road either side of a junction
	bearingM = 15
	// turns closer together than this are one (a crossing, a slip road)
	mergeM = 25
)

// steps turns a route's legs into directions: a step where the route turns
// at a junction that offers a choice, where it moves onto a differently
// named road, and one per roundabout.
// No "keep left/right" at forks and no lane advice.
func (g *Graph) steps(legs []leg, line []Point, total float64, m Mode) []Step {
	name := func(l leg) string { return g.names[g.sname[l.seg]] }
	round := func(l leg) bool { return g.sflags[l.seg]&roundabout != 0 }
	out := []Step{{Type: "depart", Name: name(legs[0])}}
	road := name(legs[0]) // the road the directions last named
	for i := 1; i < len(legs); i++ {
		l, prev := legs[i], legs[i-1]
		if round(l) && !round(prev) {
			exit, j := 1, i+1
			for ; j < len(legs) && round(legs[j]); j++ {
				if g.choices(legs[j].v, legs[j-1].seg, m, true) > 0 {
					exit++
				}
			}
			s := Step{AlongM: l.along, Type: "roundabout", Exit: exit}
			if j < len(legs) {
				s.Name = name(legs[j])
			}
			out = append(out, s)
			road = s.Name
			i = j // leaving the roundabout needs no step of its own
			continue
		}
		if g.choices(l.v, prev.seg, m, false) < 2 {
			continue // the road only bends
		}
		in, leave := bearing(line, l.at, -1), bearing(line, l.at, 1)
		s := Step{AlongM: l.along, Type: "turn", Modifier: modifier(leave - in), Name: name(l), in: in}
		if last := &out[len(out)-1]; (last.Type == "turn" || last.Type == "continue") && s.AlongM-last.AlongM < mergeM {
			// One turn from the bearing before the first to after the last.
			s.AlongM, s.in, s.Modifier = last.AlongM, last.in, modifier(leave-last.in)
			if s.Name == "" {
				s.Name = last.Name
			}
			out = out[:len(out)-1]
		}
		if s.Modifier == "straight" {
			// Only worth a step onto a new road with a real name.
			if s.Name == "" || s.Name == road || !hasLetter(s.Name) {
				continue
			}
			s.Type = "continue"
		}
		out = append(out, s)
		road = s.Name
	}
	return append(out, Step{AlongM: total, Type: "arrive"})
}

// hasLetter tells a street name from a bare road number.
func hasLetter(s string) bool {
	for _, r := range s {
		if unicode.IsLetter(r) {
			return true
		}
	}
	return false
}

// choices counts the ways mode m may leave vertex v, other than back along
// the segment it came by. leaving counts only roads off a roundabout.
func (g *Graph) choices(v, came uint32, m Mode, leaving bool) int {
	n := 0
	for _, he := range g.adj[g.first[v]:g.first[v+1]] {
		s := he >> 1
		if s == came || !g.usable(s, he&1 == 1, m) || (leaving && g.sflags[s]&roundabout != 0) {
			continue
		}
		n++
	}
	return n
}

// bearing is the compass direction of the line near line[at]: arriving
// there (dir -1) or leaving it (dir 1), over about bearingM of road.
func bearing(line []Point, at, dir int) float64 {
	p := line[at]
	q := p
	for i := at + dir; i >= 0 && i < len(line); i += dir {
		q = line[i]
		if distM(p.Lat, p.Lon, q.Lat, q.Lon) >= bearingM {
			break
		}
	}
	if dir < 0 {
		p, q = q, p
	}
	dx := (q.Lon - p.Lon) * math.Cos(p.Lat*math.Pi/180)
	return math.Atan2(dx, q.Lat-p.Lat) * 180 / math.Pi
}

// modifier names a change of direction in degrees (positive: clockwise).
func modifier(d float64) string {
	d = math.Mod(d+540, 360) - 180
	side := "right"
	if d < 0 {
		side = "left"
	}
	switch a := math.Abs(d); {
	case a < straightDeg:
		return "straight"
	case a < slightDeg:
		return "slight " + side
	case a < turnDeg:
		return side
	case a < sharpDeg:
		return "sharp " + side
	}
	return "uturn"
}
