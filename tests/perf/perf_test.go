// Package perf holds the benchmarks of QA-1.3, in a headless window over the real core.
package perf

import (
	"testing"

	"aurink/internal/contract"
	"aurink/internal/core/document"
	"aurink/internal/core/editing"
	"aurink/internal/core/projection"
	"aurink/internal/views/liveedit"
	"aurink/tests/corpus"
	"github.com/egoist/mygo/ui"
)

func openEditor(b testing.TB, src string) (*liveedit.Editor, *ui.Tester) {
	m := document.Factory{}.New("doc", src, contract.DocMeta{Dominant: "\n", Encoding: "utf-8"}, contract.ExtDefault)
	e := liveedit.New(liveedit.Options{Model: m, Projector: projection.New(), Editing: editing.New()})
	b.Cleanup(e.Close)
	e.Focus()
	tt := ui.NewTester(func(c *ui.Context) { e.View(c) }, 1280, 800)
	tt.Frame()
	return e, tt
}

func benchType(b *testing.B, size int) {
	src := corpus.Generate(size, 1)
	e, tt := openEditor(b, src)
	e.Model().SetSelection(contract.Caret(len(src) / 2))
	tt.Frame()
	b.ResetTimer()
	for range b.N {
		tt.Type("x")
		tt.Frame()
	}
}

func BenchmarkTypeFrame200KB(b *testing.B) { benchType(b, 200<<10) }
func BenchmarkTypeFrame1MB(b *testing.B)   { benchType(b, 1<<20) }

func BenchmarkScroll10kBlocks(b *testing.B) {
	src := ""
	for range 10000 {
		src += "a line of a paragraph\n\n"
	}
	e, tt := openEditor(b, src)
	e.ResetStats()
	b.ResetTimer()
	dir := 400.0
	for i := range b.N {
		if i%200 == 199 {
			dir = -dir
		}
		e.ScrollBy(dir)
		tt.Frame()
	}
	b.ReportMetric(float64(e.Stats.Shapes)/float64(b.N), "shapes/frame")
}

func BenchmarkOpen1MB(b *testing.B) {
	src := corpus.Generate(1<<20, 1)
	b.ResetTimer()
	for range b.N {
		openEditor(b, src)
	}
}

// Scrolling never reshapes a whole screen per frame (06 §8).
func TestScrollShapesNoWholeScreen(t *testing.T) {
	src := ""
	for range 10000 {
		src += "a line of a paragraph\n\n"
	}
	e, tt := openEditor(t, src)
	for i := range 300 {
		e.ResetStats()
		e.ScrollBy(120)
		tt.Frame()
		first, last := e.VisibleRows()
		if e.Stats.MaxFrameShapes > (last-first+1)*2 {
			t.Fatalf("frame %d shaped %d rows with %d in view", i, e.Stats.MaxFrameShapes, last-first+1)
		}
	}
}
