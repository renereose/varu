// Command varu-index builds the search index of a ZIM archive: the file the
// library would otherwise build on each device the first time it opens the
// book. The release builds them, so phones never have to.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/renereose/varu/zim"
)

func main() {
	in := flag.String("zim", "", "ZIM archive")
	out := flag.String("out", "", "index to write (default: the archive's name with .search)")
	flag.Parse()
	if *in == "" {
		flag.Usage()
		os.Exit(2)
	}
	if *out == "" {
		*out = strings.TrimSuffix(*in, ".zim") + ".search"
	}
	a, err := zim.Open(*in)
	if err != nil {
		log.Fatal(err)
	}
	defer a.Close()
	last := -1
	err = zim.BuildIndex(a, *out, func(done, total int) {
		if pct := done * 100 / max(total, 1); pct/10 != last/10 {
			last = pct
			fmt.Fprintf(os.Stderr, "%s: %d%%\n", *in, pct)
		}
	})
	if err != nil {
		log.Fatal(err)
	}
}
