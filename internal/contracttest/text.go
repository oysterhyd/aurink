package contracttest

import (
	"strings"
	"sync"
	"testing"

	"aurink/internal/contract"
)

var textSamples = []string{
	"",
	"a",
	"a\n",
	"\n",
	"\n\n",
	"一行\n二行\n三行",
	"a\r\nb\r\n",
	"a\nb\r\nc",
	"尾部无换行 🐈",
}

// TestText checks docs/04 §4.1 for a snapshot. A line ends after "\n", so a
// final newline starts an empty last line. newText returns a snapshot of s.
func TestText(t *testing.T, newText func(t *testing.T, s string) contract.Text) {
	for _, s := range textSamples {
		t.Run(s, func(t *testing.T) {
			x := newText(t, s)
			if x.String() != s || x.Len() != len(s) {
				t.Fatalf("String %q, Len %d", x.String(), x.Len())
			}
			if want := strings.Count(s, "\n") + 1; x.LineCount() != want {
				t.Fatalf("LineCount %d, want %d", x.LineCount(), want)
			}
			if x.LineStart(0) != 0 || x.LineStart(x.LineCount()) != len(s) {
				t.Fatalf("LineStart(0) = %d, LineStart(LineCount) = %d", x.LineStart(0), x.LineStart(x.LineCount()))
			}
			line := 0
			for off := 0; off <= len(s); off++ {
				if got := x.LineOf(off); got != line {
					t.Fatalf("LineOf(%d) = %d, want %d", off, got, line)
				}
				if off < len(s) && s[off] == '\n' {
					line++
					if x.LineStart(line) != off+1 {
						t.Fatalf("LineStart(%d) = %d, want %d", line, x.LineStart(line), off+1)
					}
				}
			}
			for start := 0; start <= len(s); start++ {
				for end := start; end <= len(s); end++ {
					if got := x.Slice(contract.Range{Start: start, End: end}); got != s[start:end] {
						t.Fatalf("Slice(%d,%d) = %q", start, end, got)
					}
				}
			}
		})
	}
	t.Run("concurrent reads", func(t *testing.T) {
		x := newText(t, strings.Repeat("一行\n", 1000))
		var wg sync.WaitGroup
		for range 8 {
			wg.Go(func() {
				for off := 0; off < x.Len(); off += 97 {
					if x.LineStart(x.LineOf(off)) > off {
						t.Errorf("line of %d starts after it", off)
					}
				}
			})
		}
		wg.Wait()
	})
}
