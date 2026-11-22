package editing

import (
	"strconv"
	"strings"

	"aurink/internal/contract"
	"aurink/internal/core/markdown"
)

// Run executes a formatting or block command in the supplied snapshot.
func (Editor) Run(ctx contract.EditContext, cmd contract.EditCommand) (contract.Transaction, bool, error) {
	if !valid(ctx) {
		return contract.Transaction{}, false, &contract.Error{Code: contract.CodeInvalid, Op: "editing.Run", Detail: "invalid context or selection"}
	}
	if !cmd.Replace.Empty() {
		return runWithReplace(ctx, cmd)
	}
	if strings.HasPrefix(string(cmd.ID), "table.") {
		return tableCommand(ctx, cmd)
	}
	if cmd.ID == contract.CmdInsertFootnote || cmd.ID == contract.CmdInsertImages {
		return insertCommand(ctx, cmd)
	}
	if cmd.ID == contract.CmdInsertFrontMatter {
		return insertFrontMatter(ctx)
	}
	if strings.HasPrefix(string(cmd.ID), "format.") || cmd.ID == contract.CmdInsertLink {
		inTable := selectionHas(ctx, contract.BlockTable)
		if inTable && ctx.Selection.Primary().Empty() {
			if _, ok := findTable(ctx); !ok {
				return contract.Transaction{}, false, nil
			}
		}
		tx, ok, err := inlineCommand(ctx, cmd)
		if ok && inTable && !preservesTables(ctx, tx.Edits) {
			return contract.Transaction{}, false, nil
		}
		return tx, ok, err
	}
	if cmd.ID == contract.CmdBlockHeadingUp || cmd.ID == contract.CmdBlockHeadingDown {
		_, b := leaf(ctx, ctx.Selection.Primary().Range().Start)
		if b == nil || b.Kind != contract.BlockParagraph && b.Kind != contract.BlockHeading {
			return contract.Transaction{}, false, nil
		}
		level := b.Level
		if cmd.ID == contract.CmdBlockHeadingUp {
			if level == 1 {
				return contract.Transaction{}, false, nil
			}
			if level == 0 {
				level = 7
			}
			level--
		} else {
			if level == 0 {
				return contract.Transaction{}, false, nil
			}
			level++
		}
		if level == 7 {
			cmd.ID = contract.CmdBlockParagraph
		} else {
			cmd.ID, cmd.Level = contract.CmdBlockHeading, level
		}
	}
	if strings.HasPrefix(string(cmd.ID), string(contract.CmdBlockHeading)) && cmd.ID != contract.CmdBlockHeading {
		level, err := strconv.Atoi(strings.TrimPrefix(string(cmd.ID), string(contract.CmdBlockHeading)))
		if err != nil || level < 1 || level > 6 {
			return contract.Transaction{}, false, nil
		}
		cmd.ID, cmd.Level = contract.CmdBlockHeading, level
	}
	if strings.HasPrefix(string(cmd.ID), "block.") {
		if selectionHas(ctx, contract.BlockHTML, contract.BlockFrontMatter, contract.BlockMath, contract.BlockTable) && cmd.ID != contract.CmdBlockMoveUp && cmd.ID != contract.CmdBlockMoveDown {
			return contract.Transaction{}, false, nil
		}
		if selectionHas(ctx, contract.BlockCode) && cmd.ID != contract.CmdBlockCode && cmd.ID != contract.CmdBlockIndent && cmd.ID != contract.CmdBlockOutdent && cmd.ID != contract.CmdBlockMoveUp && cmd.ID != contract.CmdBlockMoveDown {
			return contract.Transaction{}, false, nil
		}
	}
	switch cmd.ID {
	case contract.CmdBlockHeading:
		if cmd.Level < 1 || cmd.Level > 6 {
			return contract.Transaction{}, false, &contract.Error{Code: contract.CodeInvalid, Op: "editing.Run", Detail: "heading level must be 1..6"}
		}
	case contract.CmdBlockIndent, contract.CmdBlockOutdent:
		tx, ok := indentation(ctx, cmd.ID == contract.CmdBlockOutdent, contract.OriginCommand)
		return tx, ok, nil
	case contract.CmdBlockToggleTask:
		return toggleTask(ctx)
	case contract.CmdBlockCode:
		return codeCommand(ctx, cmd)
	case contract.CmdBlockMath:
		return fenceCommand(ctx, cmd, true)
	case contract.CmdBlockMermaid:
		cmd.Text = "mermaid"
		return codeCommand(ctx, cmd)
	case contract.CmdBlockTOC:
		return insertTOC(ctx)
	case contract.CmdBlockHR:
		r := ctx.Selection.Primary().Range()
		s := "---" + eol(ctx) + eol(ctx)
		if strings.TrimSpace(lineAt(ctx, r.Start).s) != "" {
			s = eol(ctx) + eol(ctx) + s
		}
		return replace(r, s, contract.OriginCommand, "block"), true, nil
	case contract.CmdBlockMoveUp, contract.CmdBlockMoveDown:
		return moveBlock(ctx, cmd.ID == contract.CmdBlockMoveDown)
	case contract.CmdBlockParagraph, contract.CmdBlockQuote, contract.CmdBlockBulletList, contract.CmdBlockOrderedList, contract.CmdBlockTaskList:
	default:
		return contract.Transaction{}, false, nil
	}
	return changeBlock(ctx, cmd)
}

func changeBlock(ctx contract.EditContext, cmd contract.EditCommand) (contract.Transaction, bool, error) {
	r := ctx.Selection.Primary().Range()
	if ancestor(ctx, r.Start, contract.BlockCode) != nil || ancestor(ctx, r.Start, contract.BlockTable) != nil {
		return contract.Transaction{}, false, nil
	}
	var edits []contract.TextEdit
	lines := selectedLines(ctx)
	remove := true
	for _, l := range lines {
		if !r.Empty() && strings.TrimSpace(l.s[l.content:]) == "" {
			continue
		}
		if !blockMatches(l, cmd.ID) {
			remove = false
		}
	}
	n := 0
	for _, l := range lines {
		if !r.Empty() && strings.TrimSpace(l.s[l.content:]) == "" {
			if cmd.ID == contract.CmdBlockQuote && l.list < 0 {
				quoted := false
				for _, part := range l.parts {
					if part.kind == '>' {
						quoted = true
						if remove {
							edits = append(edits, edit(l.start+part.a, l.start+part.b, ""))
						}
						break
					}
				}
				if !remove && !quoted {
					edits = append(edits, edit(l.start+l.indent, l.start+l.indent, ">"))
				}
			}
			continue
		}
		at, end, mark := l.content, l.content, ""
		heading := -1
		for j, p := range l.parts {
			if p.kind == '#' {
				heading = j
			}
		}
		switch cmd.ID {
		case contract.CmdBlockHeading, contract.CmdBlockParagraph:
			separator := " "
			if heading >= 0 {
				at = l.parts[heading].a
				separator = trailingSpace(l.s[at:end])
			}
			_, block := leaf(ctx, l.start+l.content)
			if cmd.ID == contract.CmdBlockHeading && (block == nil || block.Kind != contract.BlockHeading || block.Level != cmd.Level) {
				mark = strings.Repeat("#", cmd.Level) + separator
			}
			// Setext underlines and optional ATX closing markers are separate edits.
			_, b := leaf(ctx, l.start+l.content)
			if b != nil && b.Kind == contract.BlockHeading {
				for _, m := range b.Markers {
					if m.Start >= l.start+end && !m.Empty() {
						if cmd.ID == contract.CmdBlockHeading && mark != "" && m.Start <= l.end {
							continue
						}
						if m.Start > l.end {
							before := lineAt(ctx, m.Start)
							edits = append(edits, edit(before.start, before.next, ""))
						} else {
							edits = append(edits, edit(m.Start, m.End, ""))
						}
					}
				}
			}
		case contract.CmdBlockQuote:
			at, end = l.indent, l.indent
			mark = "> "
			for _, p := range l.parts {
				if p.kind == '>' {
					at, end, mark = p.a, p.b, ""
					if !remove {
						mark = l.s[at:end]
					}
				}
			}
		case contract.CmdBlockBulletList, contract.CmdBlockOrderedList, contract.CmdBlockTaskList:
			separator := " "
			if l.list >= 0 {
				p := l.parts[l.list]
				at, end = p.a, p.b
				separator = trailingSpace(l.s[at:end])
			}
			switch cmd.ID {
			case contract.CmdBlockBulletList:
				mark = bullet(ctx) + separator
				if l.list >= 0 && l.delim != '.' && l.delim != ')' && l.task < 0 {
					mark = ""
					if !remove {
						mark = l.s[at:end]
					}
				}
			case contract.CmdBlockOrderedList:
				mark = strconv.Itoa(n+1) + "." + separator
				if l.list >= 0 && (l.delim == '.' || l.delim == ')') && l.task < 0 {
					mark = ""
					if !remove {
						mark = l.s[at:end]
					}
				}
			case contract.CmdBlockTaskList:
				mark = bullet(ctx) + " [ ]" + separator
				if l.task >= 0 {
					mark = ""
					if !remove {
						mark = l.s[at:end]
					}
				}
			}
		}
		if (at != end || mark != "") && l.s[at:end] != mark {
			edits = append(edits, edit(l.start+at, l.start+end, mark))
		}
		n++
	}
	edits = orderedEdits(edits)
	return transaction(edits, mapped(ctx.Selection, edits), contract.OriginCommand, "block"), true, nil
}

func blockMatches(l sourceLine, id contract.CommandID) bool {
	switch id {
	case contract.CmdBlockBulletList:
		return l.list >= 0 && l.delim != '.' && l.delim != ')' && l.task < 0
	case contract.CmdBlockOrderedList:
		return l.list >= 0 && (l.delim == '.' || l.delim == ')') && l.task < 0
	case contract.CmdBlockTaskList:
		return l.task >= 0
	case contract.CmdBlockQuote:
		for _, p := range l.parts {
			if p.kind == '>' {
				return true
			}
		}
	}
	return false
}

func trailingSpace(s string) string {
	i := len(s)
	for i > 0 && space(s[i-1]) {
		i--
	}
	return s[i:]
}

func toggleTask(ctx contract.EditContext) (contract.Transaction, bool, error) {
	var edits []contract.TextEdit
	for _, l := range selectedLines(ctx) {
		at := -1
		if l.task >= 0 {
			at = l.start + l.task + 1
		} else if item := ancestor(ctx, l.start+l.content, contract.BlockItem); item != nil && !item.TaskMarker.Empty() {
			at = item.TaskMarker.Start + 1
		}
		if at >= 0 {
			mark := "x"
			if ctx.Text.String()[at] != ' ' {
				mark = " "
			}
			edits = append(edits, edit(at, at+1, mark))
		}
	}
	edits = orderedEdits(edits)
	return transaction(edits, ctx.Selection, contract.OriginCommand, "task"), len(edits) > 0, nil
}

func codeCommand(ctx contract.EditContext, cmd contract.EditCommand) (contract.Transaction, bool, error) {
	return fenceCommand(ctx, cmd, false)
}

func fenceCommand(ctx contract.EditContext, cmd contract.EditCommand, math bool) (contract.Transaction, bool, error) {
	if strings.ContainsAny(cmd.Text, "\r\n`") {
		return contract.Transaction{}, false, &contract.Error{Code: contract.CodeInvalid, Op: "editing.Run", Detail: "invalid code language"}
	}
	r := ctx.Selection.Primary().Range()
	if r.Empty() {
		_, block := leaf(ctx, r.Start)
		if block != nil && block.Kind == contract.BlockParagraph && strings.TrimSpace(ctx.Text.Slice(block.Content)) != "" {
			last := lineAt(ctx, block.Range.End)
			at := last.end
			lead := lineAt(ctx, block.Range.Start).containerContinuation()
			fence := ctx.Prefs.FenceDelim
			if fence != "~~~" {
				fence = "```"
			}
			prefix := eol(ctx) + lead + eol(ctx) + lead
			if math {
				fence, prefix = "$$", eol(ctx)+lead
			}
			open := prefix + fence + cmd.Text + eol(ctx) + lead
			tx := replace(contract.Range{Start: at, End: at}, open+eol(ctx)+lead+fence, contract.OriginCommand, "block")
			sel := contract.Caret(at + len(open))
			tx.Selection = &sel
			return tx, true, nil
		}
	}
	if selectionHas(ctx, contract.BlockCode) {
		b := ancestor(ctx, r.Start, contract.BlockCode)
		if b == nil || r.End > b.Range.End {
			return contract.Transaction{}, false, nil
		}
	}
	if b := ancestor(ctx, r.Start, contract.BlockCode); b != nil {
		var edits []contract.TextEdit
		if b.Fenced {
			for j, m := range b.Markers {
				l := lineAt(ctx, m.Start)
				if j == 0 && len(b.Lines) > 0 {
					body := lineAt(ctx, b.Lines[0].Source.Start)
					lead := l.containerContinuation()
					end := body.start
					if strings.HasPrefix(body.s, lead) {
						end += len(lead)
					}
					edits = append(edits, edit(m.Start, end, ""))
				} else {
					edits = append(edits, edit(l.start, l.next, ""))
				}
			}
		} else {
			for _, ln := range b.Lines {
				l := lineAt(ctx, ln.Source.Start)
				end := min(l.indent, 4)
				if end > 0 {
					edits = append(edits, edit(l.start, l.start+end, ""))
				}
			}
		}
		edits = orderedEdits(edits)
		return transaction(edits, mapped(ctx.Selection, edits), contract.OriginCommand, "block"), true, nil
	}
	if ancestor(ctx, r.Start, contract.BlockTable) != nil {
		return contract.Transaction{}, false, nil
	}
	lines := selectedLines(ctx)
	a, b := lines[0].start, lines[len(lines)-1].end
	if ctx.Selection.Primary().Empty() {
		_, block := leaf(ctx, r.Start)
		if block != nil {
			a, b = lineAt(ctx, block.Range.Start).start, lineAt(ctx, block.Range.End).end
		}
	}
	first := lineAt(ctx, a)
	lead, remove := "", 0
	keepContainer := true
	for _, l := range lines[1:] {
		if !strings.HasPrefix(l.s, first.containerContinuation()) {
			keepContainer = false
		}
	}
	if len(lines) == 1 || keepContainer {
		a = first.start + first.content
		lead = first.containerContinuation()
		for _, part := range first.parts {
			if part.kind == '#' {
				a, remove = first.start+part.a, part.b-part.a
			}
		}
	}
	s := ctx.Text.Slice(contract.Range{Start: a, End: b})
	fence := ctx.Prefs.FenceDelim
	if fence != "~~~" {
		fence = "```"
	}
	if math {
		fence = "$$"
		if strings.Contains(s, fence) {
			return contract.Transaction{}, false, nil
		}
	} else {
		for strings.Contains(s, fence) {
			fence += fence[:1]
		}
	}
	nl := eol(ctx)
	open, close := fence+cmd.Text+nl+lead, nl+lead+fence
	edits := []contract.TextEdit{edit(a, a+remove, open), edit(b, b, close)}
	_, block := leaf(ctx, r.Start)
	if block != nil && block.Kind == contract.BlockHeading {
		for _, m := range block.Markers {
			if m.Start < a+remove {
				continue
			}
			start := m.Start
			if start > first.end {
				start = first.end
			}
			edits = append(edits, edit(start, m.End, ""))
		}
	}
	edits = orderedEdits(edits)
	if a == b {
		edits = []contract.TextEdit{edit(a, a, open+close)}
	}
	sel := mapped(ctx.Selection, edits).Primary()
	old := ctx.Selection.Primary()
	if old.Anchor == b {
		sel.Anchor -= len(close)
	}
	if old.Head == b {
		sel.Head -= len(close)
	}
	return transaction(edits, contract.Selection{Ranges: []contract.SelRange{sel}}, contract.OriginCommand, "block"), true, nil
}

func insertTOC(ctx contract.EditContext) (contract.Transaction, bool, error) {
	r := ctx.Selection.Primary().Range()
	_, b := leaf(ctx, r.Start)
	if b == nil || b.Parent != 0 || r.End > b.Range.End {
		return contract.Transaction{}, false, nil
	}
	if b.Kind == contract.BlockBlank {
		tx := replace(r, "[TOC]", contract.OriginCommand, "block")
		return tx, true, nil
	}
	at := b.Range.End
	insert := eol(ctx) + eol(ctx) + "[TOC]"
	tx := replace(contract.Range{Start: at, End: at}, insert, contract.OriginCommand, "block")
	return tx, true, nil
}

func insertFrontMatter(ctx contract.EditContext) (contract.Transaction, bool, error) {
	if ctx.AST == nil {
		return contract.Transaction{}, false, nil
	}
	if b, ok := ctx.AST.FrontMatter(); ok {
		at := b.Content.Start
		if len(b.Lines) > 0 {
			at = b.Lines[len(b.Lines)-1].Source.End
		}
		return transaction(nil, contract.Caret(at), contract.OriginCommand, "block"), true, nil
	}
	nl := eol(ctx)
	s := "---" + nl + nl + "---" + nl + nl
	tx := replace(contract.Range{}, s, contract.OriginCommand, "block")
	sel := contract.Caret(3 + len(nl))
	tx.Selection = &sel
	return tx, true, nil
}

func moveBlock(ctx contract.EditContext, down bool) (contract.Transaction, bool, error) {
	if ctx.AST == nil {
		return contract.Transaction{}, false, nil
	}
	r := ctx.Selection.Primary().Range()
	_, b := leaf(ctx, r.Start)
	if item := ancestor(ctx, r.Start, contract.BlockItem); item != nil {
		b = item
	} else {
		for b != nil && b.Parent > 0 {
			b = ctx.AST.Block(b.Parent)
		}
	}
	if b == nil {
		return contract.Transaction{}, false, nil
	}
	var blocks []*contract.Block
	at := -1
	for i := range ctx.AST.Children(b.Parent) {
		block := ctx.AST.Block(i)
		if block.Kind == contract.BlockBlank {
			continue
		}
		if block.ID == b.ID {
			at = len(blocks)
		}
		blocks = append(blocks, block)
	}
	next := at - 1
	if down {
		next = at + 1
	}
	if at < 0 || next < 0 || next >= len(blocks) || r.End > b.Range.End {
		return contract.Transaction{}, false, nil
	}
	other := blocks[next]
	unit := moveRange(ctx, b)
	a, z := unit, moveRange(ctx, other)
	if a.Start > z.Start {
		a, z = z, a
	}
	// Keep separator bytes at their original positions between the swapped blocks.
	edits := []contract.TextEdit{edit(a.Start, a.End, ctx.Text.Slice(z)), edit(z.Start, z.End, ctx.Text.Slice(a))}
	shift := a.Start - unit.Start
	if down {
		shift = z.Start + z.Len() - a.Len() - unit.Start
	}
	sel := ctx.Selection.Primary()
	sel.Anchor += shift
	sel.Head += shift
	return transaction(edits, contract.Selection{Ranges: []contract.SelRange{sel}}, contract.OriginCommand, "move"), true, nil
}

func moveRange(ctx contract.EditContext, b *contract.Block) contract.Range {
	r := b.Range
	if b.Kind == contract.BlockItem {
		r.Start = lineAt(ctx, r.Start).start
		if r.End > r.Start {
			r.End = lineAt(ctx, r.End-1).end
		}
	}
	return r
}

func runWithReplace(ctx contract.EditContext, cmd contract.EditCommand) (contract.Transaction, bool, error) {
	rep := cmd.Replace
	if rep.Start < 0 || rep.End < rep.Start || rep.End > ctx.Text.Len() {
		return contract.Transaction{}, false, &contract.Error{Code: contract.CodeInvalid, Op: "editing.Run", Detail: "invalid replace range"}
	}
	nextText, nextAST, err := markdown.Rebase(ctx.AST, ctx.Text, ctx.Prefs.Exts, []contract.TextEdit{{Range: rep}})
	if err != nil {
		return contract.Transaction{}, false, err
	}
	next := ctx
	next.Text = nextText
	next.AST = nextAST
	next.Selection = contract.Caret(rep.Start)
	cmd.Replace = contract.Range{}
	l := lineAt(next, rep.Start)
	if strings.TrimSpace(l.s[l.content:]) == "" && blockMatches(l, cmd.ID) {
		mark := ""
		if cmd.ID == contract.CmdBlockQuote {
			mark = "> "
		}
		edits := []contract.TextEdit{{Range: rep, Text: mark}}
		return transaction(edits, contract.Caret(rep.Start+len(mark)), contract.OriginCommand, "block"), true, nil
	}
	tx, ok, err := Editor{}.Run(next, cmd)
	if err != nil || !ok {
		return tx, ok, err
	}
	tx.Edits = unshiftEdits(tx.Edits, rep.Start, rep.Len())
	return tx, true, nil
}

// unshiftEdits maps edits computed on text without [from, from+d) back onto
// the original text containing [from, from+d).
func unshiftEdits(edits []contract.TextEdit, from, d int) []contract.TextEdit {
	out := make([]contract.TextEdit, 0, len(edits)+1)
	taken := false
	for _, ed := range edits {
		r := ed.Range
		switch {
		case r.End < from:
		case r.Start <= from && !taken:
			r.End += d
			taken = true
		default:
			r.Start, r.End = r.Start+d, r.End+d
		}
		if !taken && r.Start > from {
			out = append(out, contract.TextEdit{Range: contract.Range{Start: from, End: from + d}})
			taken = true
		}
		out = append(out, contract.TextEdit{Range: r, Text: ed.Text})
	}
	if !taken {
		out = append(out, contract.TextEdit{Range: contract.Range{Start: from, End: from + d}})
	}
	return out
}
