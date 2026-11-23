package zim

import (
	"io"
	"path/filepath"
	"testing"

	"github.com/renereose/varu/zim/zimtest"
)

func TestArchive(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.zim")
	err := zimtest.Write(path, []zimtest.Entry{
		{Namespace: 'C', Path: "Tallinn", Title: "Tallinn", MIME: "text/html", Data: []byte("<p>capital</p>")},
		{Namespace: 'C', Path: "Tartu", Title: "Tartu", MIME: "text/html", Data: []byte("<p>university</p>")},
		{Namespace: 'C', Path: "Täht", Title: "Täht", MIME: "text/html", Data: []byte("<p>star</p>")},
		{Namespace: 'C', Path: "Reval", Redirect: "Tallinn"},
		{Namespace: 'C', Path: "flag.png", MIME: "image/png", Data: []byte("PNG"), Stored: true},
		{Namespace: 'M', Path: "Title", MIME: "text/plain", Data: []byte("Vikipeedia")},
	}, "Tallinn")
	if err != nil {
		t.Fatal(err)
	}
	a, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()

	read := func(e Entry) string {
		t.Helper()
		r, err := a.Content(e)
		if err != nil {
			t.Fatalf("content of %s: %v", e.Path, err)
		}
		b, _ := io.ReadAll(r)
		return string(b)
	}
	main, err := a.MainPage()
	if err != nil || main.Path != "Tallinn" || read(main) != "<p>capital</p>" {
		t.Fatalf("main page %+v %v", main, err)
	}
	if e, err := a.Find('C', "Tartu"); err != nil || read(e) != "<p>university</p>" || a.MIME(e) != "text/html" {
		t.Errorf("Tartu: %+v %v", e, err)
	}
	if e, err := a.Find('C', "flag.png"); err != nil || read(e) != "PNG" || a.MIME(e) != "image/png" {
		t.Errorf("stored image: %+v %v", e, err)
	}
	redirect, _ := a.Find('C', "Reval")
	if e, err := a.Resolve(redirect); !redirect.IsRedirect() || err != nil || e.Path != "Tallinn" {
		t.Errorf("redirect: %+v %v", e, err)
	}
	if _, err := a.Find('C', "Pärnu"); err != ErrNotFound {
		t.Errorf("missing entry: %v", err)
	}
	if got := a.Metadata("Title"); got != "Vikipeedia" {
		t.Errorf("title metadata %q", got)
	}

	titles := func(q string) (out []string) {
		found, err := a.Titles(q, 10)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range found {
			out = append(out, e.Title)
		}
		return out
	}
	if got := titles("ta"); len(got) != 2 || got[0] != "Tallinn" || got[1] != "Tartu" {
		t.Errorf(`"ta" found %v`, got)
	}
	if got := titles("TÄ"); len(got) != 1 || got[0] != "Täht" {
		t.Errorf(`"TÄ" found %v`, got)
	}
	if got := titles("Reval"); len(got) != 0 {
		t.Errorf("redirects are not articles, found %v", got)
	}
}
