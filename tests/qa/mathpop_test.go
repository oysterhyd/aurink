package qa

import (
	"testing"
)

// bubble counts the pixels of the inline math preview: its near-black fill,
// and the fake rendering (#336) on it.
func (a *app) bubble() (fill, formula int) {
	img := a.tt.Image()
	b := img.Bounds()
	for y := 46; y < b.Max.Y; y++ {
		for x := 0; x < b.Max.X; x++ {
			c := img.RGBAAt(x, y)
			r, g, bl := int(c.R), int(c.G), int(c.B)
			switch {
			case r < 14 && g < 14 && bl < 16:
				fill++
			case r > 40 && r < 64 && g > 40 && g < 64 && bl > 90 && bl < 115:
				formula++
			}
		}
	}
	return fill, formula
}

// With the caret in inline math, or just after it, its rendering floats
// under the source on a dark bubble, as in Typora; elsewhere there is none.
func TestInlineMathPreviewBubble(t *testing.T) {
	a := newApp(t)
	a.render.available = true
	a.open("m.md", "x $abc$ y\n\nnext\n")
	a.caret(0)
	a.animate()
	if fill, _ := a.bubble(); fill > 0 {
		t.Fatalf("a bubble with the caret outside: %d pixels", fill)
	}
	for _, at := range []int{5, 7} { // inside, and just after the closing $
		a.caret(at)
		a.until("the bubble", func() bool {
			a.animate()
			fill, formula := a.bubble()
			return fill > 200 && formula > 20
		})
	}
	a.caret(9)
	a.animate()
	if fill, _ := a.bubble(); fill > 0 {
		t.Fatalf("the bubble stayed after the caret left: %d pixels", fill)
	}
}
