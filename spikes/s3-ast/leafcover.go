package astspike

import (
	"regexp"
	"strings"

	"aurink.local/spikes/contract-rc/contract"
)

// leaf kinds that contract-rc does not have yet (CR-001).
const (
	kindBlank   contract.BlockKind = 200
	kindLinkDef contract.BlockKind = 201
)

// coverLeaf is one leaf of the coverage rules of 04 §4.2.8 and §4.2.9.
type coverLeaf struct {
	Kind                 contract.BlockKind
	Lead, Range, Trailer contract.Range
}

var (
	separatorRe = regexp.MustCompile(`^[ \t>]*$`)
	footnoteRe  = regexp.MustCompile(`\[\^[^\]]+\]:`)
	// structuralRe is what a Lead or Trailer may hold: container and table
	// syntax, never document text.
	structuralRe = regexp.MustCompile(`^[ \t\r\n>|:\-+*.)=#~` + "`" + `0-9\[\]xX]*$`)
)

type coverage struct {
	src    string
	leaves []coverLeaf
	// overlaps counts parser leaves that start before the previous one ends.
	overlaps int
}

// cover assigns every source byte to a leaf.
func (s *snapshot) cover() *coverage {
	c := &coverage{src: s.source}
	pos := 0
	for _, it := range s.coverItems() {
		start := it.Range.Start
		if start < pos {
			c.overlaps++
			start = pos
			it.Range.Start = pos
			it.Range.End = max(it.Range.End, pos)
		}
		it.Lead = c.gap(pos, start, true)
		it.Range.Start = start
		if lead := s.source[it.Lead.Start:it.Lead.End]; it.Kind == kindBlank && !structuralRe.MatchString(lead) {
			// A container that holds only a link definition.
			it.Kind = kindLinkDef
			it.Lead.End = it.Lead.Start + len(lead) - len(strings.TrimLeft(lead, " 	>"))
			it.Range.Start = it.Lead.End
		}
		c.leaves = append(c.leaves, it)
		pos = it.Range.End
	}
	c.gap(pos, len(s.source), false)
	n := len(c.leaves)
	switch {
	case n == 0:
		c.leaves = append(c.leaves, coverLeaf{Kind: kindBlank})
	case c.endsWithSeparator(c.leaves[n-1].Trailer):
		end := contract.Range{Start: len(s.source), End: len(s.source)}
		c.leaves = append(c.leaves, coverLeaf{Kind: kindBlank, Lead: end, Range: end, Trailer: end})
	}
	return c
}

// coverItems lists the parser's leaves and one blank per empty container.
func (s *snapshot) coverItems() []coverLeaf {
	var items []coverLeaf
	for i := 1; i < len(s.blocks); i++ {
		b := s.blocks[i]
		switch {
		case isLeaf(b.Kind):
			items = append(items, coverLeaf{Kind: b.Kind, Range: b.Range})
		case b.Next == i+1 && (b.Kind == contract.BlockQuote || b.Kind == contract.BlockItem || b.Kind == contract.BlockFootnoteDef):
			end := contract.Range{Start: b.Range.End, End: b.Range.End}
			items = append(items, coverLeaf{Kind: kindBlank, Range: end})
		}
	}
	return items
}

// gap splits [from, to): the previous leaf's Trailer, then blank and link
// definition leaves, then the Lead of the next item, which it returns.
func (c *coverage) gap(from, to int, hasNext bool) contract.Range {
	p := from
	if len(c.leaves) > 0 {
		last := &c.leaves[len(c.leaves)-1]
		end, next := lineEnd(c.src, p)
		if next > to || next == end {
			if hasNext {
				last.Trailer = contract.Range{Start: from, End: from}
				return contract.Range{Start: from, End: to}
			}
			last.Trailer = contract.Range{Start: from, End: to}
			return contract.Range{Start: to, End: to}
		}
		p = c.absorbSeparator(next, to, hasNext)
		last.Trailer = contract.Range{Start: from, End: p}
	}
	for p < to {
		end, next := lineEnd(c.src, p)
		if next > to || (next == end && hasNext) {
			break
		}
		line := c.src[p:end]
		switch {
		case separatorRe.MatchString(line):
			at := p + strings.LastIndexByte(line, '>') + 1
			c.leaves = append(c.leaves, coverLeaf{Kind: kindBlank,
				Lead:    contract.Range{Start: p, End: at},
				Range:   contract.Range{Start: at, End: end},
				Trailer: contract.Range{Start: end, End: next}})
			p = next
		case c.isLinkDef(p, to):
			rangeEnd := end
			for next < to {
				e, n := lineEnd(c.src, next)
				if n > to || separatorRe.MatchString(c.src[next:e]) {
					break
				}
				rangeEnd, next = e, n
			}
			after := c.absorbSeparator(next, to, hasNext)
			c.leaves = append(c.leaves, coverLeaf{Kind: kindLinkDef,
				Lead:    contract.Range{Start: p, End: p},
				Range:   contract.Range{Start: p, End: rangeEnd},
				Trailer: contract.Range{Start: rangeEnd, End: after}})
			p = after
		default:
			if !hasNext {
				// Whole lines without a block at the end are a definition.
				c.leaves = append(c.leaves, coverLeaf{Kind: kindLinkDef,
					Lead:    contract.Range{Start: p, End: p},
					Range:   contract.Range{Start: p, End: to},
					Trailer: contract.Range{Start: to, End: to}})
				return contract.Range{Start: to, End: to}
			}
			return contract.Range{Start: p, End: to}
		}
	}
	return contract.Range{Start: p, End: to}
}

// isLinkDef reports whether the whole lines at p hold document text. The parser
// makes no block for link definitions, so they are the only text left in a gap.
func (c *coverage) isLinkDef(p, to int) bool {
	for p < to {
		end, next := lineEnd(c.src, p)
		line := c.src[p:end]
		if next > to || separatorRe.MatchString(line) {
			return false
		}
		if line = footnoteRe.ReplaceAllString(line, ""); !structuralRe.MatchString(line) {
			return true
		}
		if next == end {
			return false
		}
		p = next
	}
	return false
}

// absorbSeparator returns the offset after at most one separator line at p.
func (c *coverage) absorbSeparator(p, to int, hasNext bool) int {
	if p >= to {
		return p
	}
	end, next := lineEnd(c.src, p)
	if next > to || (next == end && hasNext) || !separatorRe.MatchString(c.src[p:end]) {
		return p
	}
	return next
}

// endsWithSeparator reports whether r ends with a separator line after its
// first line terminator.
func (c *coverage) endsWithSeparator(r contract.Range) bool {
	_, next := lineEnd(c.src, r.Start)
	if next >= r.End {
		return false
	}
	end, _ := lineEnd(c.src, next)
	return separatorRe.MatchString(c.src[next:min(end, r.End)])
}
