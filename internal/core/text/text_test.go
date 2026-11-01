package text

import (
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"

	"aurink/internal/contract"
	"aurink/internal/contracttest"
)

func TestContract(t *testing.T) {
	contracttest.TestText(t, func(t *testing.T, s string) contract.Text {
		return New(s, 1)
	})
}

func TestBytesAndLines(t *testing.T) {
	for _, s := range []string{
		"", "\r", "\r\r", "\r\n", "\n\r\n\n", "a\rb\nc\r\nd\r",
		"中文🙂\r\n🐈\n終\r行\n", "e\u0301👨‍👩‍👧‍👦\n\u2028\u2029",
		"\x00\xff\xfe\r\n\x80",
	} {
		t.Run(fmt.Sprintf("%q", s), func(t *testing.T) {
			x := New(s, 42)
			if x.Version() != 42 {
				t.Fatalf("Version = %d, want 42", x.Version())
			}
			checkText(t, x, s)
			for start := 0; start <= len(s); start++ {
				for end := start; end <= len(s); end++ {
					r := contract.Range{Start: start, End: end}
					if got := x.Slice(r); got != s[start:end] {
						t.Fatalf("Slice(%v) = %q, want %q", r, got, s[start:end])
					}
				}
			}
		})
	}
}

func TestInvalidQueries(t *testing.T) {
	x := New("a\nb\n", 1)
	for _, r := range []contract.Range{
		{Start: -1, End: 1}, {Start: 2, End: 1},
		{Start: 0, End: 5}, {Start: 5, End: 5},
	} {
		if got := x.Slice(r); got != "" {
			t.Errorf("Slice(%v) = %q, want empty", r, got)
		}
	}
	if x.LineStart(-1) != 0 || x.LineOf(-1) != 0 {
		t.Fatal("negative line queries must return zero")
	}
	maxInt := int(^uint(0) >> 1)
	if x.LineStart(maxInt) != x.Len() || x.LineOf(maxInt) != x.LineCount()-1 {
		t.Fatal("past-end line queries must return the end")
	}
}

func TestLineIndexBuiltOnce(t *testing.T) {
	for _, s := range []string{"", "a\n", strings.Repeat("中文🙂\r\nx\n", 1000)} {
		t.Run(fmt.Sprintf("bytes=%d", len(s)), func(t *testing.T) {
			x := New(s, 7)
			if x.String() != s || x.Len() != len(s) || x.Version() != 7 || x.Slice(contract.Range{End: len(s)}) != s {
				t.Fatal("non-line reads changed the snapshot bytes or version")
			}
			if x.lines != nil || x.lineBuilds != 0 {
				t.Fatal("non-line reads built the line index")
			}

			start := make(chan struct{})
			var wg sync.WaitGroup
			for worker := range 48 {
				wg.Go(func() {
					<-start
					for j := range 1000 {
						switch (worker + j) % 3 {
						case 0:
							if got := x.LineCount(); got != strings.Count(s, "\n")+1 {
								t.Errorf("LineCount = %d", got)
								return
							}
						case 1:
							if x.LineStart(0) != 0 {
								t.Error("first line does not start at zero")
								return
							}
						case 2:
							off := (worker*97 + j) % (len(s) + 1)
							if got, want := x.LineOf(off), strings.Count(s[:off], "\n"); got != want {
								t.Errorf("LineOf(%d) = %d, want %d", off, got, want)
								return
							}
						}
					}
				})
			}
			close(start)
			wg.Wait()
			if x.lineBuilds != 1 {
				t.Fatalf("built line index %d times, want 1", x.lineBuilds)
			}
			checkText(t, x, s)
		})
	}
}

func checkText(t *testing.T, x *Text, want string) {
	t.Helper()
	if x.String() != want || x.Len() != len(want) {
		t.Fatalf("text = %q (%d bytes), want %q (%d bytes)", x.String(), x.Len(), want, len(want))
	}
	starts := []int{0}
	for i := range len(want) {
		if want[i] == '\n' {
			starts = append(starts, i+1)
		}
	}
	if x.LineCount() != len(starts) {
		t.Fatalf("LineCount = %d, want %d", x.LineCount(), len(starts))
	}
	for line, start := range starts {
		if got := x.LineStart(line); got != start {
			t.Fatalf("LineStart(%d) = %d, want %d", line, got, start)
		}
	}
	if x.LineStart(len(starts)) != len(want) {
		t.Fatal("LineStart(LineCount) does not return Len")
	}
	line := 0
	for off := 0; off <= len(want); off++ {
		if got := x.LineOf(off); got != line {
			t.Fatalf("LineOf(%d) = %d, want %d", off, got, line)
		}
		if off < len(want) && want[off] == '\n' {
			line++
		}
	}
	if !slices.Equal(x.lines, starts) {
		t.Fatalf("index = %v, want %v", x.lines, starts)
	}
}

func BenchmarkLineOf1MB(b *testing.B) {
	const size = 1 << 20
	for _, width := range []int{16, 128, 1024} {
		b.Run(fmt.Sprintf("line_bytes=%d", width), func(b *testing.B) {
			s := strings.Repeat(strings.Repeat("x", width-1)+"\n", size/width)
			x := New(s, 1)
			x.LineCount()
			b.ReportAllocs()
			b.ResetTimer()
			var total int
			for i := 0; i < b.N; i++ {
				off := (i * 7919) & (size - 1)
				got := x.LineOf(off)
				if got != off/width {
					b.Fatalf("LineOf(%d) = %d, want %d", off, got, off/width)
				}
				total += got
			}
			benchmarkLine = total
			b.ReportMetric(float64(x.LineCount()), "lines")
		})
	}
}

var benchmarkLine int
