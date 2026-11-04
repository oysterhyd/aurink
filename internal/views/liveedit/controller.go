package liveedit

import (
	"aurink/internal/views/theme"
	"sort"

	"aurink/internal/contract"
	"aurink/internal/views/editorapi"
	"github.com/egoist/mygo/ui"
)

var _ editorapi.Controller = (*Editor)(nil)

// Session returns the session the editor was built with, which may be nil.
func (e *Editor) Session() contract.Session { return e.opts.Session }

// Model returns the document.
func (e *Editor) Model() contract.DocumentModel { return e.model }

// Focus asks for the keyboard. There is no context here, so the next build does it.
func (e *Editor) Focus() {
	e.focusReq = true
	e.invalidate()
}

// Focused reports whether the editor has the keyboard.
func (e *Editor) Focused() bool { return e.focused }

// Mode returns the display mode.
func (e *Editor) Mode() editorapi.Mode { return e.mode }

// SetMode changes the display mode. Reading hides every marker, so all forms go.
func (e *Editor) SetMode(m editorapi.Mode) {
	if m == e.mode {
		return
	}
	if m == editorapi.ModeRead {
		e.UnmarkText()
	}
	e.mode = m
	e.dropAllForms()
	e.invalidate()
}

// Typography returns the document's type.
func (e *Editor) Typography() theme.Typography { return e.ty }

// SetTypography changes the document's type (Settings.Appearance, the zoom,
// reading mode's serif body). Every row is laid out again; the anchor row
// keeps its place on screen.
func (e *Editor) SetTypography(ty theme.Typography) {
	if ty == e.ty {
		return
	}
	e.ty = ty
	e.relayAll()
	e.invalidate()
}

// Reveal returns how marks and soft breaks show.
func (e *Editor) Reveal() (contract.RevealMode, contract.SoftBreakMode) {
	return e.revealMode, e.softBreak
}

// SetReveal changes how marks and soft breaks show (Settings.Editor). Every
// row is projected and laid out again.
func (e *Editor) SetReveal(m contract.RevealMode, sb contract.SoftBreakMode) {
	if m == e.revealMode && sb == e.softBreak {
		return
	}
	e.revealMode, e.softBreak = m, sb
	e.relayAll()
	e.invalidate()
}

// SetReduceMotion turns off the editor's movements: the format bar's entrance,
// the lightbox's flight and the gliding caret (02 §4.4).
func (e *Editor) SetReduceMotion(on bool) { e.reduceMotion = on }

// FocusMode reports whether rows other than the caret's are dimmed.
func (e *Editor) FocusMode() bool { return e.focusMode }

// SetFocusMode dims the rows other than the caret's.
func (e *Editor) SetFocusMode(on bool) {
	e.focusMode = on
	e.invalidate()
}

// Typewriter reports whether the caret stays centered.
func (e *Editor) Typewriter() bool { return e.typewriter }

// SetTypewriter keeps the caret centered.
func (e *Editor) SetTypewriter(on bool) {
	e.typewriter = on
	e.revealPending = on
	e.invalidate()
}

// RevealOffset scrolls to a source offset and puts the caret there.
func (e *Editor) RevealOffset(off int, align editorapi.ScrollAlign) {
	e.UnmarkText()
	e.model.BreakCoalescing()
	e.hasSticky = false
	off = e.snap(off, contract.Downstream)
	e.selectRaw(off, off)
	e.revealPending = true
	e.revealAlign = align
}

// RevealRange selects a source range, as the find bar does with a match, and
// scrolls it into view.
func (e *Editor) RevealRange(rg contract.Range, align editorapi.ScrollAlign) {
	e.UnmarkText()
	e.model.BreakCoalescing()
	e.hasSticky = false
	end := max(0, min(rg.End, e.text.Len()))
	e.selectRaw(max(0, min(rg.Start, end)), end)
	e.revealAlign = align
}

// SetFindHighlights sets the find ranges, in source order; current is -1 for none.
func (e *Editor) SetFindHighlights(ranges []contract.Range, current int) {
	e.find, e.findCur = ranges, current
	e.invalidate()
}

// ClearFindHighlights removes the find ranges.
func (e *Editor) ClearFindHighlights() { e.SetFindHighlights(nil, -1) }

// VisibleHeading returns the heading of the section at the top of the view.
func (e *Editor) VisibleHeading() (contract.BlockID, bool) {
	if len(e.rows) == 0 {
		return 0, false
	}
	// The row under the top margin, where a jump to a heading puts it: the rows
	// above show in the margin but belong to the section before.
	r := max(0, min(e.hs.at(e.scroll+padTop+2), len(e.rows)-1))
	off := e.block(r).Range.Start
	o := e.ast.Outline()
	i := sort.Search(len(o), func(k int) bool { return o[k].Range.Start > off })
	if i == 0 {
		return 0, false
	}
	return o[i-1].Block, true
}

// ScrollFraction returns how far through the document the view is, 0 to 1.
func (e *Editor) ScrollFraction() float32 {
	span := e.maxScroll() + padTop
	if span <= 0 {
		return 0
	}
	return float32(max(0, min((e.scroll+padTop)/span, 1)))
}

// ScrollOffset returns the scroll offset in DIPs.
func (e *Editor) ScrollOffset() float32 { return float32(e.scroll) }

// CaretBlock returns the caret's row, from 1, and how many rows there are.
func (e *Editor) CaretBlock() (index, total int) { return e.caretRow + 1, len(e.rows) }

// SelectionRect returns the box of the selection or the caret in the view, in
// window coordinates; false before the first frame or when none of it shows.
func (e *Editor) SelectionRect() (editorapi.Rect, bool) {
	if e.width <= 0 {
		return editorapi.Rect{}, false
	}
	from, to := min(e.anchor, e.head), max(e.anchor, e.head)
	if from == to {
		x, y, h := e.caretRect()
		return editorapi.Rect{X: e.orgX + x, Y: e.orgY + y, W: 1, H: h}, y+h > 0 && y < e.viewH
	}
	var out editorapi.Rect
	found := false
	pr := e.opts.Projector
	for i := e.first; i <= e.last && i < len(e.rows); i++ {
		b := e.block(i)
		if from >= b.Trailer.End || to <= b.Lead.Start {
			continue
		}
		l := e.current(i)
		a := pr.ToDisplay(&l.proj, clampTo(from, e.rowRange(i)), contract.Upstream)
		z := pr.ToDisplay(&l.proj, clampTo(to, e.rowRange(i)), contract.Downstream)
		x0, y0 := e.orgX+e.contentX()+l.m.x, e.orgY+e.rowTextY(i)
		for _, s := range l.layout.SelectionRects(ui.TextInputRange{Start: a, End: z}) {
			r := editorapi.Rect{X: x0 + s.X, Y: y0 + s.Y, W: s.W, H: s.H}
			if !found {
				out, found = r, true
				continue
			}
			right, bottom := max(out.X+out.W, r.X+r.W), max(out.Y+out.H, r.Y+r.H)
			out.X, out.Y = min(out.X, r.X), min(out.Y, r.Y)
			out.W, out.H = right-out.X, bottom-out.Y
		}
	}
	return out, found
}

// CopyMarkdown copies the selection as Markdown only; CopyHTML as an HTML
// fragment, its source as the text; PastePlain pastes the clipboard's text as
// it is (02 §2.1).
func (e *Editor) CopyMarkdown() { e.copySelection(copyMarkdown) }

// CopyHTML copies the selection as HTML.
func (e *Editor) CopyHTML() { e.copySelection(copyHTML) }

// PastePlain pastes the clipboard's plain text, without converting it.
func (e *Editor) PastePlain() { e.paste(true) }

// RunEdit runs a command on the selection.
func (e *Editor) RunEdit(cmd contract.EditCommand) error {
	if e.readOnly() {
		return nil
	}
	e.UnmarkText()
	// In a link, Primary+K edits its destination rather than unlinking it (02 §5).
	if cmd.ID == contract.CmdFormatLink && e.editLink() {
		return nil
	}
	tx, ok, err := e.opts.Editing.Run(e.editContext(), cmd)
	if err != nil || !ok {
		return err
	}
	_, err = e.model.Apply(tx)
	return err
}

// Formatting returns the selection's format state.
func (e *Editor) Formatting() contract.FormatState {
	return e.opts.Editing.Formatting(e.editContext())
}

// ViewAnchor returns the row at the top of the view and how far into it the
// view starts, in DIP: the scroll by block, which survives a change of font
// size or width (D10).
func (e *Editor) ViewAnchor() (row int, off float32) {
	if len(e.rows) == 0 {
		return 0, 0
	}
	return e.anchorIdx, float32(e.anchorOff)
}

// SetViewAnchor scrolls so that row is at the top, off into it, leaving the
// caret where it is; a row past the end goes to the last.
func (e *Editor) SetViewAnchor(row int, off float32) {
	if len(e.rows) == 0 {
		return
	}
	e.anchorIdx, e.anchorOff = max(0, min(row, len(e.rows)-1)), float64(off)
	e.revealPending = false
	e.invalidate()
}

// focusAlpha is how much of the text rows other than the caret's keep in focus
// mode; typewriterLine is where typewriter mode keeps the caret, as a fraction
// of the view's height (02 §5.10).
const (
	focusAlpha     = 0.28
	typewriterLine = 0.45
)

// TitleAbove returns the text of the document's first level-1 heading and
// whether the view has scrolled past it, for reading mode's title.
func (e *Editor) TitleAbove() (string, bool) {
	for _, h := range e.ast.Outline() {
		if h.Level != 1 {
			continue
		}
		r, ok := e.rowOf[h.Block]
		if !ok {
			return h.Text, false
		}
		return h.Text, e.rowTop(r)+float64(e.rows[r].h) <= e.scroll
	}
	return "", false
}
