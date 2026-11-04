package qa

import (
	"strings"
	"testing"

	"aurink/locales"
	"github.com/egoist/mygo/ui"
)

// teal counts the accent-coloured pixels in the capsule left of its label.
func (a *app) teal(label string) int {
	a.t.Helper()
	r, ok := a.tt.Find(label)
	if !ok {
		a.t.Fatalf("no %q", label)
	}
	img := a.tt.Image()
	n := 0
	for x := int(r.X) - 40; x < int(r.X); x++ {
		for y := int(r.Y); y < int(r.Y+r.H); y++ {
			c := img.RGBAAt(x, y)
			if int(c.G) > int(c.R)+50 && int(c.B) > int(c.R)+30 {
				n++
			}
		}
	}
	return n
}

// The status capsule has the scroll progress ring (02 §3): the accent part
// grows as the document scrolls.
func TestCapsuleRingFollowsTheScroll(t *testing.T) {
	a := newApp(t)
	a.open("long.md", strings.Repeat("para\n\n", 200))
	a.animate()
	at0 := a.teal(locales.ShellSaved())
	l := a.s.ActiveLive()
	a.tt.Scroll(l.RowText(3).X+a.s.SheetLeft(), 300, 0, 3000)
	a.animate()
	if l.ScrollFraction() <= 0 {
		t.Fatal("the document did not scroll")
	}
	if got := a.teal(locales.ShellSaved()); got <= at0+3 {
		t.Fatalf("the ring has %d accent pixels at the top and %d after a scroll", at0, got)
	}
}

// Unsaved shows a hollow dot (02 §5.5): its centre is not the warning colour.
func TestCapsuleUnsavedDotIsHollow(t *testing.T) {
	a := newApp(t)
	a.open("a.md", "text\n")
	a.caret(4)
	a.typed("x")
	a.animate()
	r, ok := a.tt.Find(locales.ShellUnsaved())
	if !ok {
		t.Fatal("not unsaved")
	}
	img := a.tt.Image()
	// The dot sits left of the label, 8 apart: its centre and its rim.
	y := int(r.Y + r.H/2)
	cx := int(r.X) - 8 - 4
	rim, centre := 255, img.RGBAAt(cx, y)
	for x := cx - 5; x <= cx+5; x++ {
		c := img.RGBAAt(x, y)
		rim = min(rim, int(c.B))
	}
	// The warning colour is orange: little blue. The rim has it, the centre not.
	if rim > 150 || centre.B < 150 {
		t.Fatalf("dot: rim blue %d, centre %v", rim, centre)
	}
	_ = ui.KeyEnter
}

// Hovered, the capsule opens to the reading time and the caret's block (02 §3).
func TestCapsuleHoverShowsReadingTime(t *testing.T) {
	a := newApp(t)
	a.open("a.md", "# A\n\none\n\ntwo\n")
	a.caret(len("# A\n\non"))
	a.animate()
	want := locales.ShellCapsuleDetail(1, 2, 3)
	if a.shows(want) {
		t.Fatal("the detail shows before a hover")
	}
	r, ok := a.tt.Find(locales.ShellSaved())
	if !ok {
		t.Fatal("no capsule")
	}
	a.tt.Move(r.X+r.W/2, r.Y+r.H/2)
	a.tt.Frame()
	if !a.shows(want) {
		t.Fatalf("hovered, the capsule does not say %q", want)
	}
}
