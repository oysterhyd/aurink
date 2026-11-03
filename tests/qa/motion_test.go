package qa

import (
	"strings"
	"testing"
	"time"

	"aurink/internal/contract"
	"aurink/internal/views/motion"
	"aurink/locales"
	"github.com/egoist/mygo/ui"
)

// 02 §4.4 rule 5: with motion reduced, by the app's setting while the
// system's is off, nothing slides: the tab pill and the outline's pill jump.
func TestPillsSlideUnlessMotionIsReduced(t *testing.T) {
	for _, still := range []bool{false, true} {
		a := newApp(t)
		if still {
			if err := a.store.Update(func(v *contract.Settings) { v.Appearance.ReduceMotion = "on" }); err != nil {
				t.Fatal(err)
			}
		}
		a.open("a.md", "# a\n")
		a.open("b.md", "# b\n")
		a.animate()
		a.click("a.md")
		a.tt.Frame()
		if a.tt.FramePending() == still {
			t.Fatalf("still %v: the tab pill slid %v", still, a.tt.FramePending())
		}
		a.animate()

		var b strings.Builder
		for i := range 40 {
			b.WriteString("# Heading " + string(rune('A'+i%26)) + "\n\ntext\n\n")
		}
		a.open("long.md", b.String())
		a.run(contract.CmdViewOutline)
		a.animate()
		a.tt.Scroll(640, 400, 0, 600)
		a.tt.Frame()
		a.tt.Frame()
		moving := a.tt.FramePending()
		a.animate()
		if moving == still {
			t.Fatalf("still %v: the outline's pill slid %v", still, moving)
		}
	}
}

func TestMotionStill(t *testing.T) {
	in := ui.ElementTransition{Animation: &motion.Snappy, Match: 1,
		Enter: &ui.Motion{Y: 12, Scale: .9, Opacity: 0}, Exit: &ui.Motion{Collapse: true, Opacity: 0}}
	if got := motion.Still(false, in); got.Match != 1 || got.Enter.Y != 12 {
		t.Fatalf("moving: %+v", got)
	}
	got := motion.Still(true, in)
	if got.Match != nil || got.Enter.Y != 0 || got.Enter.Scale != 0 || got.Exit.Collapse || !got.Colors || got.Position || got.Size {
		t.Fatalf("still: %+v", got)
	}
	_ = time.Second
}

// Pressed buttons spring (rule 7) and the palette flies from the omnibox
// (rule 1), unless motion is reduced (rule 5).
func TestPressAndPaletteUnlessMotionIsReduced(t *testing.T) {
	for _, still := range []bool{false, true} {
		a := newApp(t)
		if still {
			if err := a.store.Update(func(v *contract.Settings) { v.Appearance.ReduceMotion = "on" }); err != nil {
				t.Fatal(err)
			}
		}
		a.animate()
		r, ok := a.tt.Find(locales.ShellNew())
		if !ok {
			t.Fatal("no New button")
		}
		a.tt.Press(r.X+r.W/2, r.Y+r.H/2)
		a.tt.Frame()
		if a.tt.FramePending() == still {
			t.Fatalf("still %v: the press sprang %v", still, a.tt.FramePending())
		}
		a.tt.Release(r.X+r.W/2+400, r.Y+r.H/2+200)
		a.animate()
		a.run(contract.CmdNavQuickOpen)
		a.tt.Frame()
		if a.tt.FramePending() == still {
			t.Fatalf("still %v: the palette flew %v", still, a.tt.FramePending())
		}
		a.key(0, ui.KeyEscape)
		a.animate()
	}
}
