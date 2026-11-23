// Package zimtest writes small ZIM archives for tests: one stored cluster
// and one zstd cluster, a front-article title list, and a main page.
package zimtest

import (
	"encoding/binary"
	"os"
	"sort"

	"github.com/klauspost/compress/zstd"
)

// Entry is one item to write. Redirect, when set, is the path (in the same
// namespace) it points to; otherwise Data is its content, compressed unless
// Stored.
type Entry struct {
	Namespace   byte
	Path, Title string
	MIME        string
	Data        []byte
	Stored      bool
	Redirect    string
}

// Write creates a ZIM 6.1 archive at path whose main page is the C entry
// named main. Every C entry that is not a redirect counts as an article.
func Write(path string, entries []Entry, main string) error {
	sort.Slice(entries, func(i, j int) bool {
		return string(entries[i].Namespace)+entries[i].Path < string(entries[j].Namespace)+entries[j].Path
	})
	index := map[string]uint32{}
	for i, e := range entries {
		index[string(e.Namespace)+e.Path] = uint32(i)
	}
	// The front-article list sorts after C, M and W, so adding it keeps the
	// other indices.
	var articles []uint32
	for i, e := range entries {
		if e.Namespace == 'C' && e.Redirect == "" && e.MIME == "text/html" {
			articles = append(articles, uint32(i))
		}
	}
	sort.Slice(articles, func(i, j int) bool { return title(entries[articles[i]]) < title(entries[articles[j]]) })
	list := make([]byte, 4*len(articles))
	for i, a := range articles {
		binary.LittleEndian.PutUint32(list[4*i:], a)
	}
	entries = append(entries, Entry{Namespace: 'X', Path: "listing/titleOrdered/v1", MIME: "application/octet-stream", Data: list})

	var mimes []string
	mimeIndex := map[string]uint16{}
	var blobs [2][][]byte // 0: stored, 1: zstd
	var dirents [][]byte
	for _, e := range entries {
		d := []byte{0, 0, 0, e.Namespace, 0, 0, 0, 0}
		if e.Redirect != "" {
			binary.LittleEndian.PutUint16(d, 0xffff)
			d = binary.LittleEndian.AppendUint32(d, index[string(e.Namespace)+e.Redirect])
		} else {
			m, ok := mimeIndex[e.MIME]
			if !ok {
				m = uint16(len(mimes))
				mimeIndex[e.MIME] = m
				mimes = append(mimes, e.MIME)
			}
			binary.LittleEndian.PutUint16(d, m)
			c := 1
			if e.Stored {
				c = 0
			}
			d = binary.LittleEndian.AppendUint32(d, uint32(c))
			d = binary.LittleEndian.AppendUint32(d, uint32(len(blobs[c])))
			blobs[c] = append(blobs[c], e.Data)
		}
		d = append(append(append(d, e.Path...), 0), e.Title...)
		dirents = append(dirents, append(d, 0))
	}

	var clusters [2][]byte
	for c, list := range blobs {
		body := make([]byte, 4*(len(list)+1))
		off := uint32(len(body))
		for i, b := range list {
			binary.LittleEndian.PutUint32(body[4*i:], off)
			off += uint32(len(b))
			body = append(body, b...)
		}
		binary.LittleEndian.PutUint32(body[4*len(list):], off)
		if c == 0 {
			clusters[c] = append([]byte{1}, body...)
			continue
		}
		enc, _ := zstd.NewWriter(nil)
		clusters[c] = append([]byte{5}, enc.EncodeAll(body, nil)...)
	}

	n := len(entries)
	var mimeList []byte
	for _, m := range mimes {
		mimeList = append(append(mimeList, m...), 0)
	}
	mimeList = append(mimeList, 0)
	mimePos := uint64(80)
	pathPtr := mimePos + uint64(len(mimeList))
	titlePtr := pathPtr + 8*uint64(n)
	direntPos := titlePtr + 4*uint64(n)
	pos := direntPos
	var pathList, titleList []byte
	for _, d := range dirents {
		pathList = binary.LittleEndian.AppendUint64(pathList, pos)
		pos += uint64(len(d))
	}
	byTitle := make([]int, n)
	for i := range byTitle {
		byTitle[i] = i
	}
	sort.Slice(byTitle, func(i, j int) bool {
		a, b := entries[byTitle[i]], entries[byTitle[j]]
		return string(a.Namespace)+title(a) < string(b.Namespace)+title(b)
	})
	for _, i := range byTitle {
		titleList = binary.LittleEndian.AppendUint32(titleList, uint32(i))
	}
	clusterPtr := pos
	pos += 16
	var clusterList []byte
	for _, c := range clusters {
		clusterList = binary.LittleEndian.AppendUint64(clusterList, pos)
		pos += uint64(len(c))
	}
	checksum := pos

	h := make([]byte, 80)
	binary.LittleEndian.PutUint32(h[0:], 72173914)
	binary.LittleEndian.PutUint16(h[4:], 6)
	binary.LittleEndian.PutUint16(h[6:], 1)
	binary.LittleEndian.PutUint32(h[24:], uint32(n))
	binary.LittleEndian.PutUint32(h[28:], 2)
	binary.LittleEndian.PutUint64(h[32:], pathPtr)
	binary.LittleEndian.PutUint64(h[40:], titlePtr)
	binary.LittleEndian.PutUint64(h[48:], clusterPtr)
	binary.LittleEndian.PutUint64(h[56:], mimePos)
	binary.LittleEndian.PutUint32(h[64:], index["C"+main])
	binary.LittleEndian.PutUint32(h[68:], 0xffffffff)
	binary.LittleEndian.PutUint64(h[72:], checksum)

	out := append(h, mimeList...)
	out = append(append(out, pathList...), titleList...)
	for _, d := range dirents {
		out = append(out, d...)
	}
	out = append(out, clusterList...)
	out = append(append(out, clusters[0]...), clusters[1]...)
	out = append(out, make([]byte, 16)...) // checksum, not verified by the reader
	return os.WriteFile(path, out, 0o644)
}

func title(e Entry) string {
	if e.Title == "" {
		return e.Path
	}
	return e.Title
}
