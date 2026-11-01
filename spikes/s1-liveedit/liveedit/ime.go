package liveedit

import (
	"unicode/utf8"

	"aurink.local/spikes/contract-rc/contract"
	"github.com/egoist/mygo/ui"
)

// The input method's document is the caret block's display text with the preedit
// at the caret (03 §6.4), in UTF-16 units of that string. A selection reaching
// other blocks is clipped to the caret's block when reported, and removed from
// the source, as one step with what is then committed, when composing starts.

var _ ui.TextInputClient = (*Editor)(nil)

// imeBase returns the caret block without the preedit and the caret's index in it.
func (e *Editor) imeBase() (*laid, int) {
	l := e.laidFor(e.caretBlock, true)
	return l, toDisplay(&l.proj, e.caret, contract.Downstream)
}

// virtualToSource maps a unit of the input method's document to a source offset;
// any unit inside the preedit is the caret, where the preedit stands.
func (e *Editor) virtualToSource(u int) int {
	l, at := e.imeBase()
	n := utf16Len(e.preedit)
	switch {
	case u <= at:
		return toSource(&l.proj, u, contract.Downstream)
	case u <= at+n:
		return e.caret
	}
	return toSource(&l.proj, u-n, contract.Downstream)
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

// Selection reports the selection within the caret's block.
func (e *Editor) Selection() ui.TextInputSelection {
	l, at := e.imeBase()
	if e.preedit != "" {
		return ui.TextInputSelection{Range: ui.TextInputRange{Start: at + e.preSelected.Start, End: at + e.preSelected.End}}
	}
	if e.anchor == e.caret {
		return ui.TextInputSelection{Range: ui.TextInputRange{Start: at, End: at}}
	}
	b := &e.d.blocks[e.caretBlock]
	other := toDisplay(&l.proj, max(b.CStart, min(e.anchor, b.CEnd)), contract.Downstream)
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

// ReplaceText is the only way text enters the source: typed text with a nil
// range, or an input method's commit, which is a single edit however long the
// composition was.
func (e *Editor) ReplaceText(r *ui.TextInputRange, s string) {
	switch {
	case r != nil:
		a, z := e.virtualToSource(min(r.Start, r.End)), e.virtualToSource(max(r.Start, r.End))
		e.clearPreedit()
		join := e.joinNext
		e.joinNext = false
		e.commit(a, z, s, contract.OriginIME, false, join)
	case e.preedit != "":
		e.clearPreedit()
		join := e.joinNext
		e.joinNext = false
		e.commit(e.caret, e.caret, s, contract.OriginIME, false, join)
	default:
		e.replaceSelection(s, contract.OriginUser, e.anchor == e.caret && isTyping(s))
	}
}

func isTyping(s string) bool { return utf8.RuneCountInString(s) == 1 && s != "\n" }

func (e *Editor) clearPreedit() {
	e.preedit = ""
	e.preSelected = ui.TextInputRange{}
	e.preLaid = nil
	e.revealPending = true
}

// SetMarkedText shows the composition without touching the source. Only a range
// it replaces is removed from the source, joined to the commit that follows.
func (e *Editor) SetMarkedText(r *ui.TextInputRange, s string, selected ui.TextInputRange) {
	if s == "" {
		// The composition ended or was cancelled: nothing was ever written.
		wasComposing := e.preedit != ""
		e.clearPreedit()
		if wasComposing {
			e.current(e.caretBlock)
		}
		e.joinNext = false
		return
	}
	if r != nil {
		a, z := e.virtualToSource(min(r.Start, r.End)), e.virtualToSource(max(r.Start, r.End))
		e.clearPreedit()
		if a != z {
			e.commit(a, z, "", contract.OriginIME, false, false)
			e.joinNext = true
		} else {
			e.setSelection(a, a)
		}
	} else if e.preedit == "" && e.anchor != e.caret {
		e.replaceSelection("", contract.OriginIME, false)
		e.joinNext = true
	}
	e.breakTyping()
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
	s := e.preedit
	e.clearPreedit()
	join := e.joinNext
	e.joinNext = false
	e.commit(e.caret, e.caret, s, contract.OriginIME, false, join)
}

// BoundsForRange returns element-relative geometry from the caret block's layout,
// where candidate windows are placed.
func (e *Editor) BoundsForRange(r ui.TextInputRange) (ui.Rect, ui.TextInputRange, bool) {
	if e.width <= 0 {
		return ui.Rect{}, ui.TextInputRange{}, false
	}
	l := e.current(e.caretBlock)
	n := utf16Len(l.display)
	start := max(0, min(min(r.Start, r.End), n))
	end := max(start, min(max(r.Start, r.End), n))
	x, y, h := e.boundsAt(e.caretBlock, l, start)
	b := ui.Rect{X: x, Y: y, W: 1, H: h}
	if end > start {
		// Up to the end of the range or of its first line.
		if x2, y2, _ := e.boundsAt(e.caretBlock, l, end); y2 == y && x2 > x {
			b.W = x2 - x
		}
	}
	return b, ui.TextInputRange{Start: start, End: end}, y+h > 0 && y < e.viewH
}

// IndexForPoint hit-tests within the caret's block, the input method's document.
func (e *Editor) IndexForPoint(p ui.Point) (int, bool) {
	if e.width <= 0 {
		return 0, false
	}
	l := e.current(e.caretBlock)
	pos := l.layout.PositionAt(ui.Point{X: p.X - e.contentX(), Y: p.Y - e.blockTextY(e.caretBlock)})
	return pos.Index, true
}
