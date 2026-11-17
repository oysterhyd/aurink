package liveedit

import (
	"aurink/internal/contract"
	"aurink/internal/views/editorapi"
	"aurink/internal/views/theme"
	"aurink/locales"
	"github.com/egoist/mygo/ui"
)

// The block objects of Phase 3 (FE-1.6, 02 §5.9): the Front Matter header and
// the table of contents. Each is one SegObject of its row; the caret on the
// row shows its source instead (the projection's reveal rule).

const (
	headerH   = 28 // the Front Matter header: kicker · tags · date
	tagPadX   = 10
	tocLineH  = 28
	tocPadY   = 10
	tocIndent = 18
)

type textKey struct {
	text   string
	family string
	size   float32
	weight int
}

// shaped returns a cached layout of a short text in a font.
func (e *Editor) shaped(text string, f ui.Font) *ui.TextLayout {
	k := textKey{text, f.Family, f.Size, f.Weight}
	l := e.texts[k]
	if l == nil {
		if e.texts == nil || len(e.texts) > 2048 {
			e.texts = map[textKey]*ui.TextLayout{}
		}
		l = ui.ShapeText(text, f, 0)
		e.texts[k] = l
	}
	return l
}

// blockObject returns the row's block object when its projection is one.
func blockObject(p *contract.BlockProjection) *contract.ObjectRef {
	for i := range p.Segments {
		s := &p.Segments[i]
		if s.Kind == contract.SegObject && s.Object != nil && s.Object.Display {
			return s.Object
		}
	}
	return nil
}

// objectSize returns the box of a block object of the Phase 3 kinds; ok false
// for the others.
func (e *Editor) objectSize(o *contract.ObjectRef, m *rowMetrics) (w, h float32, ok bool) {
	if o == nil {
		return 0, 0, false
	}
	switch o.Kind {
	case contract.ObjFrontMatter:
		return m.width, headerH, true
	case contract.ObjTOC:
		n := max(len(e.outline()), 1)
		return m.width, float32(n)*tocLineH + 2*tocPadY, true
	}
	return 0, 0, false
}

// paintBlockObject paints a block object into its box; false for the kinds
// it does not paint.
func (rc *rowContext) paintBlockObject(o *contract.ObjectRef, box ui.Rect) bool {
	switch o.Kind {
	case contract.ObjFrontMatter:
		if o.Meta != nil {
			rc.paintHeader(o.Meta, box)
		}
		return true
	case contract.ObjTOC:
		rc.paintTOC(box)
		return true
	}
	return false
}

// paintHeader is the editorial header (Main.png): the kicker in the accent,
// the tags as capsules, the date, separated by dots.
func (rc *rowContext) paintHeader(meta *contract.FrontMatterMeta, box ui.Rect) {
	e, t, p := rc.e, rc.t, rc.p
	x, mid := box.X, box.Y+box.H/2
	ui13 := ui.Font{Family: theme.Sans, Size: theme.UIText}
	put := func(l *ui.TextLayout, c ui.Color) {
		w, h := l.Size()
		p.TextLayout(l, x, mid-h/2, c)
		x += w
	}
	dot := func() {
		x += 8
		put(e.shaped("·", ui13), t.Muted)
		x += 8
	}
	first := true
	if meta.Kicker != "" {
		put(e.shaped(meta.Kicker, ui.Font{Family: theme.Sans, Size: theme.UIText, Weight: 600}), t.Accent)
		first = false
	}
	if len(meta.Tags) > 0 {
		if !first {
			dot()
		}
		for i, tag := range meta.Tags {
			l := e.shaped(tag, ui.Font{Family: theme.Sans, Size: theme.UIStatus})
			w, h := l.Size()
			pill := ui.Rect{X: x, Y: mid - 11, W: w + 2*tagPadX, H: 22}
			p.Fill(pill, t.Surface, 11)
			p.Stroke(pill, t.Border, 11, 1)
			p.TextLayout(l, x+tagPadX, mid-h/2, t.Muted)
			x += pill.W
			if i < len(meta.Tags)-1 {
				x += 8
			}
		}
		first = false
	}
	if meta.Date != "" {
		if !first {
			dot()
		}
		put(e.shaped(meta.Date, ui13), t.Muted)
	}
}

// tocEntries are the table of contents' lines: each heading, indented by its
// level under the shallowest.
func (e *Editor) tocEntries() (out []contract.Heading, top int) {
	out = e.outline()
	top = 6
	for _, h := range out {
		top = min(top, h.Level)
	}
	return out, top
}

func (rc *rowContext) paintTOC(box ui.Rect) {
	e, t, p := rc.e, rc.t, rc.p
	p.Fill(ui.Rect{X: box.X, Y: box.Y + tocPadY, W: 2, H: box.H - 2*tocPadY}, t.Border, 1)
	hs, top := e.tocEntries()
	f := e.ty.BodyFont()
	f.Size *= .94
	if len(hs) == 0 {
		l := e.shaped(locales.LiveeditTocEmpty(), f)
		_, h := l.Size()
		p.TextLayout(l, box.X+tocIndent, box.Y+tocPadY+(tocLineH-h)/2, t.Muted)
		return
	}
	for i, hd := range hs {
		l := e.shaped(hd.Text, f)
		_, h := l.Size()
		x := box.X + tocIndent + float32(hd.Level-top)*tocIndent
		y := box.Y + tocPadY + float32(i)*tocLineH + (tocLineH-h)/2
		c := t.Text
		if hd.Level > top {
			c = t.Muted
		}
		p.TextLayout(l, x, y, c)
	}
}

// tocClick follows a click on a line of the table of contents in row r to its
// heading, as a link does; it reports whether it took the click.
func (e *Editor) tocClick(r int, x, y float32) bool {
	if e.rows[r].kind != contract.BlockTOC {
		return false
	}
	l := e.current(r)
	o := blockObject(&l.proj)
	if o == nil || o.Kind != contract.ObjTOC {
		return false
	}
	objs := l.layout.Objects()
	if len(objs) == 0 {
		return false
	}
	top := e.rowTextY(r) + objs[0].Rect.Y + tocPadY
	i := int((y - top) / tocLineH)
	hs, _ := e.tocEntries()
	if y < top || i < 0 || i >= len(hs) {
		return false
	}
	idx, ok := e.ast.Index(hs[i].Block)
	if !ok {
		return false
	}
	off := e.ast.Block(idx).Range.Start
	e.model.SetSelection(contract.Caret(off))
	e.RevealOffset(off, editorapi.AlignTop)
	return true
}
