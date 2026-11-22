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
		if b.Kind == contract.BlockTableCell && b.Content.Empty() {
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
		var localSrc string
		useLocal := false
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
			localSrc = s.text.Slice(contract.Range{Start: start, End: end})
			d, input = parserDocument(localSrc, parserExtensions(s.exts))
			shift = start
			useLocal = true
		} else {
			d = s.inlineDoc()
			input = s.lazyInput
		}
		var in []parser.Inline
		found := false
		taskStart := -1
		if b.Parent > 0 && i == b.Parent+1 && len(b.Lines) > 0 && b.Lines[0].Source.Empty() {
			item := s.Block(b.Parent)
			if item.Kind == contract.BlockItem && item.Task != contract.TaskNone {
				if useLocal && item.TaskMarker.Start >= shift && b.Range.Start <= shift+len(localSrc) {
					if end, ok := emptyTaskLine(localSrc, item.TaskMarker.Start-shift); ok && b.Range.Start == shift+end {
						taskStart = item.TaskMarker.End + 1
					}
				} else if end, ok := emptyTaskLine(s.text.String(), item.TaskMarker.Start); ok && b.Range.Start == end {
					taskStart = item.TaskMarker.End + 1
				}
			}
		}
		matches := func(d *parser.Document, input parserInput, j, shift int, lines bool) bool {
			p := d.Block(j)
			start := b.Range.Start
			if taskStart >= 0 {
				start = taskStart
			}
			if contract.BlockKind(p.Kind) != b.Kind || input.offset(p.Start)+shift != start {
				return false
			}
			if lines {
				if d.LineCount(j) != len(b.Lines) {
					return false
				}
				for k, line := range b.Lines {
					_, at := d.Line(j, k)
					if k == 0 && taskStart >= 0 {
						if input.offset(at)+shift != taskStart {
							return false
						}
					} else if input.offset(at)+shift != line.Source.Start {
						return false
					}
				}
			}
			return true
		}
		for j := 1; j < d.Len(); j++ {
			if matches(d, input, j, shift, true) {
				in = d.Inlines(j)
				found = true
				break
			}
		}
		if !found && !s.hasDefinitions {
			// A top-level leaf can begin in another block's table delimiter or
			// depend on a lazy continuation before it. Recover its full context.
			useLocal = false
			d, shift = s.inlineDoc(), s.bodyAt
			input = s.lazyInput
			for j := 1; j < d.Len(); j++ {
				if matches(d, input, j, shift, false) {
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
			useLocal = false
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
					if matches(next, nextInput, j, shift, true) {
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
		if useLocal {
			adapted := adaptInlines(localSrc, in, 0, input, s.exts)
			if taskStart >= 0 {
				adapted = trimEmptyTaskInlines(adapted, b.Range.Start-shift)
			}
			e.in = extendInlines(localSrc, adapted, s.exts)
			shiftInlines(e.in, shift)
			return
		}
		adapted := adaptInlines(s.text.String(), in, shift, input, s.exts)
		if taskStart >= 0 {
			adapted = trimEmptyTaskInlines(adapted, b.Range.Start)
		}
		e.in = extendInlines(s.text.String(), adapted, s.exts)
	})
	return e.in
}

// The temporary task byte and the remaining marker whitespace are not content.
func trimEmptyTaskInlines(in []contract.Inline, end int) []contract.Inline {
	n := 0
	for n < len(in) && in[n].Range.End <= end && in[n].Next == n+1 {
		n++
	}
	in = in[n:]
	for i := range in {
		in[i].Next -= n
	}
	if len(in) > 0 && (in[0].Kind == contract.InlineSoftBreak || in[0].Kind == contract.InlineLineBreak) {
		in[0].Range.Start = max(in[0].Range.Start, end)
		in[0].Kind = contract.InlineSoftBreak
	}
	return in
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
