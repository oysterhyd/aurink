package liveedit

import (
	"aurink.local/spikes/contract-rc/contract"
	"github.com/egoist/mygo/ui"
)

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
		e.UnmarkText()
		off := e.hit(ev.X, ev.Y)
		anchor := off
		if ev.Mods&ui.Shift != 0 {
			anchor = e.anchor
		}
		e.breakTyping()
		e.hasSticky = false
		e.setSelection(anchor, off)
		e.dragging = true
		e.blinkStart = ev.Time
		return true
	case ui.InputPointerMove:
		if e.dragging && ev.Button == 0 {
			e.setSelection(e.anchor, e.hit(ev.X, ev.Y))
			return true
		}
	case ui.InputPointerUp:
		was := e.dragging
		e.dragging = false
		return was
	case ui.InputScroll:
		before := e.scroll
		e.setScroll(e.scroll + float64(ev.DY))
		return e.scroll != before
	}
	return false
}

func (e *Editor) key(ev ui.InputEvent) bool {
	extend := ev.Mods&ui.Shift != 0
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
			e.moveTo(len(e.d.src()), extend)
			return true
		}
		return false
	}
	if ev.Mods&^ui.Shift != 0 {
		return false
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
		e.replaceSelection(e.d.eol+e.d.eol, contract.OriginUser, false)
	case ui.KeyBackspace:
		e.backspace()
	case ui.KeyDelete:
		e.deleteForward()
	default:
		return false
	}
	return true
}

func (e *Editor) moveTo(off int, extend bool) {
	e.breakTyping()
	if extend {
		e.setSelection(e.anchor, off)
	} else {
		e.setSelection(off, off)
	}
}

// moveChar moves a grapheme in the display, crossing to the neighbouring block's
// edge at the block's own edge.
func (e *Editor) moveChar(dir int, extend bool) {
	e.hasSticky = false
	e.aff = ui.TextDownstream
	if !extend && e.anchor != e.caret {
		edge := min(e.anchor, e.caret)
		if dir > 0 {
			edge = max(e.anchor, e.caret)
		}
		e.moveTo(edge, false)
		return
	}
	i := e.caretBlock
	b := &e.d.blocks[i]
	l := e.laidFor(i, true)
	switch {
	case dir < 0 && e.caret <= b.CStart:
		if i > 0 {
			e.moveTo(e.d.blocks[i-1].CEnd, extend)
		}
	case dir > 0 && e.caret >= b.CEnd:
		if i+1 < len(e.d.blocks) {
			e.moveTo(e.d.blocks[i+1].CStart, extend)
		}
	case dir < 0:
		d := toDisplay(&l.proj, e.caret, contract.Upstream)
		e.moveTo(toSource(&l.proj, l.layout.PreviousBoundary(d), contract.Downstream), extend)
	default:
		d := toDisplay(&l.proj, e.caret, contract.Downstream)
		e.moveTo(toSource(&l.proj, l.layout.NextBoundary(d), contract.Upstream), extend)
	}
}

// moveLine moves up or down a visual line keeping the x it started from, within
// the block and across blocks. The target block is hit-tested in its revealed
// form, since that is how it shows once the caret is in it.
func (e *Editor) moveLine(dir int, extend bool) {
	i := e.caretBlock
	l := e.laidFor(i, true)
	idx := toDisplay(&l.proj, e.caret, contract.Downstream)
	r := l.layout.CaretAt(ui.TextCaretPosition{Index: idx, Affinity: e.aff})
	if !e.hasSticky {
		e.stickyX, e.hasSticky = r.X, true
	}
	lines := l.layout.Lines()
	k := lineIndex(lines, r.Y+r.H/2)
	k += dir
	if k < 0 || k >= len(lines) {
		i += dir
		if i < 0 {
			e.moveToKeepSticky(0, extend)
			return
		}
		if i >= len(e.d.blocks) {
			e.moveToKeepSticky(len(e.d.src()), extend)
			return
		}
		l = e.laidFor(i, true)
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
	e.moveToKeepSticky(toSource(&l.proj, pos.Index, contract.Downstream), extend)
}

func (e *Editor) moveToKeepSticky(off int, extend bool) {
	sticky, has := e.stickyX, e.hasSticky
	e.moveTo(off, extend)
	e.stickyX, e.hasSticky = sticky, has
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
	e.hasSticky = false
	l := e.laidFor(e.caretBlock, true)
	idx := toDisplay(&l.proj, e.caret, contract.Downstream)
	r := l.layout.CaretAt(ui.TextCaretPosition{Index: idx, Affinity: e.aff})
	mid := r.Y + r.H/2
	if dir < 0 {
		pos := l.layout.PositionAt(ui.Point{X: -1e6, Y: mid})
		e.aff = ui.TextDownstream
		e.moveTo(toSource(&l.proj, pos.Index, contract.Downstream), extend)
		return
	}
	pos := l.layout.PositionAt(ui.Point{X: 1e6, Y: mid})
	e.aff = ui.TextUpstream
	e.moveTo(toSource(&l.proj, pos.Index, contract.Upstream), extend)
}

func (e *Editor) movePage(dir int, extend bool) {
	x, y, _ := e.caretRect()
	if !e.hasSticky {
		e.stickyX, e.hasSticky = x-e.contentX(), true
	}
	step := float32(dir) * e.viewH
	e.setScroll(e.scroll + float64(step))
	e.moveToKeepSticky(e.hit(e.stickyX+e.contentX(), y), extend)
}

// hit returns the source offset at a point of the element.
func (e *Editor) hit(x, y float32) int {
	yc := float64(y) + e.scroll
	p := e.hs.at(max(yc, 0))
	for range len(e.views) {
		l := e.current(p)
		top := e.hs.top(p)
		switch {
		case yc < top && p > 0:
			p--
		case yc >= e.hs.top(p+1) && p < len(e.views)-1:
			p++
		default:
			above, _ := blockSpace(&e.d.blocks[p])
			pos := l.layout.PositionAt(ui.Point{X: x - e.contentX(), Y: float32(yc-top) - above})
			e.aff = pos.Affinity
			return toSource(&l.proj, pos.Index, contract.Downstream)
		}
	}
	return len(e.d.src())
}

func (e *Editor) backspace() {
	if e.anchor != e.caret {
		e.replaceSelection("", contract.OriginUser, false)
		return
	}
	i := e.caretBlock
	b := &e.d.blocks[i]
	if e.caret <= b.CStart {
		if i > 0 {
			e.commit(e.d.blocks[i-1].CEnd, e.caret, "", contract.OriginUser, false, false)
		}
		return
	}
	l := e.laidFor(i, true)
	d := toDisplay(&l.proj, e.caret, contract.Upstream)
	e.commit(toSource(&l.proj, l.layout.PreviousBoundary(d), contract.Downstream), e.caret, "", contract.OriginUser, false, false)
}

func (e *Editor) deleteForward() {
	if e.anchor != e.caret {
		e.replaceSelection("", contract.OriginUser, false)
		return
	}
	i := e.caretBlock
	b := &e.d.blocks[i]
	if e.caret >= b.CEnd {
		if i+1 < len(e.d.blocks) {
			e.commit(e.caret, e.d.blocks[i+1].CStart, "", contract.OriginUser, false, false)
		}
		return
	}
	l := e.laidFor(i, true)
	d := toDisplay(&l.proj, e.caret, contract.Downstream)
	e.commit(e.caret, toSource(&l.proj, l.layout.NextBoundary(d), contract.Upstream), "", contract.OriginUser, false, false)
}

func (e *Editor) replaceSelection(text string, origin contract.Origin, typing bool) {
	e.commit(min(e.anchor, e.caret), max(e.anchor, e.caret), text, origin, typing, false)
}

// commit makes an edit a step of history, the caret after the inserted text.
// typing joins it to the step before while typing goes on.
func (e *Editor) commit(from, to int, text string, origin contract.Origin, typing, join bool) {
	if from == to && text == "" {
		return
	}
	old := e.d.src()[from:to]
	a0, c0 := e.anchor, e.caret
	e.applyEdit(from, to, text)
	e.hasSticky = false
	e.aff = ui.TextDownstream
	after := from + len(text)
	e.setSelection(after, after)
	e.redo = e.redo[:0]
	if n := len(e.undo); typing && from == to && n > 0 && e.undo[n-1].typing && e.undo[n-1].from+len(e.undo[n-1].new) == from {
		e.undo[n-1].new += text
		e.undo[n-1].a1, e.undo[n-1].c1 = e.anchor, e.caret
		return
	}
	e.undo = append(e.undo, edit{from: from, old: old, new: text, a0: a0, c0: c0, a1: e.anchor, c1: e.caret, join: join, typing: typing, origin: origin})
}

func (e *Editor) breakTyping() {
	if n := len(e.undo); n > 0 {
		e.undo[n-1].typing = false
	}
}

func (e *Editor) command(name string) bool {
	switch name {
	case "undo":
		e.UnmarkText()
		for len(e.undo) > 0 {
			u := e.undo[len(e.undo)-1]
			e.undo = e.undo[:len(e.undo)-1]
			e.applyEdit(u.from, u.from+len(u.new), u.old)
			e.setSelection(u.a0, u.c0)
			e.redo = append(e.redo, u)
			if !u.join {
				break
			}
		}
	case "redo":
		e.UnmarkText()
		for first := true; len(e.redo) > 0; first = false {
			u := e.redo[len(e.redo)-1]
			if !first && !u.join {
				break
			}
			e.redo = e.redo[:len(e.redo)-1]
			e.applyEdit(u.from, u.from+len(u.old), u.new)
			e.setSelection(u.a1, u.c1)
			u.typing = false
			e.undo = append(e.undo, u)
		}
	case "selectAll":
		e.breakTyping()
		e.setSelection(0, len(e.d.src()))
		e.revealPending = false
	case "copy", "cut":
		a, z := min(e.anchor, e.caret), max(e.anchor, e.caret)
		if a == z {
			return true
		}
		e.services.WriteClipboard(e.d.src()[a:z])
		if name == "cut" {
			e.replaceSelection("", contract.OriginUser, false)
		}
	case "paste":
		if s := e.services.ReadClipboard(); s != "" {
			e.replaceSelection(s, contract.OriginPaste, false)
		}
	case "delete":
		e.replaceSelection("", contract.OriginUser, false)
	default:
		return false
	}
	return true
}
