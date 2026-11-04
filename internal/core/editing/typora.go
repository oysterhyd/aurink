package editing

import (
	"strings"

	"aurink/internal/contract"
	"aurink/internal/core/markdown"
	"aurink/internal/core/text"
)

// emptyParagraph excludes separator lines owned by a neighbouring trailer.
func emptyParagraph(ctx contract.EditContext, at int) (int, *contract.Block) {
	i, b := leaf(ctx, at)
	if b == nil || b.Kind != contract.BlockBlank || b.Parent != 0 || at < b.Range.Start || at > b.Range.End {
		return -1, nil
	}
	l := lineAt(ctx, at)
	if strings.TrimSpace(l.s) != "" || b.Range.Start < l.start || b.Range.Start > l.end {
		return -1, nil
	}
	return i, b
}

func deleteEmptyParagraph(ctx contract.EditContext, at int, back bool) (contract.Transaction, bool) {
	i, b := emptyParagraph(ctx, at)
	if b == nil {
		return contract.Transaction{}, false
	}
	previous, next := -1, -1
	for j := range ctx.AST.Leaves() {
		if j == i {
			next = i
			continue
		}
		if next == i {
			next = j
			break
		}
		previous = j
	}
	if back && previous >= 0 {
		start := ctx.AST.Block(previous).Range.End
		return replace(contract.Range{Start: start, End: lineAt(ctx, at).end}, "", contract.OriginUser, "delete"), true
	}
	if !back && next >= 0 && next != i {
		start, end := lineAt(ctx, at).start, ctx.AST.Block(next).Lead.Start
		return replace(contract.Range{Start: start, End: end}, "", contract.OriginUser, "delete"), true
	}
	return transaction(nil, ctx.Selection, contract.OriginUser, "delete"), true
}

func tableFromLine(ctx contract.EditContext, l sourceLine) (contract.Transaction, bool) {
	if len(l.parts) == 0 && l.indent != 0 {
		return contract.Transaction{}, false
	}
	for _, p := range l.parts {
		if p.kind == '#' {
			return contract.Transaction{}, false
		}
	}
	line := l.s[l.content:]
	if len(line) < 3 || line[0] != '|' || line[len(line)-1] != '|' {
		return contract.Transaction{}, false
	}
	var cells []string
	start, slashes := 1, 0
	for i := 1; i < len(line); i++ {
		if line[i] == '|' && slashes%2 == 0 {
			cells = append(cells, strings.TrimSpace(line[start:i]))
			start = i + 1
		}
		if line[i] == '\\' {
			slashes++
		} else {
			slashes = 0
		}
	}
	if start != len(line) || len(cells) < 2 {
		return contract.Transaction{}, false
	}
	var header strings.Builder
	header.WriteString("|")
	for _, cell := range cells {
		header.WriteString(" " + cell + " |")
	}
	nl, lead := eol(ctx), l.containerContinuation()
	delimiter := "|" + strings.Repeat(" --- |", len(cells))
	body := "|" + strings.Repeat("  |", len(cells))
	replacement := header.String() + nl + lead + delimiter + nl + lead + body
	startAt := l.start + l.content
	tx := replace(contract.Range{Start: startAt, End: l.end}, replacement, contract.OriginUser, "newline")
	sel := contract.Caret(startAt + len(header.String()) + len(nl+lead+delimiter+nl+lead) + 2)
	tx.Selection = &sel
	return tx, true
}

func fenceOpener(s string) string {
	s = strings.TrimLeft(s, " \t")
	if len(s) == 0 || s[0] != '`' && s[0] != '~' {
		return ""
	}
	i := 0
	for i < len(s) && s[i] == s[0] {
		i++
	}
	if i < 3 || s[0] == '`' && strings.ContainsRune(s[i:], '`') {
		return ""
	}
	return s[:i]
}

func fenceFollowUp(ctx contract.EditContext) (contract.Transaction, bool) {
	p := ctx.Selection.Primary().Head
	l := lineAt(ctx, p)
	if p != l.end {
		return contract.Transaction{}, false
	}
	fence := fenceOpener(l.s[l.content:])
	if fence == "" || l.s[l.content:] != fence {
		return contract.Transaction{}, false
	}
	code := ancestor(ctx, p-1, contract.BlockCode)
	if code == nil || !code.Fenced || len(code.Markers) != 1 || lineAt(ctx, code.Markers[0].Start).start != l.start {
		return contract.Transaction{}, false
	}
	for _, line := range code.Lines {
		if line.Source.Start >= l.next && strings.TrimSpace(ctx.Text.Slice(line.Source)) != "" {
			return transaction([]contract.TextEdit{edit(p, p, eol(ctx)+l.containerContinuation()+fence)}, ctx.Selection, contract.OriginInputRule, "input_rule"), true
		}
	}
	return contract.Transaction{}, false
}

// Apply the closer with the keystroke. FollowUp also handles contexts supplied
// directly by clients, but an ordinary typing path needs only one undo step.
func closeFenceWithTyping(ctx contract.EditContext, in contract.Intent, tx contract.Transaction) contract.Transaction {
	r := ctx.Selection.Primary().Range()
	if !r.Empty() || len(in.Text) == 0 || strings.Trim(in.Text, "`") != "" && strings.Trim(in.Text, "~") != "" ||
		len(tx.Edits) != 1 || tx.Selection == nil || tx.Edits[0].Range != r || tx.Selection.Primary().Head != r.Start+len(tx.Edits[0].Text) {
		return tx
	}
	l := lineAt(ctx, r.Start)
	if r.Start != l.end {
		return tx
	}
	line := scanLine(l.s + tx.Edits[0].Text)
	fence := fenceOpener(line.s[line.content:])
	if fence == "" || line.s[line.content:] != fence {
		return tx
	}
	oldFence := fenceOpener(l.s[l.content:])
	if oldFence != "" && l.s[l.content:] == oldFence && strings.HasPrefix(fence, oldFence) {
		code := ancestor(ctx, r.Start-1, contract.BlockCode)
		if code != nil && code.Fenced && len(code.Markers) == 2 {
			closer := lineAt(ctx, code.Markers[1].Start)
			if closer.start == l.next && strings.TrimSpace(closer.s[closer.content:]) == oldFence {
				at := closer.start + closer.content + len(oldFence)
				tx.Edits = append(tx.Edits, edit(at, at, tx.Edits[0].Text))
				tx.Coalesce = ""
				return tx
			}
		}
	}
	src := ctx.Text.String()
	newSource := src[:r.Start] + tx.Edits[0].Text + src[r.End:]
	p, err := markdown.New(newSource, ctx.Text.Version()+1, ctx.Prefs.Exts)
	if err != nil {
		return tx
	}
	next := ctx
	next.Text = text.New(newSource, ctx.Text.Version()+1)
	next.AST, next.Selection = p.Snapshot(), *tx.Selection
	if follow, ok := fenceFollowUp(next); ok {
		tx.Edits[0].Text += follow.Edits[0].Text
		tx.Coalesce = ""
	}
	return tx
}

func deleteEmptyFenceCloser(ctx contract.EditContext, at int) (contract.Transaction, bool) {
	if at == 0 {
		return contract.Transaction{}, false
	}
	code := ancestor(ctx, at-1, contract.BlockCode)
	if code == nil || !code.Fenced || code.Open || len(code.Markers) < 2 {
		return contract.Transaction{}, false
	}
	opener, closer := lineAt(ctx, code.Markers[0].Start), lineAt(ctx, code.Markers[len(code.Markers)-1].Start)
	if lineAt(ctx, at).start != opener.start || at <= opener.start+opener.content || at > opener.end {
		return contract.Transaction{}, false
	}
	src := ctx.Text.String()
	before := previousGrapheme(src, at)
	remaining := src[opener.start+opener.content:before] + src[at:opener.end]
	if fenceOpener(remaining) != "" {
		return contract.Transaction{}, false
	}
	for _, line := range code.Lines {
		if strings.TrimSpace(ctx.Text.Slice(line.Source)) != "" {
			return contract.Transaction{}, false
		}
	}
	if closer.start <= opener.end {
		return contract.Transaction{}, false
	}
	edits := []contract.TextEdit{edit(before, at, ""), edit(previousGrapheme(src, closer.start), closer.end, "")}
	return transaction(edits, contract.Caret(before), contract.OriginUser, "delete"), true
}
