package main

// How the map looks: the only file to edit when tuning. Sizes are in panel
// pixels; zoom tables start at zoom 10 and the last value holds for every
// zoom above.

import "slices"

// rule draws one kind of feature from one tile layer.
type rule struct {
	layer   string
	kinds   []string // the feature's "kind"; empty for any
	minZoom int
	area    [2]pattern // fill for 1-bit and 4-grey panels; zero for none
	widths  []float64  // line width from zoom 10 up; 0 hides the line
	cased   int        // from this zoom the line is a white road with black edges
	inner   bool       // draws the white middle of a cased road
	dash    [2]float64 // on and off px
	grey    uint8      // line colour on a 4-grey panel (1-bit is always black)
	dotted  bool       // on a 1-bit panel broken into short dashes, to tell it from a road
	white   bool       // a white line, such as the dashes inside a railway
	test    func(f feature) bool
}

func (r rule) matches(f feature, z int) bool {
	return z >= r.minZoom && (len(r.kinds) == 0 || slices.Contains(r.kinds, f.str("kind"))) && (r.test == nil || r.test(f))
}

func (r rule) width(z int) float64 {
	if len(r.widths) == 0 {
		return 0
	}
	return r.widths[min(max(z-10, 0), len(r.widths)-1)]
}

var (
	waterArea    = [2]pattern{{"####", "oooo", "oooo", "oooo"}, {"llll", "llll", "llll", "llll"}}
	forestArea   = [2]pattern{{"#...", "....", "....", "...."}, {"l...", "....", "..l.", "...."}}
	buildingArea = [2]pattern{{"#ooo", "o#oo", "oo#o", "ooo#"}, {"dddd", "dddd", "dddd", "dddd"}}
)

func kindDetail(level float64) func(f feature) bool {
	return func(f feature) bool { return f.num("kind_detail") <= level }
}

func isArea(f feature) bool { return f.kind == 3 }
func isLine(f feature) bool { return f.kind == 2 }

// Pedestrian streets are paths in the tiles but streets on the ground.
func isStreet(f feature) bool {
	k := f.str("kind")
	return k == "minor_road" || k == "other" || k == "path" && f.str("kind_detail") == "pedestrian"
}

func isPath(f feature) bool { return f.str("kind") == "path" && !isStreet(f) }

var (
	highway = []float64{2, 3, 4, 5, 7, 9}
	major   = []float64{1, 1.5, 3, 4, 6, 8}
	minor   = []float64{0, 0, 1, 1.5, 4, 6}
)

// Drawn in this order, each over the ones before.
var rules = []rule{
	{layer: "landuse", kinds: []string{"forest", "wood"}, area: forestArea, minZoom: 10},
	{layer: "water", area: waterArea, test: isArea},
	{layer: "water", kinds: []string{"river", "canal"}, widths: []float64{1, 1, 2, 2, 3, 3}, grey: dark, dotted: true, test: isLine},
	{layer: "water", kinds: []string{"stream", "ditch", "drain"}, widths: []float64{1}, minZoom: 13, grey: dark, dotted: true, test: isLine},
	{layer: "buildings", area: buildingArea, minZoom: 14},
	{layer: "boundaries", widths: []float64{2}, dash: [2]float64{8, 4}, test: kindDetail(2)},

	{layer: "roads", widths: []float64{1}, dash: [2]float64{3, 2}, minZoom: 13, grey: dark, test: isPath},
	{layer: "roads", kinds: []string{"rail"}, widths: []float64{1, 1, 1.5, 2, 3}, test: notSiding},
	{layer: "roads", kinds: []string{"rail"}, widths: []float64{0, 0, 0, 0, 1}, dash: [2]float64{4, 4}, white: true, test: notSiding},
	{layer: "roads", widths: minor, cased: 14, test: isStreet},
	{layer: "roads", widths: minor, cased: 14, inner: true, test: isStreet},
	{layer: "roads", kinds: []string{"major_road"}, widths: major, cased: 12},
	{layer: "roads", kinds: []string{"major_road"}, widths: major, cased: 12, inner: true},
	{layer: "roads", kinds: []string{"highway"}, widths: highway, cased: 12},
	{layer: "roads", kinds: []string{"highway"}, widths: highway, cased: 12, inner: true},
}

func notSiding(f feature) bool { return f.str("service") == "" }

// Place names: size by kind, hidden below the zoom the tiles give. A
// small screen shows villages a zoom sooner, or the countryside has no
// names at all at zoom 10.
func placeSize(f feature, z int) float64 {
	size, sooner := 11.0, 0.0 // hamlets, quarters and neighbourhoods
	switch f.props["kind_detail"] {
	case "city":
		size = 18
	case "town":
		size = 15
	case "village":
		size, sooner = 12, 1
	default:
		if z < 13 {
			return 0
		}
	}
	if f.num("min_zoom") > float64(z)+sooner {
		return 0
	}
	return size
}

// Street names: 11 px on major roads from zoom 13 and on the rest from 14.
func streetSize(f feature, z int) float64 {
	switch f.str("kind") {
	case "highway", "major_road":
		if z >= 13 {
			return 11
		}
	case "minor_road", "path":
		if z >= 14 && isStreet(f) {
			return 11
		}
	}
	return 0
}

// symbol is how one place layer is drawn, in priority order: when two
// overlap, the one earlier in the list stays.
type symbol struct {
	id      string
	minZoom int
	art     stamp
}

var symbols = []symbol{
	{"hospital", 8, art(
		"###############",
		"###############",
		"###############",
		"######ooo######",
		"######ooo######",
		"######ooo######",
		"###ooooooooo###",
		"###ooooooooo###",
		"###ooooooooo###",
		"######ooo######",
		"######ooo######",
		"######ooo######",
		"###############",
		"###############",
		"###############")},
	{"shelter", 11, fallbackArt},
	{"water", 10, art(
		".......#.......",
		".......#.......",
		"......###......",
		"......###......",
		".....#####.....",
		".....#####.....",
		"....#######....",
		"...#########...",
		"..###########..",
		"..###########..",
		"..###########..",
		"..###########..",
		"...#########...",
		"....#######....",
		"......###......")},
	{"border", 8, art(
		".......#.......",
		"......###......",
		".....#####.....",
		"....#######....",
		"...#########...",
		"..###########..",
		".#############.",
		"###############",
		".#############.",
		"..###########..",
		"...#########...",
		"....#######....",
		".....#####.....",
		"......###......",
		".......#.......")},
	{"rescue", 10, art(
		"......#........",
		"......##.......",
		".....###.......",
		".....####......",
		"....#####...#..",
		"....######..##.",
		"...#######..##.",
		"...###########.",
		"..############.",
		"..######o#####.",
		"..#####ooo####.",
		"..####ooooo###.",
		"...###ooooo##..",
		"....##ooooo#...",
		".....#######...")},
	{"police", 10, art(
		"###############",
		"###############",
		"###############",
		"###############",
		"###############",
		"###############",
		".#############.",
		".#############.",
		"..###########..",
		"..###########..",
		"...#########...",
		"....#######....",
		".....#####.....",
		"......###......",
		".......#.......")},
	{"spring", 11, art(
		".......#.......",
		".......#.......",
		"......###......",
		"......#o#......",
		".....##o##.....",
		".....#ooo#.....",
		"....##ooo##....",
		"...##ooooo##...",
		"..##ooooooo##..",
		"..#ooooooooo#..",
		"..#ooooooooo#..",
		"..##ooooooo##..",
		"...##ooooo##...",
		"....#######....",
		"......###......")},
	{"clinic", 12, art(
		"###############",
		"#ooooooooooooo#",
		"#ooooooooooooo#",
		"#ooooo###ooooo#",
		"#ooooo###ooooo#",
		"#ooooo###ooooo#",
		"#oo#########oo#",
		"#oo#########oo#",
		"#oo#########oo#",
		"#ooooo###ooooo#",
		"#ooooo###ooooo#",
		"#ooooo###ooooo#",
		"#ooooooooooooo#",
		"#ooooooooooooo#",
		"###############")},
	{"pharmacy", 12, art(
		".....#####.....",
		"...##ooooo##...",
		"..#ooooooooo#..",
		".#oooo###oooo#.",
		".#oooo###oooo#.",
		"#ooooo###ooooo#",
		"#oo#########oo#",
		"#oo#########oo#",
		"#oo#########oo#",
		"#ooooo###ooooo#",
		".#oooo###oooo#.",
		".#oooo###oooo#.",
		"..#ooooooooo#..",
		"...##ooooo##...",
		".....#####.....")},
	{"fuel", 12, art(
		".########......",
		".#oooooo#......",
		".#oooooo#.##...",
		".#oooooo#..#...",
		".########..#...",
		".########..#...",
		".########..#...",
		".#########.#...",
		".########.##...",
		".########.#....",
		".########......",
		".########......",
		".########......",
		"###########....",
		"###########....")},
	{"food", 13, art(
		"....#.....#....",
		"...#.......#...",
		"..#.........#..",
		".#...........#.",
		"###############",
		"###############",
		".#############.",
		".##o#o#o#o#o##.",
		".#############.",
		"..###########..",
		"..##o#o#o#o##..",
		"..###########..",
		"...#########...",
		"...#########...",
		"...............")},
}

// fallbackArt marks shelters and any place layer without a symbol of its own.
var fallbackArt = art(
	".....#####.....",
	"...#########...",
	"..###ooooo###..",
	".##ooooooooo##.",
	".##ooooooooo##.",
	"##oooo###oooo##",
	"##ooo#####ooo##",
	"##ooo#####ooo##",
	"##ooo#####ooo##",
	"##oooo###oooo##",
	".##ooooooooo##.",
	".##ooooooooo##.",
	"..###ooooo###..",
	"...#########...",
	".....#####.....")

var homeArt = art(
	".........#.........",
	"........###........",
	".......#####.......",
	"......#######......",
	".....#########.....",
	"....###########....",
	"...#############...",
	"..###############..",
	".#################.",
	"###################",
	"..#ooooooooooooo#..",
	"..#ooooooooooooo#..",
	"..#ooooo###ooooo#..",
	"..#ooooo###ooooo#..",
	"..#ooooo###ooooo#..",
	"..#ooooo###ooooo#..",
	"..#ooooo###ooooo#..",
	"..#ooooo###ooooo#..",
	"..###############..")
