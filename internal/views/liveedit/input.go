package liveedit

import (
	"path"
	"strings"
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
		if e.slash.open && e.slashKey(ev) {
			e.blinkStart = ev.Time
			return true
		}
		if e.key(ev) {
			e.blinkStart = ev.Time
			return true
		}
	case ui.InputCommand:
		return e.command(ev.Text)
	case ui.InputPointerDown:
		if ev.Button == 1 {
			e.secondaryDown(ev)
			return false // the context menu opens on it
		}
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
		link, note := e.linkAt(ev.X, ev.Y), e.noteAt(ev.X, ev.Y)
		if link != e.hoverLink || note != e.hoverNote || ((link != "" || note != "") && (ev.X != e.hoverX || ev.Y != e.hoverY)) {
			e.hoverLink, e.hoverNote, e.hoverX, e.hoverY = link, note, ev.X, ev.Y
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

// linkAt returns the target of the link at a point of the element.
func (e *Editor) linkAt(x, y float32) string {
	r := e.rowAtPoint(x, y)
	if r < 0 {
		return ""
	}
	l := e.current(r)
	if l.html != nil {
		return e.htmlLinkAt(r, l, x, y)
	}
	pos := l.layout.PositionAt(ui.Point{X: x - e.contentX() - l.m.x, Y: y - e.rowTextY(r)})
	src := e.opts.Projector.ToSource(&l.proj, pos.Index, contract.Downstream)
	for i := range l.proj.Segments {
		if s := &l.proj.Segments[i]; s.Link != "" && src >= s.Src.Start && src < s.Src.End {
			if s.Style&contract.StyleWikiLink != 0 {
				return wikiPath(s.Link)
			}
			return s.Link
		}
	}
	return ""
}

// wikiPath is the file a wiki link names: [[Note]] is Note.md next to the
// document, [[Note#Part]] the same file.
func wikiPath(target string) string {
	if k := strings.IndexByte(target, '#'); k >= 0 {
		target = target[:k]
	}
	target = strings.TrimSpace(target)
	if target == "" {
		return ""
	}
	if path.Ext(target) == "" {
		target += ".md"
	}
	return target
}

func openable(link string) bool {
	return strings.HasPrefix(link, "https://") || strings.HasPrefix(link, "http://") || strings.HasPrefix(link, "mailto:")
}

// rowCommand handles a press on what a row draws around its text. It reports whether
// the press is used up: a checkbox toggles one byte, a code card's button copies, a
// modified click follows a link.
func (e *Editor) rowCommand(ev ui.InputEvent) bool {
	r := e.rowAtPoint(ev.X, ev.Y)
	if r < 0 {
		return false
	}
	if e.tocClick(r, ev.X, ev.Y) {
		return true
	}
	if e.htmlClick(r, ev) {
		return true
	}
	if c := e.taskBoxAt(r, ev.X, ev.Y); c != nil {
		e.toggleTask(c)
		return true
	}
	b := e.block(r)
	if b.Kind == contract.BlockCode {
		top := float32(e.hs.top(r) - e.scroll)
		right := e.contentX() + e.width
		if ev.Y >= top && ev.Y < top+codeHeader && ev.X >= right-90 && ev.X <= right {
			if e.hasView {
				e.services.WriteClipboard(e.codeText(b))
			}
			e.copiedBlock, e.copiedAt = b.ID, ev.Time
			return true
		}
	}
	if ev.Mods&ui.Cmd != 0 {
		if link := e.linkAt(ev.X, ev.Y); link != "" && !strings.HasPrefix(link, "#") {
			e.openLink(link)
			return true
		}
	}
	return false
}

// codeText returns a code block's content, without its fences.
func (e *Editor) codeText(b *contract.Block) string { return e.text.Slice(b.Content) }

// toggleTask flips the one byte between the brackets of a task marker.
func (e *Editor) toggleTask(c *contract.ContainerDecor) {
	if e.readOnly() || c.TaskSrc.End-c.TaskSrc.Start != 3 {
		return
	}
	at := c.TaskSrc.Start + 1
	ch := " "
	if c.Task == contract.TaskOpen {
		ch = "x"
		e.tickBlock, e.tickStart = c.Block, time.Now()
	}
	e.apply(contract.Transaction{Edits: []contract.TextEdit{{Range: contract.Range{Start: at, End: at + 1}, Text: ch}}, Origin: contract.OriginCommand, Label: "editing_undo_task"})
}

// rowAtPoint returns the row under a point of the element, the cell of a table's
// band by x, or -1 past the document's ends.
func (e *Editor) rowAtPoint(x, y float32) int {
	if len(e.rows) == 0 || e.width <= 0 {
		return -1
	}
	yc := float64(y) + e.scroll
	if yc < 0 || yc >= e.hs.top(len(e.rows)) {
		return -1
	}
	p := e.hs.at(yc)
	return e.cellByX(p, x)
}

// cellByX moves a row found by height, the last cell of a band, to the column at x.
func (e *Editor) cellByX(p int, x float32) int {
	v := &e.rows[p]
	if v.cols == 0 {
		return p
	}
	col := int((x - e.contentX()) / (e.width / float32(v.cols)))
	col = max(0, min(col, v.cols-1))
	return min(p-v.col+col, len(e.rows)-1)
}

// secondaryDown moves the caret to a press with the secondary button outside the
// selection, so the context menu acts where it was opened.
func (e *Editor) secondaryDown(ev ui.InputEvent) {
	off := e.hit(ev.X, ev.Y)
	if from, to := min(e.anchor, e.head), max(e.anchor, e.head); from < to && off >= from && off <= to {
		return
	}
	e.model.SetSelection(contract.Caret(off))
}

func (e *Editor) pointerDown(ev ui.InputEvent) {
	e.UnmarkText()
	if e.imageClick(ev) {
		return
	}
	if ev.Clicks <= 1 && ev.Mods&ui.Shift == 0 && e.rowCommand(ev) {
		e.blinkStart = ev.Time
		return
	}
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
		d := pr.ToDisplay(&l.proj, clampTo(off, e.rowRange(r)), contract.Downstream)
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
	if ev.Key == ui.KeyEnter && ev.Mods == ui.Ctrl && e.inTable() && !e.readOnly() {
		e.tableEdit(contract.EditCommand{ID: contract.CmdTableRowBelow, Rows: -1})
		return true
	}
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
			if extend { // Primary+Shift+V pastes plain text (edit.pastePlain).
				e.paste(true)
				return true
			}
			return e.command("paste")
		case ui.KeyHome:
			e.moveTo(e.snapShown(0), extend)
			return true
		case ui.KeyEnd:
			e.moveTo(e.text.Len(), extend)
			return true
		case ui.KeyEnter:
			// A row below the caret's, in a table (02 §5.8).
			if !extend && e.inTable() && !e.readOnly() {
				e.tableEdit(contract.EditCommand{ID: contract.CmdTableRowBelow, Rows: -1})
				return true
			}
		}
		if !word {
			switch ev.Key { // macOS
			case ui.KeyLeft:
				e.moveEdge(-1, extend)
				return true
			case ui.KeyRight:
				e.moveEdge(1, extend)
				return true
			case ui.KeyUp:
				e.moveTo(0, extend)
				return true
			case ui.KeyDown:
				e.moveTo(e.text.Len(), extend)
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
	off = clampTo(off, e.rowRange(r))
	p := e.project(b, e.rows[r].leaf, revealKey{caret: off - b.Lead.Start})
	return e.opts.Projector.Snap(&p, off, aff)
}

// snapShown moves off past the marks hidden while the caret is elsewhere: Home
// lands where the text starts, after "# " or "- ", as in Typora, not on the
// marks the caret there would show.
func (e *Editor) snapShown(off int) int {
	r := e.rowAt(off)
	b := e.block(r)
	off = clampTo(off, e.rowRange(r))
	p := e.project(b, e.rows[r].leaf, revealKey{caret: -1})
	return e.opts.Projector.Snap(&p, off, contract.Downstream)
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
	rg := e.rowRange(r)
	l := e.form(r)
	pr := e.opts.Projector
	head := clampTo(e.head, rg)
	if dir < 0 {
		if head > rg.Start {
			d := pr.ToDisplay(&l.proj, head, contract.Upstream)
			if d > 0 {
				if t := pr.ToSource(&l.proj, l.layout.PreviousBoundary(d), contract.Downstream); t < head {
					e.moveTo(t, extend)
					return
				}
			}
			e.moveTo(rg.Start, extend)
			return
		}
		if r > 0 {
			e.moveTo(e.rowRange(r-1).End, extend)
		}
		return
	}
	if head < rg.End {
		d := pr.ToDisplay(&l.proj, head, contract.Downstream)
		if t := pr.ToSource(&l.proj, l.layout.NextBoundary(d), contract.Upstream); t > head {
			e.moveTo(t, extend)
			return
		}
		e.moveTo(rg.End, extend)
		return
	}
	if r+1 < len(e.rows) {
		e.moveTo(e.rowRange(r+1).Start, extend)
	}
}

// moveWord moves over a word of the display, like moveChar at the edges.
func (e *Editor) moveWord(dir int, extend bool) {
	e.aff = ui.TextDownstream
	r := e.caretRow
	rg := e.rowRange(r)
	l := e.form(r)
	pr := e.opts.Projector
	head := clampTo(e.head, rg)
	if dir < 0 {
		if head <= rg.Start {
			if r > 0 {
				e.moveTo(e.rowRange(r-1).End, extend)
			}
			return
		}
		e.moveTo(min(pr.ToSource(&l.proj, prevWord(l.display, pr.ToDisplay(&l.proj, head, contract.Upstream)), contract.Downstream), head), extend)
		return
	}
	if head >= rg.End {
		if r+1 < len(e.rows) {
			e.moveTo(e.rowRange(r+1).Start, extend)
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
	idx := pr.ToDisplay(&l.proj, clampTo(e.head, e.rowRange(r)), contract.Downstream)
	c := l.layout.CaretAt(ui.TextCaretPosition{Index: idx, Affinity: e.aff})
	if !e.hasSticky {
		e.stickyX, e.hasSticky = c.X, true
	}
	lines := l.layout.Lines()
	k := lineIndex(lines, c.Y+c.H/2) + dir
	if k < 0 || k >= len(lines) {
		r = e.rowAcross(r, dir)
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

// rowAcross returns the row above or below r: past a table cell, the same column of
// the next table row. It may return a row outside the document.
func (e *Editor) rowAcross(r, dir int) int {
	v := e.rows[r]
	if v.cols == 0 {
		return r + dir
	}
	first := r - v.col
	if dir < 0 {
		if first == 0 {
			return -1
		}
		up := first - 1
		if w := e.rows[up]; w.cols > 0 {
			return up - w.col + min(v.col, w.cols-1)
		}
		return up
	}
	down := first + v.cols
	if down >= len(e.rows) {
		return down
	}
	if w := e.rows[down]; w.cols > 0 {
		return down + min(v.col, w.cols-1)
	}
	return down
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
	idx := pr.ToDisplay(&l.proj, clampTo(e.head, e.rowRange(e.caretRow)), contract.Downstream)
	c := l.layout.CaretAt(ui.TextCaretPosition{Index: idx, Affinity: e.aff})
	mid := c.Y + c.H/2
	if dir < 0 {
		pos := l.layout.PositionAt(ui.Point{X: -1e6, Y: mid})
		e.aff = ui.TextDownstream
		e.moveTo(e.snapShown(pr.ToSource(&l.proj, pos.Index, contract.Downstream)), extend)
		return
	}
	pos := l.layout.PositionAt(ui.Point{X: 1e6, Y: mid})
	e.aff = ui.TextUpstream
	e.moveTo(pr.ToSource(&l.proj, pos.Index, contract.Upstream), extend)
}

func (e *Editor) movePage(dir int, extend bool) {
	x, y, _ := e.caretRect()
	if !e.hasSticky {
		e.stickyX, e.hasSticky = x-e.contentX()-e.form(e.caretRow).m.x, true
	}
	e.setScroll(e.scroll + float64(float32(dir)*e.viewH))
	e.place()
	e.moveKeepSticky(e.hit(e.stickyX+e.contentX()+e.form(e.caretRow).m.x, y), extend)
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
			if q := e.cellByX(p, x); q != p {
				p = q
				l = e.current(p)
			}
			pos := l.layout.PositionAt(ui.Point{X: x - e.contentX() - l.m.x, Y: float32(yc-top) - l.m.above})
			e.aff = pos.Affinity
			if p == e.caretRow && e.preedit != "" {
				return e.head
			}
			return clampTo(e.opts.Projector.ToSource(&l.proj, pos.Index, contract.Downstream), e.rowRange(p))
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
	if in.Kind == contract.IntentInsertText {
		e.typing()
		defer e.afterTyping()
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
		if e.copySelection(copyAll) && name == "cut" {
			e.deleteSelection()
		}
	case "paste":
		e.paste(false)
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
