package ui_test

import (
	"strings"
	"testing"

	"aurink/internal/contract"
	"aurink/internal/core/document"
	"aurink/internal/core/editing"
	"aurink/internal/core/projection"
	"aurink/internal/views/liveedit"
	"github.com/egoist/mygo/ui"
)

// openRail opens src with the structure rail beside the editor, as the shell
// shows it.
func openRail(t *testing.T, src string) *harness {
	t.Helper()
	m := document.Factory{}.New("doc", src, contract.DocMeta{Dominant: "\n", Encoding: "utf-8"}, 0)
	e := liveedit.New(liveedit.Options{Model: m, Projector: projection.New(), Editing: editing.New()})
	t.Cleanup(e.Close)
	e.Focus()
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Box(c).Fill().Children(func() {
			e.View(c.Key("live"))
			e.Rail(c.Key("rail"))
		})
	}, 1000, 700)
	tt.Frame()
	tt.Frame()
	return &harness{t, m, e, tt}
}

func sections(n int) string {
	var b strings.Builder
	for i := range n {
		b.WriteString("## Section ")
		b.WriteString(string(rune('A' + i%26)))
		b.WriteString("\n\n")
		for range 12 {
			b.WriteString("Some words to fill a paragraph of the section.\n\n")
		}
	}
	return b.String()
}

// TestRailGoesToAHeading marks each heading down the rail, and a click on a
// mark brings its heading to the top (FE-1.4).
func TestRailGoesToAHeading(t *testing.T) {
	h := openRail(t, sections(4))
	ticks := h.e.RailTicks()
	if len(ticks) != 4 || h.e.Stats.RailMarks != 4 {
		t.Fatalf("ticks %v, %d marks", ticks, h.e.Stats.RailMarks)
	}
	for i := 1; i < len(ticks); i++ {
		if ticks[i] <= ticks[i-1] {
			t.Fatalf("ticks out of order: %v", ticks)
		}
	}
	r, ok := h.tt.Find("Structure")
	if !ok {
		t.Fatal("no rail")
	}
	const box = 12
	h.tt.ClickAt(r.X+r.W-4, r.Y+ticks[2]*(r.H-box)+box/2)
	h.tt.Frame()
	h.tt.Frame()
	o := h.m.AST().Outline()
	if id, _ := h.e.VisibleHeading(); id != o[2].Block {
		t.Fatalf("the view is at heading %d, want %d", id, o[2].Block)
	}
	h.wantCaret(o[2].Range.Start)
	// A click between the marks goes nowhere.
	before := h.e.ScrollOffset()
	h.tt.ClickAt(r.X+r.W-4, r.Y+(ticks[0]+ticks[1])/2*(r.H-box)+box/2)
	h.tt.Frame()
	if h.e.ScrollOffset() != before {
		t.Fatal("a click between marks scrolled")
	}
}

// TestRailMarksFindHits adds the find ranges to the rail.
func TestRailMarksFindHits(t *testing.T) {
	h := openRail(t, sections(3))
	h.e.SetFindHighlights([]contract.Range{{Start: 0, End: 2}, {Start: 400, End: 402}, {Start: 900, End: 902}}, 0)
	h.tt.Frame()
	if h.e.Stats.RailMarks != 3+3 {
		t.Fatalf("%d marks", h.e.Stats.RailMarks)
	}
}

// TestRailHidesWithOneHeading: one heading is no structure.
func TestRailHidesWithOneHeading(t *testing.T) {
	h := openRail(t, sections(1))
	if _, ok := h.tt.Find("Structure"); ok {
		t.Fatal("the rail shows for one heading")
	}
}

// TestRailDoesNotLayOutTheDocument places 1,000 headings of a 10,000-block
// document from the heights index: a frame lays out nothing new (07 FE-1.4).
func TestRailDoesNotLayOutTheDocument(t *testing.T) {
	var b strings.Builder
	for i := range 10000 {
		if i%10 == 0 {
			b.WriteString("## Heading\n\n")
		} else {
			b.WriteString("A line of the document.\n\n")
		}
	}
	h := openRail(t, b.String())
	h.e.ResetStats()
	h.tt.Frame()
	if h.e.Stats.RailMarks != 1000 {
		t.Fatalf("%d marks", h.e.Stats.RailMarks)
	}
	if h.e.Stats.Shapes != 0 {
		t.Fatalf("the rail's frame laid out %d rows", h.e.Stats.Shapes)
	}
	// Scrolling to the end lays out only what comes into view.
	h.e.ScrollBy(1e9)
	h.tt.Frame()
	if n := h.e.Stats.Shapes; n > 200 {
		t.Fatalf("scrolling to the end laid out %d rows", n)
	}
}
