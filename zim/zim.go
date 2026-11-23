// Package zim reads ZIM archives (https://openzim.org): the offline
// Wikipedia and other books in a drive's library/. Entries are found by
// path, or by title for search. Read-only.
//
// Format: https://wiki.openzim.org/wiki/ZIM_file_format
package zim

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"github.com/klauspost/compress/zstd"
)

const (
	magic        = 72173914
	noEntry      = 0xffffffff
	redirectMime = 0xffff
	headerSize   = 80
	cacheSize    = 8 // decompressed clusters kept; a cluster is a few MB at most
)

var (
	ErrNotFound    = errors.New("not in this archive")
	ErrUnsupported = errors.New("this archive uses a compression Varu cannot read (xz); use a newer edition")
	errCorrupt     = errors.New("damaged archive")
	le             = binary.LittleEndian
	// ponytail: one shared decoder; DecodeAll is safe for concurrent use.
	zstdDecoder, _ = zstd.NewReader(nil, zstd.WithDecoderLowmem(true))
)

// Archive is an open ZIM file. Its methods are safe for concurrent use.
type Archive struct {
	f                             *os.File
	size                          int64
	entries, clusters             uint32
	pathPtr, titlePtr, clusterPtr int64
	checksumPos                   int64
	mainPage                      uint32
	mimes                         []string
	contentNS                     byte // 'C' since ZIM 6.1, 'A' before
	titlesOnce                    sync.Once
	titles                        []uint32 // front articles by title (X/listing/titleOrdered/v1)
	mu                            sync.Mutex
	cache                         []cluster // most recently used first
}

type cluster struct {
	index uint32
	data  []byte
}

// Entry is one item in the archive: an article, image, stylesheet,
// metadata value, or a redirect to another entry.
type Entry struct {
	Index     uint32
	Namespace byte
	Path      string
	Title     string
	mime      uint16
	redirect  uint32
	cluster   uint32
	blob      uint32
}

func (e Entry) IsRedirect() bool { return e.mime == redirectMime }

// Open reads the header and MIME list of a ZIM file.
func Open(path string) (*Archive, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	a, err := open(f)
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return a, nil
}

func open(f *os.File) (*Archive, error) {
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	var h [headerSize]byte
	if _, err := f.ReadAt(h[:], 0); err != nil || le.Uint32(h[0:]) != magic {
		return nil, errors.New("not a ZIM archive")
	}
	major, minor := le.Uint16(h[4:]), le.Uint16(h[6:])
	a := &Archive{
		f: f, size: info.Size(),
		entries: le.Uint32(h[24:]), clusters: le.Uint32(h[28:]),
		pathPtr: int64(le.Uint64(h[32:])), titlePtr: int64(le.Uint64(h[40:])),
		clusterPtr: int64(le.Uint64(h[48:])), mainPage: le.Uint32(h[64:]),
		checksumPos: int64(le.Uint64(h[72:])), contentNS: 'A',
	}
	if major > 6 || major == 6 && minor >= 1 {
		a.contentNS = 'C'
	}
	for _, end := range []int64{a.pathPtr + 8*int64(a.entries), a.titlePtr + 4*int64(a.entries), a.clusterPtr + 8*int64(a.clusters)} {
		if end > a.size {
			return nil, errCorrupt
		}
	}
	if a.checksumPos <= 0 || a.checksumPos > a.size {
		a.checksumPos = a.size
	}

	// The MIME list: zero-terminated strings, ending with an empty one.
	mimeList := int64(le.Uint64(h[56:]))
	buf := make([]byte, min(64<<10, max(0, a.size-mimeList)))
	n, _ := f.ReadAt(buf, mimeList)
	for rest := buf[:n]; ; {
		i := bytes.IndexByte(rest, 0)
		if i < 0 {
			return nil, errCorrupt
		}
		if i == 0 {
			break
		}
		a.mimes = append(a.mimes, string(rest[:i]))
		rest = rest[i+1:]
	}
	return a, nil
}

func (a *Archive) Close() error { return a.f.Close() }

// ContentNamespace is where articles live: 'C', or 'A' in archives older
// than ZIM 6.1 (which also keep images in 'I' and layout files in '-').
func (a *Archive) ContentNamespace() byte { return a.contentNS }

// Entry returns entry number i in path order.
func (a *Archive) Entry(i uint32) (Entry, error) {
	if i >= a.entries {
		return Entry{}, ErrNotFound
	}
	var b [8]byte
	if _, err := a.f.ReadAt(b[:], a.pathPtr+8*int64(i)); err != nil {
		return Entry{}, err
	}
	e, err := a.direntAt(int64(le.Uint64(b[:])))
	e.Index = i
	return e, err
}

func (a *Archive) direntAt(off int64) (Entry, error) {
	for size := 512; size <= 1<<16; size *= 4 {
		buf := make([]byte, size)
		n, err := a.f.ReadAt(buf, off)
		if n == 0 {
			if err == nil || err == io.EOF {
				err = errCorrupt
			}
			return Entry{}, err
		}
		if e, ok := parseDirent(buf[:n]); ok {
			return e, nil
		}
		if n < size {
			break
		}
	}
	return Entry{}, errCorrupt
}

// parseDirent reads a directory entry: MIME (or redirect marker),
// parameter length, namespace, revision, then the target, path and title.
func parseDirent(b []byte) (Entry, bool) {
	if len(b) < 12 {
		return Entry{}, false
	}
	e := Entry{mime: le.Uint16(b), Namespace: b[3]}
	rest := b[8:]
	switch {
	case e.mime == redirectMime:
		e.redirect, rest = le.Uint32(rest), rest[4:]
	case e.mime >= 0xfffd: // link target or deleted entry: no content
		e.cluster = noEntry
	default:
		if len(rest) < 8 {
			return Entry{}, false
		}
		e.cluster, e.blob, rest = le.Uint32(rest), le.Uint32(rest[4:]), rest[8:]
	}
	p := bytes.IndexByte(rest, 0)
	if p < 0 {
		return Entry{}, false
	}
	t := bytes.IndexByte(rest[p+1:], 0)
	if t < 0 {
		return Entry{}, false
	}
	e.Path, e.Title = string(rest[:p]), string(rest[p+1:p+1+t])
	if e.Title == "" {
		e.Title = e.Path
	}
	return e, true
}

// Find looks up an entry by namespace and path.
func (a *Archive) Find(ns byte, path string) (Entry, error) {
	key := string(ns) + path
	lo, hi := uint32(0), a.entries
	for lo < hi {
		mid := lo + (hi-lo)/2
		e, err := a.Entry(mid)
		if err != nil {
			return Entry{}, err
		}
		switch c := strings.Compare(string(e.Namespace)+e.Path, key); {
		case c == 0:
			return e, nil
		case c < 0:
			lo = mid + 1
		default:
			hi = mid
		}
	}
	return Entry{}, ErrNotFound
}

// Resolve follows redirects to the entry that has content.
func (a *Archive) Resolve(e Entry) (Entry, error) {
	for hops := 0; e.IsRedirect(); hops++ {
		if hops == 16 {
			return Entry{}, errCorrupt
		}
		var err error
		if e, err = a.Entry(e.redirect); err != nil {
			return Entry{}, err
		}
	}
	return e, nil
}

// MainPage is the archive's start page (often a redirect).
func (a *Archive) MainPage() (Entry, error) {
	if a.mainPage == noEntry {
		return Entry{}, ErrNotFound
	}
	return a.Entry(a.mainPage)
}

// MIME is the entry's media type ("" for redirects).
func (a *Archive) MIME(e Entry) string {
	if int(e.mime) < len(a.mimes) {
		return a.mimes[e.mime]
	}
	return ""
}

// Content returns the entry's data. Stored (uncompressed) items, such as
// images, PDFs and videos, are read straight from the file, so they can be
// streamed and seeked; compressed ones come from a decompressed cluster.
func (a *Archive) Content(e Entry) (*io.SectionReader, error) {
	if e.IsRedirect() || e.cluster >= a.clusters {
		return nil, ErrNotFound
	}
	var b [16]byte
	if _, err := a.f.ReadAt(b[:], a.clusterPtr+8*int64(e.cluster)); err != nil && err != io.EOF {
		return nil, err
	}
	start, end := int64(le.Uint64(b[:])), a.checksumPos
	if e.cluster+1 < a.clusters {
		end = int64(le.Uint64(b[8:]))
	}
	if start < 0 || end > a.size || end <= start+1 {
		return nil, errCorrupt
	}
	var info [1]byte
	if _, err := a.f.ReadAt(info[:], start); err != nil {
		return nil, err
	}
	offSize := int64(4)
	if info[0]&0x10 != 0 { // extended cluster: 64-bit offsets
		offSize = 8
	}
	var data io.ReaderAt
	switch info[0] & 0x0f {
	case 0, 1:
		data = io.NewSectionReader(a.f, start+1, end-start-1)
	case 5:
		raw, err := a.decompress(e.cluster, start+1, end)
		if err != nil {
			return nil, err
		}
		data = bytes.NewReader(raw)
	case 4:
		return nil, ErrUnsupported
	default:
		return nil, errCorrupt
	}
	offs := make([]byte, 2*offSize)
	first := make([]byte, offSize)
	if _, err := data.ReadAt(first, 0); err != nil {
		return nil, errCorrupt
	}
	if count := readOff(first) / offSize; int64(e.blob)+1 >= count {
		return nil, errCorrupt
	}
	if _, err := data.ReadAt(offs, int64(e.blob)*offSize); err != nil {
		return nil, errCorrupt
	}
	from, to := readOff(offs[:offSize]), readOff(offs[offSize:])
	if to < from {
		return nil, errCorrupt
	}
	return io.NewSectionReader(data, from, to-from), nil
}

func readOff(b []byte) int64 {
	if len(b) == 8 {
		return int64(le.Uint64(b))
	}
	return int64(le.Uint32(b))
}

// decompress returns cluster c's data, from the cache when it was used lately.
func (a *Archive) decompress(c uint32, start, end int64) ([]byte, error) {
	a.mu.Lock()
	for i, cl := range a.cache {
		if cl.index == c {
			copy(a.cache[1:i+1], a.cache[:i])
			a.cache[0] = cl
			a.mu.Unlock()
			return cl.data, nil
		}
	}
	a.mu.Unlock()

	if end-start > 256<<20 {
		return nil, errCorrupt
	}
	raw := make([]byte, end-start)
	if _, err := a.f.ReadAt(raw, start); err != nil && err != io.EOF {
		return nil, err
	}
	data, err := zstdDecoder.DecodeAll(raw, nil)
	if err != nil {
		return nil, fmt.Errorf("cluster %d: %w", c, err)
	}
	a.mu.Lock()
	a.cache = append([]cluster{{c, data}}, a.cache...)
	if len(a.cache) > cacheSize {
		a.cache = a.cache[:cacheSize]
	}
	a.mu.Unlock()
	return data, nil
}

// Metadata returns a value from the M namespace, such as "Title",
// "Description" or "Language"; "" when it is not there.
func (a *Archive) Metadata(name string) string {
	e, err := a.Find('M', name)
	if err != nil {
		return ""
	}
	r, err := a.Content(e)
	if err != nil || r.Size() > 1<<20 {
		return ""
	}
	b, _ := io.ReadAll(r)
	return string(b)
}

// Titles finds up to limit articles whose title starts with prefix. Titles
// are sorted case-sensitively, so it also tries the prefix with a capital
// first letter, in lower case and with every word capitalised.
func (a *Archive) Titles(prefix string, limit int) ([]Entry, error) {
	n, at := a.titleOrder()
	seen := map[uint32]bool{}
	var out []Entry
	for _, p := range variants(prefix) {
		key := string(a.contentNS) + p
		lo, hi := 0, n
		for lo < hi {
			mid := lo + (hi-lo)/2
			e, err := a.Entry(at(mid))
			if err != nil {
				return nil, err
			}
			if string(e.Namespace)+e.Title < key {
				lo = mid + 1
			} else {
				hi = mid
			}
		}
		for i := lo; i < n && len(out) < limit; i++ {
			e, err := a.Entry(at(i))
			if err != nil {
				return nil, err
			}
			if !strings.HasPrefix(string(e.Namespace)+e.Title, key) {
				break
			}
			if !seen[e.Index] {
				seen[e.Index] = true
				out = append(out, e)
			}
		}
	}
	return out, nil
}

// titleOrder is the list to search titles in: the archive's list of front
// articles when it has one, otherwise every entry in (namespace, title) order.
func (a *Archive) titleOrder() (int, func(int) uint32) {
	a.titlesOnce.Do(func() {
		e, err := a.Find('X', "listing/titleOrdered/v1")
		if err != nil {
			return
		}
		r, err := a.Content(e)
		if err != nil || r.Size()%4 != 0 || r.Size() > 4*int64(a.entries) {
			return
		}
		raw := make([]byte, r.Size())
		if _, err := r.ReadAt(raw, 0); err != nil && err != io.EOF {
			return
		}
		a.titles = make([]uint32, len(raw)/4)
		for i := range a.titles {
			a.titles[i] = le.Uint32(raw[4*i:])
		}
	})
	if a.titles != nil {
		return len(a.titles), func(i int) uint32 { return a.titles[i] }
	}
	return int(a.entries), func(i int) uint32 {
		var b [4]byte
		_, _ = a.f.ReadAt(b[:], a.titlePtr+4*int64(i))
		return le.Uint32(b[:])
	}
}

func variants(s string) []string {
	out := []string{s}
	add := func(v string) {
		for _, have := range out {
			if have == v {
				return
			}
		}
		out = append(out, v)
	}
	if r, size := utf8.DecodeRuneInString(s); r != utf8.RuneError {
		add(string(unicode.ToUpper(r)) + s[size:])
	}
	add(strings.ToLower(s))
	words := strings.Fields(strings.ToLower(s))
	for i, w := range words {
		r, size := utf8.DecodeRuneInString(w)
		words[i] = string(unicode.ToUpper(r)) + w[size:]
	}
	if len(words) > 0 {
		title := strings.Join(words, " ")
		if strings.HasSuffix(s, " ") {
			title += " "
		}
		add(title)
	}
	return out
}
