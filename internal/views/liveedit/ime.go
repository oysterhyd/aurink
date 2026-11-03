package liveedit

import (
	"aurink/internal/contract"
	"github.com/egoist/mygo/ui"
)

// The input method's document is the caret row's display text with the preedit at
// the caret (03 §6.4), in UTF-16 units of that string. A selection is kept until
// the commit, which replaces it in one step, so that a cancelled composition
// changes nothing.

var _ ui.TextInputClient = (*Editor)(nil)

// imeBase returns the caret row without the preedit and the caret's index in it.
func (e *Editor) imeBase() (*laid, int) {
	l := e.form(e.caretRow)
	return l, e.opts.Projector.ToDisplay(&l.proj, clampTo(e.head, e.block(e.caretRow).Range), contract.Downstream)
}

// virtualToSource maps a unit of the input method's document to a source offset;
// any unit inside the preedit is the caret, where the preedit stands.
func (e *Editor) virtualToSource(u int) int {
	l, at := e.imeBase()
	n := utf16Len(e.preedit)
	switch {
	case u <= at:
		return e.opts.Projector.ToSource(&l.proj, u, contract.Downstream)
	case u <= at+n:
		return e.head
	}
	return e.opts.Projector.ToSource(&l.proj, u-n, contract.Downstream)
}

// TextForRange returns the display text with the preedit in it.
func (e *Editor) TextForRange(r ui.TextInputRange) (string, ui.TextInputRange) {
	l, at := e.imeBase()
	cut := utf16ByteOffset(l.display, at)
	text := l.display[:cut] + e.preedit + l.display[cut:]
	a := utf16ByteOffset(text, max(r.Start, 0))
	z := utf16ByteOffset(text, max(r.End, r.Start))
	return text[a:z], ui.TextInputRange{Start: utf16Len(text[:a]), End: utf16Len(text[:z])}
}

// Selection reports the selection within the caret's row.
func (e *Editor) Selection() ui.TextInputSelection {
	l, at := e.imeBase()
	if e.preedit != "" {
		return ui.TextInputSelection{Range: ui.TextInputRange{Start: at + e.preSelected.Start, End: at + e.preSelected.End}}
	}
	if e.anchor == e.head {
		return ui.TextInputSelection{Range: ui.TextInputRange{Start: at, End: at}}
	}
	other := e.opts.Projector.ToDisplay(&l.proj, clampTo(e.anchor, e.block(e.caretRow).Range), contract.Downstream)
	return ui.TextInputSelection{Range: ui.TextInputRange{Start: min(at, other), End: max(at, other)}, Reversed: at < other}
}

// MarkedRange reports the preedit's range.
func (e *Editor) MarkedRange() (ui.TextInputRange, bool) {
	if e.preedit == "" {
		return ui.TextInputRange{}, false
	}
	_, at := e.imeBase()
	return ui.TextInputRange{Start: at, End: at + utf16Len(e.preedit)}, true
}

// ReplaceText is how text enters the source: typed text with a nil range, or an
// input method's commit, which is a single edit however long the composition was.
func (e *Editor) ReplaceText(r *ui.TextInputRange, s string) {
	if e.readOnly() {
		e.cancelPreedit()
		return
	}
	switch {
	case e.preedit != "":
		rg := e.preRange
		if r != nil {
			if a, z := e.virtualToSource(min(r.Start, r.End)), e.virtualToSource(max(r.Start, r.End)); a < z {
				rg = contract.Range{Start: min(rg.Start, a), End: max(rg.End, z)}
			}
		}
		e.cancelPreedit()
		e.commitIME(rg, s)
	case r != nil:
		e.commitIME(contract.Range{Start: e.virtualToSource(min(r.Start, r.End)), End: e.virtualToSource(max(r.Start, r.End))}, s)
	default:
		if !e.intent(contract.Intent{Kind: contract.IntentInsertText, Text: s}) {
			e.commitPlain(s)
		}
	}
	e.slashTyped(s)
}

func (e *Editor) commitIME(rg contract.Range, s string) {
	if rg.Start == rg.End && s == "" {
		return
	}
	c := contract.Caret(rg.Start + len(s))
	if e.apply(contract.Transaction{Edits: []contract.TextEdit{{Range: rg, Text: s}}, Selection: &c, Origin: contract.OriginIME, Label: "editing_undo_typing"}) {
		e.followUp()
	}
}

// commitPlain is for text the editing rules decline: it still must not be lost.
func (e *Editor) commitPlain(s string) {
	rg := contract.SelRange{Anchor: e.anchor, Head: e.head}.Range()
	c := contract.Caret(rg.Start + len(s))
	e.apply(contract.Transaction{Edits: []contract.TextEdit{{Range: rg, Text: s}}, Selection: &c, Origin: contract.OriginUser, Coalesce: "typing", Label: "editing_undo_typing"})
}

func (e *Editor) cancelPreedit() {
	e.preedit = ""
	e.preSelected = ui.TextInputRange{}
	e.preLaid = nil
	e.revealPending = true
	e.invalidate()
}

// SetMarkedText shows the composition without touching the source.
func (e *Editor) SetMarkedText(r *ui.TextInputRange, s string, selected ui.TextInputRange) {
	if e.readOnly() {
		return
	}
	if s == "" {
		// The composition ended or was cancelled: nothing was ever written.
		e.cancelPreedit()
		return
	}
	if e.preedit == "" {
		e.preRange = contract.SelRange{Anchor: e.anchor, Head: e.head}.Range()
		e.model.BreakCoalescing()
	}
	if r != nil {
		a, z := e.virtualToSource(min(r.Start, r.End)), e.virtualToSource(max(r.Start, r.End))
		if e.preedit == "" || a < z {
			e.preRange = contract.Range{Start: a, End: z}
			e.selectRaw(a, z)
		}
	}
	n := utf16Len(s)
	e.preedit = s
	e.preSelected = ui.TextInputRange{Start: min(selected.Start, n), End: min(selected.End, n)}
	e.preLaid = nil
	e.revealPending = true
}

// UnmarkText ends the composition and keeps its text, which is a commit.
func (e *Editor) UnmarkText() {
	if e.preedit == "" {
		return
	}
	s, rg := e.preedit, e.preRange
	e.cancelPreedit()
	if !e.readOnly() {
		e.commitIME(rg, s)
	}
}

// BoundsForRange returns element-relative geometry from the caret row's layout,
// where candidate windows are placed.
func (e *Editor) BoundsForRange(r ui.TextInputRange) (ui.Rect, ui.TextInputRange, bool) {
	if e.width <= 0 {
		return ui.Rect{}, ui.TextInputRange{}, false
	}
	l := e.current(e.caretRow)
	n := utf16Len(l.display)
	start := max(0, min(min(r.Start, r.End), n))
	end := max(start, min(max(r.Start, r.End), n))
	x, y, h := e.boundsAt(e.caretRow, l, start)
	b := ui.Rect{X: x, Y: y, W: 1, H: h}
	if end > start {
		// Up to the end of the range or of its first line.
		if x2, y2, _ := e.boundsAt(e.caretRow, l, end); y2 == y && x2 > x {
			b.W = x2 - x
		}
	}
	return b, ui.TextInputRange{Start: start, End: end}, y+h > 0 && y < e.viewH
}

// IndexForPoint hit-tests within the caret's row, the input method's document.
func (e *Editor) IndexForPoint(p ui.Point) (int, bool) {
	if e.width <= 0 {
		return 0, false
	}
	l := e.current(e.caretRow)
	return l.layout.PositionAt(ui.Point{X: p.X - e.contentX() - l.m.x, Y: p.Y - e.rowTextY(e.caretRow)}).Index, true
}
