package editing

import (
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"aurink/internal/contract"
	"aurink/internal/core/projection"
)

type prefix struct {
	kind byte
	a, b int
}

type sourceLine struct {
	start, end, next int
	s                string
	content, indent  int
	parts            []prefix
	list             int
	number           int
	delim            byte
	task             int
}

func space(c byte) bool { return c == ' ' || c == '\t' }

func lineAt(ctx contract.EditContext, at int) sourceLine {
	t := ctx.Text
	n := t.LineOf(at)
	a, b := t.LineStart(n), t.LineStart(n+1)
	end := b
	s := t.Slice(contract.Range{Start: a, End: b})
	if strings.HasSuffix(s, "\n") {
		end--
		s = s[:len(s)-1]
		if strings.HasSuffix(s, "\r") {
			end--
			s = s[:len(s)-1]
		}
	}
	l := scanLine(s)
	l.start, l.end, l.next = a, end, b
	return l
}

func scanLine(s string) sourceLine {
	l := sourceLine{s: s, list: -1, task: -1}
	i := 0
	for i < len(s) && space(s[i]) {
		i++
	}
	l.indent = i
	for i < len(s) {
		a := i
		kind := byte(0)
		switch s[i] {
		case '>':
			i++
			if i < len(s) && space(s[i]) {
				i++
			}
			kind = '>'
		case '#':
			for i < len(s) && s[i] == '#' {
				i++
			}
			if i-a > 6 || (i < len(s) && !space(s[i])) {
				return l.withContent(a)
			}
			kind = '#'
		default:
			j := i
			for j < len(s) && s[j] >= '0' && s[j] <= '9' {
				j++
			}
			if j > i && j-i <= 9 && j < len(s) && (s[j] == '.' || s[j] == ')') {
				l.number, _ = strconv.Atoi(s[i:j])
				l.delim = s[j]
				i = j + 1
			} else if s[i] == '-' || s[i] == '*' || s[i] == '+' {
				l.number, l.delim = 0, s[i]
				i++
			} else {
				return l.withContent(a)
			}
			if i < len(s) && !space(s[i]) {
				return l.withContent(a)
			}
			kind = '-'
			l.list = len(l.parts)
		}
		for i < len(s) && space(s[i]) {
			i++
		}
		if kind == '-' && i+3 < len(s) && s[i] == '[' && (s[i+1] == ' ' || s[i+1] == 'x' || s[i+1] == 'X') && s[i+2] == ']' && space(s[i+3]) {
			l.task = i
			i += 3
			for i < len(s) && space(s[i]) {
				i++
			}
		}
		l.parts = append(l.parts, prefix{kind: kind, a: a, b: i})
		if kind == '#' {
			break
		}
	}
	return l.withContent(i)
}

func (l sourceLine) withContent(i int) sourceLine { l.content = i; return l }

func (l sourceLine) continuation() string {
	s := l.s[:l.content]
	if l.list >= 0 && l.delim != '-' && l.delim != '*' && l.delim != '+' {
		p := l.parts[l.list]
		j := p.a
		for j < len(s) && s[j] >= '0' && s[j] <= '9' {
			j++
		}
		s = s[:p.a] + strconv.Itoa(l.number+1) + s[j:]
	}
	if l.task >= 0 {
		// An incremented ordered marker can have a different width.
		j := strings.LastIndex(s, "[")
		s = s[:j+1] + " " + s[j+2:]
	}
	return s
}

func (l sourceLine) containerContinuation() string {
	s := []byte(l.s[:l.content])
	for _, p := range l.parts {
		if p.kind == '#' {
			s = s[:p.a]
			break
		}
		if p.kind == '-' {
			for i := p.a; i < p.b; i++ {
				if s[i] != '\t' {
					s[i] = ' '
				}
			}
		}
	}
	return string(s)
}

// pendingLine uses the same classification as the visible projection, so a
// marker inside a code block or a setext underline keeps its existing behavior.
func pendingLine(ctx contract.EditContext, l sourceLine) (contract.BlockProjection, contract.Range, bool) {
	if ctx.AST == nil || l.end == l.start || l.content != len(l.s) || len(l.parts) == 0 ||
		!strings.ContainsRune("#-+*.)>", rune(l.s[len(l.s)-1])) {
		return contract.BlockProjection{}, contract.Range{}, false
	}
	i, b := leaf(ctx, l.end-1)
	if b == nil || b.Kind != contract.BlockHeading && b.Kind != contract.BlockBlank {
		return contract.BlockProjection{}, contract.Range{}, false
	}
	p := projection.New().Project(ctx.AST, ctx.Text, i, contract.Reveal{Mode: contract.RevealNone})
	if p.Kind == contract.BlockParagraph {
		for _, s := range p.Segments {
			if s.Kind == contract.SegText && s.Src.Start >= l.start && s.Src.End == l.end {
				return p, s.Src, true
			}
		}
	}
	return contract.BlockProjection{}, contract.Range{}, false
}

func pendingContinuation(ctx contract.EditContext, l sourceLine, p contract.BlockProjection, marker contract.Range) string {
	outer := scanLine(l.s[:marker.Start-l.start])
	if outer.list >= 0 {
		return outer.continuation()
	}
	if len(outer.parts) > 0 && outer.parts[len(outer.parts)-1].kind == '>' {
		return outer.s[:outer.content]
	}
	for i := len(p.Containers) - 1; i >= 0; i-- {
		if p.Containers[i].Kind == contract.BlockItem {
			if index, ok := ctx.AST.Index(p.Containers[i].Block); ok {
				return lineAt(ctx, ctx.AST.Block(index).Range.Start).continuation()
			}
		}
	}
	return eol(ctx)
}

func newline(ctx contract.EditContext) contract.Transaction {
	r := ctx.Selection.Primary().Range()
	l := lineAt(ctx, r.Start)
	nl := eol(ctx)
	if r.Empty() && r.Start == l.end && !rawAt(ctx, r.Start) {
		if strings.TrimSpace(l.s[l.content:]) == "$$" {
			lead := l.containerContinuation()
			tx := replace(r, nl+lead+nl+lead+"$$", contract.OriginUser, "newline")
			sel := contract.Caret(r.Start + len(nl) + len(lead))
			tx.Selection = &sel
			return tx
		}
		if tx, ok := tableFromLine(ctx, l); ok {
			return tx
		}
	}
	if _, b := leaf(ctx, r.Start); b != nil && (b.Kind == contract.BlockHTML || b.Kind == contract.BlockFrontMatter || b.Kind == contract.BlockMath) {
		return replace(r, nl+rawContinuation(ctx, l, b), contract.OriginUser, "newline")
	}
	if code := ancestor(ctx, r.Start, contract.BlockCode); code != nil {
		trim := strings.TrimSpace(l.s[l.content:])
		if code.Fenced && len(code.Markers) > 0 {
			opener := lineAt(ctx, code.Markers[0].Start)
			fence := strings.TrimLeft(opener.s[opener.content:], " \t")
			j := 0
			for j < len(fence) && (fence[j] == '`' || fence[j] == '~') && fence[j] == fence[0] {
				j++
			}
			if l.start == opener.start && r.Start == l.end && code.Open && j >= 3 {
				lead := l.containerContinuation()
				tx := replace(r, nl+lead+nl+lead+fence[:j], contract.OriginUser, "newline")
				sel := contract.Caret(r.Start + len(nl) + len(lead))
				tx.Selection = &sel
				return tx
			}
			if l.start != opener.start && len(trim) >= j && j >= 3 && strings.Trim(trim, string(fence[0])) == "" {
				return replace(r, nl+nl, contract.OriginUser, "newline")
			}
		}
		lead := rawContinuation(ctx, l, code)
		return replace(r, nl+lead, contract.OriginUser, "newline")
	}
	if p, marker, ok := pendingLine(ctx, l); ok && r.Start >= marker.Start && r.End <= marker.End {
		return replace(r, nl+pendingContinuation(ctx, l, p, marker), contract.OriginUser, "newline")
	}
	if r.Start >= l.start+l.content && strings.TrimSpace(l.s[l.content:]) == "" && len(l.parts) > 0 {
		p := l.parts[len(l.parts)-1]
		if p.kind == '-' && l.indent > 0 {
			n := min(l.indent, tabs(ctx))
			edits := []contract.TextEdit{edit(l.start, l.start+n, "")}
			return transaction(edits, mapped(ctx.Selection, edits), contract.OriginUser, "newline")
		}
		if p.kind == '-' && r.Empty() {
			i, b := leaf(ctx, r.Start)
			if b != nil && b.Kind == contract.BlockBlank && b.Parent > 0 {
				item := ctx.AST.Block(b.Parent)
				list := ctx.AST.Block(item.Parent)
				if item.Kind == contract.BlockItem && item.Task != contract.TaskNone && item.Next == i+1 &&
					list.Kind == contract.BlockList && b.Parent == item.Parent+1 && item.Next == list.Next {
					return replace(contract.Range{Start: l.start + p.a, End: r.End}, "", contract.OriginUser, "newline")
				}
			}
		}
		if p.kind == '-' || p.kind == '>' {
			if p.kind == '>' && l.start > 0 {
				previous := lineAt(ctx, l.start-1)
				if strings.TrimSpace(previous.s) != "" && strings.TrimRight(previous.s, " \t") == strings.TrimRight(l.s[:l.content], " \t") {
					outer := l.s[:p.a]
					return replace(contract.Range{Start: previous.start, End: r.End}, strings.TrimRight(outer, " \t")+nl+outer, contract.OriginUser, "newline")
				}
			}
			return replace(contract.Range{Start: l.start + p.a, End: r.End}, nl+l.s[:p.a], contract.OriginUser, "newline")
		}
	}
	if l.list >= 0 {
		return replace(r, nl+l.continuation(), contract.OriginUser, "newline")
	}
	// Continuation lines inherit the item's original marker and indentation.
	if item := ancestor(ctx, r.Start, contract.BlockItem); item != nil {
		first := lineAt(ctx, item.Range.Start)
		quote := ancestor(ctx, r.Start, contract.BlockQuote)
		insideQuote := false
		if quote != nil {
			for i := item.Parent; i > 0; i = ctx.AST.Block(i).Parent {
				if ctx.AST.Block(i).ID == quote.ID {
					insideQuote = true
					break
				}
			}
		}
		if first.list >= 0 && (quote == nil || insideQuote) {
			return replace(r, nl+first.continuation(), contract.OriginUser, "newline")
		}
	}
	if len(l.parts) > 0 && l.parts[len(l.parts)-1].kind == '>' {
		lead := l.s[:l.content]
		return replace(r, nl+strings.TrimRight(lead, " \t")+nl+lead, contract.OriginUser, "newline")
	}
	if strings.TrimSpace(l.s) == "" {
		if _, b := emptyParagraph(ctx, r.Start); b != nil {
			return replace(r, nl+nl, contract.OriginUser, "newline")
		}
		return replace(r, nl, contract.OriginUser, "newline")
	}
	if r.Empty() {
		_, b := leaf(ctx, r.Start)
		if b != nil && b.Kind == contract.BlockParagraph {
			for j, ln := range b.Lines {
				if r.Start == ln.Source.Start && j > 0 {
					return replace(r, nl, contract.OriginUser, "newline")
				}
				if r.Start == ln.Source.End && j+1 < len(b.Lines) {
					tx := replace(r, nl, contract.OriginUser, "newline")
					sel := contract.Caret(r.Start + len(nl) + l.next - l.end)
					tx.Selection = &sel
					return tx
				}
			}
		}
	}
	return replace(r, nl+nl, contract.OriginUser, "newline")
}

func rawContinuation(ctx contract.EditContext, l sourceLine, b *contract.Block) string {
	for _, ln := range b.Lines {
		if ln.Source.Start < l.start || ln.Source.Start > l.end {
			continue
		}
		at := ln.Source.Start - l.start
		for at < len(l.s) && space(l.s[at]) {
			at++
		}
		return l.s[:at]
	}
	if ancestor(ctx, l.start+l.content, contract.BlockItem) != nil || ancestor(ctx, l.start+l.content, contract.BlockQuote) != nil {
		return l.containerContinuation()
	}
	return l.s[:l.indent]
}

func selectedLines(ctx contract.EditContext) []sourceLine {
	r := ctx.Selection.Primary().Range()
	last := r.End
	if last > r.Start && lineAt(ctx, last).start == last {
		last--
	}
	var lines []sourceLine
	for l := lineAt(ctx, r.Start); ; l = lineAt(ctx, l.next) {
		lines = append(lines, l)
		if l.next > last || l.next == ctx.Text.Len() || l.next == l.start {
			break
		}
	}
	return lines
}

func indentation(ctx contract.EditContext, out bool, origin contract.Origin) (contract.Transaction, bool) {
	var edits []contract.TextEdit
	var numberEdits []contract.TextEdit
	r := ctx.Selection.Primary().Range()
	l := lineAt(ctx, r.Start)
	width := tabs(ctx)
	item := ancestor(ctx, r.Start, contract.BlockItem)
	if rawAt(ctx, r.Start) {
		item = nil
	}
	continuation := false
	if out && item != nil && l.list >= 0 && l.start != lineAt(ctx, item.Range.Start).start {
		first := lineAt(ctx, item.Range.Start)
		if l.indent >= first.indent+itemWidth(first) {
			width = itemWidth(first)
			continuation = true
		}
	}
	if item != nil {
		first := lineAt(ctx, item.Range.Start)
		number := first.number
		if out && !continuation {
			list := ctx.AST.Block(item.Parent)
			parent := ctx.AST.Block(list.Parent)
			if parent.Kind != contract.BlockItem {
				return transaction(nil, ctx.Selection, origin, "indent"), false
			}
			parentLine := lineAt(ctx, parent.Range.Start)
			width = itemWidth(parentLine)
			outer := ctx.AST.Block(parent.Parent)
			if list.Ordered && outer.Ordered && list.Marker == outer.Marker {
				number = parentLine.number + 1
			}
		} else if !out {
			var prev *contract.Block
			for i := range ctx.AST.Children(item.Parent) {
				b := ctx.AST.Block(i)
				if b.ID == item.ID {
					break
				}
				prev = b
			}
			if prev == nil {
				list := ctx.AST.Block(item.Parent)
				return transaction(nil, ctx.Selection, origin, "indent"), ctx.AST.Block(list.Parent).Kind != contract.BlockItem
			}
			width = itemWidth(lineAt(ctx, prev.Range.Start))
			previous := lineAt(ctx, first.start-1)
			_, paragraph := leaf(ctx, previous.start+previous.content)
			if paragraph != nil && paragraph.Kind == contract.BlockParagraph && ctx.AST.Block(paragraph.Parent).ID == prev.ID && strings.TrimSpace(previous.s[previous.content:]) != "" {
				// Only 1 can interrupt a paragraph to start an ordered list.
				number = 1
			}
		}
		if r.Empty() || l.start == first.start {
			numberEdits = orderedNumberEdits(first, number)
		}
	}
	_, block := leaf(ctx, r.Start)
	plain := block == nil || block.Kind == contract.BlockParagraph || block.Kind == contract.BlockBlank
	before, _ := utf8.DecodeLastRuneInString(ctx.Text.String()[l.start:r.Start])
	after, _ := utf8.DecodeRuneInString(ctx.Text.String()[r.Start:l.end])
	inlineTab := unicode.IsLetter(before) || unicode.IsNumber(before) || unicode.IsLetter(after) || unicode.IsNumber(after) || strings.TrimSpace(l.s) == ""
	if !out && r.Empty() && plain && inlineTab && l.indent < width && l.list < 0 && ancestor(ctx, r.Start, contract.BlockItem) == nil && ancestor(ctx, r.Start, contract.BlockQuote) == nil {
		return replace(r, strings.Repeat(" ", tabs(ctx)), origin, "indent"), true
	}
	if out && r.Empty() && plain && l.indent < width && item == nil && ancestor(ctx, r.Start, contract.BlockQuote) == nil && r.Start > l.start+l.indent {
		at := r.Start
		for at > l.start && r.Start-at < width && space(ctx.Text.String()[at-1]) {
			at--
		}
		if r.Start-at == width {
			return replace(contract.Range{Start: at, End: r.Start}, "", origin, "indent"), true
		}
	}
	lineCtx := ctx
	if continuation && r.Empty() {
		end := l.end
		for next := l.next; next < ctx.Text.Len(); {
			child := lineAt(ctx, next)
			if child.indent < l.indent || child.list >= 0 && child.indent == l.indent {
				break
			}
			end = child.end
			if child.next == next {
				break
			}
			next = child.next
		}
		lineCtx.Selection = contract.Selection{Ranges: []contract.SelRange{{Anchor: l.start, Head: end}}}
	}
	if r.Empty() && !continuation {
		if item != nil {
			lineCtx.Selection = contract.Selection{Ranges: []contract.SelRange{{Anchor: item.Range.Start, Head: item.Range.End}}}
		}
	}
	for _, l := range selectedLines(lineCtx) {
		at := indentAt(l)
		if item != nil {
			first := lineAt(ctx, item.Range.Start)
			at = min(l.end, l.start+indentAt(first)-first.start)
		}
		if !out {
			edits = append(edits, edit(at, at, strings.Repeat(" ", width)))
			continue
		}
		end := at
		for end < l.end && end-at < width && space(ctx.Text.String()[end]) {
			c := ctx.Text.String()[end]
			end++
			if c == '\t' {
				break
			}
		}
		if end > at {
			edits = append(edits, edit(at, end, ""))
		}
	}
	edits = orderedEdits(append(edits, numberEdits...))
	return transaction(edits, mapped(ctx.Selection, edits), origin, "indent"), len(edits) > 0
}

func orderedNumberEdits(l sourceLine, number int) []contract.TextEdit {
	if l.list < 0 || l.delim != '.' && l.delim != ')' {
		return nil
	}
	a := l.parts[l.list].a
	b := a
	for b < len(l.s) && l.s[b] >= '0' && l.s[b] <= '9' {
		b++
	}
	if number == l.number {
		return nil
	}
	return []contract.TextEdit{edit(l.start+a, l.start+b, strconv.Itoa(number))}
}

func indentAt(l sourceLine) int {
	at := l.start
	for _, p := range l.parts {
		if p.kind != '>' {
			break
		}
		at = l.start + p.a + 1
		if at == l.end || !space(l.s[at-l.start]) {
			return l.start
		}
		if at < l.end && space(l.s[at-l.start]) {
			at++
		}
	}
	return at
}

func itemWidth(l sourceLine) int {
	if l.list < 0 {
		return 2
	}
	p := l.parts[l.list]
	end := p.b
	if l.task >= 0 {
		end = l.task
	}
	col := 0
	for _, c := range l.s[p.a:end] {
		if c == '\t' {
			col += 4 - col%4
		} else {
			col++
		}
	}
	return col
}
