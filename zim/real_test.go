package zim

import (
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// TestRealArchives reads real books. Set VARU_ZIM to a .zim file or a
// folder of them (a drive's library/). Cross-compile it to try a phone or
// a Pi (GOOS=linux GOARCH=arm64 go test -c).
func TestRealArchives(t *testing.T) {
	dir := os.Getenv("VARU_ZIM")
	if dir == "" {
		t.Skip("set VARU_ZIM to check real archives")
	}
	files, _ := filepath.Glob(filepath.Join(dir, "*.zim"))
	if strings.HasSuffix(dir, ".zim") {
		files = []string{dir}
	}
	for _, file := range files {
		start := time.Now()
		a, err := Open(file)
		if err != nil {
			t.Fatal(err)
		}
		opened := time.Since(start)
		main, err := a.MainPage()
		if err == nil {
			main, err = a.Resolve(main)
		}
		if err != nil {
			t.Fatalf("%s main page: %v", file, err)
		}
		start = time.Now()
		r, err := a.Content(main)
		if err != nil {
			t.Fatalf("%s main page content: %v", file, err)
		}
		body, _ := io.ReadAll(r)
		first := time.Since(start)
		prefix := strings.Fields(strings.TrimSpace(main.Title) + " a")[0][:1]
		start = time.Now()
		found, err := a.Titles(prefix, 5)
		if err != nil || len(found) == 0 {
			t.Errorf("%s: no titles for %q (%v)", file, prefix, err)
		}
		var titles []string
		for _, e := range found {
			titles = append(titles, e.Title)
		}
		t.Logf("%s: %s, %d entries; opened in %v; main page %q (%s, %d bytes) in %v; %q → %v in %v",
			filepath.Base(file), a.Metadata("Title"), a.entries, opened, main.Path, a.MIME(main), len(body),
			first, prefix, titles, time.Since(start))
		a.Close()
	}
}

// TestRealSearch builds search indexes for real books and times a few
// searches. Set VARU_ZIM as above and VARU_INDEX to a folder for the indexes.
func TestRealSearch(t *testing.T) {
	dir, out := os.Getenv("VARU_ZIM"), os.Getenv("VARU_INDEX")
	if dir == "" || out == "" {
		t.Skip("set VARU_ZIM and VARU_INDEX to build real search indexes")
	}
	files, _ := filepath.Glob(filepath.Join(dir, "*.zim"))
	for _, file := range files {
		a, err := Open(file)
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(out, strings.TrimSuffix(filepath.Base(file), ".zim")+".search")
		start := time.Now()
		if err := BuildIndex(a, path, nil); err != nil {
			t.Fatal(err)
		}
		built := time.Since(start)
		var ms runtime.MemStats
		runtime.ReadMemStats(&ms)
		info, _ := os.Stat(path)
		x, err := OpenIndex(path, a)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("%s: %d articles indexed in %v, %d MB, peak heap %d MB", filepath.Base(file), x.docs, built.Round(time.Second), info.Size()>>20, ms.HeapSys>>20)
		for _, q := range []string{"malaria treatment", "dehydration", "bleeding first aid", "tallinn", "vesi", "esmaabi", "joogivesi keetmine", "tartu ülikool"} {
			start := time.Now()
			hits, err := x.Search(q, 10)
			if err != nil {
				t.Fatal(err)
			}
			var top []string
			for _, h := range hits[:min(3, len(hits))] {
				top = append(top, h.Entry.Title)
			}
			t.Logf("  %q: %d hits in %v: %v", q, len(hits), time.Since(start).Round(time.Millisecond), top)
		}
		x.Close()
		a.Close()
	}
}
