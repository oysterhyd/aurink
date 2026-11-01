package mock

import (
	"sort"
	"strings"

	"aurink/internal/contract"
)

// Text is an immutable contract.Text over a string.
type Text struct {
	s       string
	version contract.Version
	lines   []int // first byte of each line
}

// NewText returns a Text at the given version.
func NewText(s string, v contract.Version) *Text {
	lines := []int{0}
	for i := 0; i < len(s); {
		j := strings.IndexByte(s[i:], '\n')
		if j < 0 {
			break
		}
		i += j + 1
		lines = append(lines, i)
	}
	return &Text{s, v, lines}
}

func (t *Text) Version() contract.Version { return t.version }
func (t *Text) Len() int                  { return len(t.s) }
func (t *Text) String() string            { return t.s }
func (t *Text) LineCount() int            { return len(t.lines) }

func (t *Text) Slice(r contract.Range) string { return t.s[r.Start:r.End] }

func (t *Text) LineStart(line int) int {
	if line >= len(t.lines) {
		return len(t.s)
	}
	return t.lines[line]
}

func (t *Text) LineOf(offset int) int {
	return sort.SearchInts(t.lines, offset+1) - 1
}
