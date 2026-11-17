package liveedit

import (
	"slices"
	"strings"

	"aurink/internal/contract"
	"aurink/internal/views/theme"
	"github.com/egoist/mygo/ui"
)

// The live preview (02 §5.9, 09 D9, as Typora does): a math block, diagram,
// or standalone image paragraph whose source shows, because the caret is in it,
// keeps its rendering under the source, which follows the typing. While a new
// version renders, the last good one stays; a source that does not render
// shows its error instead.

const (
	previewGap  = 10
	previewMaxH = 360
)

// previewObject is the object a revealed math, diagram, or standalone image
// block renders as, with the source the projection gives it unrevealed, so
// that both share a rendering; nil for the other blocks, or a block already
// shown as its object.
func (e *Editor) previewObject(r int, b *contract.Block, p *contract.BlockProjection) *contract.ObjectRef {
	if blockObject(p) != nil {
		return nil
	}
	for i := range p.Segments {
		if p.Segments[i].Kind == contract.SegObject && p.Segments[i].Object != nil && p.Segments[i].Object.Kind == contract.ObjImage {
			return nil
		}
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
	case contract.BlockParagraph:
		return e.standaloneImagePreview(r)
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

// standaloneImagePreview returns an ObjImage ref when row r is a paragraph
// containing exclusively one Markdown image (ignoring surrounding spaces/tabs).
func (e *Editor) standaloneImagePreview(r int) *contract.ObjectRef {
	if r < 0 || r >= len(e.rows) {
		return nil
	}
	ins := e.ast.Inlines(e.rows[r].leaf)
	if len(ins) == 0 {
		return nil
	}
	imgIdx := -1
	for i := 0; i < len(ins); {
		n := &ins[i]
		switch n.Kind {
		case contract.InlineText:
			if strings.Trim(n.Text, " \t") != "" {
				return nil
			}
			i = n.Next
		case contract.InlineImage:
			if imgIdx >= 0 {
				return nil
			}
			imgIdx = i
			i = n.Next
		default:
			return nil
		}
	}
	if imgIdx < 0 || ins[imgIdx].Dest == "" {
		return nil
	}
	n := &ins[imgIdx]
	var alt strings.Builder
	for j := imgIdx + 1; j < n.Next; j++ {
		if ins[j].Next == j+1 {
			if ins[j].Kind == contract.InlineSoftBreak || ins[j].Kind == contract.InlineLineBreak {
				alt.WriteByte(' ')
			} else {
				alt.WriteString(ins[j].Text)
			}
		}
	}
	return &contract.ObjectRef{
		Kind:    contract.ObjImage,
		Source:  n.Dest,
		Alt:     alt.String(),
		Title:   n.Title,
		Display: true,
	}
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

// imagePreviewSize returns the fitted (w, h) of a standalone image preview
// within availW and previewMaxH; false when the image is not loaded.
func (e *Editor) imagePreviewSize(src string, availW float32) (w, h float32, ok bool) {
	st := e.image(src)
	if st == nil || st.src == nil {
		return 0, 0, false
	}
	iw, ih := ui.ImageSize(st.src)
	if iw <= 0 || ih <= 0 {
		return 0, 0, false
	}
	w, h = iw, ih
	if availW > 0 && w > availW {
		w, h = availW, h*availW/w
	}
	if h > previewMaxH {
		w, h = w*previewMaxH/h, previewMaxH
	}
	return max(w, 1), max(h, 1), true
}

// imagePreviewRect returns the view-coordinate box of row r's revealed image
// preview when present and loaded.
func (e *Editor) imagePreviewRect(r int, l *laid, o *contract.ObjectRef) (ui.Rect, bool) {
	if o == nil || o.Kind != contract.ObjImage || l.m.preview <= 0 {
		return ui.Rect{}, false
	}
	w, ih, ok := e.imagePreviewSize(o.Source, l.m.width)
	if !ok {
		return ui.Rect{}, false
	}
	x := e.contentX() + l.m.x
	y := float32(e.rowTop(r)-e.scroll) + e.rows[r].h - l.m.preview + previewGap
	return ui.Rect{X: x, Y: y, W: w, H: ih}, true
}

// PreviewImageBox returns the view-coordinate box of row r's image preview
// under its revealed source, for tests.
func (e *Editor) PreviewImageBox(r int) (ui.Rect, bool) {
	if r < 0 || r >= len(e.rows) {
		return ui.Rect{}, false
	}
	l := e.current(r)
	return e.imagePreviewRect(r, l, e.previewObject(r, e.block(r), &l.proj))
}

// previewHeight is the room the preview takes under the source, its gap
// included; 0 for none.
func (e *Editor) previewHeight(b *contract.Block, o *contract.ObjectRef, availW float32) float32 {
	if o == nil {
		return 0
	}
	if o.Kind == contract.ObjImage {
		if _, h, ok := e.imagePreviewSize(o.Source, availW); ok {
			return h + 2*previewGap
		}
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
	o := e.previewObject(rc.row, rc.b, &rc.l.proj)
	if o == nil {
		return
	}
	if o.Kind == contract.ObjImage {
		st := e.image(o.Source)
		if st == nil || st.src == nil || e.flying(rc.row, o.Source) {
			return
		}
		w, ih, ok := e.imagePreviewSize(o.Source, rc.l.m.width)
		if !ok {
			return
		}
		imgBox := ui.Rect{X: rc.textX(), Y: rc.top + rc.height - h + previewGap, W: w, H: ih}
		p.Image(st.src, imgBox, ui.Contain)
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
