package qa

import (
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
