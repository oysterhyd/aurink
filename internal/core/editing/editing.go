package editing

import (
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"

	"aurink/internal/contract"
)

// Editor converts editing requests into transactions without retaining state.
// Its zero value is ready to use and is safe for concurrent calls.
type Editor struct{}

var _ contract.Editing = Editor{}

// New returns a stateless editing service.
func New() *Editor { return &Editor{} }

func valid(ctx contract.EditContext) bool {
	r := ctx.Selection.Primary().Range()
	return ctx.Text != nil && r.Start >= 0 && r.End <= ctx.Text.Len()
}

func eol(ctx contract.EditContext) string {
	if ctx.Meta.Dominant == "\r\n" {
		return "\r\n"
	}
	return "\n"
}

func tabs(ctx contract.EditContext) int {
	if ctx.Prefs.TabSize > 0 && ctx.Prefs.TabSize <= 16 {
		return ctx.Prefs.TabSize
	}
	return 2
}

func bullet(ctx contract.EditContext) string {
	if strings.ContainsRune("-*+", rune(ctx.Prefs.BulletMarker)) && ctx.Prefs.BulletMarker != 0 {
		return string(ctx.Prefs.BulletMarker)
	}
	return "-"
}

func edit(a, b int, s string) contract.TextEdit {
	return contract.TextEdit{Range: contract.Range{Start: a, End: b}, Text: s}
}

func transaction(edits []contract.TextEdit, sel contract.Selection, origin contract.Origin, label string) contract.Transaction {
	return contract.Transaction{Edits: edits, Selection: &sel, Origin: origin, Label: "editing_undo_" + label}
}

func replace(r contract.Range, s string, origin contract.Origin, label string) contract.Transaction {
	return transaction([]contract.TextEdit{edit(r.Start, r.End, s)}, contract.Caret(r.Start+len(s)), origin, label)
}

func mapped(sel contract.Selection, edits []contract.TextEdit) contract.Selection {
	point := func(p int) int {
		shift := 0
		for _, e := range edits {
			if p < e.Range.Start {
				break
			}
			if p < e.Range.End {
				return e.Range.Start + shift + len(e.Text)
			}
			shift += len(e.Text) - e.Range.Len()
		}
		return p + shift
	}
	r := sel.Primary()
	return contract.Selection{Ranges: []contract.SelRange{{Anchor: point(r.Anchor), Head: point(r.Head)}}}
}

func selected(ctx contract.EditContext, a, b int) contract.Selection {
	if ctx.Selection.Primary().Anchor > ctx.Selection.Primary().Head {
		a, b = b, a
	}
	return contract.Selection{Ranges: []contract.SelRange{{Anchor: a, Head: b}}}
}

func leaf(ctx contract.EditContext, at int) (int, *contract.Block) {
	if ctx.AST == nil || ctx.AST.Len() == 0 {
		return -1, nil
	}
	i := ctx.AST.LeafAt(at)
	return i, ctx.AST.Block(i)
}

func ancestor(ctx contract.EditContext, at int, kind contract.BlockKind) *contract.Block {
	i, b := leaf(ctx, at)
	for i >= 0 && b != nil {
		if b.Kind == kind {
			return b
		}
		i = b.Parent
		if i >= 0 {
			b = ctx.AST.Block(i)
		}
	}
	return nil
}

func normalizeNewlines(s, ending string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\n", ending)
}

// HandleIntent handles text, deletion, newline, indentation, and text paste.
func (Editor) HandleIntent(ctx contract.EditContext, in contract.Intent) (contract.Transaction, bool) {
	if valid(ctx) {
		if tx, ok := tableIntent(ctx, in); ok {
			return tx, true
		}
	}
	tx, ok := handleIntent(ctx, in)
	if !ok {
		return tx, false
	}
	r := ctx.Selection.Primary().Range()
	typing := in.Kind == contract.IntentInsertText
	row := ancestor(ctx, r.Start, contract.BlockTableRow)
	inRow := row != nil && r.End <= row.Range.End && !strings.ContainsAny(in.Text, "\r\n")
	check := !typing || !r.Empty() || rawAt(ctx, r.Start) || strings.ContainsAny(in.Text, "\r\n`~<")
	if typing && strings.TrimSpace(lineAt(ctx, r.Start).s) == "" {
		check = true
	}
	if in.Kind == contract.IntentNewline && r.Empty() && !rawAt(ctx, r.Start) {
		l := lineAt(ctx, r.Start)
		if l.list >= 0 && r.Start == l.end && strings.TrimSpace(l.s[l.content:]) != "" {
			check = false
		}
	}
	if (!typing && !tableSafe(ctx, tx.Edits)) || check && !inRow && !preservesTables(ctx, tx.Edits) {
		// TODO(BE-1.6): Navigate table cells without changing row structure.
		return transaction(nil, ctx.Selection, tx.Origin, "table"), true
	}
	return tx, ok
}

func handleIntent(ctx contract.EditContext, in contract.Intent) (contract.Transaction, bool) {
	if !valid(ctx) {
		return contract.Transaction{}, false
	}
	r := ctx.Selection.Primary().Range()
	if (in.Kind == contract.IntentIndent || in.Kind == contract.IntentOutdent || in.Kind == contract.IntentNewline || in.Kind == contract.IntentLineBreak) && selectionHas(ctx, contract.BlockTable) {
		// TODO(BE-1.6): Table cell navigation.
		return transaction(nil, ctx.Selection, contract.OriginUser, "table"), true
	}
	switch in.Kind {
	case contract.IntentInsertText:
		if !utf8.ValidString(in.Text) {
			return contract.Transaction{}, false
		}
		if ctx.Prefs.AutoPair && !strings.ContainsAny(in.Text, "\r\n") {
			if tx, ok := pair(ctx, in.Text); ok {
				return tx, true
			}
		}
		tx := replace(r, normalizeNewlines(in.Text, eol(ctx)), contract.OriginUser, "typing")
		if !strings.ContainsAny(in.Text, "\r\n") {
			tx.Coalesce = "typing"
		}
		return tx, true
	case contract.IntentNewline:
		return newline(ctx), true
	case contract.IntentLineBreak:
		if rawAt(ctx, r.Start) {
			_, b := leaf(ctx, r.Start)
			return replace(r, eol(ctx)+rawContinuation(ctx, lineAt(ctx, r.Start), b), contract.OriginUser, "newline"), true
		}
		return replace(r, "  "+eol(ctx), contract.OriginUser, "newline"), true
	case contract.IntentDeleteBackward, contract.IntentDeleteForward:
		return deletion(ctx, in), true
	case contract.IntentIndent, contract.IntentOutdent:
		return indentation(ctx, in.Kind == contract.IntentOutdent, contract.OriginUser)
	case contract.IntentPaste:
		if in.Paste == nil {
			return contract.Transaction{}, false
		}
		s := in.Paste.Plain
		if !in.Paste.AsPlain && !selectionHas(ctx, contract.BlockCode) && in.Paste.Markdown != "" {
			s = in.Paste.Markdown
		}
		if !in.Paste.AsPlain && !r.Empty() && !selectionHas(ctx, contract.BlockCode) && s == in.Paste.Plain && singleURL(s) {
			return replace(r, "["+ctx.Text.Slice(r)+"]("+linkDestination(s)+")", contract.OriginPaste, "paste"), true
		}
		return replace(r, normalizeNewlines(s, eol(ctx)), contract.OriginPaste, "paste"), true
	}
	return contract.Transaction{}, false
}

func singleURL(s string) bool {
	if strings.TrimSpace(s) != s || strings.ContainsAny(s, " \t\r\n") {
		return false
	}
	u, err := url.Parse(s)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

func pair(ctx contract.EditContext, s string) (contract.Transaction, bool) {
	r := ctx.Selection.Primary().Range()
	source := ctx.Text.String()
	if len(s) != 1 || selectionHas(ctx, contract.BlockCode, contract.BlockHTML, contract.BlockFrontMatter, contract.BlockMath) {
		return contract.Transaction{}, false
	}
	if r.Empty() && strings.ContainsAny(s, ")]}`") && strings.HasPrefix(source[r.Start:], s) {
		return transaction(nil, contract.Caret(r.Start+1), contract.OriginUser, "typing"), true
	}
	if inlineLiteral(ctx) {
		return contract.Transaction{}, false
	}
	if s == "`" && r.Empty() {
		l := lineAt(ctx, r.Start)
		if r.Start >= l.start+l.content {
			before := source[l.start+l.content : r.Start]
			if len(before) >= 2 && strings.Trim(before, "`") == "" {
				return replace(r, "`", contract.OriginUser, "typing"), true
			}
		}
	}
	close := ""
	switch s {
	case "(":
		close = ")"
	case "[":
		close = "]"
	case "`":
		close = "`"
	case "*", "_", "~":
		if !r.Empty() {
			close = s
		}
	}
	if close == "" || escaped(source, r.Start) {
		return contract.Transaction{}, false
	}
	if r.Empty() {
		if r.Start < len(source) {
			next, _ := utf8.DecodeRuneInString(source[r.Start:])
			if !unicode.IsSpace(next) && !strings.ContainsRune(")]}>\"'`", next) {
				return contract.Transaction{}, false
			}
		}
		return transaction([]contract.TextEdit{edit(r.Start, r.End, s+close)}, contract.Caret(r.Start+1), contract.OriginUser, "typing"), true
	}
	if s == "*" || s == "_" || s == "`" {
		id := contract.CmdFormatEmphasis
		if s == "`" {
			id = contract.CmdFormatCode
		} else {
			ctx.Prefs.EmphasisDelim = s
		}
		tx, ok, err := inlineCommand(ctx, contract.EditCommand{ID: id})
		if ok && err == nil {
			tx.Origin, tx.Label = contract.OriginUser, "editing_undo_typing"
			return tx, true
		}
	}
	return transaction([]contract.TextEdit{edit(r.Start, r.Start, s), edit(r.End, r.End, close)}, selected(ctx, r.Start+1, r.End+1), contract.OriginUser, "typing"), true
}

func rawAt(ctx contract.EditContext, at int) bool {
	_, b := leaf(ctx, at)
	return b != nil && (b.Kind == contract.BlockCode || b.Kind == contract.BlockHTML || b.Kind == contract.BlockFrontMatter || b.Kind == contract.BlockMath)
}

func inlineLiteral(ctx contract.EditContext) bool {
	r := ctx.Selection.Primary().Range()
	i, b := leaf(ctx, r.Start)
	if b == nil {
		return false
	}
	for _, n := range ctx.AST.Inlines(i) {
		c := contentRange(n)
		if (n.Kind == contract.InlineCode || n.Kind == contract.InlineMath) && c.Start <= r.Start && c.End >= r.End {
			return true
		}
	}
	return false
}

func selectionHas(ctx contract.EditContext, kinds ...contract.BlockKind) bool {
	for _, i := range selectedLeaves(ctx) {
		b := ctx.AST.Block(i)
		for b != nil {
			for _, kind := range kinds {
				if b.Kind == kind {
					return true
				}
			}
			if b.Parent < 0 {
				break
			}
			b = ctx.AST.Block(b.Parent)
		}
	}
	return false
}

func tableSafe(ctx contract.EditContext, edits []contract.TextEdit) bool {
	if ctx.AST == nil || len(edits) == 0 {
		return true
	}
	for _, e := range edits {
		probe := ctx
		probe.Selection = contract.Selection{Ranges: []contract.SelRange{{Anchor: e.Range.Start, Head: e.Range.End}}}
		for _, i := range selectedLeaves(probe) {
			b := ctx.AST.Block(i)
			if ancestor(ctx, b.Range.Start, contract.BlockTable) == nil {
				continue
			}
			inside := b.Kind == contract.BlockTableCell && e.Range.Start >= b.Range.Start && e.Range.End <= b.Range.End
			if !inside && e.Range.Empty() && i > 0 {
				prev := ctx.AST.Block(i - 1)
				inside = prev.Kind == contract.BlockTableCell && prev.Parent == b.Parent && prev.Range.End == e.Range.Start
			}
			if !inside || strings.ContainsAny(e.Text+ctx.Text.Slice(e.Range), "|\r\n") {
				return false
			}
		}
	}
	return true
}

func escaped(s string, at int) bool {
	n := 0
	for at > 0 && s[at-1] == '\\' {
		n++
		at--
	}
	return n%2 != 0
}
