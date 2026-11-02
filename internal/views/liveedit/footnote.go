package liveedit

import (
	"strings"
	"unicode/utf8"

	"aurink/internal/contract"
	"aurink/internal/views/theme"
	"github.com/egoist/mygo/ui"
)

// A footnote reference shows as its label raised and small in the accent
// (02 §2.1, the prototype's .72em superscript); the pointer over it previews the
// definition.
const (
	noteScale = .72
	noteRaise = .38
	// notePreviewMax caps the runes of the definition the preview shows.
	notePreviewMax = 280
	notePreviewW   = 320
)

// noteSpan is a reference's object: as wide as its label at the small size.
func noteSpan(s *contract.Segment, m *rowMetrics) ui.Span {
	size := m.font.Size * noteScale
	w := float32(2)
	for _, r := range s.Object.Source {
		if r < 0x80 {
			w += size * .62
		} else {
			w += size
		}
	}
	return ui.Span{Text: s.Display, ObjectWidth: w, ObjectHeight: size, BaselineShift: m.font.Size * noteRaise}
}

// paintNote draws a reference's label in its box.
func (rc *rowContext) paintNote(box ui.Rect, label string) {
	size := rc.l.m.font.Size * noteScale
	rc.p.RichText(box.X+1, box.Y+(box.H-size*1.3)/2, box.W+8, ui.Span{Text: label, Size: size, Weight: 600, Color: rc.t.Accent})
}

// noteAt returns the label of the footnote reference at a point, "" for none.
func (e *Editor) noteAt(x, y float32) string {
	r := e.rowAtPoint(x, y)
	if r < 0 {
		return ""
	}
	l := e.current(r)
	pos := l.layout.PositionAt(ui.Point{X: x - e.contentX() - l.m.x, Y: y - e.rowTextY(r)})
	src := e.opts.Projector.ToSource(&l.proj, pos.Index, contract.Downstream)
	for i := range l.proj.Segments {
		s := &l.proj.Segments[i]
		if s.Kind == contract.SegObject && s.Object != nil && s.Object.Kind == contract.ObjFootnoteRef &&
			src >= s.Src.Start && src < s.Src.End {
			return s.Object.Source
		}
	}
	return ""
}

// noteText returns the text of a footnote's definition, flattened to one
// paragraph and cut to notePreviewMax runes.
func (e *Editor) noteText(label string) string {
	key := strings.ToLower(strings.Join(strings.Fields(label), " "))
	i, ok := e.ast.Footnotes()[key]
	if !ok {
		for k, v := range e.ast.Footnotes() {
			if strings.EqualFold(k, key) {
				i, ok = v, true
				break
			}
		}
	}
	if !ok {
		return ""
	}
	b := e.ast.Block(i)
	if b == nil {
		return ""
	}
	src := e.model.Text().Slice(b.Range)
	if k := strings.Index(src, "]:"); k >= 0 {
		src = src[k+2:]
	}
	text := strings.Join(strings.Fields(src), " ")
	if utf8.RuneCountInString(text) > notePreviewMax {
		cut := 0
		for range notePreviewMax {
			_, size := utf8.DecodeRuneInString(text[cut:])
			cut += size
		}
		text = text[:cut] + "…"
	}
	return text
}

// paintNotePreview shows the definition of the reference under the pointer.
func (e *Editor) paintNotePreview(p *ui.Painter, r ui.Rect) {
	if e.hoverNote == "" {
		return
	}
	text := e.noteText(e.hoverNote)
	if text == "" {
		return
	}
	span := ui.Span{Text: text, Size: theme.UIText, Color: e.t.Text}
	w, h := p.MeasureText(notePreviewW, span)
	x, y := r.X+max(8, min(e.hoverX-24, r.W-w-32)), r.Y+e.hoverY+16
	box := ui.Rect{X: x, Y: y, W: w + 24, H: h + 16}
	p.Fill(box, e.t.Solid, 9)
	p.Stroke(box, e.t.Border, 9, 1)
	p.RichText(x+12, y+8, notePreviewW, span)
}

// NoteHint is the label of the footnote reference under the pointer, for tests.
func (e *Editor) NoteHint() string { return e.hoverNote }
