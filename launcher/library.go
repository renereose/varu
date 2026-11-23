package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/url"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/renereose/varu/zim"
)

// library serves the books in library/*.zim with Varu's own reader:
//
//	/library/                 the library page (app/library.html)
//	/library/<book>/          the book's main page
//	/library/<book>/<path>    a page, image or file; pages get Varu's bar
//	/api/library              the books, for the library page
//	/api/library/search?q=    titles that start with q, then articles that
//	                          contain its words, in every book
//	/api/library/icon?book=   a book's icon
//
// A book is its file name without ".zim". Archives open on first use and
// stay open. Searching inside the text needs Varu's index of the book
// (<book>.search beside it); when prepare is set, missing ones are built in
// the background, one book at a time.
type library struct {
	root    string
	prepare bool
	once    sync.Once
	books   map[string]*book
	order   []string
}

type book struct {
	ID, Title, Description, Language string
	file                             string
	a                                *zim.Archive
	index                            atomic.Pointer[zim.Index]
	progress                         atomic.Int32 // percent while its index is built, else -1
}

func (b *book) indexPath() string { return strings.TrimSuffix(b.file, ".zim") + ".search" }

func (l *library) open() {
	l.once.Do(func() {
		l.books = map[string]*book{}
		files, _ := filepath.Glob(filepath.Join(l.root, "library", "*.zim"))
		sort.Strings(files)
		var unindexed []*book
		for _, file := range files {
			a, err := zim.Open(file)
			if err != nil {
				log.Printf("Library: %v", err)
				continue
			}
			b := &book{ID: strings.TrimSuffix(filepath.Base(file), ".zim"), file: file, a: a,
				Title: a.Metadata("Title"), Description: a.Metadata("Description"), Language: a.Metadata("Language")}
			if b.Title == "" {
				b.Title = b.ID
			}
			b.progress.Store(-1)
			if x, err := zim.OpenIndex(b.indexPath(), a); err == nil {
				b.index.Store(x)
			} else if l.prepare {
				b.progress.Store(0)
				unindexed = append(unindexed, b)
			}
			l.books[b.ID] = b
			l.order = append(l.order, b.ID)
		}
		if len(unindexed) > 0 {
			go prepareSearch(unindexed)
		}
	})
}

// prepareSearch builds the books' search indexes, one at a time.
func prepareSearch(books []*book) {
	for _, b := range books {
		start := time.Now()
		if err := buildIndex(b); err != nil {
			log.Printf("Library: cannot prepare search inside %s: %v", b.Title, err)
			b.progress.Store(-1)
			continue
		}
		log.Printf("Library: search inside %s is ready (%v)", b.Title, time.Since(start).Round(time.Second))
	}
}

func buildIndex(b *book) error {
	// Its own copy of the archive, so building does not push the pages
	// people are reading out of the shared cache.
	a, err := zim.Open(b.file)
	if err != nil {
		return err
	}
	defer a.Close()
	err = zim.BuildIndex(a, b.indexPath(), func(done, total int) {
		b.progress.Store(int32(100 * done / max(total, 1)))
	})
	if err != nil {
		return err
	}
	x, err := zim.OpenIndex(b.indexPath(), b.a)
	if err != nil {
		return err
	}
	b.index.Store(x)
	b.progress.Store(-1)
	return nil
}

// hasBooks says whether the drive has a library, without opening it.
func hasBooks(root string) bool {
	files, _ := filepath.Glob(filepath.Join(root, "library", "*.zim"))
	return len(files) > 0
}

func (l *library) register(mux *http.ServeMux) {
	mux.HandleFunc("/library", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/library/", http.StatusFound)
	})
	mux.HandleFunc("/library/", l.serve)
	mux.HandleFunc("/api/library", func(w http.ResponseWriter, r *http.Request) {
		l.open()
		type info struct {
			ID          string `json:"id"`
			Title       string `json:"title"`
			Description string `json:"description"`
			Language    string `json:"language"`
			Search      string `json:"search"` // "ready", "preparing" or "titles" (only)
			Progress    int    `json:"progress,omitempty"`
		}
		list := []info{}
		for _, id := range l.order {
			b := l.books[id]
			i := info{ID: b.ID, Title: b.Title, Description: b.Description, Language: b.Language, Search: "titles"}
			if b.index.Load() != nil {
				i.Search = "ready"
			} else if p := b.progress.Load(); p >= 0 {
				i.Search, i.Progress = "preparing", int(p)
			}
			list = append(list, i)
		}
		writeJSON(w, list)
	})
	mux.HandleFunc("/api/library/search", l.search)
	mux.HandleFunc("/api/library/icon", func(w http.ResponseWriter, r *http.Request) {
		l.open()
		b := l.books[r.URL.Query().Get("book")]
		if b == nil {
			http.NotFound(w, r)
			return
		}
		e, err := b.a.Find('M', "Illustration_48x48@1")
		if err != nil {
			http.NotFound(w, r)
			return
		}
		l.content(w, r, b, e)
	})
}

func (l *library) serve(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/library/")
	if rest == "" {
		http.ServeFile(w, r, filepath.Join(l.root, "app", "library.html"))
		return
	}
	l.open()
	id, path, _ := strings.Cut(rest, "/")
	b := l.books[id]
	if b == nil {
		messagePage(w, http.StatusNotFound, "Not in the library", "This drive has no such book.", false)
		return
	}
	if path == "" {
		main, err := b.a.MainPage()
		if err == nil {
			main, err = b.a.Resolve(main)
		}
		if err != nil {
			messagePage(w, http.StatusNotFound, "No start page", "This book has no start page; search it from the library.", false)
			return
		}
		http.Redirect(w, r, b.url(main), http.StatusFound)
		return
	}
	ns := b.a.ContentNamespace()
	if ns == 'A' { // archives before ZIM 6.1 put the namespace in the path: A/Tallinn, I/flag.png
		if n, p, ok := strings.Cut(path, "/"); ok && len(n) == 1 {
			ns, path = n[0], p
		}
	}
	e, err := b.a.Find(ns, path)
	if err != nil {
		messagePage(w, http.StatusNotFound, "Page not found", "This book does not have that page.", false)
		return
	}
	if e.IsRedirect() {
		target, err := b.a.Resolve(e)
		if err != nil {
			messagePage(w, http.StatusNotFound, "Page not found", "This book does not have that page.", false)
			return
		}
		http.Redirect(w, r, b.url(target), http.StatusFound)
		return
	}
	l.content(w, r, b, e)
}

// barScript is added to every page of a book: a way back, to the library
// and to search, wherever the page came from.
var barScript = []byte(`<script type="module" src="/js/library-bar.js"></script>`)

func (l *library) content(w http.ResponseWriter, r *http.Request, b *book, e zim.Entry) {
	data, err := b.a.Content(e)
	if err != nil {
		msg := "This part of the book could not be read; the file may be damaged."
		if errors.Is(err, zim.ErrUnsupported) {
			msg = zim.ErrUnsupported.Error()
		}
		messagePage(w, http.StatusInternalServerError, "Cannot read this page", msg, false)
		return
	}
	mime := b.a.MIME(e)
	if strings.HasPrefix(mime, "text/") && !strings.Contains(mime, "charset") {
		mime += "; charset=utf-8"
	}
	w.Header().Set("Content-Type", mime)
	w.Header().Set("Cache-Control", "max-age=3600") // a book never changes
	if !strings.HasPrefix(mime, "text/html") || data.Size() > 32<<20 {
		http.ServeContent(w, r, "", time.Time{}, data)
		return
	}
	page, err := io.ReadAll(data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if i := bytes.Index(bytes.ToLower(page), []byte("</head>")); i >= 0 {
		page = append(page[:i:i], append(barScript, page[i:]...)...)
	} else {
		page = append(barScript, page...)
	}
	w.Header().Set("Content-Length", strconv.Itoa(len(page)))
	_, _ = w.Write(page)
}

// url is the address of an entry of this book.
func (b *book) url(e zim.Entry) string {
	path := e.Path
	if b.a.ContentNamespace() == 'A' {
		path = string(e.Namespace) + "/" + path
	}
	return (&url.URL{Path: "/library/" + b.ID + "/" + path}).EscapedPath()
}

type hit struct {
	Book    string `json:"book"`
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet,omitempty"`
	words   int    // text matches only: query words found, and how well
	score   float64
}

// search answers with titles that start with the query, then articles that
// contain its words (best first, across books), and which books cannot be
// searched inside yet.
func (l *library) search(w http.ResponseWriter, r *http.Request) {
	l.open()
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	type preparing struct {
		Title    string `json:"title"`
		Progress int    `json:"progress"`
	}
	out := struct {
		Hits      []hit       `json:"hits"`
		Preparing []preparing `json:"preparing"`
	}{[]hit{}, []preparing{}}
	seen := map[string]bool{}
	var text []hit
	for _, id := range l.order {
		b := l.books[id]
		if p := b.progress.Load(); p >= 0 {
			out.Preparing = append(out.Preparing, preparing{b.Title, int(p)})
		}
		if q == "" {
			continue
		}
		titles, err := b.a.Titles(q, 3)
		if err != nil {
			log.Printf("Library search in %s: %v", id, err)
		}
		for _, e := range titles {
			if target, err := b.a.Resolve(e); err == nil {
				e = target // several titles often lead to one article
			}
			if h := (hit{Book: id, Title: e.Title, URL: b.url(e)}); !seen[h.URL] {
				seen[h.URL] = true
				out.Hits = append(out.Hits, h)
			}
		}
		if x := b.index.Load(); x != nil {
			found, err := x.Search(q, 8)
			if err != nil {
				log.Printf("Library search in %s: %v", id, err)
			}
			for _, f := range found {
				text = append(text, hit{Book: id, Title: f.Entry.Title, URL: b.url(f.Entry), Snippet: f.Snippet, words: f.Words, score: f.Score})
			}
		}
	}
	sort.SliceStable(text, func(i, j int) bool {
		if text[i].words != text[j].words {
			return text[i].words > text[j].words // all the words before some of them
		}
		return text[i].score > text[j].score
	})
	for _, h := range text {
		if !seen[h.URL] && len(out.Hits) < 20 {
			seen[h.URL] = true
			out.Hits = append(out.Hits, h)
		}
	}
	writeJSON(w, out)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
