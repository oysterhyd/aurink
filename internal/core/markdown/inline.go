package markdown

import (
	"strings"

	"aurink/internal/contract"
	"github.com/egoist/mygo/ui/markdown/parser"
)

func (s *Snapshot) inlineDoc() *parser.Document {
	s.lazyOnce.Do(func() { s.lazy = parser.New(parser.Default); s.lazy.Set(s.text.String()[s.bodyAt:]) })
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
				d := parser.New(parser.Default)
				local := "# " + line
				d.Set(local)
				if d.Len() != 2 || d.LineCount(1) != 1 {
					return
				}
				got, _ := d.Line(1, 0)
				if got != line {
					return
				}
				data.inline = adaptInlines(local, d.Inlines(1), 0)
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
			d = parser.New(parser.Default)
			d.Set(s.text.String()[start:end])
			shift = start
		} else {
			d = s.inlineDoc()
		}
		var in []parser.Inline
		found := false
		for j := 1; j < d.Len(); j++ {
			p := d.Block(j)
			if contract.BlockKind(p.Kind) == b.Kind && p.Start+shift == b.Range.Start && d.LineCount(j) == len(b.Lines) {
				matches := true
				for k, line := range b.Lines {
					_, at := d.Line(j, k)
					matches = matches && at+shift == line.Source.Start
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
			for j := 1; j < d.Len(); j++ {
				p := d.Block(j)
				if contract.BlockKind(p.Kind) == b.Kind && p.Start+shift == b.Range.Start {
					in, found = d.Inlines(j), true
					break
				}
			}
		}
		if !found && b.Kind == contract.BlockTableCell {
			// Overflow cells have no upstream block. A synthetic table preserves
			// cell escapes, with the snapshot as its global reference context.
			d = parser.New(parser.Default)
			context := ""
			if s.hasDefinitions {
				context = s.text.String()[s.bodyAt:]
			}
			d.Set("| " + s.text.Slice(b.Range) + " |\n|---|\n\n" + context)
			in = d.Inlines(3)
			shift = b.Range.Start - 2
		}
		e.in = adaptInlines(s.text.String(), in, shift)
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

func adaptInlines(src string, in []parser.Inline, shift int) []contract.Inline {
	var out []contract.Inline
	var walk func(int, int, bool)
	walk = func(from, end int, literal bool) {
		for k := from; k < end; k = in[k].Next {
			n := in[k]
			r := contract.Range{Start: n.Start + shift, End: n.End + shift}
			if r.Start < 0 || r.End > len(src) || r.End < r.Start {
				continue
			}
			if n.Kind == parser.Text && !literal {
				parts := scanText(src, r)
				var decoded strings.Builder
				for _, p := range parts {
					decoded.WriteString(p.Text)
				}
				if decoded.String() != n.Text && strings.TrimRight(decoded.String(), " ") == n.Text && len(parts) > 0 {
					parts[len(parts)-1].Text = strings.TrimRight(parts[len(parts)-1].Text, " ")
				}
				for _, p := range parts {
					p.Next = len(out) + 1
					out = append(out, p)
				}
				continue
			}
			j := len(out)
			p := contract.Inline{Kind: contract.InlineKind(n.Kind), Range: r, Text: n.Text, Dest: n.Dest, Title: n.Title, Auto: n.Auto, Display: n.Display}
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
