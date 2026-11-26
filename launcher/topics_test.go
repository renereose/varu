package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/renereose/varu/zim"
)

// TestTopics checks viewer/topics.json, the hand-picked survival and DIY
// articles: every entry is complete, and, when VARU_ZIM_DIR names a folder
// with the books (the release passes its download cache), every article
// exists in the newest edition there and is not a redirect.
func TestTopics(t *testing.T) {
	raw, err := os.ReadFile("../viewer/topics.json")
	if err != nil {
		t.Fatal(err)
	}
	var list struct {
		Topics []struct {
			ID, Et, En string
			Articles   []struct{ Book, Path, Title string }
		}
	}
	if err := json.Unmarshal(raw, &list); err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	for _, topic := range list.Topics {
		if topic.ID == "" || topic.Et == "" || topic.En == "" || len(topic.Articles) == 0 || ids[topic.ID] {
			t.Errorf("topic %+v: needs a unique id, et, en and articles", topic.ID)
		}
		ids[topic.ID] = true
		for _, a := range topic.Articles {
			if a.Book == "" || a.Path == "" || a.Title == "" {
				t.Errorf("%s: incomplete article %+v", topic.ID, a)
			}
		}
	}

	dir := os.Getenv("VARU_ZIM_DIR")
	if dir == "" {
		t.Skip("set VARU_ZIM_DIR to a folder with the books to check every article exists")
	}
	books := map[string]*zim.Archive{}
	for _, topic := range list.Topics {
		for _, a := range topic.Articles {
			b, ok := books[a.Book]
			if !ok {
				editions, _ := filepath.Glob(filepath.Join(dir, a.Book+"_*.zim"))
				sort.Strings(editions)
				if len(editions) > 0 {
					if b, err = zim.Open(editions[len(editions)-1]); err != nil {
						t.Fatal(err)
					}
					defer b.Close()
				}
				books[a.Book] = b
			}
			if b == nil {
				t.Errorf("%s: no %s book in %s", topic.ID, a.Book, dir)
				continue
			}
			e, err := b.Find(b.ContentNamespace(), a.Path)
			if err != nil {
				t.Errorf("%s: %s has no %q", topic.ID, a.Book, a.Path)
			} else if e.IsRedirect() {
				t.Errorf("%s: %s %q is a redirect; use its target", topic.ID, a.Book, a.Path)
			}
		}
	}
}
