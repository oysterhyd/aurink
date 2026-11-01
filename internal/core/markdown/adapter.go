package markdown

import (
	"fmt"
	"strings"

	"aurink/internal/contract"
	"github.com/egoist/mygo/ui/markdown/parser"
)

type adapted struct {
	source string
	blocks []contract.Block
	doc    *parser.Document
	bodyAt int
}

func parseAdapted(src string, ext parser.Extensions, frontMatter bool) (*adapted, error) {
	at, _ := front(src, frontMatter)
	d := parser.New(ext)
	d.Set(src[at:])
	return adapt(src, d, at)
}

// front recognizes exact delimiter lines, including CRLF and lone CR. YAML
// validation is deliberately separate from detecting the source envelope.
func front(src string, enabled bool) (int, contract.Range) {
	if !enabled || src == "" {
		return 0, contract.Range{}
	}
	end, next := lineEnd(src, 0)
	if src[:end] != "---" || next == end {
		return 0, contract.Range{}
	}
	contentStart := next
	for next < len(src) {
		start := next
		end, next = lineEnd(src, start)
		if s := src[start:end]; s == "---" || s == "..." {
			return next, contract.Range{Start: contentStart, End: start}
		}
	}
	return 0, contract.Range{}
}

func adapt(src string, d *parser.Document, at int) (*adapted, error) {
	s := &adapted{source: src, doc: d, bodyAt: at}
	shift := 0
	if at > 0 {
		shift = 1
	}
	s.blocks = make([]contract.Block, d.Len()+shift)
	s.blocks[0] = contract.Block{Kind: contract.BlockRoot, Parent: -1, Next: len(s.blocks), Range: contract.Range{End: len(src)}, Content: contract.Range{End: len(src)}}
	if at > 0 {
		_, content := front(src, true)
		s.blocks[1] = contract.Block{Kind: contract.BlockFrontMatter, Parent: 0, Next: 2,
			Range: contract.Range{End: at}, Content: content,
			Markers: []contract.Range{{End: content.Start}, {Start: content.End, End: at}}}
		for start := content.Start; start < content.End; {
			end, next := lineEnd(src, start)
			s.blocks[1].Lines = append(s.blocks[1].Lines, contract.BlockLine{Source: contract.Range{Start: start, End: min(end, content.End)}})
			start = next
		}
	}
	for i := 1; i < d.Len(); i++ {
		p := d.Block(i)
		j, parent := i+shift, p.Parent
		if parent > 0 {
			parent += shift
		}
		b := contract.Block{Kind: contract.BlockKind(p.Kind), Parent: parent, Next: p.Next + shift,
			Range: contract.Range{Start: p.Start + at, End: p.End + at}, Level: p.Level,
			Ordered: p.Ordered, Number: p.Number, Marker: p.Marker, Tight: p.Tight,
			Task: contract.TaskState(p.Task), Fenced: p.Fenced, Header: p.Header, Open: p.Open,
			Info: strings.Clone(p.Info()), Lang: strings.Clone(p.Language()), Label: strings.Clone(p.Label())}
		for _, a := range p.Align {
			b.Align = append(b.Align, contract.Align(a))
		}
		for k := 0; k < d.LineCount(i); k++ {
			text, start := d.Line(i, k)
			start += at
			// Line's offset names the first SOURCE byte, not the synthesized
			// spaces. Recover the residual tab width (0..3), without unsafe or
			// depending on private seg fields. Verify every byte at that offset.
			pad := -1
			for n := 0; n <= 3 && n <= len(text); n++ {
				end := start + len(text) - n
				if strings.Trim(text[:n], " ") == "" && end <= len(src) && src[start:end] == text[n:] {
					pad = n
					break
				}
			}
			if pad < 0 {
				return nil, fmt.Errorf("block %d line %d cannot map %q at %d", i, k, text, start)
			}
			b.Lines = append(b.Lines, contract.BlockLine{Source: contract.Range{Start: start, End: start + len(text) - pad}, Padding: uint8(pad)})
		}
		if len(b.Lines) > 0 {
			ls := b.Lines
			b.Content = contract.Range{Start: ls[0].Source.Start, End: ls[len(ls)-1].Source.End}
			b.Range.Start = min(b.Range.Start, b.Content.Start)
			b.Range.End = max(b.Range.End, b.Content.End)
		} else {
			b.Content = contract.Range{Start: b.Range.Start, End: b.Range.Start}
		}
		// finalize overwrites a setext heading's End with its last content
		// segment. Restore the underline; likewise math's closing delimiter.
		if p.Kind == parser.Heading {
			firstEnd, _ := lineEnd(src, b.Range.Start)
			first := src[b.Range.Start:firstEnd]
			n := len(first) - len(strings.TrimLeft(first, "#"))
			atx := n > 0 && n <= 6 && (n == len(first) || first[n] == ' ' || first[n] == '\t')
			if atx {
				b.Range.End = firstEnd
			} else {
				_, next := lineEnd(src, b.Range.End)
				if next < len(src) {
					end, _ := lineEnd(src, next)
					b.Range.End = end
				}
			}
		}
		if p.Kind == parser.MathBlock && len(b.Lines) > 0 {
			b.Range.End = mathEnd(src, b.Range.Start, b.Range.End)
		}
		if isLeaf(b.Kind) {
			fillGaps(src, &b, b.Lines)
		}
		if b.Kind == contract.BlockItem && b.Task != contract.TaskNone {
			end, _ := lineEnd(src, b.Range.Start)
			if n := strings.IndexByte(src[b.Range.Start:end], '['); n >= 0 {
				b.TaskMarker = contract.Range{Start: b.Range.Start + n, End: b.Range.Start + n + 3}
			}
		}
		s.blocks[j] = b
	}
	// Correct container envelopes after restoring leaf delimiters.
	for i := len(s.blocks) - 1; i > 0; i-- {
		b := s.blocks[i]
		p := &s.blocks[b.Parent]
		p.Range.End = max(p.Range.End, b.Range.End)
	}
	return s, nil
}

func isLeaf(k contract.BlockKind) bool {
	switch k {
	case contract.BlockBlank, contract.BlockLinkDef, contract.BlockParagraph, contract.BlockHeading, contract.BlockThematicBreak,
		contract.BlockCode, contract.BlockHTML, contract.BlockMath, contract.BlockTableCell,
		contract.BlockFrontMatter, contract.BlockTOC:
		return true
	}
	return false
}

func lineEnd(src string, start int) (end, next int) {
	end = start
	for end < len(src) && src[end] != '\r' && src[end] != '\n' {
		end++
	}
	next = end
	if next < len(src) {
		next++
		if src[end] == '\r' && next < len(src) && src[next] == '\n' {
			next++
		}
	}
	return
}

func mathEnd(src string, start, end int) int {
	close := "$$"
	if strings.HasPrefix(src[start:], `\[`) {
		close = `\]`
	}
	// The parser may place End immediately before a same-line closer.
	lineLast, next := lineEnd(src, end)
	if strings.HasSuffix(strings.TrimRight(src[end:lineLast], " \t"), close) {
		return lineLast
	}
	if next < len(src) {
		e, _ := lineEnd(src, next)
		// Only a delimiter-only line may be outside the parser's End.
		if strings.Trim(src[next:e], " \t>") == close {
			return e
		}
	}
	return end
}

// fillGaps derives deletion spans from the parser's PUBLIC line offsets.
// Complete marker-only lines belong to the block; first-line ancestor markers
// precede Range.Start and belong to the parent. EOLs are left as source bytes.
func fillGaps(src string, b *contract.Block, lines []contract.BlockLine) {
	k := 0
	for start := b.Range.Start; start < b.Range.End; {
		end, next := lineEnd(src, start)
		end = min(end, b.Range.End)
		if k < len(lines) && lines[k].Source.Start <= end {
			l := lines[k].Source
			if start < l.Start {
				r := contract.Range{Start: start, End: l.Start}
				if start == b.Range.Start && (b.Kind == contract.BlockHeading || b.Kind == contract.BlockMath) {
					b.Markers = append(b.Markers, r)
				} else {
					b.Prefixes = append(b.Prefixes, r)
				}
			}
			if l.End < end {
				b.Markers = append(b.Markers, contract.Range{Start: l.End, End: end})
			}
			k++
		} else {
			marker := start
			if start != b.Range.Start {
				needle := ""
				switch b.Kind {
				case contract.BlockHeading:
					needle = "-"
					if b.Level == 1 {
						needle = "="
					}
				case contract.BlockCode:
					if b.Fenced {
						needle = src[b.Range.Start : b.Range.Start+1]
					}
				case contract.BlockMath:
					needle = "$$"
					if strings.HasPrefix(src[b.Range.Start:], `\[`) {
						needle = `\]`
					}
				}
				if needle != "" {
					if n := strings.Index(src[start:end], needle); n > 0 {
						marker += n
					}
				}
			}
			if marker > start {
				b.Prefixes = append(b.Prefixes, contract.Range{Start: start, End: marker})
			}
			b.Markers = append(b.Markers, contract.Range{Start: marker, End: end})
		}
		if next <= start {
			break
		}
		start = next
	}
}
