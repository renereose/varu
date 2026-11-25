// varu-pocket draws e-ink map screens from a Varu drive: the start of a
// pocket device (a Raspberry Pi Zero with an e-ink panel and GPS). For now
// it writes PNG mock-ups to judge the panel size and the style.
//
//	varu-pocket -drive DIR [-center LAT,LON] [-zoom 12] [-size 400x300] [-grey] -o OUT.png
//	varu-pocket -drive DIR -legend -o LEGEND.png
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"

	"github.com/renereose/varu/pmtiles"
)

func main() {
	dir := flag.String("drive", "", "the Varu drive (its folder, with manifest.json)")
	center := flag.String("center", "", "LAT,LON of the middle of the screen (default: home, or the map's centre)")
	zoom := flag.Int("zoom", 12, "zoom, as in the Varu map (10: ~16 km across, 14: ~1 km)")
	size := flag.String("size", "400x300", "panel size in px")
	grey := flag.Bool("grey", false, "draw for a 4-grey panel instead of black and white")
	legend := flag.Bool("legend", false, "draw the key to the symbols, lines and areas instead of a map")
	out := flag.String("o", "", "the PNG to write")
	flag.Parse()
	if err := run(*dir, *center, *zoom, *size, *grey, *legend, *out); err != nil {
		fmt.Fprintln(os.Stderr, "varu-pocket:", err)
		os.Exit(1)
	}
}

func run(dir, center string, zoom int, size string, grey, legend bool, out string) error {
	if dir == "" || out == "" {
		return errors.New("needs -drive and -o")
	}
	d, lang, mid, err := openDrive(dir)
	if err != nil {
		return err
	}
	defer d.tiles.Close()
	v := view{lat: mid.y, lon: mid.x, zoom: zoom, grey: grey}
	if _, err := fmt.Sscanf(size, "%dx%d", &v.w, &v.h); err != nil || v.w < 50 || v.h < 50 || v.w > 4000 || v.h > 4000 {
		return fmt.Errorf("size %q: want WIDTHxHEIGHT, such as 400x300", size)
	}
	if center != "" {
		if _, err := fmt.Sscanf(center, "%g,%g", &v.lat, &v.lon); err != nil {
			return fmt.Errorf("center %q: want LAT,LON, such as 59.437,24.745", center)
		}
	}
	var img *image.Gray
	if legend {
		img = drawLegend(d, v, lang)
	} else if img, err = render(d, v); err != nil {
		return err
	}
	f, err := os.Create(out)
	if err != nil {
		return err
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// openDrive reads what the renderer needs from a drive: the map, the
// font, the place layers and home. It returns the drive's first language
// and the point to centre on when none is given.
func openDrive(dir string) (*drive, string, pt, error) {
	var m struct {
		Languages []string `json:"languages"`
		Map       struct {
			File    string    `json:"file"`
			Center  []float64 `json:"center"`
			MaxZoom int       `json:"maxzoom"`
		} `json:"map"`
		Pois []struct {
			ID    string            `json:"id"`
			File  string            `json:"file"`
			Label map[string]string `json:"label"`
		} `json:"pois"`
		Home *struct {
			Lon, Lat float64
		} `json:"home"`
	}
	b, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return nil, "", pt{}, fmt.Errorf("not a Varu drive: %w", err)
	}
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, "", pt{}, fmt.Errorf("manifest.json: %w", err)
	}
	lang := "en"
	if len(m.Languages) > 0 {
		lang = m.Languages[0]
	}
	tiles, err := pmtiles.Open(filepath.Join(dir, filepath.FromSlash(m.Map.File)))
	if err != nil {
		return nil, "", pt{}, fmt.Errorf("map: %w", err)
	}
	d := &drive{
		tiles:   tiles,
		maxZoom: int(tiles.Header.MaxZoom),
		font:    openFont(filepath.Join(dir, "app", "assets", "fonts", "Noto Sans Medium")),
		pois:    map[string][]pt{},
		labels:  map[string]string{},
	}
	mid := pt{25, 58.6}
	if len(m.Map.Center) == 2 {
		mid = pt{m.Map.Center[0], m.Map.Center[1]}
	}
	if m.Home != nil {
		d.home = &pt{m.Home.Lon, m.Home.Lat}
		mid = *d.home
	}
	for _, p := range m.Pois {
		var fc struct {
			Features []struct {
				Geometry struct {
					Type        string    `json:"type"`
					Coordinates []float64 `json:"coordinates"`
				} `json:"geometry"`
			} `json:"features"`
		}
		b, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(p.File)))
		if err == nil {
			err = json.Unmarshal(b, &fc)
		}
		if err != nil {
			tiles.Close()
			return nil, "", pt{}, fmt.Errorf("%s: %w", p.File, err)
		}
		points := []pt{}
		for _, f := range fc.Features {
			if c := f.Geometry.Coordinates; f.Geometry.Type == "Point" && len(c) >= 2 {
				points = append(points, pt{c[0], c[1]})
			}
		}
		d.pois[p.ID] = points
		d.order = append(d.order, p.ID)
		d.labels[p.ID] = p.Label[lang]
		if d.labels[p.ID] == "" {
			d.labels[p.ID] = p.ID
		}
	}
	return d, lang, mid, nil
}

// drawLegend draws the key: every symbol, then the lines and areas.
func drawLegend(d *drive, v view, lang string) *image.Gray {
	img := image.NewGray(image.Rect(0, 0, v.w, v.h))
	for i := range img.Pix {
		img.Pix[i] = white
	}
	mode := 0
	if v.grey {
		mode = 1
	}
	words := func(et, en string) string {
		if lang == "et" {
			return et
		}
		return en
	}
	type row struct {
		draw func(x, y int) // a 24×16 px sample with its top left at x, y
		text string
	}
	var rows []row
	add := func(s stamp, text string) {
		rows = append(rows, row{func(x, y int) { drawStamp(img, s, x+12-s.w/2, y+8-s.h/2) }, text})
	}
	add(homeArt, words("Kodu", "Home"))
	for _, s := range symbols {
		text := d.labels[s.id]
		if text == "" {
			text = s.id
		}
		add(s.art, text)
	}
	sample := func(kind int, props map[string]any, text string) {
		f := feature{kind: kind, props: props}
		rows = append(rows, row{func(x, y int) {
			geom := [][]pt{{{float64(x + 4), float64(y) + 8.5}, {float64(x + 20), float64(y) + 8.5}}}
			if kind == 3 {
				geom = [][]pt{{{float64(x), float64(y)}, {float64(x + 24), float64(y)}, {float64(x + 24), float64(y + 16)}, {float64(x), float64(y + 16)}, {float64(x), float64(y)}}}
				stroke(img, geom[0], 1, black)
			}
			for _, r := range rules {
				if r.layer == props["layer"] && r.matches(f, 14) {
					drawFeature(img, r, f, geom, 14, mode)
				}
			}
		}, text})
	}
	line := func(layer, kind, et, en string) {
		sample(2, map[string]any{"layer": layer, "kind": kind, "kind_detail": 2.0}, words(et, en))
	}
	line("roads", "highway", "Kiirtee", "Motorway")
	line("roads", "major_road", "Põhimaantee, tänav", "Main road")
	line("roads", "minor_road", "Kõrvaltee, tänav", "Minor road, street")
	line("roads", "path", "Rada", "Path")
	line("roads", "rail", "Raudtee", "Railway")
	line("water", "river", "Jõgi", "River")
	line("boundaries", "country", "Riigipiir", "Country border")
	area := func(layer, kind, et, en string) {
		sample(3, map[string]any{"layer": layer, "kind": kind}, words(et, en))
	}
	area("water", "water", "Veekogu", "Water")
	area("landuse", "forest", "Mets", "Forest")
	area("buildings", "building", "Hooned", "Buildings")

	perColumn := (v.h - 8) / 20
	for i, r := range rows {
		x, y := 6+(i/perColumn)*(v.w/2), 6+(i%perColumn)*20
		r.draw(x, y)
		drawStamp(img, d.font.text(r.text, 11, 0), x+30, y+1)
	}
	return img
}
