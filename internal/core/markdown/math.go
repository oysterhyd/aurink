package markdown

import (
	"sort"
	"strings"

	"github.com/egoist/mygo/ui/markdown/parser"
)

// parserInput records temporary bytes used to guide block parsing. The parser
// sees ordinary inline content there; public ranges and text map back to the
// unchanged source.
type parserInput struct {
	source, text string
	guard        string
	insertions   []parserInsertion
	starts, sums []int
}

type parserInsertion struct {
	at   int
	text string
}

func (p parserInput) offset(at int) int {
	i := sort.SearchInts(p.starts, at)
	if i == 0 {
		return at
	}
	removed := p.sums[i]
	end := p.starts[i-1] + len(p.insertions[i-1].text)
	if at < end {
		removed -= end - at
	}
	return at - removed
}

func (p parserInput) clean(s string) string {
	if p.guard == "" {
		return s
	}
	return strings.ReplaceAll(s, p.guard, "")
}

func makeParserInput(src, guard string, insertions []parserInsertion) parserInput {
	sort.SliceStable(insertions, func(i, j int) bool { return insertions[i].at < insertions[j].at })
	p := parserInput{source: src, guard: guard, insertions: insertions, sums: make([]int, 1, len(insertions)+1)}
	var out strings.Builder
	out.Grow(len(src) + len(insertions))
	last := 0
	for _, insertion := range insertions {
		out.WriteString(src[last:insertion.at])
		p.starts = append(p.starts, out.Len())
		out.WriteString(insertion.text)
		p.sums = append(p.sums, p.sums[len(p.sums)-1]+len(insertion.text))
		last = insertion.at
	}
	out.WriteString(src[last:])
	p.text = out.String()
	return p
}

// The upstream inline parser also pairs $$ across lines. Escape those opening
// dollars in a private reparse so that normal inline syntax inside the span is
// still parsed and a later same-line pair can match on its own line.
func (p parserInput) escapeMultilineMath(in []parser.Inline) (parserInput, bool) {
	insertions := append([]parserInsertion(nil), p.insertions...)
	for _, n := range in {
		if n.Kind != parser.Math {
			continue
		}
		start, end := p.offset(n.Start), p.offset(n.End)
		if start < 0 || end > len(p.source) || end-start < 4 {
			continue
		}
		raw := p.source[start:end]
		if strings.HasPrefix(raw, "$$") && strings.ContainsAny(raw, "\r\n") {
			insertions = append(insertions, parserInsertion{at: start, text: `\`}, parserInsertion{at: start + 1, text: `\`})
			lineLimit, _ := lineEnd(p.source, end)
			if !strings.Contains(p.source[end:lineLimit], "$$") {
				insertions = append(insertions, parserInsertion{at: end - 2, text: `\`}, parserInsertion{at: end - 1, text: `\`})
			}
		}
	}
	if len(insertions) == len(p.insertions) {
		return p, false
	}
	return makeParserInput(p.source, p.guard, insertions), true
}

func parserDocument(src string, ext parser.Extensions) (*parser.Document, parserInput) {
	d := parser.New(ext)
	input := parserInput{source: src, text: src}
	checkMath := ext&parser.ExtDollarMath != 0 && strings.Contains(src, "$$")
	checkTasks := ext&parser.ExtTaskLists != 0 &&
		(strings.Contains(src, "[ ] ") || strings.Contains(src, "[ ]\t") ||
			strings.Contains(src, "[x] ") || strings.Contains(src, "[x]\t") ||
			strings.Contains(src, "[X] ") || strings.Contains(src, "[X]\t"))
	if !checkMath && !checkTasks {
		d.Set(src)
		return d, input
	}
	guard := "\x01"
	for strings.Contains(src, guard) {
		guard += "\x01"
	}
	var positions []int
	for {
		if len(positions) > 0 {
			insertions := make([]parserInsertion, 0, len(positions))
			for _, pos := range positions {
				insertions = append(insertions, parserInsertion{at: pos, text: guard})
			}
			input = makeParserInput(src, guard, insertions)
		}
		d.Set(input.text)
		var added []int
		seen := make(map[int]bool, len(positions))
		for _, pos := range positions {
			seen[pos] = true
		}
		add := func(pos int) {
			if !seen[pos] {
				added = append(added, pos)
				seen[pos] = true
			}
		}
		for i := 1; i < d.Len(); i++ {
			b := d.Block(i)
			if checkTasks && b.Kind == parser.Item && b.Task == parser.NoTask && i+1 < b.Next {
				first := i + 1
				// A delimiter on the next line may have turned the first
				// paragraph into the header cell of a table.
				if d.Block(first).Kind == parser.Table && first+2 < b.Next && d.Block(first+1).Kind == parser.TableRow {
					first += 2
				}
				child := d.Block(first)
				if (child.Kind == parser.Paragraph || child.Kind == parser.Heading || child.Kind == parser.TableCell) && d.LineCount(first) > 0 {
					line, at := d.Line(first, 0)
					start := input.offset(at)
					firstEnd, _ := lineEnd(src, input.offset(b.Start))
					if strings.HasPrefix(line, "[") && start < firstEnd {
						if _, ok := emptyTaskLine(src, start); ok {
							// Force the upstream parser to keep the item's normal
							// continuation grammar; the byte is never exposed.
							add(start + 4)
						}
					}
				}
			}
			if checkMath && b.Kind == parser.MathBlock {
				start := input.offset(b.Start)
				if !strings.HasPrefix(src[start:], "$$") {
					continue
				}
				end, next := lineEnd(src, start)
				if strings.Trim(src[start:end], " \t") != "$$" || next == end {
					add(start)
					// An invalid opener can swallow many following lines. Find
					// further candidate openers in those lines in the same pass.
					for k := 0; k < d.LineCount(i); k++ {
						_, at := d.Line(i, k)
						candidate := input.offset(at)
						for candidate < len(src) && (src[candidate] == ' ' || src[candidate] == '\t') {
							candidate++
						}
						if candidate == start || !strings.HasPrefix(src[candidate:], "$$") {
							continue
						}
						last, after := lineEnd(src, candidate)
						if strings.Trim(src[candidate:last], " \t") != "$$" || after == last {
							add(candidate)
						}
					}
				}
			}
		}
		if len(added) == 0 {
			return d, input
		}
		positions = append(positions, added...)
		sort.Ints(positions)
	}
}
