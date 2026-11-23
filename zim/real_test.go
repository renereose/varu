package zim

import (
	"io"
	"os"
	"path/filepath"
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
