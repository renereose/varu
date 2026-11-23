package zim

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/renereose/varu/zim/zimtest"
)

func TestSearch(t *testing.T) {
	dir := t.TempDir()
	page := func(title, body string) []byte {
		return []byte("<html><head><title>" + title + "</title><style>p{}</style></head><body><h1>" + title + "</h1>" + body + "</body></html>")
	}
	book := filepath.Join(dir, "book.zim")
	err := zimtest.Write(book, []zimtest.Entry{
		{Namespace: 'C', Path: "Tallinn", Title: "Tallinn", MIME: "text/html", Data: page("Tallinn", "<p>Tallinn on Eesti pealinn &amp; sadam. Tallinnas elab palju inimesi.</p>")},
		{Namespace: 'C', Path: "Tartu", Title: "Tartu", MIME: "text/html", Data: page("Tartu", "<p>Tartu ülikool asub Tallinnast 180 km kaugusel.</p>")},
		{Namespace: 'C', Path: "Vesi", Title: "Vesi", MIME: "text/html", Data: page("Vesi", "<p>Joogivesi tuleb keeta.</p><script>salasõna()</script>")},
		{Namespace: 'C', Path: "Reval", Redirect: "Tallinn"},
	}, "Tallinn")
	if err != nil {
		t.Fatal(err)
	}
	a, err := Open(book)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	saved := batchSize
	batchSize = 2 // several run files, merged
	defer func() { batchSize = saved }()
	idx := filepath.Join(dir, "book.search")
	if err := BuildIndex(a, idx, nil); err != nil {
		t.Fatal(err)
	}
	x, err := OpenIndex(idx, a)
	if err != nil {
		t.Fatal(err)
	}
	defer x.Close()

	search := func(q string) (titles []string, hits []Hit) {
		t.Helper()
		hits, err := x.Search(q, 10)
		if err != nil {
			t.Fatal(err)
		}
		for _, h := range hits {
			titles = append(titles, h.Entry.Title)
		}
		return titles, hits
	}
	if got, hits := search("tallinn"); strings.Join(got, ",") != "Tallinn,Tartu" || !strings.Contains(hits[1].Snippet, "Tallinnast") {
		t.Errorf(`"tallinn" (and its forms): %v %+v`, got, hits)
	}
	if got, _ := search("Tallinn Eesti"); strings.Join(got, ",") != "Tallinn,Tartu" {
		t.Errorf("all words first, then some: %v", got)
	}
	if got, _ := search("sadam"); strings.Join(got, ",") != "Tallinn" {
		t.Errorf("entity next to a word: %v", got)
	}
	if got, _ := search("salasõna"); len(got) != 0 {
		t.Errorf("script text is not searchable, found %v", got)
	}
	if got, _ := search("ja"); len(got) != 0 {
		t.Errorf("unknown word found %v", got)
	}

	other := filepath.Join(dir, "other.zim")
	if err := zimtest.Write(other, []zimtest.Entry{{Namespace: 'C', Path: "A", MIME: "text/html", Data: []byte("x")}}, "A"); err != nil {
		t.Fatal(err)
	}
	b, _ := Open(other)
	defer b.Close()
	if _, err := OpenIndex(idx, b); !errors.Is(err, ErrStaleIndex) {
		t.Errorf("index of another book: %v", err)
	}
}
