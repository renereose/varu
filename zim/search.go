package zim

// Search inside a book's text, with Varu's own index: built once from the
// archive's articles and kept in a file next to it.
//
// Index file ("VARUIDX1", little-endian):
//
//	header    magic, archive UUID and size, counts and section offsets
//	documents per article: entry index (u32), length in words (u16)
//	postings  per term: (document delta, uvarint; times in it, byte)...
//	terms     sorted, in blocks of 64: term, document count, postings length
//	blocks    per block: its first term, where it starts in terms and postings

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"html"
	"io"
	"math"
	"os"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	indexMagic  = "VARUIDX1"
	indexHeader = 8 + 16 + 8 + 4*4 + 8*4
	blockTerms  = 64
	expandMax   = 64 // inflected forms a query word may match
)

// batchSize is how many postings are held in memory before they go to a
// run file; a variable so tests can force several runs.
var batchSize = 2 << 20

// UUID identifies the archive's edition.
func (a *Archive) UUID() [16]byte {
	var u [16]byte
	_, _ = a.f.ReadAt(u[:], 8)
	return u
}

// ---------- text ----------

// Text is the readable text of an HTML page: tags, scripts and styles
// removed, entities decoded.
func Text(page []byte) string {
	var out strings.Builder
	out.Grow(len(page) / 2)
	for len(page) > 0 {
		lt := bytes.IndexByte(page, '<')
		if lt < 0 {
			out.WriteString(html.UnescapeString(string(page)))
			break
		}
		out.WriteString(html.UnescapeString(string(page[:lt])))
		page = page[lt:]
		gt := bytes.IndexByte(page, '>')
		if gt < 0 {
			break
		}
		tag := bytes.ToLower(page[1:min(gt, 8)])
		page = page[gt+1:]
		for _, skip := range []string{"script", "style"} {
			if bytes.HasPrefix(tag, []byte(skip)) {
				end := bytes.Index(bytes.ToLower(page), []byte("</"+skip))
				if end < 0 {
					end = len(page)
				}
				page = page[end:]
			}
		}
		out.WriteByte(' ')
	}
	return out.String()
}

// words calls fn with each lower-case word of 2 to 32 letters or digits.
func words(text string, fn func(word string)) {
	var w []rune
	flush := func() {
		if len(w) >= 2 && len(w) <= 32 {
			fn(string(w))
		}
		w = w[:0]
	}
	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			w = append(w, unicode.ToLower(r))
		} else {
			flush()
		}
	}
	flush()
}

// ---------- building ----------

// titleMark starts the terms made from article titles, which are kept apart
// from the text's: a word in the title counts for much more.
const titleMark = "\x01"

type doc struct {
	entry, cluster, blob uint32
	mime                 uint16
	title                string
}

// BuildIndex writes a search index of a's articles to path (through a
// temporary file, so a half-built index never takes its place). progress,
// when set, hears how many articles are done out of how many.
func BuildIndex(a *Archive, path string, progress func(done, total int)) error {
	// The articles, in storage order, so each cluster is unpacked once.
	n, at := a.titleOrder()
	var docs []doc
	for i := 0; i < n; i++ {
		e, err := a.Entry(at(i))
		if err != nil {
			return err
		}
		if !e.IsRedirect() && e.Namespace == a.contentNS && strings.HasPrefix(a.MIME(e), "text/html") {
			docs = append(docs, doc{e.Index, e.cluster, e.blob, e.mime, e.Title})
		}
	}
	sort.Slice(docs, func(i, j int) bool {
		return docs[i].cluster < docs[j].cluster || docs[i].cluster == docs[j].cluster && docs[i].blob < docs[j].blob
	})

	tmp := path + ".part"
	var runs []string
	defer func() {
		for _, r := range runs {
			os.Remove(r)
		}
	}()
	docTable := make([]byte, 0, 6*len(docs))
	// Postings for the current batch: per term, (document, uvarint; count, byte)...
	batch := map[string][]byte{}
	held := 0
	add := func(term string, d, count int) {
		batch[term] = append(binary.AppendUvarint(batch[term], uint64(d)), byte(min(count, 255)))
		held++
	}
	tf := map[string]int{}
	var totalLen float64
	for d, e := range docs {
		r, err := a.Content(Entry{mime: e.mime, cluster: e.cluster, blob: e.blob})
		if err != nil {
			return err
		}
		page, err := io.ReadAll(r)
		if err != nil {
			return err
		}
		clear(tf)
		length := 0
		words(Text(page), func(w string) { tf[w]++; length++ })
		for w, c := range tf {
			add(w, d, c)
		}
		// Title words carry the title's length in place of a count.
		clear(tf)
		words(e.title, func(w string) { tf[w]++ })
		for w := range tf {
			add(titleMark+w, d, len(tf))
		}
		totalLen += float64(length)
		docTable = binary.LittleEndian.AppendUint32(docTable, e.entry)
		docTable = binary.LittleEndian.AppendUint16(docTable, uint16(min(length, math.MaxUint16)))
		if held >= batchSize || d == len(docs)-1 {
			run := fmt.Sprintf("%s.run%d", tmp, len(runs))
			runs = append(runs, run)
			if err := writeRun(run, batch); err != nil {
				return err
			}
			batch, held = map[string][]byte{}, 0
		}
		if progress != nil && (d%2000 == 0 || d == len(docs)-1) {
			progress(d+1, len(docs))
		}
	}
	avg := 0.0
	if len(docs) > 0 {
		avg = totalLen / float64(len(docs))
	}
	if err := mergeRuns(tmp, runs, a, docTable, len(docs), avg); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}

// writeRun stores one batch: terms in order, each with its postings.
func writeRun(path string, batch map[string][]byte) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	w := bufio.NewWriterSize(f, 256<<10)
	terms := make([]string, 0, len(batch))
	for t := range batch {
		terms = append(terms, t)
	}
	sort.Strings(terms)
	var b [binary.MaxVarintLen64]byte
	for _, t := range terms {
		w.Write(b[:binary.PutUvarint(b[:], uint64(len(t)))])
		w.WriteString(t)
		w.Write(b[:binary.PutUvarint(b[:], uint64(len(batch[t])))])
		w.Write(batch[t])
	}
	if err := w.Flush(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

type runReader struct {
	r    *bufio.Reader
	f    *os.File
	term string
	ok   bool
}

func (rr *runReader) next() error {
	n, err := binary.ReadUvarint(rr.r)
	if err == io.EOF {
		rr.ok = false
		return nil
	}
	if err != nil {
		return err
	}
	b := make([]byte, n)
	if _, err := io.ReadFull(rr.r, b); err != nil {
		return err
	}
	rr.term, rr.ok = string(b), true
	return nil
}

type countingWriter struct {
	w *bufio.Writer
	n int64
}

func (c *countingWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.n += int64(n)
	return n, err
}

func (c *countingWriter) WriteString(s string) (int, error) {
	n, err := c.w.WriteString(s)
	c.n += int64(n)
	return n, err
}

func uvarintLen(v uint64) int {
	var b [binary.MaxVarintLen64]byte
	return binary.PutUvarint(b[:], v)
}

// mergeRuns joins the runs (each covering later documents than the one
// before) into the final index file.
func mergeRuns(path string, runs []string, a *Archive, docTable []byte, docs int, avg float64) error {
	readers := make([]*runReader, len(runs))
	for i, run := range runs {
		f, err := os.Open(run)
		if err != nil {
			return err
		}
		defer f.Close()
		readers[i] = &runReader{r: bufio.NewReaderSize(f, 256<<10), f: f}
		if err := readers[i].next(); err != nil {
			return err
		}
	}
	out, err := os.Create(path)
	if err != nil {
		return err
	}
	defer out.Close()
	postingsAt := int64(indexHeader + len(docTable))
	if _, err := out.Seek(postingsAt, io.SeekStart); err != nil {
		return err
	}
	pw := bufio.NewWriterSize(out, 1<<20)
	// The term list goes to its own file while the postings are written, then
	// after them: a book has millions of distinct words.
	termsFile, err := os.Create(path + ".terms")
	if err != nil {
		return err
	}
	defer os.Remove(termsFile.Name())
	defer termsFile.Close()
	terms := &countingWriter{w: bufio.NewWriterSize(termsFile, 256<<10)}
	var blocks []byte
	var b [binary.MaxVarintLen64]byte
	var postOff int64
	count := 0
	for {
		term, any := "", false
		for _, r := range readers {
			if r.ok && (!any || r.term < term) {
				term, any = r.term, true
			}
		}
		if !any {
			break
		}
		if count%blockTerms == 0 {
			blocks = binary.AppendUvarint(blocks, uint64(len(term)))
			blocks = append(blocks, term...)
			blocks = binary.AppendUvarint(blocks, uint64(terms.n))
			blocks = binary.AppendUvarint(blocks, uint64(postOff))
		}
		var n, written uint64
		prev := uint64(0)
		for _, r := range readers {
			if !r.ok || r.term != term {
				continue
			}
			size, err := binary.ReadUvarint(r.r)
			if err != nil {
				return err
			}
			for left := int64(size); left > 0; {
				doc, err := binary.ReadUvarint(r.r)
				if err != nil {
					return err
				}
				tf, err := r.r.ReadByte()
				if err != nil {
					return err
				}
				left -= int64(uvarintLen(doc)) + 1
				k := binary.PutUvarint(b[:], doc-prev)
				pw.Write(b[:k])
				pw.WriteByte(tf)
				written += uint64(k) + 1
				prev = doc
				n++
			}
			if err := r.next(); err != nil {
				return err
			}
		}
		terms.Write(b[:binary.PutUvarint(b[:], uint64(len(term)))])
		terms.WriteString(term)
		terms.Write(b[:binary.PutUvarint(b[:], n)])
		terms.Write(b[:binary.PutUvarint(b[:], written)])
		postOff += int64(written)
		count++
	}
	if err := pw.Flush(); err != nil {
		return err
	}
	if err := terms.w.Flush(); err != nil {
		return err
	}
	termsAt := postingsAt + postOff
	if _, err := termsFile.Seek(0, io.SeekStart); err != nil {
		return err
	}
	if _, err := io.Copy(out, termsFile); err != nil {
		return err
	}
	if _, err := out.Write(blocks); err != nil {
		return err
	}
	uuid := a.UUID()
	h := append([]byte(indexMagic), uuid[:]...)
	h = binary.LittleEndian.AppendUint64(h, uint64(a.size))
	h = binary.LittleEndian.AppendUint32(h, uint32(docs))
	h = binary.LittleEndian.AppendUint32(h, uint32(count))
	h = binary.LittleEndian.AppendUint32(h, math.Float32bits(float32(avg)))
	h = binary.LittleEndian.AppendUint32(h, uint32((count+blockTerms-1)/blockTerms))
	for _, off := range []int64{indexHeader, postingsAt, termsAt, termsAt + terms.n} {
		h = binary.LittleEndian.AppendUint64(h, uint64(off))
	}
	if _, err := out.WriteAt(append(h, docTable...), 0); err != nil {
		return err
	}
	return out.Close()
}

// ---------- searching ----------

// Index is an open search index for one archive.
type Index struct {
	f                                  *os.File
	a                                  *Archive
	docs                               int
	avg                                float64
	docTable, postings, terms, termEnd int64
	blocks                             []block
	entries                            []uint32 // per document: its entry in the archive
	lengths                            []uint16 // per document: words
}

type block struct {
	first            string
	termOff, postOff int64
}

// ErrStaleIndex means the index was made for another edition of the book.
var ErrStaleIndex = errors.New("search index is for another edition")

// OpenIndex opens the index at path, made for archive a.
func OpenIndex(path string, a *Archive) (*Index, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	x, err := openIndex(f, a)
	if err != nil {
		f.Close()
		return nil, err
	}
	return x, nil
}

func openIndex(f *os.File, a *Archive) (*Index, error) {
	h := make([]byte, indexHeader)
	if _, err := f.ReadAt(h, 0); err != nil || string(h[:8]) != indexMagic {
		return nil, errCorrupt
	}
	if uuid := a.UUID(); !bytes.Equal(h[8:24], uuid[:]) || int64(le.Uint64(h[24:])) != a.size {
		return nil, ErrStaleIndex
	}
	x := &Index{f: f, a: a, docs: int(le.Uint32(h[32:])), avg: float64(math.Float32frombits(le.Uint32(h[40:])))}
	nblocks := int(le.Uint32(h[44:]))
	x.docTable, x.postings, x.terms, x.termEnd = int64(le.Uint64(h[48:])), int64(le.Uint64(h[56:])), int64(le.Uint64(h[64:])), int64(le.Uint64(h[72:]))
	info, err := f.Stat()
	if err != nil || x.termEnd > info.Size() {
		return nil, errCorrupt
	}
	raw := make([]byte, info.Size()-x.termEnd)
	if _, err := f.ReadAt(raw, x.termEnd); err != nil && err != io.EOF {
		return nil, err
	}
	r := bytes.NewReader(raw)
	for i := 0; i < nblocks; i++ {
		n, err1 := binary.ReadUvarint(r)
		if err1 != nil || n > 256 {
			return nil, errCorrupt
		}
		first := make([]byte, n)
		_, err2 := io.ReadFull(r, first)
		termOff, err3 := binary.ReadUvarint(r)
		postOff, err4 := binary.ReadUvarint(r)
		if err := errors.Join(err2, err3, err4); err != nil {
			return nil, errCorrupt
		}
		x.blocks = append(x.blocks, block{string(first), int64(termOff), int64(postOff)})
	}
	table := make([]byte, 6*x.docs)
	if _, err := f.ReadAt(table, x.docTable); err != nil {
		return nil, errCorrupt
	}
	x.entries, x.lengths = make([]uint32, x.docs), make([]uint16, x.docs)
	for i := range x.entries {
		x.entries[i], x.lengths[i] = le.Uint32(table[6*i:]), le.Uint16(table[6*i+4:])
	}
	return x, nil
}

func (x *Index) Close() error { return x.f.Close() }

type termInfo struct {
	term          string
	count, length int64
	postOff       int64
}

// lookup returns the term itself and, when prefix is set, up to expandMax
// terms that begin with it.
func (x *Index) lookup(word string, prefix bool) ([]termInfo, error) {
	i := sort.Search(len(x.blocks), func(i int) bool { return x.blocks[i].first > word }) - 1
	if i < 0 {
		i = 0
	}
	var out []termInfo
	for ; i < len(x.blocks); i++ {
		end := x.termEnd - x.terms
		if i+1 < len(x.blocks) {
			end = x.blocks[i+1].termOff
		}
		raw := make([]byte, end-x.blocks[i].termOff)
		if _, err := x.f.ReadAt(raw, x.terms+x.blocks[i].termOff); err != nil && err != io.EOF {
			return nil, err
		}
		r := bytes.NewReader(raw)
		postOff := x.blocks[i].postOff
		for r.Len() > 0 {
			n, _ := binary.ReadUvarint(r)
			t := make([]byte, n)
			io.ReadFull(r, t)
			count, _ := binary.ReadUvarint(r)
			length, _ := binary.ReadUvarint(r)
			term := string(t)
			switch {
			case term == word || prefix && strings.HasPrefix(term, word):
				out = append(out, termInfo{term, int64(count), int64(length), postOff})
				if len(out) > expandMax {
					return out, nil
				}
			case term > word && !strings.HasPrefix(term, word):
				return out, nil
			}
			postOff += int64(length)
		}
	}
	return out, nil
}

// Hit is one search result.
type Hit struct {
	Entry   Entry
	Words   int // how many of the query's words it has
	Score   float64
	Snippet string
}

// Search finds articles containing every word of the query (words of four
// or more letters also match longer forms: "tallinn" finds "tallinnas"),
// best first; articles with only some of the words follow. A word in the
// title counts for much more than one in the text, the more so in a short
// title.
func (x *Index) Search(query string, limit int) ([]Hit, error) {
	var qwords []string
	seen := map[string]bool{}
	words(query, func(w string) {
		if !seen[w] && len(qwords) < 16 {
			seen[w] = true
			qwords = append(qwords, w)
		}
	})
	if len(qwords) == 0 || x.docs == 0 {
		return nil, nil
	}
	// BM25 with a gentle length penalty (b), since the main article
	// on a subject is usually the long one; titles weigh titleWeight times more.
	const k1, b, titleWeight, otherForm = 1.2, 0.4, 4.0, 0.6
	scores := map[uint32]float64{}
	matched := map[uint32]uint16{}
	for qi, w := range qwords {
		expand := utf8.RuneCountInString(w) >= 4
		text, err := x.lookup(w, expand)
		if err != nil {
			return nil, err
		}
		title, err := x.lookup(titleMark+w, expand)
		if err != nil {
			return nil, err
		}
		// One weight for the word and all its forms, so that a rare form
		// does not outweigh the word itself.
		df := 0.0
		for _, t := range text {
			df += float64(t.count)
		}
		df = min(df, float64(x.docs))
		idf := math.Log(1 + (float64(x.docs)-df+0.5)/(df+0.5))

		best := map[uint32]float64{}
		for i, terms := range [][]termInfo{text, title} {
			for _, t := range terms {
				weight := 1.0
				if t.term != w && t.term != titleMark+w {
					weight = otherForm
				}
				err := x.eachPosting(t, func(id uint32, n float64) {
					var s float64
					if i == 0 {
						s = weight * idf * n * (k1 + 1) / (n + k1*(1-b+b*float64(x.lengths[id])/max(x.avg, 1)))
					} else { // n is the title's length in words
						s = titleWeight * weight * idf * 2 / (1 + n/2)
					}
					key := id<<1 | uint32(i) // the best form in the text and in the title, each
					best[key] = max(best[key], s)
				})
				if err != nil {
					return nil, err
				}
			}
		}
		for key, s := range best {
			scores[key>>1] += s
			matched[key>>1] |= 1 << qi
		}
	}

	type cand struct {
		id    uint32
		words int
		score float64
	}
	cands := make([]cand, 0, len(scores))
	for id, s := range scores {
		n := 0
		for m := matched[id]; m != 0; m &= m - 1 {
			n++
		}
		cands = append(cands, cand{id, n, s})
	}
	sort.Slice(cands, func(i, j int) bool {
		if cands[i].words != cands[j].words {
			return cands[i].words > cands[j].words
		}
		return cands[i].score > cands[j].score
	})
	hits := make([]Hit, 0, limit)
	for _, c := range cands[:min(len(cands), limit)] {
		e, err := x.entry(c.id)
		if err != nil {
			return nil, err
		}
		hits = append(hits, Hit{Entry: e, Words: c.words, Score: c.score, Snippet: x.snippet(e, qwords)})
	}
	return hits, nil
}

// eachPosting calls fn with each document of a term and its count (or, for
// title terms, the title's length).
func (x *Index) eachPosting(t termInfo, fn func(id uint32, n float64)) error {
	raw := make([]byte, t.length)
	if _, err := x.f.ReadAt(raw, x.postings+t.postOff); err != nil && err != io.EOF {
		return err
	}
	doc := uint64(0)
	for p := 0; p < len(raw); {
		d, k := binary.Uvarint(raw[p:])
		if k <= 0 || p+k >= len(raw) {
			return errCorrupt
		}
		doc += d
		if doc >= uint64(x.docs) {
			return errCorrupt
		}
		fn(uint32(doc), float64(raw[p+k]))
		p += k + 1
	}
	return nil
}

func (x *Index) entry(id uint32) (Entry, error) { return x.a.Entry(x.entries[id]) }

// footnote matches reference marks such as "[1]" or "[ 12 ]".
var footnote = regexp.MustCompile(`\[\s*\d+\s*\]`)

// snippet is a line of the article around the first query word.
func (x *Index) snippet(e Entry, qwords []string) string {
	r, err := x.a.Content(e)
	if err != nil {
		return ""
	}
	page, err := io.ReadAll(io.LimitReader(r, 4<<20))
	if err != nil {
		return ""
	}
	text := strings.Join(strings.Fields(footnote.ReplaceAllString(Text(page), "")), " ")
	// Pages start with their title, often twice (<title> and the heading).
	for i := 0; i < 3 && strings.HasPrefix(text, e.Title); i++ {
		text = strings.TrimSpace(text[len(e.Title):])
	}
	lower := strings.ToLower(text)
	start := 0
	at := -1
	for _, w := range qwords {
		if i := strings.Index(lower[start:], w); i >= 0 && (at < 0 || start+i < at) {
			at = start + i
		}
	}
	if at < 0 {
		at = start
	}
	from, to := max(0, at-60), min(len(text), at+140)
	for from > 0 && !utf8.RuneStart(text[from]) {
		from--
	}
	for to < len(text) && !utf8.RuneStart(text[to]) {
		to++
	}
	s := strings.TrimSpace(text[from:to])
	if from > 0 {
		s = "…" + s
	}
	if to < len(text) {
		s += "…"
	}
	return s
}
