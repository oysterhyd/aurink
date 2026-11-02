package liveedit

import (
	"aurink/internal/views/editorapi"
	"github.com/egoist/mygo/ui"
)

// railHitsMax caps the find hits the rail marks: past it they would blur into
// a bar anyway.
const railHitsMax = 1000

// railTick is a heading's mark on the structure rail (02 §5.3, FE-1.4).
type railTick struct {
	y    float32 // 0–1 down the document
	w    float32
	off  int
	text string
	cur  bool
}

// railMarks places the headings and the find hits down the document. It reads
// the heights index only: no row is laid out for it, whatever the length.
func (e *Editor) railMarks() (ticks []railTick, hits []float32) {
	if len(e.rows) == 0 {
		return nil, nil
	}
	total := e.hs.top(len(e.rows))
	if total <= 0 {
		return nil, nil
	}
	at := func(off int) float32 {
		return float32(max(0, min(e.rowTop(e.rowAt(off))/total, 1)))
	}
	cur, _ := e.VisibleHeading()
	for _, h := range e.ast.Outline() {
		w := float32(6)
		switch h.Level {
		case 1:
			w = 12
		case 2:
			w = 9
		}
		if h.Block == cur {
			w = 16
		}
		ticks = append(ticks, railTick{y: at(h.Range.Start), w: w, off: h.Range.Start, text: h.Text, cur: h.Block == cur})
	}
	for i, r := range e.find {
		if i == railHitsMax {
			break
		}
		hits = append(hits, at(r.Start))
	}
	return ticks, hits
}

// Rail shows the document's headings down the right edge, with the view's
// place and the find hits; a click on a heading's mark goes to it. It shows for
// a document of two headings or more.
func (e *Editor) Rail(c *ui.Context) {
	ticks, hits := e.railMarks()
	if len(ticks) < 2 || e.viewH <= 0 {
		return
	}
	t := e.t
	h := e.viewH * .7
	const tickBox = 12
	rail := ui.Box(c).Absolute().Right(16).Top(e.viewH * .15).Width(18).Height(h).Label("Structure")
	// The mark nearest the pointer, within half a mark's box, takes the hover.
	near := -1
	if _, py, ok := rail.PointerPosition(); ok {
		best := float32(tickBox / 2)
		for i, k := range ticks {
			if d := abs32(k.y*(h-tickBox) + tickBox/2 - py); d <= best {
				near, best = i, d
			}
		}
	}
	if near >= 0 {
		rail.Tooltip(ticks[near].text).Cursor(ui.CursorPointer)
		if rail.Clicked() {
			e.RevealOffset(ticks[near].off, editorapi.AlignTop)
			e.Focus()
		}
	}
	frac := e.ScrollFraction()
	rail.Draw(func(p *ui.Painter, r ui.Rect) {
		thumb := r.H * .14
		p.Fill(ui.Rect{X: r.X + r.W - 3, Y: r.Y + frac*(r.H-thumb), W: 3, H: thumb}, t.SurfaceHover, 1.5)
		for i, k := range ticks {
			w, col := k.w, t.Faint
			if k.cur {
				col = t.Accent
			}
			if i == near && !k.cur {
				w, col = max(w, 12), t.Muted
			}
			y := r.Y + k.y*(r.H-tickBox) + tickBox/2 - 1
			p.Fill(ui.Rect{X: r.X + r.W - w, Y: y, W: w, H: 2}, col, 1)
		}
		for _, y := range hits {
			p.Fill(ui.Rect{X: r.X + r.W - 7, Y: r.Y + y*(r.H-tickBox) + tickBox/2 - 1.5, W: 7, H: 3}, t.Warning, 1.5)
		}
	})
	e.Stats.RailMarks = len(ticks) + len(hits)
}

func abs32(x float32) float32 {
	if x < 0 {
		return -x
	}
	return x
}

// RailTicks returns where the rail marks the headings, 0 to 1 down its track,
// for tests.
func (e *Editor) RailTicks() []float32 {
	ticks, _ := e.railMarks()
	out := make([]float32, len(ticks))
	for i, k := range ticks {
		out[i] = k.y
	}
	return out
}
