package liveedit

import "aurink/internal/contract"

// A draft is Typora's way with a block typed on its line: "# ab", "---",
// "```js" or "<div>" turn into a heading, a rule, a code block or HTML only
// once the caret leaves the line (Enter, a click elsewhere). Until then the
// line keeps a paragraph's size and place, its marks showing, and a heading's
// text bold, so nothing moves while it is typed (measured on Typora 1.0,
// 2026-11-04). Lists, quotes and tasks keep a line's height and turn at once,
// as in Typora.
//
// The block is already the new kind in the source and the AST; the draft is
// only how its row shows: its first line, as a paragraph.

// draftable reports the kinds that wait for the caret to leave their line.
func draftable(k contract.BlockKind) bool {
	switch k {
	case contract.BlockHeading, contract.BlockThematicBreak, contract.BlockCode, contract.BlockHTML, contract.BlockMath:
		return true
	}
	return false
}

// typing notes the kind of the block under the caret before a keystroke's
// text goes in; afterTyping drafts the block if the text turned it into a
// draftable kind.
func (e *Editor) typing() {
	e.typedFrom, e.typingNow = contract.BlockBlank, false
	if e.caretRow >= 0 && e.caretRow < len(e.rows) {
		e.typedFrom, e.typingNow = e.block(e.caretRow).Kind, true
	}
}

func (e *Editor) afterTyping() { e.typingNow = false }

// noteDraft runs after each edit and caret move: it starts a draft when
// typing turned the caret's block into a draftable kind, and ends one when
// the caret has left its first line.
func (e *Editor) noteDraft() {
	if e.caretRow < 0 || e.caretRow >= len(e.rows) {
		e.draftAt = -1
		return
	}
	b := e.block(e.caretRow)
	if e.typingNow && draftable(b.Kind) && b.Kind != e.typedFrom && e.draftAt != b.Range.Start {
		e.draftAt = b.Range.Start
	}
	if e.draftAt < 0 {
		return
	}
	if !draftable(b.Kind) || b.Range.Start != e.draftAt || e.head > e.firstLineEnd(b) || e.anchor != e.head {
		e.draftAt = -1
		e.touched = append(e.touched, e.rows[e.caretRow].id)
	}
}

// firstLineEnd returns where the first line of b ends, before its line break.
func (e *Editor) firstLineEnd(b *contract.Block) int {
	line := e.text.LineOf(b.Range.Start)
	end := min(e.text.LineStart(line+1), b.Range.End)
	if end > b.Range.Start && end <= e.text.Len() && e.text.Slice(contract.Range{Start: end - 1, End: end}) == "\n" {
		end--
		if end > b.Range.Start && e.text.Slice(contract.Range{Start: end - 1, End: end}) == "\r" {
			end--
		}
	}
	return end
}

// isDraft reports whether row r shows as a draft.
func (e *Editor) isDraft(r int, b *contract.Block) bool {
	return r == e.caretRow && e.draftAt >= 0 && b.Range.Start == e.draftAt && draftable(b.Kind)
}

// draftOf turns the block's projection, all its marks shown, into the draft:
// a paragraph showing the first line; the lines after it are hidden.
func (e *Editor) draftOf(b *contract.Block, p contract.BlockProjection) contract.BlockProjection {
	end := e.firstLineEnd(b)
	segs := make([]contract.Segment, 0, len(p.Segments)+1)
	for _, s := range p.Segments {
		switch {
		case s.Src.Start >= end:
			s.Kind, s.Display, s.Object, s.Style = contract.SegHidden, "", nil, 0
			segs = append(segs, s)
		case s.Src.End > end && (s.Kind == contract.SegText || s.Kind == contract.SegMarker) && len(s.Display) == s.Src.Len():
			// The segment runs on past the line: its first part shows.
			cut := end - s.Src.Start
			head, tail := s, s
			head.Src.End, head.Display = end, s.Display[:cut]
			tail.Src.Start, tail.Kind, tail.Display, tail.Style = end, contract.SegHidden, "", 0
			segs = append(segs, head, tail)
		default:
			segs = append(segs, s)
		}
	}
	p.Segments = segs
	p.Kind, p.Level, p.Lang = contract.BlockParagraph, 0, ""
	p.Sig ^= 0x9e3779b97f4a7c15
	return p
}
