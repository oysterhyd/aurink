package shell

import (
	"time"

	"aurink/internal/views/motion"
	"aurink/internal/views/theme"
	"github.com/egoist/mygo/ui"
)

// The document skeleton (02 §5.16, Loading.png): a document that takes more
// than 100 ms to open shows the shape of a page with a light sweeping across
// it until it comes. Faster ones show nothing.

const (
	loadingDelay  = 100 * time.Millisecond
	loadingSweep  = 1400 * time.Millisecond
	loadingColumn = 640
)

// opening is the document being opened, and when the first frame saw it.
type opening struct {
	path  string
	since time.Time
	shown bool
}

func (s *Shell) startOpening(path string) {
	s.opening = opening{path: path}
	s.invalidate()
}

func (s *Shell) doneOpening(path string) {
	if s.opening.path == path {
		s.opening = opening{}
		s.invalidate()
	}
}

// LoadingShown reports whether the document skeleton showed in the last
// frame, for tests.
func (s *Shell) LoadingShown() bool { return s.opening.shown }

// loading draws the skeleton over the sheet while a document opens.
func (s *Shell) loading(c *ui.Context, t *theme.Tokens) {
	o := &s.opening
	o.shown = false
	if o.path == "" {
		return
	}
	now := c.Now()
	if o.since.IsZero() {
		o.since = now
	}
	if wait := loadingDelay - now.Sub(o.since); wait > 0 {
		c.After(wait)
		return
	}
	o.shown = true
	still := s.reduceMotion(c)
	page := ui.Box(c.Key("loading")).Absolute().Fill().Background(t.Sheet)
	page.Transition(ui.ElementTransition{Animation: &motion.Quick, Exit: &ui.Motion{Opacity: 0}})
	page.Draw(func(p *ui.Painter, r ui.Rect) {
		w := min(loadingColumn, r.W-96)
		x, y := r.X+(r.W-w)/2, r.Y+88
		bar := func(dy, fw, h float32) {
			p.Fill(ui.Rect{X: x, Y: y + dy, W: w * fw, H: h}, t.Surface, h/2)
		}
		p.Fill(ui.Rect{X: x, Y: y, W: w * .6, H: 40}, t.Surface, 8)
		bar(68, 1, 14)
		bar(96, .92, 14)
		bar(124, .74, 14)
		p.Fill(ui.Rect{X: x, Y: y + 174, W: w * .35, H: 22}, t.Surface, 6)
		bar(216, .96, 14)
		bar(244, .66, 14)
		if still {
			return
		}
		// The light: a soft band moving across the column, over the bars.
		f := float32(p.Now().UnixMilli()%loadingSweep.Milliseconds()) / float32(loadingSweep.Milliseconds())
		half := w * .2
		bx := x - 2*half + (w+2*half)*f
		area := ui.Rect{X: x, Y: y, W: w, H: 260}
		p.Clip(area, 0, func() {
			p.FillGradient(ui.Rect{X: bx, Y: y, W: half, H: 260}, ui.LinearGradient{From: t.Sheet.Alpha(0), To: t.Sheet.Alpha(.7), Angle: 90}, 0)
			p.FillGradient(ui.Rect{X: bx + half, Y: y, W: half, H: 260}, ui.LinearGradient{From: t.Sheet.Alpha(.7), To: t.Sheet.Alpha(0), Angle: 90}, 0)
		})
		p.AnimationFrame()
	})
}
