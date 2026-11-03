package qa

import (
	"strings"
	"testing"
	"time"

	"aurink/internal/contract"
	"aurink/locales"
)

// The smooth caret (Settings.Editor.SmoothCaret): it glides, then rests and
// asks for no more frames; off, or with motion reduced, it jumps.

func (a *app) caretX() float32 {
	x, _, _ := a.s.ActiveLive().CaretDrawn()
	return x
}

func TestSmoothCaretGlides(t *testing.T) {
	a := newApp(t)
	a.open("c.md", "hello world\n")
	a.caret(0)
	a.animate()
	start := a.caretX()
	a.caret(11)
	end := func() float32 {
		a.tt.Advance(time.Second)
		a.tt.Frame()
		return a.caretX()
	}
	a.tt.Advance(30 * time.Millisecond)
	a.tt.Frame()
	mid := a.caretX()
	if !(mid > start) || !a.tt.FramePending() {
		t.Fatalf("the caret did not glide: %v then %v, pending %v", start, mid, a.tt.FramePending())
	}
	stop := end()
	if !(stop > mid) {
		t.Fatalf("the caret stopped at %v, from %v", stop, mid)
	}
	a.tt.Advance(time.Second)
	a.tt.Frame()
	if a.caretX() != stop {
		t.Fatal("the caret moved at rest")
	}

	// Off, it jumps.
	if err := a.store.Update(func(v *contract.Settings) { v.Editor.SmoothCaret = false }); err != nil {
		t.Fatal(err)
	}
	a.animate()
	a.caret(0)
	if got := a.caretX(); got != start {
		t.Fatalf("with smooth caret off the caret is at %v, not %v", got, start)
	}
	// With motion reduced, too.
	if err := a.store.Update(func(v *contract.Settings) {
		v.Editor.SmoothCaret = true
		v.Appearance.ReduceMotion = "on"
	}); err != nil {
		t.Fatal(err)
	}
	a.animate()
	a.caret(11)
	if got := a.caretX(); got != stop {
		t.Fatalf("with motion reduced the caret is at %v, not %v", got, stop)
	}
}

// A jump farther than the view does not glide: the caret is there at once.
func TestSmoothCaretJumpsFar(t *testing.T) {
	a := newApp(t)
	a.open("long.md", strings.Repeat("line\n\n", 200)+"end\n")
	a.caret(0)
	a.animate()
	a.caret(len(a.text()) - 1)
	_, y, _ := a.s.ActiveLive().CaretDrawn()
	a.tt.Advance(time.Second)
	a.tt.Frame()
	// Rows measured as they come into view may still shift it a little.
	if _, rest, _ := a.s.ActiveLive().CaretDrawn(); y-rest > 40 || rest-y > 40 {
		t.Fatalf("the caret glided across the document: %v, then %v", y, rest)
	}
}

func TestSmoothCaretSetting(t *testing.T) {
	a := newApp(t)
	a.openSettings()
	a.category(1)
	x, y := a.control(locales.SettingsSmoothCaret(), locales.SettingsSmoothCaretHint(), 38)
	a.tt.ClickAt(x, y)
	a.settle()
	if a.settings().Editor.SmoothCaret {
		t.Fatal("the switch did not turn the smooth caret off")
	}
}
