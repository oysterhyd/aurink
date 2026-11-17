package text

import (
	"sort"
	"strings"
	"sync"

	"aurink/internal/contract"
)

const (
	targetChunk = 16 << 10
	maxChunk    = 32 << 10
)

// Text is an immutable byte-preserving snapshot, safe for concurrent readers.
// Large snapshots store 16 KB string chunks so edits do not copy megabytes per
// keystroke while String() still caches a contiguous view on demand.
// A Text must not be copied; use New to construct snapshots.
type Text struct {
	chunks  []string
	offs    []int // len(chunks)+1 prefix byte offsets
	version contract.Version

	strOnce sync.Once
	s       string

	lineOnce   sync.Once
	lines      []int
	lineBuilds int
}

var _ contract.Text = (*Text)(nil)

// New returns a snapshot of s at version v without building its line index.
// It preserves all bytes; callers handle any file BOM before construction.
func New(s string, v contract.Version) *Text {
	t := &Text{version: v, s: s}
	t.strOnce.Do(func() {})
	if len(s) <= maxChunk {
		if len(s) == 0 {
			t.offs = []int{0}
		} else {
			t.chunks = []string{s}
			t.offs = []int{0, len(s)}
		}
		return t
	}
	n := (len(s) + targetChunk - 1) / targetChunk
	t.chunks = make([]string, 0, n)
	t.offs = make([]int, 0, n+1)
	t.offs = append(t.offs, 0)
	for at := 0; at < len(s); {
		end := min(at+targetChunk, len(s))
		t.chunks = append(t.chunks, s[at:end])
		t.offs = append(t.offs, end)
		at = end
	}
	return t
}

// Version returns the snapshot's version.
func (t *Text) Version() contract.Version { return t.version }

// Len returns the number of bytes in the snapshot.
func (t *Text) Len() int { return t.offs[len(t.chunks)] }

// String returns the snapshot's original bytes as a string.
func (t *Text) String() string {
	t.strOnce.Do(func() {
		switch len(t.chunks) {
		case 0:
			t.s = ""
		case 1:
			t.s = t.chunks[0]
		default:
			var b strings.Builder
			b.Grow(t.Len())
			for _, c := range t.chunks {
				b.WriteString(c)
			}
			t.s = b.String()
		}
	})
	return t.s
}

// Slice returns the bytes in r, or an empty string if r is out of range.
// Byte offsets may split UTF-8 sequences.
func (t *Text) Slice(r contract.Range) string {
	n := t.Len()
	if r.Start < 0 || r.End < r.Start || r.End > n || r.Start == r.End {
		return ""
	}
	if len(t.chunks) == 1 {
		return t.chunks[0][r.Start:r.End]
	}
	i := sort.Search(len(t.chunks), func(i int) bool { return t.offs[i+1] > r.Start })
	if r.End <= t.offs[i+1] {
		base := t.offs[i]
		return t.chunks[i][r.Start-base : r.End-base]
	}
	var b strings.Builder
	b.Grow(r.End - r.Start)
	pos := r.Start
	for ; i < len(t.chunks) && pos < r.End; i++ {
		cStart, cEnd := t.offs[i], t.offs[i+1]
		end := min(r.End, cEnd)
		b.WriteString(t.chunks[i][pos-cStart : end-cStart])
		pos = end
	}
	return b.String()
}

func (t *Text) lineStarts() []int {
	t.lineOnce.Do(func() {
		t.lineBuilds++
		s := t.String()
		starts := []int{0}
		for i := 0; i < len(s); {
			j := strings.IndexByte(s[i:], '\n')
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
		return t.Len()
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
