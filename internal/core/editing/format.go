package editing

import (
	"sort"
	"strings"
	"unicode"

	"aurink/internal/contract"
)

func inlineKind(id contract.CommandID) contract.InlineKind {
	switch id {
	case contract.CmdFormatStrong:
		return contract.InlineStrong
	case contract.CmdFormatEmphasis:
		return contract.InlineEmphasis
	case contract.CmdFormatStrike:
		return contract.InlineStrike
	case contract.CmdFormatCode:
		return contract.InlineCode
	case contract.CmdFormatLink:
		return contract.InlineLink
	case contract.CmdFormatHighlight:
		return contract.InlineHighlight
	case contract.CmdFormatSup:
		return contract.InlineSup
	case contract.CmdFormatSub:
		return contract.InlineSub
	}
	return contract.InlineText
}

func selectedLeaves(ctx contract.EditContext) []int {
	if ctx.AST == nil {
		return nil
	}
	r := ctx.Selection.Primary().Range()
	var result []int
	first := ctx.AST.LeafAt(r.Start)
	for i := first; i < ctx.AST.Len(); i++ {
		b := ctx.AST.Block(i)
		if i != first && b.Lead.Start >= r.End {
			break
		}
		if b.Next == i+1 {
			result = append(result, i)
		}
	}
	return result
}

func contentRange(n contract.Inline) contract.Range {
	r := n.Range
	if !n.Open.Empty() {
		r.Start = n.Open.End
	}
	if !n.Close.Empty() {
		r.End = n.Close.Start
	}
	return r
}

// Formatting describes the visible contents of the primary selection.
func (Editor) Formatting(ctx contract.EditContext) contract.FormatState {
	var f contract.FormatState
	if !valid(ctx) {
		return f
	}
	r := ctx.Selection.Primary().Range()
	_, b := leaf(ctx, r.Start)
	if b != nil {
		f.Block, f.Level = b.Kind, b.Level
	}
	f.InCode = ancestor(ctx, r.Start, contract.BlockCode) != nil
	f.InTable = ancestor(ctx, r.Start, contract.BlockTable) != nil
	if list := ancestor(ctx, r.Start, contract.BlockList); list != nil {
		f.List = contract.ListBullet
		if list.Ordered {
			f.List = contract.ListOrdered
		}
		if item := ancestor(ctx, r.Start, contract.BlockItem); item != nil && item.Task != contract.TaskNone {
			f.List = contract.ListTask
		}
	}
	kinds := []contract.InlineKind{contract.InlineStrong, contract.InlineEmphasis, contract.InlineStrike, contract.InlineCode, contract.InlineHighlight, contract.InlineLink, contract.InlineSup, contract.InlineSub}
	var yes, no [8]bool
	destSet := false
	for _, i := range selectedLeaves(ctx) {
		nodes := ctx.AST.Inlines(i)
		for j, n := range nodes {
			if n.Next > j+1 {
				continue
			}
			part := contentRange(n)
			if r.Empty() {
				if r.Start < part.Start || r.Start > part.End {
					continue
				}
			} else if !part.Overlaps(r) {
				continue
			}
			for k, kind := range kinds {
				found := false
				for _, parent := range nodes {
					pr := contentRange(parent)
					if parent.Kind == kind && pr.Start <= part.Start && pr.End >= part.End {
						found = true
						if kind == contract.InlineLink {
							if !destSet {
								f.LinkDest, destSet = parent.Dest, true
							} else if f.LinkDest != parent.Dest {
								f.LinkDest = ""
							}
						}
					}
				}
				if found {
					yes[k] = true
				} else {
					no[k] = true
				}
			}
		}
	}
	states := []*contract.TriState{&f.Strong, &f.Emphasis, &f.Strike, &f.Code, &f.Highlight, &f.Link, &f.Sup, &f.Sub}
	for k, state := range states {
		if yes[k] {
			*state = contract.On
			if no[k] {
				*state = contract.Mixed
			}
		}
	}
	return f
}

func orderedEdits(edits []contract.TextEdit) []contract.TextEdit {
	sort.SliceStable(edits, func(i, j int) bool { return edits[i].Range.Start < edits[j].Range.Start })
	var result []contract.TextEdit
	for _, e := range edits {
		if len(result) > 0 {
			last := &result[len(result)-1]
			if e.Range.Start < last.Range.End {
				continue // Nested delimiters already covered by a removed outer marker.
			}
			if e.Range.Start == last.Range.Start && last.Range.Empty() {
				last.Range.End = e.Range.End
				last.Text += e.Text
				continue
			}
		}
		result = append(result, e)
	}
	return result
}

func delimiters(ctx contract.EditContext, id contract.CommandID, s string) (string, string) {
	switch id {
	case contract.CmdFormatStrong:
		if ctx.Prefs.StrongDelim == "__" {
			return "__", "__"
		}
		return "**", "**"
	case contract.CmdFormatEmphasis:
		if ctx.Prefs.EmphasisDelim == "_" {
			return "_", "_"
		}
		return "*", "*"
	case contract.CmdFormatStrike:
		return "~~", "~~"
	case contract.CmdFormatHighlight:
		return "==", "=="
	case contract.CmdFormatSup:
		return "^", "^"
	case contract.CmdFormatSub:
		return "~", "~"
	case contract.CmdFormatUnderline:
		return "<u>", "</u>"
	case contract.CmdFormatCode:
		longest, run := 0, 0
		for _, c := range s {
			if c == '`' {
				run++
				longest = max(longest, run)
			} else {
				run = 0
			}
		}
		d := strings.Repeat("`", longest+1)
		if strings.HasPrefix(s, "`") || strings.HasSuffix(s, "`") || strings.HasPrefix(s, " ") && strings.HasSuffix(s, " ") && strings.TrimSpace(s) != "" {
			return d + " ", " " + d
		}
		return d, d
	}
	return "", ""
}

func inlineCommand(ctx contract.EditContext, cmd contract.EditCommand) (contract.Transaction, bool, error) {
	if selectionHas(ctx, contract.BlockCode, contract.BlockHTML, contract.BlockFrontMatter, contract.BlockMath) {
		return contract.Transaction{}, false, nil
	}
	if cmd.ID != contract.CmdFormatCode && cmd.ID != contract.CmdFormatClear && inlineLiteral(ctx) {
		return contract.Transaction{}, false, nil
	}
	r := ctx.Selection.Primary().Range()
	if (cmd.ID == contract.CmdFormatSup || cmd.ID == contract.CmdFormatSub) && strings.IndexFunc(ctx.Text.Slice(r), unicode.IsSpace) >= 0 {
		return contract.Transaction{}, false, nil
	}
	if cmd.ID == contract.CmdFormatClear {
		return clearFormat(ctx), true, nil
	}
	isLink := cmd.ID == contract.CmdFormatLink || cmd.ID == contract.CmdInsertLink
	open, _ := delimiters(ctx, cmd.ID, "")
	if open == "" && !isLink {
		return contract.Transaction{}, false, nil
	}
	if r.Empty() {
		return inlinePart(ctx, cmd)
	}
	var parts []contract.Range
	for _, i := range selectedLeaves(ctx) {
		b := ctx.AST.Block(i)
		if b.Kind != contract.BlockParagraph && b.Kind != contract.BlockHeading && b.Kind != contract.BlockTableCell {
			continue
		}
		part := contract.Range{Start: max(r.Start, b.Content.Start), End: min(r.End, b.Content.End)}
		if part.End <= part.Start {
			continue
		}
		s := ctx.Text.Slice(part)
		part.Start += len(s) - len(strings.TrimLeftFunc(s, unicode.IsSpace))
		part.End -= len(s) - len(strings.TrimRightFunc(s, unicode.IsSpace))
		if part.End > part.Start {
			parts = append(parts, part)
		}
	}
	if len(parts) == 0 {
		return transaction(nil, ctx.Selection, contract.OriginCommand, "format"), true, nil
	}
	allOn := !isLink
	for _, part := range parts {
		pc := ctx
		pc.Selection = selected(ctx, part.Start, part.End)
		if !formatOn(pc, cmd.ID) {
			allOn = false
		}
	}
	var edits []contract.TextEdit
	first, last := parts[0].Start, parts[len(parts)-1].End
	startDelta, endDelta := 0, 0
	for _, part := range parts {
		pc := ctx
		pc.Selection = selected(ctx, part.Start, part.End)
		if allOn || isLink {
			tx, ok, err := inlinePart(pc, cmd)
			if !ok || err != nil {
				return contract.Transaction{}, ok, err
			}
			edits = append(edits, tx.Edits...)
			if tx.Selection != nil {
				base := mapped(pc.Selection, tx.Edits).Primary().Range()
				actual := tx.Selection.Primary().Range()
				if part.Start == first {
					startDelta = actual.Start - base.Start
				}
				if part.End == last {
					endDelta = actual.End - base.End
				}
			}
			continue
		}
		if formatOn(pc, cmd.ID) {
			for _, i := range selectedLeaves(pc) {
				for _, n := range formatNodes(pc, i, cmd.ID) {
					if n.Kind != inlineKind(cmd.ID) {
						continue
					}
					c := contentRange(n)
					if n.Range.Start == part.Start && part.Start == first {
						first = c.Start
					}
					if n.Range.End == part.End && part.End == last {
						last = c.End
					}
				}
			}
			endDelta = 0
			continue
		}
		kind := inlineKind(cmd.ID)
		var remove []contract.TextEdit
		for _, i := range selectedLeaves(pc) {
			for _, n := range formatNodes(pc, i, cmd.ID) {
				if n.Kind == kind && part.Start <= n.Range.Start && part.End >= n.Range.End {
					remove = append(remove, edit(n.Open.Start, n.Open.End, ""), edit(n.Close.Start, n.Close.End, ""))
				}
			}
		}
		// Choose code delimiters after removing any old code markers.
		remove = orderedEdits(remove)
		s := ctx.Text.Slice(part)
		for j := len(remove) - 1; j >= 0; j-- {
			e := remove[j]
			s = s[:e.Range.Start-part.Start] + s[e.Range.End-part.Start:]
		}
		op, cl := delimiters(ctx, cmd.ID, s)
		edits = append(edits, edit(part.Start, part.Start, op))
		edits = append(edits, remove...)
		edits = append(edits, edit(part.End, part.End, cl))
		endDelta = -len(cl)
	}
	edits = orderedEdits(edits)
	selection := mapped(ctx.Selection, edits)
	if !allOn {
		selection = mapped(selected(ctx, first, last), edits)
		sr := selection.Primary().Range()
		selection = selected(ctx, sr.Start+startDelta, sr.End+endDelta)
	}
	return transaction(edits, selection, contract.OriginCommand, "format"), true, nil
}

func formatOn(ctx contract.EditContext, id contract.CommandID) bool {
	f := New().Formatting(ctx)
	switch id {
	case contract.CmdFormatStrong:
		return f.Strong == contract.On
	case contract.CmdFormatEmphasis:
		return f.Emphasis == contract.On
	case contract.CmdFormatStrike:
		return f.Strike == contract.On
	case contract.CmdFormatCode:
		return f.Code == contract.On
	case contract.CmdFormatHighlight:
		return f.Highlight == contract.On
	case contract.CmdFormatSup:
		return f.Sup == contract.On
	case contract.CmdFormatSub:
		return f.Sub == contract.On
	case contract.CmdFormatUnderline:
		r := ctx.Selection.Primary().Range()
		for _, i := range selectedLeaves(ctx) {
			for _, n := range formatNodes(ctx, i, id) {
				if n.Range.Start <= r.Start && n.Range.End >= r.End || contentRange(n).Start <= r.Start && contentRange(n).End >= r.End {
					return true
				}
			}
		}
	}
	return false
}

func formatNodes(ctx contract.EditContext, i int, id contract.CommandID) []contract.Inline {
	nodes := ctx.AST.Inlines(i)
	if id != contract.CmdFormatUnderline {
		return nodes
	}
	var result []contract.Inline
	var stack []contract.Range
	for _, n := range nodes {
		if n.Kind != contract.InlineHTML {
			continue
		}
		if n.Text == "<u>" {
			stack = append(stack, n.Range)
		}
		if n.Text == "</u>" && len(stack) > 0 {
			op := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			result = append(result, contract.Inline{Kind: contract.InlineText, Range: contract.Range{Start: op.Start, End: n.Range.End}, Open: op, Close: n.Range})
		}
	}
	return result
}

func inlinePart(ctx contract.EditContext, cmd contract.EditCommand) (contract.Transaction, bool, error) {
	r := ctx.Selection.Primary().Range()
	if cmd.ID == contract.CmdFormatLink || cmd.ID == contract.CmdInsertLink {
		return linkCommand(ctx, cmd)
	}
	open, close := delimiters(ctx, cmd.ID, ctx.Text.Slice(r))
	kind := inlineKind(cmd.ID)
	var matching []contract.Inline
	for _, i := range selectedLeaves(ctx) {
		for _, n := range formatNodes(ctx, i, cmd.ID) {
			c := contentRange(n)
			if n.Kind == kind && (c.Start <= r.Start && c.End >= r.End || n.Range == r) {
				matching = append(matching, n)
			}
		}
	}
	var edits []contract.TextEdit
	if len(matching) > 0 {
		n := matching[len(matching)-1]
		edits = []contract.TextEdit{edit(n.Open.Start, n.Open.End, ""), edit(n.Close.Start, n.Close.End, "")}
		return transaction(edits, mapped(ctx.Selection, edits), contract.OriginCommand, "format"), true, nil
	}
	f := New().Formatting(ctx)
	state := contract.Off
	switch cmd.ID {
	case contract.CmdFormatStrong:
		state = f.Strong
	case contract.CmdFormatEmphasis:
		state = f.Emphasis
	case contract.CmdFormatStrike:
		state = f.Strike
	case contract.CmdFormatCode:
		state = f.Code
	case contract.CmdFormatHighlight:
		state = f.Highlight
	case contract.CmdFormatSup:
		state = f.Sup
	case contract.CmdFormatSub:
		state = f.Sub
	case contract.CmdFormatUnderline:
		if formatOn(ctx, cmd.ID) {
			state = contract.On
		}
	}
	s := ctx.Text.String()
	if cmd.ID == contract.CmdFormatUnderline && r.Start >= 3 && strings.HasSuffix(s[:r.Start], "<u>") && strings.HasPrefix(s[r.End:], "</u>") {
		edits = []contract.TextEdit{edit(r.Start-3, r.Start, ""), edit(r.End, r.End+4, "")}
		return transaction(edits, mapped(ctx.Selection, edits), contract.OriginCommand, "format"), true, nil
	}
	// Remove existing instances within a mixed selection before applying one wrap.
	for _, i := range selectedLeaves(ctx) {
		for _, n := range formatNodes(ctx, i, cmd.ID) {
			if n.Kind == kind && r.Start <= n.Range.Start && r.End >= n.Range.End {
				edits = append(edits, edit(n.Open.Start, n.Open.End, ""), edit(n.Close.Start, n.Close.End, ""))
			}
		}
	}
	if state == contract.On && len(edits) > 0 {
		edits = orderedEdits(edits)
		return transaction(edits, mapped(ctx.Selection, edits), contract.OriginCommand, "format"), true, nil
	}
	if r.Empty() {
		tx := replace(r, open+close, contract.OriginCommand, "format")
		sel := contract.Caret(r.Start + len(open))
		tx.Selection = &sel
		return tx, true, nil
	}
	edits = append([]contract.TextEdit{edit(r.Start, r.Start, open)}, edits...)
	edits = append(edits, edit(r.End, r.End, close))
	edits = orderedEdits(edits)
	end := r.End + len(open)
	for _, e := range edits {
		if e.Range.Len() > 0 {
			end -= e.Range.Len()
		}
	}
	return transaction(edits, selected(ctx, r.Start+len(open), end), contract.OriginCommand, "format"), true, nil
}

func clearFormat(ctx contract.EditContext) contract.Transaction {
	r := ctx.Selection.Primary().Range()
	var edits []contract.TextEdit
	for _, i := range selectedLeaves(ctx) {
		for _, n := range ctx.AST.Inlines(i) {
			c := contentRange(n)
			if !(c.Overlaps(r) || r.Empty() && c.Start <= r.Start && c.End >= r.End) {
				continue
			}
			switch n.Kind {
			case contract.InlineStrong, contract.InlineEmphasis, contract.InlineStrike, contract.InlineCode, contract.InlineLink, contract.InlineHighlight, contract.InlineSup, contract.InlineSub:
				if !n.Open.Empty() {
					edits = append(edits, edit(n.Open.Start, n.Open.End, ""))
				}
				if !n.Close.Empty() {
					edits = append(edits, edit(n.Close.Start, n.Close.End, ""))
				}
			case contract.InlineHTML:
				if n.Text == "<u>" || n.Text == "</u>" {
					edits = append(edits, edit(n.Range.Start, n.Range.End, ""))
				}
			}
		}
	}
	// Underline tags surrounding a content-only selection are not AST parents.
	s := ctx.Text.String()
	if r.Start >= 3 && strings.HasSuffix(s[:r.Start], "<u>") && strings.HasPrefix(s[r.End:], "</u>") {
		edits = append(edits, edit(r.Start-3, r.Start, ""), edit(r.End, r.End+4, ""))
	}
	edits = orderedEdits(edits)
	return transaction(edits, mapped(ctx.Selection, edits), contract.OriginCommand, "format")
}

func linkCommand(ctx contract.EditContext, cmd contract.EditCommand) (contract.Transaction, bool, error) {
	if strings.ContainsAny(cmd.URL+cmd.Text, "\r\n") {
		return contract.Transaction{}, false, &contract.Error{Code: contract.CodeInvalid, Op: "editing.Run", Detail: "link fields must be single line"}
	}
	r := ctx.Selection.Primary().Range()
	for _, i := range selectedLeaves(ctx) {
		for _, n := range ctx.AST.Inlines(i) {
			c := contentRange(n)
			if n.Kind == contract.InlineLink && !n.Auto && (c.Start <= r.Start && c.End >= r.End || n.Range == r) {
				if cmd.URL == "" {
					edits := []contract.TextEdit{edit(n.Open.Start, n.Open.End, ""), edit(n.Close.Start, n.Close.End, "")}
					return transaction(edits, mapped(ctx.Selection, edits), contract.OriginCommand, "format"), true, nil
				}
				close := "](" + linkDestination(cmd.URL) + ")"
				dest := inlineDestination(ctx.Text.Slice(n.Close))
				e := edit(n.Close.Start, n.Close.End, close)
				if dest.Start >= 0 {
					e = edit(n.Close.Start+dest.Start, n.Close.Start+dest.End, linkDestination(cmd.URL))
				}
				edits := []contract.TextEdit{e}
				if ctx.Text.Slice(e.Range) == e.Text {
					edits = nil
				}
				return transaction(edits, ctx.Selection, contract.OriginCommand, "format"), true, nil
			}
		}
	}
	close := "](" + linkDestination(cmd.URL) + ")"
	if r.Empty() && cmd.Text != "" {
		s := strings.NewReplacer("\\", "\\\\", "[", "\\[", "]", "\\]").Replace(cmd.Text)
		tx := replace(r, "["+s+close, contract.OriginCommand, "format")
		sel := selected(ctx, r.Start+1, r.Start+1+len(s))
		tx.Selection = &sel
		return tx, true, nil
	}
	if r.Empty() {
		tx := replace(r, "["+close, contract.OriginCommand, "format")
		sel := contract.Caret(r.Start + 1)
		tx.Selection = &sel
		return tx, true, nil
	}
	edits := []contract.TextEdit{edit(r.Start, r.Start, "["), edit(r.End, r.End, close)}
	return transaction(edits, selected(ctx, r.Start+1, r.End+1), contract.OriginCommand, "format"), true, nil
}

func inlineDestination(s string) contract.Range {
	if !strings.HasPrefix(s, "](") {
		return contract.Range{Start: -1}
	}
	a := 2
	for a < len(s) && space(s[a]) {
		a++
	}
	if a < len(s) && s[a] == '<' {
		b := a + 1
		for b < len(s) && (s[b] != '>' || escaped(s, b)) {
			b++
		}
		return contract.Range{Start: a + 1, End: b}
	}
	b, depth := a, 0
	for b < len(s) {
		if s[b] == '\\' && b+1 < len(s) {
			b += 2
			continue
		}
		if space(s[b]) || s[b] == ')' && depth == 0 {
			break
		}
		if s[b] == '(' {
			depth++
		} else if s[b] == ')' {
			depth--
		}
		b++
	}
	return contract.Range{Start: a, End: b}
}

func linkDestination(s string) string {
	return strings.NewReplacer("\\", "\\\\", "(", "\\(", ")", "\\)", " ", "%20", "<", "%3C", ">", "%3E").Replace(s)
}
