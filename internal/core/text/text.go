package text

import (
	"sort"
	"strings"
	"sync"

	"aurink/internal/contract"
)

// Text is an immutable byte-preserving snapshot, safe for concurrent readers.
// A Text must not be copied; use New to construct snapshots.
type Text struct {
	s       string
	version contract.Version

	lineOnce   sync.Once
	lines      []int
	lineBuilds int
}

var _ contract.Text = (*Text)(nil)

// New returns a snapshot of s at version v without building its line index.
// It preserves all bytes; callers handle any file BOM before construction.
func New(s string, v contract.Version) *Text {
	return &Text{s: s, version: v}
}

// Version returns the snapshot's version.
func (t *Text) Version() contract.Version { return t.version }

// Len returns the number of bytes in the snapshot.
func (t *Text) Len() int { return len(t.s) }

// String returns the snapshot's original bytes as a string.
func (t *Text) String() string { return t.s }

// Slice returns the bytes in r, or an empty string if r is out of range.
// Byte offsets may split UTF-8 sequences.
func (t *Text) Slice(r contract.Range) string {
	if r.Start < 0 || r.End < r.Start || r.End > len(t.s) {
		return ""
	}
	return t.s[r.Start:r.End]
}

func (t *Text) lineStarts() []int {
	t.lineOnce.Do(func() {
		t.lineBuilds++
		starts := []int{0}
		for i := 0; i < len(t.s); {
			j := strings.IndexByte(t.s[i:], '\n')
			if j < 0 {
				break
			}
			i += j + 1
			starts = append(starts, i)
		}
		t.lines = starts
	})
	return t.lines
}

// LineCount returns one plus the number of LF bytes, including an empty last
// line after a final LF. CR bytes remain part of their line.
func (t *Text) LineCount() int { return len(t.lineStarts()) }

// LineStart returns the first byte offset of the zero-based line.
// Negative lines return zero; lines at or beyond LineCount return Len.
func (t *Text) LineStart(line int) int {
	if line < 0 {
		return 0
	}
	starts := t.lineStarts()
	if line >= len(starts) {
		return len(t.s)
	}
	return starts[line]
}

// LineOf returns the zero-based line containing offset, clamped to [0, Len()].
// After the first line query builds the index, each lookup takes O(log lines).
func (t *Text) LineOf(offset int) int {
	if offset < 0 {
		return 0
	}
	starts := t.lineStarts()
	return sort.Search(len(starts), func(i int) bool { return starts[i] > offset }) - 1
}
