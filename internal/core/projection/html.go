package projection

import (
	"strings"

	"aurink/internal/contract"
)

var htmlStyles = [...]contract.Style{
	contract.StyleStrong, contract.StyleEmphasis, contract.StyleUnderline, contract.StyleStrike,
	contract.StyleKbd, contract.StyleHighlight, contract.StyleSub, contract.StyleSup,
}

type htmlEvent struct {
	tag             int
	paired, opening bool
	show, lineBreak bool
}

// htmlTag accepts only the exact tag spellings supported by the projection.
func htmlTag(s string) (tag int, closing, lineBreak, ok bool) {
	if len(s) < 3 || s[0] != '<' || s[len(s)-1] != '>' {
		return 0, false, false, false
	}
	for i := range len(s) {
		if s[i] >= 0x80 {
			return 0, false, false, false
		}
	}
	s = s[1 : len(s)-1]
	if strings.HasPrefix(s, "/") {
		closing, s = true, s[1:]
	}
	if !closing && (strings.EqualFold(s, "br") || strings.EqualFold(s, "br/") || strings.EqualFold(s, "br /")) {
		return 0, false, true, true
	}
	for i, name := range [...]string{"b", "i", "u", "s", "kbd", "mark", "sub", "sup"} {
		if strings.EqualFold(s, name) {
			return i, closing, false, true
		}
	}
	return 0, false, false, false
}

func htmlInline(b *builder, in []contract.Inline, i int, style contract.Style, link string, _ bool) {
	b.htmlInlines(in, i, len(in), style, link)
}

// htmlInlines is entered only when this sibling list contains HTML. Each
// opening tag takes the next available matching close in that list; descendants
// of a Markdown inline are processed by their own call to inlines.
func (b *builder) htmlInlines(in []contract.Inline, start, end int, style contract.Style, link string) {
	events := make([]htmlEvent, end-start)
	var pending [len(htmlStyles)][]int
	for i := start; i < end; i = max(i+1, in[i].Next) {
		n := in[i]
		if n.Kind != contract.InlineHTML || n.Range.Start < b.at || n.Range.End > b.block.Range.End {
			continue
		}
		tag, closing, lineBreak, ok := htmlTag(b.sub(n.Range.Start, n.Range.End))
		if !ok {
			continue
		}
		if lineBreak {
			events[i-start].lineBreak = true
		} else if !closing {
			pending[tag] = append(pending[tag], i)
		} else if len(pending[tag]) != 0 {
			open := pending[tag][0]
			pending[tag] = pending[tag][1:]
			span := contract.Inline{Range: contract.Range{Start: in[open].Range.Start, End: n.Range.End}}
			show := b.nodeRevealed(span)
			events[open-start] = htmlEvent{tag: tag, paired: true, opening: true, show: show}
			events[i-start] = htmlEvent{tag: tag, paired: true, show: show}
		}
	}

	var counts [len(htmlStyles)]int
	var active contract.Style
	from := start
	for i := start; i < end; i = max(i+1, in[i].Next) {
		n := in[i]
		if n.Kind != contract.InlineHTML || n.Range.Start < b.at || n.Range.End > b.block.Range.End {
			continue
		}
		b.inlines(in, from, i, style|active, link)
		b.raw(n.Range.Start, style|active, link, "\n")
		e := events[i-start]
		switch {
		case e.paired:
			if e.opening {
				b.marker(n.Range.End, style|active|contract.StyleHTML, link, e.show)
				counts[e.tag]++
				active |= htmlStyles[e.tag]
			} else {
				counts[e.tag]--
				if counts[e.tag] == 0 {
					active &^= htmlStyles[e.tag]
				}
				b.marker(n.Range.End, style|active|contract.StyleHTML, link, e.show)
			}
		case e.lineBreak && !b.nodeRevealed(n):
			breakInline(b, in, i, style|active, link, false)
		default:
			b.raw(n.Range.End, style|active|contract.StyleHTML|contract.StyleMarker, link, "\n")
		}
		from = max(i+1, n.Next)
	}
	b.inlines(in, from, end, style|active, link)
}
