package editing

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"aurink/internal/contract"
)

func extend(r rune) bool {
	return unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Mc, r) || unicode.Is(unicode.Me, r) ||
		r >= 0xfe00 && r <= 0xfe0f || r >= 0x1f3fb && r <= 0x1f3ff ||
		r >= 0xe0020 && r <= 0xe007f || r >= 0xe0100 && r <= 0xe01ef
}

func regional(r rune) bool { return r >= 0x1f1e6 && r <= 0x1f1ff }

func hangul(r rune) int {
	switch {
	case r >= 0x1100 && r <= 0x115f || r >= 0xa960 && r <= 0xa97c:
		return 1
	case r >= 0x1160 && r <= 0x11a7 || r >= 0xd7b0 && r <= 0xd7c6:
		return 2
	case r >= 0x11a8 && r <= 0x11ff || r >= 0xd7cb && r <= 0xd7fb:
		return 3
	case r >= 0xac00 && r <= 0xd7a3:
		if (r-0xac00)%28 == 0 {
			return 4
		}
		return 5
	}
	return 0
}

// nextGrapheme covers combining marks, emoji modifiers/ZWJ sequences, flags,
// Hangul syllables and CRLF without introducing a module dependency.
func nextGrapheme(s string, at int) int {
	if at >= len(s) {
		return len(s)
	}
	prev, n := utf8.DecodeRuneInString(s[at:])
	i, flags := at+n, 0
	if regional(prev) {
		flags = 1
	}
	for i < len(s) {
		r, n := utf8.DecodeRuneInString(s[i:])
		a, b := hangul(prev), hangul(r)
		join := prev == '\r' && r == '\n' || extend(r) || r == '\u200d' || prev == '\u200d' ||
			regional(r) && flags == 1 || a == 1 && (b == 1 || b == 2 || b == 4 || b == 5) ||
			(a == 2 || a == 4) && (b == 2 || b == 3) || (a == 3 || a == 5) && b == 3
		if !join || (prev == '\n' || prev == '\r') && r != '\n' {
			break
		}
		if regional(r) {
			flags++
		}
		i += n
		prev = r
	}
	return i
}

func previousGrapheme(s string, at int) int {
	// A line boundary is also a grapheme boundary, except inside CRLF.
	a := strings.LastIndexByte(s[:at], '\n') + 1
	if a == at && at > 0 {
		if at > 1 && s[at-2] == '\r' {
			return at - 2
		}
		return at - 1
	}
	for a < at {
		b := nextGrapheme(s, a)
		if b >= at {
			return a
		}
		a = b
	}
	return a
}

func wordClass(r rune) int {
	if unicode.IsSpace(r) {
		return 0
	}
	if unicode.Is(unicode.Han, r) {
		return 2
	}
	if unicode.IsLetter(r) || unicode.IsNumber(r) || extend(r) || r == '_' {
		return 1
	}
	return 3
}

func wordBoundary(s string, at int, back bool) int {
	class := -1
	for at > 0 && back || at < len(s) && !back {
		next := nextGrapheme(s, at)
		if back {
			next = previousGrapheme(s, at)
		}
		start := at
		if back {
			start = next
		}
		r, _ := utf8.DecodeRuneInString(s[start:])
		c := wordClass(r)
		if class >= 0 && c != class {
			break
		}
		at = next
		if c != 0 || class >= 0 {
			class = c
		}
	}
	return at
}

func deletion(ctx contract.EditContext, in contract.Intent) contract.Transaction {
	r := ctx.Selection.Primary().Range()
	if !r.Empty() {
		return replace(r, "", contract.OriginUser, "delete")
	}
	s, p := ctx.Text.String(), r.Start
	back := in.Kind == contract.IntentDeleteBackward
	if in.Gran == contract.GranChar {
		if tx, ok := deleteEmptyParagraph(ctx, p, back); ok {
			return tx
		}
		if back {
			if tx, ok := deleteEmptyFenceCloser(ctx, p); ok {
				return tx
			}
			l := lineAt(ctx, p)
			if _, marker, ok := pendingLine(ctx, l); ok && p > marker.Start && p <= marker.End {
				return replace(contract.Range{Start: previousGrapheme(s, p), End: p}, "", contract.OriginUser, "delete")
			}
			_, block := leaf(ctx, p)
			if block != nil && block.Kind == contract.BlockHeading && p == block.Content.Start && len(l.parts) == 0 {
				for _, marker := range block.Markers {
					if marker.Start > l.end {
						edits := []contract.TextEdit{edit(l.end, marker.End, "")}
						return transaction(edits, mapped(ctx.Selection, edits), contract.OriginUser, "delete")
					}
				}
			}
			if p == l.start+l.indent && l.indent > 0 && ancestor(ctx, p, contract.BlockCode) != nil {
				start := p - 1
				if l.s[l.indent-1] != '\t' {
					start = p - (l.indent-1)%tabs(ctx) - 1
				}
				return replace(contract.Range{Start: start, End: p}, "", contract.OriginUser, "delete")
			}
			if p == l.start+l.content && len(l.parts) > 0 && !rawAt(ctx, p) {
				part := l.parts[len(l.parts)-1]
				a := l.start + part.a
				if part.kind == '-' && len(l.parts) == 1 {
					if l.indent > 0 {
						edits := []contract.TextEdit{edit(l.start, l.start+min(l.indent, tabs(ctx)), "")}
						return transaction(edits, mapped(ctx.Selection, edits), contract.OriginUser, "delete")
					}
					a = l.start
				}
				return replace(contract.Range{Start: a, End: p}, "", contract.OriginUser, "delete")
			}
			if p > 0 && p < len(s) && ctx.Prefs.AutoPair && !rawAt(ctx, p) && (s[p-1] == '(' && s[p] == ')' || s[p-1] == '[' && s[p] == ']' || s[p-1] == '`' && s[p] == '`') {
				return replace(contract.Range{Start: p - 1, End: p + 1}, "", contract.OriginUser, "delete")
			}
		}
		if tx, ok := deleteVisible(ctx, p, back); ok {
			return tx
		}
	}
	a, b := p, p
	switch in.Gran {
	case contract.GranWord:
		if back {
			a = wordBoundary(s, p, true)
		} else {
			b = wordBoundary(s, p, false)
		}
	case contract.GranLine:
		l := lineAt(ctx, p)
		if back {
			a = l.start
			if a == p {
				a = previousGrapheme(s, p)
			}
		} else {
			b = l.end
			if b == p {
				b = l.next
			}
		}
	default:
		if back {
			a = previousGrapheme(s, p)
		} else {
			b = nextGrapheme(s, p)
		}
	}
	return replace(contract.Range{Start: a, End: b}, "", contract.OriginUser, "delete")
}

func deleteVisible(ctx contract.EditContext, p int, back bool) (contract.Transaction, bool) {
	i, b := leaf(ctx, p)
	if b == nil || b.Kind == contract.BlockCode {
		return contract.Transaction{}, false
	}
	pos := p
	nodes := ctx.AST.Inlines(i)
	for _, n := range nodes {
		if n.Kind == contract.InlineImage && (back && n.Range.End == p || !back && n.Range.Start == p) {
			return replace(n.Range, "", contract.OriginUser, "delete"), true
		}
	}
	for _, n := range nodes {
		// Inline math shows its $ beside the caret: they are deleted as the
		// characters they are, $abc$| to $abc|, as anything shown is.
		if n.Kind == contract.InlineMath {
			continue
		}
		if back && n.Close.End == pos && !n.Close.Empty() && n.Close.Start > n.Open.End {
			pos = n.Close.Start
		} else if !back && n.Open.Start == pos && !n.Open.Empty() && n.Open.End < n.Close.Start {
			pos = n.Open.End
		}
	}
	for _, n := range nodes {
		if n.Kind == contract.InlineEntity || n.Kind == contract.InlineEscape || n.Kind == contract.InlineImage {
			if back && n.Range.End == pos || !back && n.Range.Start == pos {
				return replace(n.Range, "", contract.OriginUser, "delete"), true
			}
		}
	}
	if pos == p {
		return contract.Transaction{}, false
	}
	a, end := pos, pos
	if back {
		a = previousGrapheme(ctx.Text.String(), pos)
	} else {
		end = nextGrapheme(ctx.Text.String(), pos)
	}
	return replace(contract.Range{Start: a, End: end}, "", contract.OriginUser, "delete"), true
}
