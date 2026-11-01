// Package liveedit is the S1 prototype of the Live Markdown engine: a custom
// element with its own TextInputClient over a source string that is the only
// truth. Every edit is a source byte replacement; display text is derived per
// block and never written back.
package liveedit

import (
	"sort"
	"strings"

	"aurink.local/spikes/contract-rc/contract"
	"github.com/egoist/mygo/ui/markdown/parser"
)

// block is a visual row of the editor. Blocks partition the whole source: each
// one starts where the previous one ends, so every byte has an owner.
//
// The parser's blocks leave blank lines to nobody, but a caret must be able to
// stand on an empty line, so the partition adds what contract.AST.Leaves lacks:
// one blank line after a block is its hidden separator, and every further one
// is an empty block of its own.
type block struct {
	ID         contract.BlockID
	Kind       contract.BlockKind
	Level      int
	Start, End int // full range, trailer included
	// CStart and CEnd are the visible content: the range without line
	// terminators and the separator.
	CStart, CEnd int
	// Node is the parser block, or -1 for an empty or unparsed block.
	Node int
}

// document holds the source, its parse and the block partition.
type document struct {
	p      *parser.Document
	blocks []block
	nextID contract.BlockID
	eol    string
}

func newDocument(src string) *document {
	d := &document{p: parser.Parse(src), eol: dominantEOL(src)}
	d.blocks = d.partition(nil)
	for i := range d.blocks {
		d.nextID++
		d.blocks[i].ID = d.nextID
	}
	return d
}

func dominantEOL(src string) string {
	if crlf := strings.Count(src, "\r\n"); crlf > 0 && crlf*2 > strings.Count(src, "\n") {
		return "\r\n"
	}
	return "\n"
}

func (d *document) src() string { return d.p.Source() }

// lineEnd returns the end of the line holding i, without its terminator, and
// the start of the line after it.
func lineEnd(s string, i int) (end, next int) {
	j := strings.IndexByte(s[i:], '\n')
	if j < 0 {
		return len(s), len(s)
	}
	end, next = i+j, i+j+1
	if end > i && s[end-1] == '\r' {
		end--
	}
	return end, next
}

func isBlank(s string) bool { return strings.TrimLeft(s, " \t") == "" }

func trimEOL(s string, start, end int) int {
	for end > start && (s[end-1] == '\n' || s[end-1] == '\r') {
		end--
	}
	return end
}

// partition computes the blocks of the current source; out is reused.
func (d *document) partition(out []block) []block {
	s := d.src()
	out = out[:0]
	pos := 0
	// sep extends next, the start of a line, past that line if it is blank.
	sep := func(next, stop int) int {
		if next < stop {
			if e, after := lineEnd(s, next); after <= stop && isBlank(s[next:e]) {
				return after
			}
		}
		return next
	}
	// gap covers [pos, stop), which no parsed block owns. The block before takes
	// its terminator and one blank line; other lines become blocks of their own.
	gap := func(stop int) {
		if n := len(out); n > 0 && out[n-1].End == pos && pos < stop {
			_, next := lineEnd(s, pos)
			next = sep(min(next, stop), stop)
			out[n-1].End, pos = next, next
		}
		for pos < stop {
			e, next := lineEnd(s, pos)
			b := block{Kind: contract.BlockParagraph, Start: pos, CStart: pos, CEnd: e, Node: -1}
			if !isBlank(s[pos:e]) {
				// A line the parser owns no block for, as a link definition: shown raw.
				b.Kind = contract.BlockHTML
				for next < stop {
					e2, n2 := lineEnd(s, next)
					if isBlank(s[next:e2]) {
						break
					}
					b.CEnd, next = e2, n2
				}
			}
			next = sep(min(next, stop), stop)
			b.End = next
			out = append(out, b)
			pos = next
		}
	}
	for i := range d.p.Children(0) {
		pb := d.p.Block(i)
		start := pb.Start
		for start > pos && s[start-1] != '\n' {
			start-- // indentation belongs to the block
		}
		gap(start)
		b := block{Kind: contract.BlockHTML, Start: start, CStart: start, CEnd: trimEOL(s, start, pb.End), Node: i}
		switch pb.Kind {
		case parser.Paragraph:
			b.Kind = contract.BlockParagraph
		case parser.Heading:
			b.Kind, b.Level = contract.BlockHeading, pb.Level
		}
		b.End = b.CEnd
		out = append(out, b)
		pos = b.End
	}
	gap(len(s))
	// A separator at the very end leaves a line to type on.
	if n := len(out); n == 0 {
		out = append(out, block{Kind: contract.BlockParagraph, Node: -1})
	} else if l := out[n-1]; l.End == len(s) && strings.Count(s[l.CEnd:l.End], "\n") >= 2 {
		out = append(out, block{Kind: contract.BlockParagraph, Start: len(s), CStart: len(s), CEnd: len(s), End: len(s), Node: -1})
	}
	return out
}

// remapped says what became of an old block after an edit: its new index, -1
// when gone, and whether the edit touched it, so that its display is stale.
type remapped struct {
	to      int
	touched bool
}

// replace applies a byte replacement. A block untouched by [from, to) keeps its
// ID; one touched keeps it when its kind and start survive, which is typing
// (04 §4.2.6).
func (d *document) replace(from, to int, text string) []remapped {
	old := d.blocks
	d.p.Edit(from, to, text)
	d.blocks = d.partition(make([]block, 0, len(old)+2))
	delta := len(text) - (to - from)
	remap := make([]remapped, len(old))
	n := 0
	for i := range old {
		o := &old[i]
		start, touched := o.Start, false
		switch {
		case o.End < from || (o.End == from && from != to):
		case o.Start > to || (o.Start == to && from != to):
			start += delta
		default:
			touched = true
		}
		remap[i] = remapped{to: -1, touched: touched}
		for n < len(d.blocks) && d.blocks[n].Start < start {
			n++
		}
		if n < len(d.blocks) && d.blocks[n].Start == start && d.blocks[n].Kind == o.Kind && d.blocks[n].ID == 0 {
			d.blocks[n].ID = o.ID
			remap[i].to = n
		}
	}
	for i := range d.blocks {
		if d.blocks[i].ID == 0 {
			d.nextID++
			d.blocks[i].ID = d.nextID
		}
	}
	return remap
}

// blockAt returns the block owning source offset off.
func (d *document) blockAt(off int) int {
	i := sort.Search(len(d.blocks), func(i int) bool { return d.blocks[i].End > off })
	return min(i, len(d.blocks)-1)
}
