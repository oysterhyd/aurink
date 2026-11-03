package liveedit

import (
	"slices"
	"strings"

	"aurink/internal/contract"
	"aurink/internal/views/theme"
	"github.com/egoist/mygo/ui"
)

// The live preview (02 §5.9, as Typora does): a math block or a diagram
// whose source shows, because the caret is in it, keeps its rendering under
// the source, which follows the typing. While a new version renders, the last
// good one stays; a source that does not render shows its error instead.

const (
	previewGap  = 10
	previewMaxH = 360
)

// previewObject is the object a revealed math or diagram block renders as,
// with the source the projection gives it unrevealed, so that both share a
// rendering; nil for the other blocks, or a block shown as its object.
func (e *Editor) previewObject(b *contract.Block, p *contract.BlockProjection) *contract.ObjectRef {
	if blockObject(p) != nil {
		return nil
	}
	o := &contract.ObjectRef{Display: true}
	switch b.Kind {
	case contract.BlockMath:
		o.Kind = contract.ObjMath
	case contract.BlockCode:
		if b.Lang == "" || !slices.Contains(e.opts.ObjectLangs, b.Lang) {
			return nil
		}
		o.Kind, o.Lang = contract.ObjDiagram, b.Lang
	default:
		return nil
	}
	lines := make([]string, 0, len(b.Lines))
	for _, line := range b.Lines {
		lines = append(lines, strings.Repeat(" ", int(line.Padding))+e.text.Slice(line.Source))
	}
	o.Source = strings.Join(lines, "\n")
	if strings.TrimSpace(o.Source) == "" {
		return nil
	}
	return o
}

// preview returns what the preview of block b shows: a rendering, its own or
// the last good one while its own renders, or the error.
func (e *Editor) preview(b *contract.Block, o *contract.ObjectRef) (st *renderState, ok bool) {
	st = e.rendered(o, e.ty.BodyFont().Size)
	if st == nil {
		return nil, false
	}
	if e.previews == nil {
		e.previews = map[contract.BlockID]*renderState{}
	}
	switch {
	case st.img != nil:
		e.previews[b.ID] = st
		return st, true
	case st.loading:
		if last := e.previews[b.ID]; last != nil {
			return last, true
		}
		return st, true
	}
	return st, st.err != ""
}

// previewHeight is the room the preview takes under the source, its gap
// included; 0 for none.
func (e *Editor) previewHeight(b *contract.Block, o *contract.ObjectRef) float32 {
	if o == nil {
		return 0
	}
	st, ok := e.preview(b, o)
	if !ok {
		return 0
	}
	switch {
	case st.img != nil:
		w, h := st.res.Width, st.res.Height
		if w > e.width {
			h = h * e.width / w
		}
		return min(h, previewMaxH) + 2*previewGap
	case st.loading:
		if o.Kind == contract.ObjDiagram {
			return diagramH + 2*previewGap
		}
		return blockMathH + 2*previewGap
	}
	return e.ty.Body*1.6 + 2*previewGap // the error, one line
}

// paintPreview paints the preview into the bottom of the row.
func (rc *rowContext) paintPreview() {
	e, p, t := rc.e, rc.p, rc.t
	h := rc.l.m.preview
	if h <= 0 {
		return
	}
	o := e.previewObject(rc.b, &rc.l.proj)
	if o == nil {
		return
	}
	st, ok := e.preview(rc.b, o)
	if !ok {
		return
	}
	box := ui.Rect{X: rc.ox, Y: rc.top + rc.height - h + previewGap, W: e.width, H: h - 2*previewGap}
	switch {
	case st.img != nil:
		w, ih := st.res.Width, st.res.Height
		if w > box.W {
			w, ih = box.W, ih*box.W/w
		}
		if ih > box.H {
			w, ih = w*box.H/ih, box.H
		}
		p.Image(st.img, ui.Rect{X: box.X + (box.W-w)/2, Y: box.Y, W: w, H: ih}, ui.Contain)
	case st.loading:
		rc.skeleton(o, box, p.Now())
	default:
		l := e.shaped(st.err, ui.Font{Family: theme.Sans, Size: theme.UIStatus})
		_, lh := l.Size()
		p.Stroke(box, t.Danger.Alpha(.6), 6, 1)
		p.Clip(box, 6, func() { p.TextLayout(l, box.X+12, box.Y+(box.H-lh)/2, t.Danger) })
	}
}
