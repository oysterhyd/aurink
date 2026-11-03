package qa

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"aurink/internal/contract"
)

// The document skeleton (02 §5.16): a document slower than 100 ms to open
// shows the shape of a page until it comes; a quicker one shows nothing.
func TestLoadingSkeleton(t *testing.T) {
	a := newApp(t)
	p := a.file("slow.md", "# Slow\n\ntext\n")
	a.s.Open(p) // the answer waits in the queue until settle
	a.tt.Frame()
	a.tt.Advance(50 * time.Millisecond)
	a.tt.Frame()
	if a.s.LoadingShown() {
		t.Fatal("the skeleton showed before 100 ms")
	}
	a.tt.Advance(100 * time.Millisecond)
	a.tt.Frame()
	if !a.s.LoadingShown() || !a.tt.FramePending() {
		t.Fatalf("no sweeping skeleton after 150 ms: shown %v", a.s.LoadingShown())
	}
	a.settle()
	a.tt.Advance(time.Second)
	a.tt.Frame()
	if a.s.LoadingShown() || a.tt.FramePending() {
		t.Fatal("the skeleton stayed after the document came")
	}
	a.want(a.text(), "# Slow\n\ntext\n")
}

func TestLoadingQuickShowsNothing(t *testing.T) {
	a := newApp(t)
	p := a.file("quick.md", "quick\n")
	a.s.Open(p)
	a.tt.Frame()
	a.settle()
	a.tt.Advance(200 * time.Millisecond)
	a.tt.Frame()
	if a.s.LoadingShown() {
		t.Fatal("a quick open showed the skeleton")
	}
}

func TestLoadingStillWithMotionReduced(t *testing.T) {
	a := newApp(t)
	if err := a.store.Update(func(v *contract.Settings) { v.Appearance.ReduceMotion = "on" }); err != nil {
		t.Fatal(err)
	}
	a.animate()
	p := a.file("slow.md", "x\n")
	a.s.Open(p)
	a.tt.Frame()
	a.tt.Advance(150 * time.Millisecond)
	a.tt.Frame()
	if !a.s.LoadingShown() || a.tt.FramePending() {
		t.Fatalf("with motion reduced: shown %v, pending %v", a.s.LoadingShown(), a.tt.FramePending())
	}
	a.settle()
}

// Scrolled, the sheet's top edge fades the text into it (02 §6); at the
// top there is no edge.
func TestScrollEdge(t *testing.T) {
	a := newApp(t)
	var b strings.Builder
	for i := range 120 {
		fmt.Fprintf(&b, "line %d of a long document\n\n", i)
	}
	a.open("long.md", b.String())
	a.caret(0)
	a.animate()
	if a.s.ScrollEdgeShown() {
		t.Fatal("an edge at the top of the document")
	}
	l := a.s.ActiveLive()
	a.tt.Scroll(l.RowText(3).X+a.s.SheetLeft(), 300, 0, 400)
	a.animate()
	if l.ScrollOffset() <= 0 || !a.s.ScrollEdgeShown() || a.tt.FramePending() {
		t.Fatalf("scrolled %v: edge %v, pending %v", l.ScrollOffset(), a.s.ScrollEdgeShown(), a.tt.FramePending())
	}
}

// The welcome page and an empty document glow (02 §6); without a GPU, as
// here, the glow is a still gradient and asks for no frames.
func TestAurora(t *testing.T) {
	a := newApp(t)
	a.tt.Frame()
	if !a.s.AuroraShown() {
		t.Fatal("no aurora on the welcome page")
	}
	img := a.tt.Image()
	if r, g, b, _ := img.At(60, 80).RGBA(); r == g && g == b {
		t.Fatal("the welcome page's corner is not tinted")
	}
	a.tt.Advance(time.Second)
	a.tt.Frame()
	if a.tt.FramePending() {
		t.Fatal("the still aurora asks for frames")
	}
	a.open("e.md", "")
	a.tt.Frame()
	if !a.s.AuroraShown() {
		t.Fatal("no aurora on the empty document")
	}
	a.typed("x")
	a.tt.Frame()
	if a.s.AuroraShown() {
		t.Fatal("the aurora stayed once there was text")
	}
}
