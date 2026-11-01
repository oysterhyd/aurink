package liveedit

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// These log the measurements in REPORT.md. They assert only counts; wall-clock
// numbers go to the log, never to a verdict of the test.

func percentile(d []time.Duration, p float64) time.Duration {
	s := slices.Clone(d)
	slices.Sort(s)
	return s[min(int(float64(len(s))*p), len(s)-1)]
}

func TestMeasureKeystroke(t *testing.T) {
	for _, c := range []struct {
		name string
		src  string
	}{
		{"200 blocks", GenerateBlocks(200)},
		{"200 KB", GenerateBytes(200 << 10)},
		{"1 MB", GenerateBytes(1 << 20)},
		{"10k blocks", GenerateBlocks(10000)},
	} {
		h := open(t, c.src)
		mid := strings.Index(c.src[len(c.src)/2:], "\n\n") + len(c.src)/2 + 2
		h.caretAt(mid)
		h.tt.Frame()
		h.e.ResetStats()
		const keys = 600
		full := make([]time.Duration, 0, keys)
		for i := range keys {
			t0 := time.Now()
			if i%60 == 59 {
				h.tt.Key(0, ui.KeyEnter) // a paragraph, so one block does not grow forever
			} else {
				h.tt.Type(string(rune('a' + i%26)))
			}
			full = append(full, time.Since(t0))
		}
		s := h.e.Stats
		t.Logf("[实测] %-10s %8d B %6d blocks | Tester key+frame p50=%v p95=%v max=%v | shapes/key=%.2f maxFrameShapes=%d",
			c.name, len(c.src), h.e.BlockCount(), percentile(full, .5).Round(10*time.Microsecond), percentile(full, .95).Round(10*time.Microsecond),
			slices.Max(full).Round(10*time.Microsecond), float64(s.Shapes)/keys, s.MaxFrameShapes)

		// The engine alone: edit, parse, repartition, reshape, place.
		engine := make([]time.Duration, 0, keys)
		for i := range keys {
			t0 := time.Now()
			if i%60 == 59 {
				h.e.replaceSelection("\n\n", 1, false)
			} else {
				h.e.ReplaceText(nil, "x")
			}
			h.e.layoutView(h.e.viewW, h.e.viewH)
			engine = append(engine, time.Since(t0))
		}
		t.Logf("[实测] %-10s engine only (no raster) p50=%v p95=%v max=%v", c.name, percentile(engine, .5).Round(10*time.Microsecond),
			percentile(engine, .95).Round(10*time.Microsecond), slices.Max(engine).Round(10*time.Microsecond))
	}
}

func TestMeasureScroll(t *testing.T) {
	for _, n := range []int{200, 10000} {
		h := open(t, GenerateBlocks(n))
		first, last := h.e.VisibleBlocks()
		visible := last - first + 1
		h.e.ResetStats()
		hist := map[int]int{}
		frames := make([]time.Duration, 0, 300)
		h.e.OnPainted = func() {}
		steps := 0
		for range 300 {
			before := h.e.Stats.Shapes
			t0 := time.Now()
			h.tt.Scroll(500, 300, 0, 120)
			frames = append(frames, time.Since(t0))
			hist[h.e.Stats.Shapes-before]++
			steps++
		}
		s := h.e.Stats
		t.Logf("[实测] %5d blocks, %d visible, 120 DIP × %d: shapes=%d (%.2f/notch) maxFrameShapes=%d frames=%d laid=%d p50=%v p95=%v histogram(shapes→notches)=%v",
			h.e.BlockCount(), visible, steps, s.Shapes, float64(s.Shapes)/float64(steps), s.MaxFrameShapes, s.Frames, h.e.LaidBlocks(),
			percentile(frames, .5).Round(10*time.Microsecond), percentile(frames, .95).Round(10*time.Microsecond), hist)
	}
}

func BenchmarkTypeFrame200KB(b *testing.B) {
	e := New(GenerateBytes(200 << 10))
	tt := ui.NewTester(func(c *ui.Context) { e.View(c) }, 1000, 700)
	tt.Frame()
	e.setSelection(len(e.Source())/2, len(e.Source())/2)
	tt.Frame()
	for i := 0; b.Loop(); i++ {
		if i%60 == 59 {
			tt.Key(0, ui.KeyEnter)
		} else {
			tt.Type("x")
		}
	}
}

func BenchmarkScroll10kBlocks(b *testing.B) {
	e := New(GenerateBlocks(10000))
	tt := ui.NewTester(func(c *ui.Context) { e.View(c) }, 1000, 700)
	tt.Frame()
	dir := float32(120)
	for b.Loop() {
		if e.Scroll() >= e.maxScroll()-1 {
			dir = -120
		} else if e.Scroll() <= -padTop {
			dir = 120
		}
		tt.Scroll(500, 300, 0, dir)
	}
	b.ReportMetric(float64(e.Stats.Shapes)/float64(b.N), "shapes/op")
}
