package liveedit

import (
	"hash/fnv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"aurink.local/spikes/contract-rc/contract"
	"github.com/egoist/mygo/ui/markdown/parser"
)

// project builds the display of block i: its segments cover the block's range
// in source order without gaps (04 §4.3.1). In a revealed block markers show;
// otherwise they are hidden.
//
// Display strings point into one private copy of the block's bytes, not into the
// source: a retained projection must not pin a whole source snapshot.
func project(d *document, i int, revealed bool) contract.BlockProjection {
	b := &d.blocks[i]
	src := d.src()
	pr := segBuilder{base: b.Start, own: strings.Clone(src[b.Start:b.End]), at: b.Start, revealed: revealed, limit: b.CEnd}
	if b.Node >= 0 && (b.Kind == contract.BlockParagraph || b.Kind == contract.BlockHeading) {
		in := d.p.Inlines(b.Node)
		if b.Kind == contract.BlockHeading {
			first := b.CEnd
			if len(in) > 0 {
				first = min(in[0].Start, b.CEnd)
			}
			pr.marker(first, 0)
			pr.inlines(in, 0, len(in), 0, src)
			pr.marker(b.CEnd, 0) // closing #s, or all of an empty heading
		} else {
			pr.inlines(in, 0, len(in), 0, src)
		}
	}
	pr.text(b.CEnd, 0)
	if b.End > pr.at || len(pr.segs) == 0 {
		pr.segs = append(pr.segs, contract.Segment{Src: contract.Range{Start: pr.at, End: b.End}, Kind: contract.SegHidden})
	}
	p := contract.BlockProjection{Block: b.ID, Index: i, Kind: b.Kind, Level: b.Level, Segments: pr.segs, Revealed: revealed}
	h := fnv.New64a()
	for _, s := range p.Segments {
		h.Write([]byte{byte(s.Kind), byte(s.Style), byte(s.Style >> 8)})
		h.Write([]byte(s.Display))
	}
	p.Sig = h.Sum64()
	return p
}

type segBuilder struct {
	base, at, limit int
	own             string
	revealed        bool
	segs            []contract.Segment
}

func (b *segBuilder) sub(from, to int) string { return b.own[from-b.base : to-b.base] }

func (b *segBuilder) emit(end int, kind contract.SegKind, display string, st contract.Style) {
	b.segs = append(b.segs, contract.Segment{Src: contract.Range{Start: b.at, End: end}, Kind: kind, Display: display, Style: st})
	b.at = end
}

// text emits the source up to end as text, CRLF as a one-unit soft break.
func (b *segBuilder) text(end int, st contract.Style) {
	for b.at < end {
		j := strings.IndexByte(b.sub(b.at, end), '\r')
		if j < 0 {
			b.emit(end, contract.SegText, b.sub(b.at, end), st)
			return
		}
		if j > 0 {
			b.emit(b.at+j, contract.SegText, b.sub(b.at, b.at+j), st)
		}
		n := 1
		if b.at+1 < end && b.own[b.at+1-b.base] == '\n' {
			n = 2
		}
		b.emit(b.at+n, contract.SegReplaced, "\n", st)
	}
}

// marker emits the source up to end as a syntax marker.
func (b *segBuilder) marker(end int, st contract.Style) {
	if end <= b.at {
		return
	}
	if b.revealed {
		b.emit(end, contract.SegMarker, b.sub(b.at, end), st|contract.StyleMarker)
	} else {
		b.emit(end, contract.SegHidden, "", st)
	}
}

func (b *segBuilder) inlines(in []parser.Inline, from, to int, st contract.Style, src string) {
	for j := from; j < to; j = max(in[j].Next, j+1) {
		n := &in[j]
		if n.Start < b.at || n.End > b.limit {
			continue // past what the block shows: left to the filler text
		}
		b.text(n.Start, st)
		switch n.Kind {
		case parser.Emphasis, parser.Strong, parser.Strikethrough:
			inner := st | styleOf(n.Kind)
			first, last := n.End, n.End
			if n.Next > j+1 {
				first, last = in[j+1].Start, in[lastTop(in, j+1, n.Next)].End
			}
			b.marker(first, inner)
			b.inlines(in, j+1, n.Next, inner, src)
			b.text(last, inner)
			b.marker(n.End, inner)
		case parser.Code:
			ticks := 0
			for n.Start+ticks < n.End && src[n.Start+ticks] == '`' {
				ticks++
			}
			b.marker(n.Start+ticks, st|contract.StyleCode)
			b.text(max(n.End-ticks, b.at), st|contract.StyleCode)
			b.marker(n.End, st|contract.StyleCode)
		default:
			// Everything else stays as its source: lossless, out of S1's scope.
			b.text(n.End, st)
		}
	}
}

// lastTop returns the index of the last inline of [from, to) at that level.
func lastTop(in []parser.Inline, from, to int) int {
	k := from
	for j := from; j < to; j = max(in[j].Next, j+1) {
		k = j
	}
	return k
}

func styleOf(k parser.InlineKind) contract.Style {
	switch k {
	case parser.Emphasis:
		return contract.StyleEmphasis
	case parser.Strong:
		return contract.StyleStrong
	}
	return contract.StyleStrike
}

// rebase shifts a cached projection after edits earlier in the document, which
// leave its text alone.
func rebase(p *contract.BlockProjection, delta int) {
	if delta == 0 {
		return
	}
	for i := range p.Segments {
		p.Segments[i].Src.Start += delta
		p.Segments[i].Src.End += delta
	}
}

// displayText joins the segments' display.
func displayText(p *contract.BlockProjection) string {
	var s strings.Builder
	for i := range p.Segments {
		s.WriteString(p.Segments[i].Display)
	}
	return s.String()
}

func utf16Len(s string) int {
	n := 0
	for _, r := range s {
		n += utf16.RuneLen(r)
	}
	return n
}

// utf16ByteOffset returns the byte of s at UTF-16 unit u, keeping a surrogate
// pair whole.
func utf16ByteOffset(s string, u int) int {
	n := 0
	for i := 0; i < len(s); {
		if n >= u {
			return i
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		n += utf16.RuneLen(r)
		i += size
	}
	return len(s)
}

// transparent reports whether a segment's bytes map one to one to its display.
func transparent(s *contract.Segment) bool {
	return s.Kind == contract.SegText || s.Kind == contract.SegMarker
}

// toDisplay maps a source offset to the display's UTF-16 index. A hidden or
// replaced segment is atomic: an offset inside it goes to the edge aff says.
func toDisplay(p *contract.BlockProjection, src int, aff contract.Affinity) int {
	u := 0
	for i := range p.Segments {
		s := &p.Segments[i]
		if src >= s.Src.End {
			u += utf16Len(s.Display)
			continue
		}
		if src <= s.Src.Start {
			return u
		}
		if transparent(s) {
			k := src - s.Src.Start
			for k > 0 && !utf8.RuneStart(s.Display[k]) {
				k-- // an offset inside a rune is at its start
			}
			return u + utf16Len(s.Display[:k])
		}
		if aff == contract.Downstream {
			u += utf16Len(s.Display)
		}
		return u
	}
	return u
}

// toSource maps a display index back to a source offset. At an edge beside a
// hidden run, Upstream stays before the run and Downstream goes past it; the
// trailing run, the block's terminator and separator, is no caret position.
func toSource(p *contract.BlockProjection, disp int, aff contract.Affinity) int {
	u := 0
	prevEnd := p.Segments[0].Src.Start
	for i := range p.Segments {
		s := &p.Segments[i]
		n := utf16Len(s.Display)
		if n == 0 {
			continue
		}
		if disp <= u {
			if aff == contract.Upstream {
				return prevEnd
			}
			return s.Src.Start
		}
		if disp < u+n {
			if transparent(s) {
				return s.Src.Start + utf16ByteOffset(s.Display, disp-u)
			}
			return s.Src.End
		}
		u += n
		prevEnd = s.Src.End
	}
	return prevEnd
}

// snap moves a source offset to a legal caret position of the block.
func snap(p *contract.BlockProjection, src int, aff contract.Affinity) int {
	return toSource(p, toDisplay(p, src, aff), aff)
}
