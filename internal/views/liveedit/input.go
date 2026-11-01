package liveedit

import (
	"time"

	"aurink/internal/contract"
	"github.com/egoist/mygo/ui"
)

// wordMod moves by word: Ctrl, or Option on macOS, where Cmd is not Ctrl.
func wordMod() ui.Modifiers {
	if ui.Cmd == ui.Ctrl {
		return ui.Ctrl
	}
	return ui.Alt
}

// input takes keys, edit commands, the pointer and the wheel. Text comes through
// the TextInputClient (ime.go), which MyGo prefers when an element has one.
func (e *Editor) input(ev ui.InputEvent) bool {
	switch ev.Kind {
	case ui.InputKeyDown:
		if e.preedit != "" {
			return false // the input method owns the keys while composing
		}
		if e.key(ev) {
			e.blinkStart = ev.Time
			return true
		}
	case ui.InputCommand:
		return e.command(ev.Text)
	case ui.InputPointerDown:
		if ev.Button != 0 {
			return false
		}
		e.pointerDown(ev)
		return true
	case ui.InputPointerMove:
		if e.dragging {
			e.dragX, e.dragY = ev.X, ev.Y
			e.dragSpeed = 0
			if ev.Y < 0 {
				e.dragSpeed = ev.Y
			} else if ev.Y > e.viewH {
				e.dragSpeed = ev.Y - e.viewH
			}
			e.dragLast = ev.Time
			e.dragTowards(ev.X, ev.Y)
			return true
		}
	case ui.InputPointerUp:
		was := e.dragging
		e.dragging = false
		e.dragSpeed = 0
		return was
	case ui.InputScroll:
		before := e.scroll
		e.setScroll(e.scroll + float64(ev.DY))
		return e.scroll != before
	}
	return false
}

func (e *Editor) pointerDown(ev ui.InputEvent) {
	e.UnmarkText()
	e.model.BreakCoalescing()
	e.hasSticky = false
	e.dragging = true
	e.dragSpeed = 0
	e.dragX, e.dragY = ev.X, ev.Y
	e.blinkStart = ev.Time
	off := e.hit(ev.X, ev.Y)
	e.dragUnit = 0
	if ev.Clicks == 2 {
		e.dragUnit = 1
	} else if ev.Clicks >= 3 {
		e.dragUnit = 2
	}
	from, to := e.unitAt(off, ev.X, ev.Y)
	if ev.Mods&ui.Shift != 0 {
		a := min(e.anchor, e.head)
		z := max(e.anchor, e.head)
		e.dragFrom, e.dragTo = a, z
		if a == z {
			e.dragFrom, e.dragTo = e.anchor, e.anchor
		}
		e.extendDrag(from, to)
		return
	}
	e.dragFrom, e.dragTo = from, to
	e.selectRaw(from, to)
}

// unitAt returns what the drag's unit covers at a point: the caret, a word or a row.
func (e *Editor) unitAt(off int, x, y float32) (from, to int) {
	switch e.dragUnit {
	case 1:
		r := e.rowAt(off)
		l := e.form(r)
		pr := e.opts.Projector
		d := pr.ToDisplay(&l.proj, clampTo(off, e.block(r).Range), contract.Downstream)
		a, z := wordAt(l.display, d)
		return pr.ToSource(&l.proj, a, contract.Downstream), pr.ToSource(&l.proj, z, contract.Upstream)
	case 2:
		rg := e.block(e.rowAt(off)).Range
		return rg.Start, rg.End
	}
	return off, off
}

func (e *Editor) dragTowards(x, y float32) {
	from, to := e.unitAt(e.hit(x, y), x, y)
	e.extendDrag(from, to)
}

// extendDrag selects from the drag's origin to a unit on either side of it.
func (e *Editor) extendDrag(from, to int) {
	if from < e.dragFrom {
		e.selectRaw(e.dragTo, from)
	} else {
		e.selectRaw(e.dragFrom, to)
	}
}

// autoscroll moves the view while the pointer is dragged out of it, faster the
// further out, and extends the selection with it.
func (e *Editor) autoscroll(now time.Time) {
	dt := now.Sub(e.dragLast).Seconds()
	e.dragLast = now
	if dt <= 0 {
		return
	}
	dt = min(dt, 0.1)
	e.setScroll(e.scroll + float64(e.dragSpeed)*dt*8)
	e.dragTowards(e.dragX, e.dragY)
}

func (e *Editor) key(ev ui.InputEvent) bool {
	extend := ev.Mods&ui.Shift != 0
	word := ev.Mods&wordMod() != 0
	if ev.Mods&ui.Cmd != 0 {
		switch ev.Key {
		case ui.KeyZ:
			if extend {
				return e.command("redo")
			}
			return e.command("undo")
		case ui.KeyY:
			return e.command("redo")
		case ui.KeyA:
			return e.command("selectAll")
		case ui.KeyC:
			return e.command("copy")
		case ui.KeyX:
			return e.command("cut")
		case ui.KeyV:
			return e.command("paste")
		case ui.KeyHome:
			e.moveTo(0, extend)
			return true
		case ui.KeyEnd:
			e.moveTo(e.text.Len(), extend)
			return true
		}
		if !word {
			switch ev.Key { // macOS
			case ui.KeyLeft:
				e.moveEdge(-1, extend)
				return true
			case ui.KeyRight:
				e.moveEdge(1, extend)
				return true
			case ui.KeyBackspace:
				e.intent(contract.Intent{Kind: contract.IntentDeleteBackward, Gran: contract.GranLine})
				return true
			}
			return false
		}
	}
	if ev.Mods&^(ui.Shift|wordMod()) != 0 {
		return false
	}
	if word {
		switch ev.Key {
		case ui.KeyLeft:
			e.moveWord(-1, extend)
		case ui.KeyRight:
			e.moveWord(1, extend)
		case ui.KeyBackspace:
			e.intent(contract.Intent{Kind: contract.IntentDeleteBackward, Gran: contract.GranWord})
		case ui.KeyDelete:
			e.intent(contract.Intent{Kind: contract.IntentDeleteForward, Gran: contract.GranWord})
		default:
			return false
		}
		return true
	}
	switch ev.Key {
	case ui.KeyLeft:
		e.moveChar(-1, extend)
	case ui.KeyRight:
		e.moveChar(1, extend)
	case ui.KeyUp:
		e.moveLine(-1, extend)
	case ui.KeyDown:
		e.moveLine(1, extend)
	case ui.KeyHome:
		e.moveEdge(-1, extend)
	case ui.KeyEnd:
		e.moveEdge(1, extend)
	case ui.KeyPageUp:
		e.movePage(-1, extend)
	case ui.KeyPageDown:
		e.movePage(1, extend)
	case ui.KeyEnter:
		if extend {
			e.intent(contract.Intent{Kind: contract.IntentLineBreak})
		} else {
			e.intent(contract.Intent{Kind: contract.IntentNewline})
		}
	case ui.KeyTab:
		if e.readOnly() {
			return false
		}
		if extend {
			e.intent(contract.Intent{Kind: contract.IntentOutdent})
		} else {
			e.intent(contract.Intent{Kind: contract.IntentIndent})
		}
	case ui.KeyBackspace:
		e.intent(contract.Intent{Kind: contract.IntentDeleteBackward})
	case ui.KeyDelete:
		e.intent(contract.Intent{Kind: contract.IntentDeleteForward})
	default:
		return false
	}
	return true
}

// selectRaw hands a selection to the model; the event brings it back.
func (e *Editor) selectRaw(anchor, head int) {
	e.model.SetSelection(contract.Selection{Ranges: []contract.SelRange{{Anchor: anchor, Head: head}}})
	e.revealPending = true
	e.invalidate()
}

// snap moves an offset to where a caret can stand, as the row shows with it there.
func (e *Editor) snap(off int, aff contract.Affinity) int {
	r := e.rowAt(off)
	b := e.block(r)
	off = clampTo(off, b.Range)
	p := e.project(b, e.rows[r].leaf, revealKey{caret: off - b.Lead.Start})
	return e.opts.Projector.Snap(&p, off, aff)
}

func (e *Editor) moveTo(off int, extend bool) {
	e.model.BreakCoalescing()
	e.hasSticky = false
	e.moveKeepSticky(off, extend)
}

func (e *Editor) moveKeepSticky(off int, extend bool) {
	e.model.BreakCoalescing()
	off = e.snap(off, contract.Downstream)
	if extend {
		e.selectRaw(e.anchor, off)
	} else {
		e.selectRaw(off, off)
	}
}

// moveChar moves a grapheme in the display, crossing to the neighbouring row's edge
// at the row's own edge. The markers around the caret show, so it walks through
// them; a position that does not move on falls through to the row's edge.
func (e *Editor) moveChar(dir int, extend bool) {
	e.aff = ui.TextDownstream
	if !extend && e.anchor != e.head {
		edge := min(e.anchor, e.head)
		if dir > 0 {
			edge = max(e.anchor, e.head)
		}
		e.moveTo(edge, false)
		return
	}
	r := e.caretRow
	b := e.block(r)
	l := e.form(r)
	pr := e.opts.Projector
	head := clampTo(e.head, b.Range)
	if dir < 0 {
		if head > b.Range.Start {
			d := pr.ToDisplay(&l.proj, head, contract.Upstream)
			if d > 0 {
				if t := pr.ToSource(&l.proj, l.layout.PreviousBoundary(d), contract.Downstream); t < head {
					e.moveTo(t, extend)
					return
				}
			}
			e.moveTo(b.Range.Start, extend)
			return
		}
		if r > 0 {
			e.moveTo(e.block(r-1).Range.End, extend)
		}
		return
	}
	if head < b.Range.End {
		d := pr.ToDisplay(&l.proj, head, contract.Downstream)
		if t := pr.ToSource(&l.proj, l.layout.NextBoundary(d), contract.Upstream); t > head {
			e.moveTo(t, extend)
			return
		}
		e.moveTo(b.Range.End, extend)
		return
	}
	if r+1 < len(e.rows) {
		e.moveTo(e.block(r+1).Range.Start, extend)
	}
}

// moveWord moves over a word of the display, like moveChar at the edges.
func (e *Editor) moveWord(dir int, extend bool) {
	e.aff = ui.TextDownstream
	r := e.caretRow
	b := e.block(r)
	l := e.form(r)
	pr := e.opts.Projector
	head := clampTo(e.head, b.Range)
	if dir < 0 {
		if head <= b.Range.Start {
			if r > 0 {
				e.moveTo(e.block(r-1).Range.End, extend)
			}
			return
		}
		e.moveTo(min(pr.ToSource(&l.proj, prevWord(l.display, pr.ToDisplay(&l.proj, head, contract.Upstream)), contract.Downstream), head), extend)
		return
	}
	if head >= b.Range.End {
		if r+1 < len(e.rows) {
			e.moveTo(e.block(r+1).Range.Start, extend)
		}
		return
	}
	e.moveTo(max(pr.ToSource(&l.proj, nextWord(l.display, pr.ToDisplay(&l.proj, head, contract.Downstream)), contract.Upstream), head), extend)
}

// moveLine moves up or down a visual line keeping the x it started from, within the
// row and across rows. A row the caret enters is hit-tested as it shows now.
func (e *Editor) moveLine(dir int, extend bool) {
	r := e.caretRow
	l := e.form(r)
	pr := e.opts.Projector
	idx := pr.ToDisplay(&l.proj, clampTo(e.head, e.block(r).Range), contract.Downstream)
	c := l.layout.CaretAt(ui.TextCaretPosition{Index: idx, Affinity: e.aff})
	if !e.hasSticky {
		e.stickyX, e.hasSticky = c.X, true
	}
	lines := l.layout.Lines()
	k := lineIndex(lines, c.Y+c.H/2) + dir
	if k < 0 || k >= len(lines) {
		r += dir
		if r < 0 {
			e.moveKeepSticky(0, extend)
			return
		}
		if r >= len(e.rows) {
			e.moveKeepSticky(e.text.Len(), extend)
			return
		}
		l = e.form(r)
		lines = l.layout.Lines()
		k = 0
		if dir < 0 {
			k = max(len(lines)-1, 0)
		}
	}
	y := float32(0)
	if k < len(lines) {
		y = lines[k].Rect.Y + lines[k].Rect.H/2
	}
	pos := l.layout.PositionAt(ui.Point{X: e.stickyX, Y: y})
	e.aff = pos.Affinity
	e.moveKeepSticky(pr.ToSource(&l.proj, pos.Index, contract.Downstream), extend)
}

func lineIndex(lines []ui.TextLine, y float32) int {
	for k := range lines {
		if y < lines[k].Rect.Y+lines[k].Rect.H {
			return k
		}
	}
	return max(len(lines)-1, 0)
}

// moveEdge goes to the start or the end of the visual line.
func (e *Editor) moveEdge(dir int, extend bool) {
	l := e.form(e.caretRow)
	pr := e.opts.Projector
	idx := pr.ToDisplay(&l.proj, clampTo(e.head, e.block(e.caretRow).Range), contract.Downstream)
	c := l.layout.CaretAt(ui.TextCaretPosition{Index: idx, Affinity: e.aff})
	mid := c.Y + c.H/2
	if dir < 0 {
		pos := l.layout.PositionAt(ui.Point{X: -1e6, Y: mid})
		e.aff = ui.TextDownstream
		e.moveTo(pr.ToSource(&l.proj, pos.Index, contract.Downstream), extend)
		return
	}
	pos := l.layout.PositionAt(ui.Point{X: 1e6, Y: mid})
	e.aff = ui.TextUpstream
	e.moveTo(pr.ToSource(&l.proj, pos.Index, contract.Upstream), extend)
}

func (e *Editor) movePage(dir int, extend bool) {
	x, y, _ := e.caretRect()
	if !e.hasSticky {
		e.stickyX, e.hasSticky = x-e.contentX(), true
	}
	e.setScroll(e.scroll + float64(float32(dir)*e.viewH))
	e.place()
	e.moveKeepSticky(e.hit(e.stickyX+e.contentX(), y), extend)
}

// hit returns the source offset at a point of the element.
func (e *Editor) hit(x, y float32) int {
	if len(e.rows) == 0 || e.width <= 0 {
		return 0
	}
	yc := float64(y) + e.scroll
	p := e.hs.at(max(yc, 0))
	for range len(e.rows) {
		l := e.current(p)
		top := e.hs.top(p)
		switch {
		case yc < top && p > 0:
			p--
		case yc >= e.hs.top(p+1) && p < len(e.rows)-1:
			p++
		default:
			above, _ := blockSpace(e.block(p))
			pos := l.layout.PositionAt(ui.Point{X: x - e.contentX(), Y: float32(yc-top) - above})
			e.aff = pos.Affinity
			if p == e.caretRow && e.preedit != "" {
				return e.head
			}
			return clampTo(e.opts.Projector.ToSource(&l.proj, pos.Index, contract.Downstream), e.block(p).Range)
		}
	}
	return e.text.Len()
}

func (e *Editor) editContext() contract.EditContext {
	return contract.EditContext{Text: e.model.Text(), AST: e.model.AST(), Selection: e.model.Selection(), Meta: e.model.Meta(), Prefs: e.opts.Prefs()}
}

// intent runs a keystroke through the editing rules. All writes go through Apply.
func (e *Editor) intent(in contract.Intent) bool {
	if e.readOnly() {
		return false
	}
	tx, ok := e.opts.Editing.HandleIntent(e.editContext(), in)
	if !ok {
		return false
	}
	if !e.apply(tx) {
		return false
	}
	switch in.Kind {
	case contract.IntentInsertText, contract.IntentNewline:
		e.followUp()
	}
	return true
}

func (e *Editor) apply(tx contract.Transaction) bool {
	if _, err := e.model.Apply(tx); err != nil {
		return false
	}
	e.hasSticky = false
	e.aff = ui.TextDownstream
	return true
}

func (e *Editor) followUp() {
	if tx, ok := e.opts.Editing.FollowUp(e.editContext()); ok {
		e.apply(tx)
	}
}

func (e *Editor) command(name string) bool {
	switch name {
	case "undo":
		e.UnmarkText()
		if !e.readOnly() {
			e.model.Undo()
		}
	case "redo":
		e.UnmarkText()
		if !e.readOnly() {
			e.model.Redo()
		}
	case "selectAll":
		e.model.BreakCoalescing()
		e.selectRaw(0, e.text.Len())
		e.revealPending = false
	case "copy", "cut":
		rg := contract.SelRange{Anchor: e.anchor, Head: e.head}.Range()
		if rg.Start == rg.End {
			return true
		}
		e.services.WriteClipboard(e.text.Slice(rg))
		if name == "cut" {
			e.deleteSelection()
		}
	case "paste":
		if s := e.services.ReadClipboard(); s != "" {
			e.intent(contract.Intent{Kind: contract.IntentPaste, Paste: &contract.PastePayload{Plain: s}})
		}
	case "delete":
		e.deleteSelection()
	default:
		return false
	}
	return true
}

func (e *Editor) deleteSelection() {
	rg := contract.SelRange{Anchor: e.anchor, Head: e.head}.Range()
	if rg.Start == rg.End || e.readOnly() {
		return
	}
	c := contract.Caret(rg.Start)
	e.apply(contract.Transaction{Edits: []contract.TextEdit{{Range: rg}}, Selection: &c, Origin: contract.OriginUser})
}
