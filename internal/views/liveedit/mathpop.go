package liveedit

import (
	"aurink/internal/contract"
	"aurink/internal/views/theme"
	"github.com/egoist/mygo/ui"
)

// The inline math preview (as Typora does): while the caret is in a $…$, or
// just after it, the formula shows as source and its rendering floats under
// it on a dark bubble that follows the typing. It covers the text below for
// as long as the caret stays and takes no room in the layout.

const (
	mathPopPadX  = 14
	mathPopPadY  = 10
	mathPopArrow = 6
	mathPopGap   = 4
)

var (
	mathPopFill = ui.Hex("#0B0B0D")
	mathPopInk  = ui.Hex("#FFFFFF")
)

// mathAtCaret is the inline math the caret is in or just after; its row too.
func (e *Editor) mathAtCaret() (contract.Inline, int, bool) {
	if !e.focused || e.anchor != e.head || e.caretRow < 0 || e.caretRow >= len(e.rows) {
		return contract.Inline{}, 0, false
	}
	c := e.head
	for _, x := range e.ast.Inlines(e.rows[e.caretRow].leaf) {
		if x.Kind == contract.InlineMath && c > x.Range.Start && c <= x.Range.End {
			return x, e.caretRow, true
		}
	}
	return contract.Inline{}, 0, false
}

// mathPop is what the bubble shows: the rendering, the last good one while a
// new one renders, or the error; nil for no bubble.
func (e *Editor) mathPop() (x contract.Inline, row int, st *renderState) {
	x, row, ok := e.mathAtCaret()
	if !ok {
		return x, 0, nil
	}
	src := e.text.Slice(contract.Range{Start: x.Open.End, End: x.Close.Start})
	if src == "" {
		return x, 0, nil
	}
	o := &contract.ObjectRef{Kind: contract.ObjMath, Source: src}
	st = e.renderedIn(o, e.ty.BodyFont().Size, mathPopInk, true)
	if st == nil || st.failed {
		return x, 0, nil
	}
	if e.previews == nil {
		e.previews = map[contract.BlockID]*renderState{}
	}
	// A paragraph has no block preview: its ID keeps the bubble's last good one.
	id := e.block(row).ID
	switch {
	case st.img != nil:
		e.previews[id] = st
	case st.loading:
		if last := e.previews[id]; last != nil && last.img != nil {
			st = last
		}
	}
	return x, row, st
}

// paintMathPop paints the bubble under the formula holding the caret.
func (e *Editor) paintMathPop(p *ui.Painter, r ui.Rect) {
	x, row, st := e.mathPop()
	if st == nil {
		return
	}
	l := e.current(row)
	idx := e.opts.Projector.ToDisplay(&l.proj, clampTo(x.Range.Start, e.block(row).Range), contract.Downstream)
	sx, _, _ := e.boundsAt(row, l, idx)
	_, cy, ch := e.caretRect()
	var w, h float32
	var errText *ui.TextLayout
	switch {
	case st.img != nil:
		w, h = st.res.Width, st.res.Height
		if lim := e.width - 2*mathPopPadX; w > lim {
			w, h = lim, h*lim/w
		}
	case st.loading:
		w, h = 40, e.ty.Body
	default:
		errText = e.shaped(st.err, ui.Font{Family: theme.Sans, Size: theme.UIStatus})
		w, h = errText.Size()
		w = min(w, e.width-2*mathPopPadX)
	}
	bw, bh := w+2*mathPopPadX, h+2*mathPopPadY
	bx := r.X + max(e.contentX(), min(sx, e.contentX()+e.width-bw))
	by := r.Y + cy + ch + mathPopGap + mathPopArrow
	ax := min(max(r.X+sx+12, bx+12), bx+bw-12)
	p.FillPath(new(ui.Path).MoveTo(ax-mathPopArrow, by).LineTo(ax, by-mathPopArrow).LineTo(ax+mathPopArrow, by).Close(), mathPopFill)
	box := ui.Rect{X: bx, Y: by, W: bw, H: bh}
	p.Fill(box, mathPopFill, 6)
	in := ui.Rect{X: bx + mathPopPadX, Y: by + mathPopPadY, W: w, H: h}
	switch {
	case st.img != nil:
		p.Image(st.img, in, ui.Contain)
	case st.loading:
		p.Fill(ui.Rect{X: in.X, Y: in.Y + h/2 - 1, W: w, H: 2}, mathPopInk.Alpha(.3), 1)
	default:
		p.Clip(in, 0, func() { p.TextLayout(errText, in.X, in.Y, e.t.Danger) })
	}
}
