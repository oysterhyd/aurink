package qa

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// A block marker typed alone (">", "-", "#") stays text with the caret after
// it, as in Typora; the space after it makes the block. Through the keys and
// through an input method's commit alike (Microsoft Pinyin commits with a
// range), the text then goes into the block, not before its marker.
func TestBlockMarkerThenSpaceThroughTheInputMethod(t *testing.T) {
	for _, m := range []struct{ marker, want string }{{">", "> ab"}, {"-", "- ab"}, {"#", "# ab"}} {
		for _, mode := range []string{"keys", "commit", "compose"} {
			a := newApp(t)
			a.open("q.md", "")
			a.caret(0)
			a.animate()
			x0, _, _ := a.s.ActiveLive().CaretDrawn()
			for i, s := range []string{m.marker, " ", "a", "b"} {
				l := a.s.ActiveLive()
				switch mode {
				case "keys":
					a.typed(s)
				case "commit":
					r := l.Selection().Range
					l.ReplaceText(&r, s)
					a.tt.Frame()
				case "compose":
					r := l.Selection().Range
					l.SetMarkedText(&r, s, r)
					a.tt.Frame()
					l.UnmarkText()
					a.tt.Frame()
				}
				a.animate()
				if i == 0 {
					if x, _, _ := l.CaretDrawn(); x <= x0 {
						t.Fatalf("%s %q: the caret stayed at %.1f after the marker (it was at %.1f)", mode, m.marker, x, x0)
					}
				}
			}
			if got := a.text(); got != m.want {
				t.Errorf("%s %q: %q, want %q", mode, m.marker, got, m.want)
			}
		}
	}
}

// The caret moves over a lone marker like over any text.
func TestCaretMovesOverALoneMarker(t *testing.T) {
	for _, doc := range []string{">", "-", "#"} {
		a := newApp(t)
		a.open("m.md", doc)
		a.caret(0)
		a.animate()
		a.key(0, ui.KeyRight)
		if h := a.s.ActiveSession().Model().Selection().Primary().Head; h != 1 {
			t.Errorf("%q: Right left the caret at %d", doc, h)
		}
		a.key(0, ui.KeyLeft)
		if h := a.s.ActiveSession().Model().Selection().Primary().Head; h != 0 {
			t.Errorf("%q: Left left the caret at %d", doc, h)
		}
	}
}
