package markdown

import (
	"strconv"
	"strings"

	"aurink/internal/contract"
	"github.com/egoist/mygo/ui/markdown/parser"
)

// coverQuoteEnds gives a trailing marker-only line its own leaf. The parser
// omits such a line from its quote's range, while cover normally absorbs it
// into the preceding leaf's trailer as a separator.
func (a *adapted) coverQuoteEnds(c *coverage) {
	a.coverQuoteDefinitions(c)
	var leaves []coverLeaf
	lastParent := 0
	var previousLine string
	previousNext, previousParent := -1, 0
	quoteCache := make(map[string]int)
	for i := 0; i < len(c.leaves); i++ {
		l := c.leaves[i]
		if l.origin > 0 || l.parent > 0 && (l.Kind == kindBlank || l.Kind == kindLinkDef) {
			lastParent = l.parent
			previousNext, previousParent = -1, 0
			_, start := lineEnd(a.source, l.Range.End)
			end, next := lineEnd(a.source, start)
			if start < end && end <= l.Trailer.End {
				line := a.source[start:end]
				if strings.Contains(line, ">") && separatorRe.MatchString(line) {
					if parent := a.trailingQuote(lastParent, start, end, quoteCache); parent > 0 && !a.quoteDefinitionFollows(c.leaves, i, parent) {
						previousLine, previousNext, previousParent = line, next, parent
						tail := l.Trailer.End
						l.Trailer.End = start
						point := contract.Range{Start: end, End: end}
						leaves = append(leaves, l, coverLeaf{Kind: kindBlank, parent: parent,
							Lead: contract.Range{Start: start, End: end}, Range: point,
							Trailer: contract.Range{Start: end, End: tail}})
						// cover already appended this EOF row for the absorbed
						// separator; the new leaf now owns the same position.
						if end == len(a.source) && i+1 < len(c.leaves) {
							next := c.leaves[i+1]
							if next.origin == 0 && next.parent == 0 && next.Kind == kindBlank && next.Lead == point && next.Range == point && next.Trailer == point {
								i++
							}
						}
						continue
					}
				}
			}
		} else if l.parent == 0 && l.Kind == kindBlank && lastParent > 0 {
			// Later marker-only lines in the same run already have coverage
			// leaves, but the parser's quote envelope excludes them too.
			start := l.Lead.Start
			end, next := lineEnd(a.source, start)
			if start < end && end == l.Range.End {
				line := a.source[start:end]
				if strings.Contains(line, ">") && separatorRe.MatchString(line) {
					parent := 0
					if previousNext == start && previousLine == line {
						parent = previousParent
					} else {
						parent = a.trailingQuote(lastParent, start, end, nil)
					}
					if parent > 0 {
						l.parent = parent
						l.Lead.End = end
						l.Range.Start = end
						previousLine, previousNext, previousParent = line, next, parent
					} else {
						previousNext, previousParent = -1, 0
					}
				} else {
					previousNext, previousParent = -1, 0
				}
			} else {
				previousNext, previousParent = -1, 0
			}
		}
		leaves = append(leaves, l)
	}
	c.leaves = leaves
}

// Definitions do not have parser blocks. Recover their quote parent before
// handling adjacent marker lines, and split a marker that coverage included
// in an otherwise empty quote's definition range.
func (a *adapted) coverQuoteDefinitions(c *coverage) {
	var leaves []coverLeaf
	lastParent := 0
	for _, l := range c.leaves {
		if l.origin > 0 {
			lastParent = l.parent
		}
		if l.Kind != kindLinkDef || l.origin != 0 {
			leaves = append(leaves, l)
			continue
		}
		if l.parent == 0 && lastParent > 0 {
			start := l.Lead.Start
			end, _ := lineEnd(a.source, start)
			prefix := start
			for prefix < end && strings.ContainsRune(" \t>", rune(a.source[prefix])) {
				prefix++
			}
			if strings.Contains(a.source[start:prefix], ">") {
				l.parent = a.trailingQuote(lastParent, start, prefix, nil)
			}
		}
		split := false
		if l.parent > 0 {
			lastParent = l.parent
			start, previousEnd := l.Range.Start, -1
			for start < l.Range.End {
				end, next := lineEnd(a.source, start)
				if next >= l.Range.End {
					if previousEnd >= 0 && end == l.Range.End {
						line := a.source[start:end]
						if strings.Contains(line, ">") && separatorRe.MatchString(line) {
							if parent := a.trailingQuote(l.parent, start, end, nil); parent > 0 {
								tail := l.Trailer.End
								l.Range.End = previousEnd
								l.Trailer = contract.Range{Start: previousEnd, End: start}
								point := contract.Range{Start: end, End: end}
								leaves = append(leaves, l, coverLeaf{Kind: kindBlank, parent: parent,
									Lead: contract.Range{Start: start, End: end}, Range: point,
									Trailer: contract.Range{Start: end, End: tail}})
								split = true
							}
						}
					}
					break
				}
				previousEnd, start = end, next
			}
		}
		if !split {
			leaves = append(leaves, l)
		}
	}
	c.leaves = leaves
}

func (a *adapted) quoteDefinitionFollows(leaves []coverLeaf, i, quote int) bool {
	if i+1 >= len(leaves) || leaves[i+1].Kind != kindLinkDef {
		return false
	}
	for parent := leaves[i+1].parent; parent > 0; parent = a.blocks[parent].Parent {
		if parent == quote {
			return true
		}
	}
	return false
}

// trailingQuote checks the parser's own continuation rules, including list
// indentation and tabs. A temporary heading makes the marker line visible in
// the parser's ranges without allowing lazy paragraph continuation; it is
// never added to the document or its snapshot.
func (a *adapted) trailingQuote(ancestor, lineStart, lineStop int, cache map[string]int) int {
	top, quoteDepth, onlyQuotes, hasTrailingQuote := ancestor, 0, true, false
	for i := ancestor; i > 0; i = a.blocks[i].Parent {
		top = i
		b := a.blocks[i]
		if b.Kind == contract.BlockQuote {
			quoteDepth++
			if b.Range.End <= lineStart {
				hasTrailingQuote = true
			}
		} else {
			onlyQuotes = false
		}
	}
	if !hasTrailingQuote {
		return 0
	}
	if onlyQuotes {
		// With no intervening list or footnote, the marker line alone
		// determines how many existing quote prefixes continue. Avoid
		// repeatedly reparsing a large outer quote with many nested quotes.
		probe := strings.TrimRight(a.source[lineStart:lineStop], " \t") + "# x"
		d, input := parserDocument(probe, a.doc.Extensions())
		depth := 0
		for i := 1; i < d.Len(); i++ {
			b := d.Block(i)
			if b.Kind == parser.Quote && input.offset(b.End) == len(probe) {
				depth++
			}
		}
		if depth == 0 || depth > quoteDepth {
			return 0
		}
		for parent, skip := ancestor, quoteDepth-depth; parent > 0; parent = a.blocks[parent].Parent {
			if skip > 0 {
				skip--
				continue
			}
			if a.blocks[parent].Range.End <= lineStart {
				return parent
			}
			return 0
		}
		return 0
	}
	// Within the same list items, the same container path and marker line
	// match the same ancestor position. Keep the parser's result for later
	// quotes so a long list does not repeatedly reparse its whole prefix.
	key := ""
	if cache != nil {
		var path strings.Builder
		path.WriteString(a.source[lineStart:lineStop])
		path.WriteByte(0)
		for i := ancestor; i > 0; i = a.blocks[i].Parent {
			path.WriteByte(byte(a.blocks[i].Kind))
			if a.blocks[i].Kind != contract.BlockQuote {
				path.WriteString(strconv.Itoa(i))
				path.WriteByte(0)
			}
		}
		key = path.String()
		if distance := cache[key]; distance > 0 {
			parent := ancestor
			for distance--; distance > 0; distance-- {
				parent = a.blocks[parent].Parent
			}
			if a.blocks[parent].Range.End <= lineStart {
				return parent
			}
			return 0
		}
	}
	if a.blocks[top].Kind == contract.BlockList {
		// A top-level item's own opening marker is sufficient context.
		for i := ancestor; i > 0; i = a.blocks[i].Parent {
			if a.blocks[i].Parent == top {
				top = i
				break
			}
		}
	}
	start := a.blocks[top].Range.Start
	if previous := strings.LastIndexAny(a.source[:start], "\r\n"); previous >= 0 {
		start = previous + 1
	} else {
		start = 0
	}
	probe := strings.TrimRight(a.source[start:lineStop], " \t") + "# x"
	d, input := parserDocument(probe, a.doc.Extensions())
	for i := d.Len() - 1; i > 0; i-- {
		b := d.Block(i)
		if b.Kind != parser.Quote || input.offset(b.End) != len(probe) {
			continue
		}
		quoteStart := start + input.offset(b.Start)
		for parent, distance := ancestor, 1; parent > 0; parent, distance = a.blocks[parent].Parent, distance+1 {
			q := a.blocks[parent]
			if q.Kind == contract.BlockQuote && q.Range.Start == quoteStart && q.Range.End <= lineStart {
				if cache != nil {
					cache[key] = distance
				}
				return parent
			}
		}
		// The innermost quote on the line is new, not an ancestor of leaf.
		return 0
	}
	return 0
}
