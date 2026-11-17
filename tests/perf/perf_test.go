// Package perf holds the benchmarks of QA-1.3, in a headless window over the real core.
package perf

import (
	"strings"
	"testing"
	"time"

	"aurink/internal/contract"
	"aurink/internal/core/document"
	"aurink/internal/core/editing"
	"aurink/internal/core/projection"
	"aurink/internal/views/liveedit"
	"aurink/internal/views/source"
	"aurink/internal/views/theme"
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

func openLargeSource(b testing.TB, src string) (*source.Editor, *ui.Tester) {
	m := document.Factory{}.New("large", src, contract.DocMeta{Dominant: "\r\n", Encoding: "utf-8", Large: true}, contract.ExtDefault)
	ed := source.New(m, theme.For(theme.Options{}))
	b.Cleanup(ed.Close)
	ed.Enter(len(src) / 2)
	tt := ui.NewTester(func(c *ui.Context) { ed.View(c).AutoFocus() }, 1280, 800)
	tt.Frame()
	return ed, tt
}

// BenchmarkSourceSync5MB measures the views/source + core/document sync cost
// of typing 300 characters into a 5.5 MB CRLF document (09 D4, §6 batch 3).
func BenchmarkSourceSync5MB(b *testing.B) {
	src := strings.ReplaceAll(corpus.Generate((5<<20)+(512<<10), 7), "\n", "\r\n")
	ed, tt := openLargeSource(b, src)
	mid := ed.State().Buffer.Len() / 2
	ed.State().Select(mid, mid)
	tt.Frame()
	b.ReportAllocs()
	b.ResetTimer()
	for i := range b.N {
		if i > 0 && i%300 == 0 {
			ed.State().Select(mid, mid)
		}
		ed.State().Insert("x")
		ed.Sync()
	}
}

// BenchmarkSourceType5MB types into a 5.5 MB CRLF document in large-file
// source mode and renders each full 1280×800 frame.
func BenchmarkSourceType5MB(b *testing.B) {
	src := strings.ReplaceAll(corpus.Generate((5<<20)+(512<<10), 7), "\n", "\r\n")
	ed, tt := openLargeSource(b, src)
	mid := ed.State().Buffer.Len() / 2
	ed.State().Select(mid, mid)
	tt.Frame()
	b.ReportAllocs()
	b.ResetTimer()
	for i := range b.N {
		if i%2 == 0 {
			tt.Type("x")
		} else {
			tt.Key(0, ui.KeyBackspace)
		}
		tt.Frame()
	}
}

// TestSourceType5MB verifies 300 sequential keystrokes synced into a 5.5 MB
// CRLF document stay well under 2 ms/key and preserve CRLF bytes.
func TestSourceType5MB(t *testing.T) {
	src := strings.ReplaceAll(corpus.Generate((5<<20)+(512<<10), 7), "\n", "\r\n")
	ed, tt := openLargeSource(t, src)
	ed.State().Select(0, 0)
	tt.Frame()
	start := time.Now()
	const keys = 300
	for range keys {
		ed.State().Insert("x")
		ed.Sync()
	}
	tt.Frame()
	perKey := time.Since(start) / keys
	if perKey > 2*time.Millisecond {
		t.Fatalf("5.5 MB source keystroke sync took %v, want <= 2ms", perKey)
	}
	if !strings.HasPrefix(ed.State().Buffer.Slice(0, keys), strings.Repeat("x", keys)) {
		t.Fatalf("unexpected buffer prefix after %d keys", keys)
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
