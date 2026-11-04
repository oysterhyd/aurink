package qa

import "testing"

// The title bar's mark is the design's (Main): the arch in the aurora
// gradient, teal at its left foot and pink at its right, over a dark drop of
// ink, not one flat colour.
func TestTitleBarMarkHasTheAuroraGradient(t *testing.T) {
	a := newApp(t)
	a.open("a.md", "text\n")
	a.animate()
	img := a.tt.Image()
	var teal, pink, ink int
	for x := 0; x < 120; x++ {
		for y := 0; y < 46; y++ {
			c := img.RGBAAt(x, y)
			r, g, b := int(c.R), int(c.G), int(c.B)
			switch {
			case g > r+60 && b > r+40:
				teal++
			case r > g+60 && b > g+20:
				pink++
			case r < 60 && g < 60 && b < 60:
				ink++
			}
		}
	}
	if teal < 3 || pink < 3 || ink < 3 {
		t.Fatalf("the mark has %d teal, %d pink and %d ink pixels", teal, pink, ink)
	}
}
