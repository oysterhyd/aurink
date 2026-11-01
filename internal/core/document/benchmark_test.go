package document

import (
	"slices"
	"strings"
	"testing"
	"time"

	"aurink/internal/contract"
	"aurink/tests/corpus"
)

type ordinaryCase struct {
	name  string
	at    int
	value string
}

func ordinaryCases(t testing.TB, source string) []ordinaryCase {
	t.Helper()
	// Use ordinary content near the middle of the generated corpus, including
	// structural edits that cannot use the parser's paragraph typing shortcut.
	start := len(source) / 2
	paragraph := strings.Index(source[start:], "段落 ")
	code := strings.Index(source[start:], "func f")
	list := strings.Index(source[start:], "- 项 ")
	if paragraph < 0 || code < 0 || list < 0 {
		t.Fatal("generated corpus lacks benchmark cases")
	}
	paragraph += start
	code += start
	list += start
	paragraph += strings.Index(source[paragraph:], "，") + len("，中文")
	list += strings.IndexByte(source[list:], '\n')
	return []ordinaryCase{
		{"typing_paragraph", paragraph, "a"},
		{"typing_code", code + len("func f"), "a"},
		{"enter_paragraph", paragraph, "\n\n"},
		{"enter_list", list, "\n- "},
	}
}

var benchmarkEpoch = time.Now()
var benchmarkNow = func() int64 { return time.Since(benchmarkEpoch).Nanoseconds() }

func BenchmarkApply(b *testing.B) { benchmarkEdit(b, false) }
func BenchmarkUndo(b *testing.B)  { benchmarkEdit(b, true) }

func benchmarkEdit(b *testing.B, undo bool) {
	source := corpus.Generate(200<<10, 1331)
	for _, c := range ordinaryCases(b, source) {
		b.Run(c.name, func(b *testing.B) {
			m := newModel(b, source)
			m.clock = time.Now
			m.SetSelection(contract.Caret(c.at))
			events, dirty, changes := 0, 0, 0
			m.Subscribe(func(e contract.DocEvent) {
				events++
				dirty += len(e.Dirty)
				changes += len(e.Changes)
			})
			tx := edit(c.at, c.at, c.value)
			samples := make([]int64, b.N)
			adapted, full := 0, 0
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if undo {
					b.StopTimer()
					apply(b, m, tx)
					b.StartTimer()
				}
				start := benchmarkNow()
				if undo {
					if !m.Undo() {
						b.Fatal("Undo failed")
					}
				} else {
					apply(b, m, tx)
				}
				samples[i] = benchmarkNow() - start
				adapted += m.lastUpdate.AdaptedBlocks
				full += m.lastUpdate.FullRebuilds
				if !undo {
					b.StopTimer()
					if !m.Undo() {
						b.Fatal("reset Undo failed")
					}
					b.StartTimer()
				}
			}
			b.StopTimer()
			slices.Sort(samples)
			p95 := samples[min(len(samples)-1, len(samples)*95/100)]
			b.ReportMetric(float64(samples[len(samples)/2]), "p50-ns/op")
			b.ReportMetric(float64(p95), "p95-ns/op")
			b.ReportMetric(float64(adapted)/float64(b.N), "adapted/op")
			b.ReportMetric(float64(full)/float64(b.N), "full-parses/op")
			if p95 > int64(4*time.Millisecond) {
				b.Errorf("p95 %s exceeds 4 ms", time.Duration(p95))
			}
			if events < 2*b.N || dirty == 0 || changes != 2*b.N || full != 0 {
				b.Fatal("benchmark did not exercise incremental edits and events", events, dirty, changes, full)
			}
		})
	}
}
