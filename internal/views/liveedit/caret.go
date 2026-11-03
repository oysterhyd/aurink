package liveedit

import (
	"time"

	"github.com/egoist/mygo/ui"
)

// The smooth caret (Settings.Editor.SmoothCaret, 02 §4.4 item 7): the caret
// glides to where it goes instead of jumping, and has a faint glow.

const caretGlide = 90 * time.Millisecond

// caretMotion is where the caret is drawn and where it goes, in document
// coordinates (the view's y plus the scroll), so scrolling does not move it.
type caretMotion struct {
	from, to [3]float32 // x, y, height
	start    time.Time
	set      bool
	// drawn is where the caret was last painted, in view coordinates.
	drawn [3]float32
}

// SetSmoothCaret turns the gliding caret and its glow on or off.
func (e *Editor) SetSmoothCaret(on bool) { e.smoothCaret = on }

// SmoothCaret reports whether the caret glides.
func (e *Editor) SmoothCaret() bool { return e.smoothCaret }

// CaretDrawn returns where the caret was last painted, in the view: its
// left, top and height. For tests.
func (e *Editor) CaretDrawn() (x, y, h float32) {
	d := e.caret.drawn
	return d[0], d[1], d[2]
}

// gliding reports whether the caret moves with motion: smooth and motion not
// reduced.
func (e *Editor) gliding() bool { return e.smoothCaret && !e.reduceMotion }

// caretPlace returns where to paint the caret going to (x, y, h) in the view,
// now, and whether it is still on its way.
func (e *Editor) caretPlace(now time.Time, x, y, h float32) (float32, float32, float32, bool) {
	m := &e.caret
	scroll := float32(e.scroll)
	to := [3]float32{x, y + scroll, h}
	if !e.gliding() || !m.set {
		m.from, m.to, m.set = to, to, true
		return x, y, h, false
	}
	if to != m.to {
		cur, _ := m.at(now)
		// A jump past the view is not a glide.
		if d := cur[1] - to[1]; d > e.viewH || -d > e.viewH {
			cur = to
		}
		m.from, m.to, m.start = cur, to, now
	}
	p, moving := m.at(now)
	return p[0], p[1] - scroll, p[2], moving
}

// at returns the caret's place at now, on its way from from to to.
func (m *caretMotion) at(now time.Time) ([3]float32, bool) {
	f := float32(now.Sub(m.start)) / float32(caretGlide)
	if f >= 1 || f < 0 {
		return m.to, false
	}
	// Ease out: fast away, gentle at the end.
	k := 1 - (1-f)*(1-f)*(1-f)
	var p [3]float32
	for i := range p {
		p[i] = m.from[i] + (m.to[i]-m.from[i])*k
	}
	return p, true
}

// paintCaret paints the caret at (x, y, h) of the view, with its glow when
// smooth; a moving caret does not blink.
func (e *Editor) paintCaret(p *ui.Painter, r ui.Rect, x, y, h float32) {
	now := p.Now()
	x, y, h, moving := e.caretPlace(now, x, y, h)
	e.caret.drawn = [3]float32{x, y, h}
	if moving {
		e.blinkStart = now
		p.AnimationFrame()
	} else {
		const blink = 530 * time.Millisecond
		if phase := now.Sub(e.blinkStart); (phase/blink)%2 != 0 && phase >= 0 {
			return
		}
	}
	c := ui.Rect{X: r.X + x, Y: r.Y + y, W: max(1/p.Scale(), 1.5), H: h}
	if e.smoothCaret {
		p.Shadow(c, 1, 0, 0, 6, 0, e.t.Accent.Alpha(.45))
	}
	p.Fill(c, e.t.Accent, 0)
}
