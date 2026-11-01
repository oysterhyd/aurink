package astspike

import (
	"fmt"
	"strings"
	"testing"

	"aurink.local/spikes/contract-rc/contract"
	"github.com/egoist/mygo/ui/markdown/parser"
)

const benchUnit = "## 标题 heading\n\neditable paragraph **bold** &amp; \\* 中文 [ref]\nsecond line\n\n> - [ ] task\n>   nested 中文\n\n```go\nvar x = 1\n```\n\n"
const benchDefs = "[ref]: /global \"title\"\n"

func benchSource(size int) string {
	room := size - len(benchDefs) - 2
	return strings.Repeat(benchUnit, room/len(benchUnit)) + strings.Repeat("z", room%len(benchUnit)) + "\n\n" + benchDefs
}

func parseInlines(d *parser.Document) int {
	n := 0
	for i := 1; i < d.Len(); i++ {
		n += len(d.Inlines(i))
	}
	return n
}

func adaptInlines(b testing.TB, s *snapshot) int {
	b.Helper()
	n := 0
	for i := 1; i < len(s.blocks); i++ {
		in, err := s.inlines(i)
		if err != nil {
			b.Fatal(err)
		}
		n += len(in)
	}
	return n
}

var benchmarkSink int

// BenchmarkCurve measures throughput; no correctness test asserts wall time.
// All full/edit modes force inline parsing, so reference work is included.
func BenchmarkCurve(b *testing.B) {
	for _, size := range []int{100 * 1024, 1024 * 1024, 5 * 1024 * 1024} {
		src := benchSource(size)
		at := size/2 + strings.Index(src[size/2:], "editable")
		b.Run(fmt.Sprintf("%dKiB", size/1024), func(b *testing.B) {
			b.Run("ParserFull", func(b *testing.B) {
				b.SetBytes(int64(size))
				for b.Loop() {
					d := parser.Parse(src)
					benchmarkSink = parseInlines(d)
				}
			})
			b.Run("ParserEdit", func(b *testing.B) {
				d := parser.Parse(src)
				parseInlines(d)
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					d.Edit(at, at+1, string(byte('a'+i%2)))
					benchmarkSink = parseInlines(d)
				}
			})
			b.Run("AdapterFull", func(b *testing.B) {
				for b.Loop() {
					e, err := newEngine(src, parser.Default, false)
					if err != nil {
						b.Fatal(err)
					}
					benchmarkSink = adaptInlines(b, e.state)
				}
			})
			b.Run("AdapterEdit", func(b *testing.B) {
				e, err := newEngine(src, parser.Default, false)
				if err != nil {
					b.Fatal(err)
				}
				adaptInlines(b, e.state)
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					if err := e.edit(contract.TextEdit{Range: contract.Range{Start: at, End: at + 1}, Text: string(byte('a' + i%2))}); err != nil {
						b.Fatal(err)
					}
					benchmarkSink = adaptInlines(b, e.state)
				}
			})
			b.Run("ClosedShardEdit", func(b *testing.B) {
				// A closed shard of whole top-level units, about 16 KiB.
				// It shares the tiny global definition suffix. No whole-text
				// copy, offset-tree update or adapter rebuild is timed here.
				local := strings.Repeat(benchUnit, (16*1024)/len(benchUnit))
				d := parser.Parse(local + benchDefs)
				parseInlines(d)
				x := strings.Index(local, "editable")
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					d.Edit(x, x+1, string(byte('a'+i%2)))
					benchmarkSink = parseInlines(d)
				}
			})
			b.Run("DefinitionBroadcast", func(b *testing.B) {
				local := strings.Repeat(benchUnit, (16*1024)/len(benchUnit))
				count := (size + len(local) - 1) / len(local)
				docs := make([]*parser.Document, count)
				for i := range docs {
					docs[i] = parser.Parse(local + benchDefs)
					parseInlines(docs[i])
				}
				x := len(local) + strings.Index(benchDefs, "global")
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					for _, d := range docs {
						d.Edit(x, x+1, string(byte('a'+i%2)))
						benchmarkSink = parseInlines(d)
					}
				}
			})
		})
	}
}

func TestBenchmarkInputs(t *testing.T) {
	for _, size := range []int{100 * 1024, 1024 * 1024, 5 * 1024 * 1024} {
		src := benchSource(size)
		if len(src) != size {
			t.Fatal("benchmark size mismatch")
		}
		d := parser.Parse(src)
		blocks, lines := d.Len(), 0
		for i := 1; i < d.Len(); i++ {
			lines += d.LineCount(i)
		}
		parseInlines(d)
		at := size/2 + strings.Index(src[size/2:], "editable")
		d.Edit(at, at+1, "X")
		if d.HTML() != parser.Parse(src[:at]+"X"+src[at+1:]).HTML() {
			t.Fatal("benchmark edit differs from full parse")
		}
		t.Logf("bytes=%d blocks=%d content_lines=%d", size, blocks, lines)
	}
}
