package markdown

import (
	"strings"

	"aurink/internal/contract"
	"github.com/egoist/mygo/ui/markdown/parser"
)

func (s *Snapshot) inlineDoc() *parser.Document {
	s.lazyOnce.Do(func() { s.lazy, s.lazyInput = parserDocument(s.text.String()[s.bodyAt:], parserExtensions(s.exts)) })
	return s.lazy
}

// Inlines lazily parses from this snapshot's source only. Its private parser is
// protected by a mutex and is never passed to the incremental writer.
func (s *Snapshot) Inlines(i int) []contract.Inline {
	if i < 0 || i >= s.Len() {
		return nil
	}
	e := &s.entries[i]
	e.inOnce.Do(func() {
		b := s.Block(i)
		if b.Kind != contract.BlockParagraph && b.Kind != contract.BlockHeading && b.Kind != contract.BlockTableCell {
			return
		}
		if !s.hasDefinitions && b.Kind != contract.BlockTableCell && len(b.Lines) == 1 {
			data := e.data
			data.inlineOnce.Do(func() {
				line := s.text.Slice(b.Lines[0].Source)
				d := parser.New(parserExtensions(s.exts))
				local := "# " + line
				d.Set(local)
				if d.Len() != 2 || d.LineCount(1) != 1 {
					return
				}
				got, _ := d.Line(1, 0)
				if got != line {
					return
				}
				data.inline = extendInlines(local, adaptInlines(local, d.Inlines(1), 0, parserInput{}, s.exts), s.exts)
				shiftInlines(data.inline, b.Lines[0].Source.Start-e.start-2)
				data.local = true
			})
			if data.local {
				e.in = append([]contract.Inline(nil), data.inline...)
				shiftInlines(e.in, e.start)
				return
			}
		}
		s.lazyMu.Lock()
		defer s.lazyMu.Unlock()
		var d *parser.Document
		var input parserInput
		shift := s.bodyAt
		if !s.hasDefinitions {
			top := i
			for s.entries[top].parent > 0 {
				top = s.entries[top].parent
			}
			first, last := firstLeaf(s, top), s.entries[top].next-1
			for !isLeaf(s.entries[last].data.block.Kind) {
				last--
			}
			start, end := s.Block(first).Lead.Start, s.Block(last).Trailer.End
			d, input = parserDocument(s.text.String()[start:end], parserExtensions(s.exts))
			shift = start
		} else {
			d = s.inlineDoc()
			input = s.lazyInput
		}
		var in []parser.Inline
		found := false
		for j := 1; j < d.Len(); j++ {
			p := d.Block(j)
			if contract.BlockKind(p.Kind) == b.Kind && input.offset(p.Start)+shift == b.Range.Start && d.LineCount(j) == len(b.Lines) {
				matches := true
				for k, line := range b.Lines {
					_, at := d.Line(j, k)
					matches = matches && input.offset(at)+shift == line.Source.Start
				}
				if !matches {
					continue
				}
				in = d.Inlines(j)
				found = true
				break
			}
		}
		if !found && !s.hasDefinitions {
			// A top-level leaf can begin in another block's table delimiter or
			// depend on a lazy continuation before it. Recover its full context.
			d, shift = s.inlineDoc(), s.bodyAt
			input = s.lazyInput
			for j := 1; j < d.Len(); j++ {
				p := d.Block(j)
				if contract.BlockKind(p.Kind) == b.Kind && input.offset(p.Start)+shift == b.Range.Start {
					in, found = d.Inlines(j), true
					break
				}
			}
		}
		if !found && b.Kind == contract.BlockTableCell {
			// Overflow cells have no upstream block. A synthetic table preserves
			// cell escapes, with the snapshot as its global reference context.
			context := ""
			if s.hasDefinitions {
				context = s.text.String()[s.bodyAt:]
			}
			d, input = parserDocument("| "+s.text.Slice(b.Range)+" |\n|---|\n\n"+context, parserExtensions(s.exts))
			in = d.Inlines(3)
			shift = b.Range.Start - 2
			found = true
		}
		if s.exts&contract.ExtDollarMath != 0 {
			for found {
				nextInput, changed := input.escapeMultilineMath(in)
				if !changed {
					break
				}
				next := parser.New(parserExtensions(s.exts))
				next.Set(nextInput.text)
				match := -1
				for j := 1; j < next.Len(); j++ {
					p := next.Block(j)
					if contract.BlockKind(p.Kind) == b.Kind && nextInput.offset(p.Start)+shift == b.Range.Start && next.LineCount(j) == len(b.Lines) {
						match = j
						break
					}
				}
				if match < 0 {
					break
				}
				d, input = next, nextInput
				in = d.Inlines(match)
			}
		}
		e.in = extendInlines(s.text.String(), adaptInlines(s.text.String(), in, shift, input, s.exts), s.exts)
	})
	return e.in
}

func shiftInlines(in []contract.Inline, delta int) {
	for i := range in {
		n := &in[i]
		n.Range = moveRange(n.Range, delta)
		if !n.Open.Empty() {
			n.Open = moveRange(n.Open, delta)
		}
		if !n.Close.Empty() {
			n.Close = moveRange(n.Close, delta)
		}
	}
}

func adaptInlines(src string, in []parser.Inline, shift int, input parserInput, exts contract.Extensions) []contract.Inline {
	var out []contract.Inline
	var walk func(int, int, bool)
	walk = func(from, end int, literal bool) {
		for k := from; k < end; k = in[k].Next {
			n := in[k]
			r := contract.Range{Start: input.offset(n.Start) + shift, End: input.offset(n.End) + shift}
			if r.Start < 0 || r.End > len(src) || r.End < r.Start {
				continue
			}
			raw := src[r.Start:r.End]
			plainMath := n.Kind == parser.Math && exts&contract.ExtDollarMath != 0 && len(raw) >= 4 && strings.HasPrefix(raw, "$$") &&
				(strings.ContainsAny(raw, "\r\n") || strings.TrimSpace(raw[2:len(raw)-2]) == "")
			if n.Kind == parser.Text && !literal || plainMath {
				parts := scanText(src, r)
				var decoded strings.Builder
				for _, p := range parts {
					decoded.WriteString(p.Text)
				}
				if decoded.String() != input.clean(n.Text) && strings.TrimRight(decoded.String(), " ") == input.clean(n.Text) && len(parts) > 0 {
					parts[len(parts)-1].Text = strings.TrimRight(parts[len(parts)-1].Text, " ")
				}
				for _, p := range parts {
					p.Next = len(out) + 1
					out = append(out, p)
				}
				continue
			}
			j := len(out)
			p := contract.Inline{Kind: contract.InlineKind(n.Kind), Range: r, Text: input.clean(n.Text), Dest: input.clean(n.Dest), Title: input.clean(n.Title), Auto: n.Auto, Display: n.Display}
			if p.Kind == contract.InlineMath && exts&contract.ExtDollarMath != 0 && strings.HasPrefix(src[r.Start:r.End], "$$") {
				p.Display = false
			}
			delimiters(src, &p)
			out = append(out, p)
			walk(k+1, n.Next, literal || n.Auto)
			out[j].Next = len(out)
		}
	}
	walk(0, len(in), false)
	return out
}

func delimiters(src string, n *contract.Inline) {
	r := n.Range
	t := src[r.Start:r.End]
	a, b := 0, 0
	switch n.Kind {
	case contract.InlineEmphasis:
		a, b = 1, 1
	case contract.InlineStrong:
		a, b = 2, 2
	case contract.InlineStrike:
		a = 1
		if strings.HasPrefix(t, "~~") && strings.HasSuffix(t, "~~") {
			a = 2
		}
		b = a
	case contract.InlineCode:
		for a < len(t) && t[a] == '`' {
			a++
		}
		b = a
	case contract.InlineMath:
		if strings.HasPrefix(t, `\(`) || strings.HasPrefix(t, `\[`) || strings.HasPrefix(t, "$$") {
			a, b = 2, 2
		} else if strings.HasPrefix(t, "$") && strings.HasSuffix(t, "$") {
			a, b = 1, 1
		}
	case contract.InlineFootnoteRef:
		a, b = 2, 1
	case contract.InlineLink, contract.InlineImage:
		if n.Auto {
			if strings.HasPrefix(t, "<") && strings.HasSuffix(t, ">") {
				a, b = 1, 1
			}
			break
		}
		a = 1
		if n.Kind == contract.InlineImage {
			a = 2
		}
		depth := 1
		for j := a; j < len(t); j++ {
			if t[j] == '\\' && j+1 < len(t) {
				j++
				continue
			}
			if t[j] == '[' {
				depth++
			}
			if t[j] == ']' {
				depth--
				if depth == 0 {
					b = len(t) - j
					break
				}
			}
		}
	}
	if a+b > len(t) {
		return
	}
	if a > 0 {
		n.Open = contract.Range{Start: r.Start, End: r.Start + a}
	}
	if b > 0 {
		n.Close = contract.Range{Start: r.End - b, End: r.End}
	}
}
