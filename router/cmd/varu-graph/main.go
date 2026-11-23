// varu-graph turns an OpenStreetMap extract into Varu's routing file:
//
//	go run ./cmd/varu-graph -in estonia.osm.pbf -out ee.graph -source URL
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/renereose/varu/router"
)

func main() {
	in := flag.String("in", "", "OpenStreetMap extract (.osm.pbf)")
	out := flag.String("out", "", "routing file to write")
	source := flag.String("source", "", "where the extract came from (recorded in the file)")
	flag.Parse()
	log.SetFlags(0)
	if *in == "" || *out == "" {
		log.Fatal("usage: varu-graph -in extract.osm.pbf -out region.graph [-source URL]")
	}

	start := time.Now()
	g, err := router.BuildPBF(*in, router.Meta{Source: *source})
	if err != nil {
		log.Fatalf("varu-graph: %v", err)
	}
	f, err := os.Create(*out)
	if err != nil {
		log.Fatalf("varu-graph: %v", err)
	}
	n, err := g.WriteTo(f)
	if err == nil {
		err = f.Close()
	}
	if err != nil {
		os.Remove(*out)
		log.Fatalf("varu-graph: %v", err)
	}
	v, s, p := g.Stats()
	fmt.Printf("%d junctions, %d segments, %d shape points, %.0f MB, %.1f s\n",
		v, s, p, float64(n)/1e6, time.Since(start).Seconds())
}
