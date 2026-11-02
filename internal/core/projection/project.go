package projection

import (
	"slices"
	"strings"

	"aurink/internal/contract"
)

// Projector is a stateless M1 projector. Its zero value is ready for use.
type Projector struct{}

// New returns a projector with no mutable state or retained snapshots.
func New() *Projector { return &Projector{} }

var _ contract.Projector = (*Projector)(nil)

type blockRule struct {
	body      func(*builder, contract.AST, int, contract.Style)
	style     contract.Style
	markers   bool
	fences    bool
	lineStart bool
	container bool
	muted     bool
	decorate  func(contract.AST, int, *contract.ContainerDecor)
}

var blockRules = map[contract.BlockKind]blockRule{
	contract.BlockParagraph:     {body: inlineBlock},
	contract.BlockHeading:       {body: inlineBlock, markers: true, lineStart: true},
	contract.BlockTableCell:     {body: inlineBlock},
	contract.BlockThematicBreak: {markers: true},
	contract.BlockCode:          {style: contract.StyleCode, markers: true, fences: true},
	contract.BlockHTML:          {style: contract.StyleHTML | contract.StyleMarker},
	contract.BlockFrontMatter:   {style: contract.StyleMarker},
	contract.BlockLinkDef:       {style: contract.StyleMarker},
	contract.BlockMath:          {style: contract.StyleMarker},
	contract.BlockTOC:           {},
	contract.BlockBlank:         {},
	contract.BlockQuote:         {container: true},
	contract.BlockItem:          {container: true, decorate: numberItem},
	contract.BlockFootnoteDef:   {container: true, muted: true},
}

type inlineRule struct {
	style contract.Style
	apply func(*builder, []contract.Inline, int, contract.Style, string, bool)
}

var inlineRules map[contract.InlineKind]inlineRule

func init() {
	inlineRules = map[contract.InlineKind]inlineRule{
		contract.InlineText:        {apply: textInline},
		contract.InlineSoftBreak:   {apply: breakInline},
		contract.InlineLineBreak:   {apply: breakInline},
		contract.InlineEmphasis:    {contract.StyleEmphasis, nestedInline},
		contract.InlineStrong:      {contract.StyleStrong, nestedInline},
		contract.InlineStrike:      {contract.StyleStrike, nestedInline},
		contract.InlineLink:        {contract.StyleLink, linkInline},
		contract.InlineCode:        {contract.StyleCode, codeInline},
		contract.InlineImage:       {apply: imageInline},
		contract.InlineEscape:      {apply: replacedInline},
		contract.InlineEntity:      {apply: replacedInline},
		contract.InlineHTML:        {apply: htmlInline},
		contract.InlineFootnoteRef: {apply: footnoteInline},
	}
}

// Project projects a leaf without modifying or retaining its inputs. Extensions
// and diagram objects are intentionally inactive in M1.
func (Projector) Project(ast contract.AST, text contract.Text, leaf int, r contract.Reveal) contract.BlockProjection {
	n := ast.Block(leaf)
	if n == nil || n.Next != leaf+1 {
		return contract.BlockProjection{}
	}
	p := contract.BlockProjection{Block: n.ID, Index: leaf, Kind: n.Kind, Level: n.Level, Lang: strings.Clone(n.Lang)}
	plain := containers(ast, leaf, &p)
	if marker, container := pendingMarker(ast, text, n, p.Containers); !marker.Empty() {
		p.Kind, p.Level = contract.BlockParagraph, 0
		if container {
			p.Containers = p.Containers[:len(p.Containers)-1]
		}
		// Empty containers can put the marker in either Lead or Trailer. The
		// boundary segments make the entire visible marker editable in both cases.
		p.Segments = []contract.Segment{
			{Src: contract.Range{Start: n.Lead.Start, End: marker.Start}, Kind: contract.SegHidden},
			{Src: marker, Kind: contract.SegText, Display: strings.Clone(text.Slice(marker))},
			{Src: contract.Range{Start: marker.End, End: n.Trailer.End}, Kind: contract.SegHidden},
		}
		p.Sig = signature(&p)
		return p
	}
	rule := blockRules[n.Kind]
	if plain {
		rule = blockRule{style: contract.StyleMarker}
	}
	b := builder{ast: ast, block: n, reveal: r, p: &p, base: n.Lead.Start, at: n.Lead.Start,
		own: strings.Clone(text.Slice(contract.Range{Start: n.Lead.Start, End: n.Trailer.End}))}
	// Keep even empty boundary segments: they encode the editable range for the
	// mapper, without retaining a block or adding fields to the contract.
	b.emit(n.Range.Start, contract.SegHidden, "", 0, "", nil)
	show := b.blockRevealed()
	if rule.lineStart && r.Mode == contract.RevealInline {
		show = r.Caret == n.Range.Start || selected(r, n.Range)
	}
	for _, prefix := range n.Prefixes {
		b.masks = append(b.masks, mask{src: prefix, prefix: true})
	}
	if rule.markers {
		if rule.fences && !show && len(n.Markers) > 0 {
			b.masks = append(b.masks, mask{src: contract.Range{Start: n.Range.Start, End: n.Content.Start}},
				mask{src: contract.Range{Start: n.Content.End, End: n.Range.End}})
		} else {
			for _, marker := range n.Markers {
				if rule.lineStart && !show && marker.Start >= n.Content.End {
					marker.Start = n.Content.End
				}
				b.masks = append(b.masks, mask{src: marker, show: show})
			}
		}
	}
	slices.SortFunc(b.masks, func(a, c mask) int {
		if a.src.Start != c.src.Start {
			return a.src.Start - c.src.Start
		}
		return c.src.End - a.src.End
	})
	if len(n.Lines) > 0 && n.Lines[0].Padding > 0 && n.Lines[0].Source.Start == n.Range.Start {
		b.emit(b.at, contract.SegReplaced, strings.Repeat(" ", int(n.Lines[0].Padding)), rule.style, "", nil)
	}
	if rule.body != nil {
		rule.body(&b, ast, leaf, rule.style)
	}
	b.raw(n.Range.End, rule.style, "", "\n")
	b.emit(n.Trailer.End, contract.SegHidden, "", 0, "", nil)
	p.Sig = signature(&p)
	return p
}

// pendingMarker finds only markers belonging to this empty leaf. In particular,
// a setext underline and a container with content on a later line stay intact.
func pendingMarker(ast contract.AST, text contract.Text, n *contract.Block, cs []contract.ContainerDecor) (contract.Range, bool) {
	if n.Kind == contract.BlockHeading {
		s := text.Slice(n.Range)
		if len(s) >= 1 && len(s) <= 6 && strings.Trim(s, "#") == "" {
			return n.Range, false
		}
	}
	if n.Kind != contract.BlockBlank || len(cs) == 0 {
		return contract.Range{}, false
	}
	s := text.Slice(contract.Range{Start: n.Lead.Start, End: n.Trailer.End})
	if end := strings.IndexAny(s, "\r\n"); end >= 0 {
		s = s[:end]
	}
	if len(s) == 0 {
		return contract.Range{}, false
	}
	last, c := len(s)-1, cs[len(cs)-1]
	if !c.First {
		return contract.Range{}, false
	}
	index, ok := ast.Index(c.Block)
	if !ok {
		return contract.Range{}, false
	}
	for i, end := index+1, ast.Block(index).Next; i < end; i++ {
		child := ast.Block(i)
		if child.Next == i+1 && child.Kind != contract.BlockBlank {
			return contract.Range{}, false
		}
	}
	start := last
	switch {
	case c.Kind == contract.BlockQuote && s[last] == '>':
	case c.Kind == contract.BlockItem && !c.Ordered && s[last] == c.Marker:
	case c.Kind == contract.BlockItem && c.Ordered && s[last] == c.Marker:
		for start > 0 && s[start-1] >= '0' && s[start-1] <= '9' {
			start--
		}
		if start == last || last-start > 9 {
			return contract.Range{}, false
		}
	default:
		return contract.Range{}, false
	}
	return contract.Range{Start: n.Lead.Start + start, End: n.Lead.Start + len(s)}, true
}

func inlineBlock(b *builder, ast contract.AST, leaf int, style contract.Style) {
	in := ast.Inlines(leaf)
	b.inlines(in, 0, len(in), style, "")
}

func selected(r contract.Reveal, span contract.Range) bool {
	return !r.Selection.Empty() && r.Selection.Overlaps(span)
}

func (b *builder) blockRevealed() bool {
	return b.reveal.Mode != contract.RevealNone && (b.reveal.Mode == contract.RevealAlways ||
		b.reveal.Caret >= b.block.Lead.Start && b.reveal.Caret <= b.block.Range.End || selected(b.reveal, b.block.Range))
}

func (b *builder) nodeRevealed(n contract.Inline) bool {
	if b.reveal.Mode == contract.RevealNone {
		return false
	}
	return b.reveal.Mode == contract.RevealAlways || selected(b.reveal, n.Range) ||
		b.reveal.Mode == contract.RevealBlock && b.blockRevealed() ||
		b.reveal.Caret >= n.Range.Start && b.reveal.Caret <= n.Range.End
}

func containers(ast contract.AST, leaf int, p *contract.BlockProjection) bool {
	plain := false
	for i := ast.Block(leaf).Parent; i > 0; i = ast.Block(i).Parent {
		n := ast.Block(i)
		rule := blockRules[n.Kind]
		if !rule.container {
			continue
		}
		first := i + 1
		for ast.Block(first).Next != first+1 {
			first++
		}
		c := contract.ContainerDecor{Kind: n.Kind, Block: n.ID, Ordered: n.Ordered, Number: n.Number,
			Marker: n.Marker, Task: n.Task, TaskSrc: n.TaskMarker, First: leaf == first, Label: strings.Clone(n.Label)}
		if rule.decorate != nil {
			rule.decorate(ast, i, &c)
		}
		plain = plain || rule.muted
		p.Containers = append(p.Containers, c)
	}
	slices.Reverse(p.Containers)
	for i := range p.Containers {
		p.Containers[i].Depth = i
	}
	return plain
}

func numberItem(ast contract.AST, i int, c *contract.ContainerDecor) {
	n := ast.Block(i)
	list := ast.Block(n.Parent)
	c.Ordered, c.Number, c.Marker = list.Ordered, list.Number, list.Marker
	for j := n.Parent + 1; j < i; j = ast.Block(j).Next {
		c.Number++
	}
}

type mask struct {
	src          contract.Range
	show, prefix bool
}

type builder struct {
	ast          contract.AST
	block        *contract.Block
	reveal       contract.Reveal
	p            *contract.BlockProjection
	own          string
	base, at, mi int
	masks        []mask
}

func (b *builder) sub(start, end int) string { return b.own[start-b.base : end-b.base] }

func (b *builder) emit(end int, kind contract.SegKind, display string, style contract.Style, link string, obj *contract.ObjectRef) {
	b.p.Segments = append(b.p.Segments, contract.Segment{Src: contract.Range{Start: b.at, End: end},
		Kind: kind, Display: display, Style: style, Link: link, Object: obj})
	b.at = end
}

func (b *builder) marker(end int, style contract.Style, link string, show bool) {
	if end <= b.at {
		return
	}
	if show {
		b.p.Revealed = true
		b.emit(end, contract.SegMarker, b.sub(b.at, end), style|contract.StyleMarker, link, nil)
	} else {
		b.emit(end, contract.SegHidden, "", style, link, nil)
	}
}

// raw applies the block's structural masks even inside multi-line inline nodes.
func (b *builder) raw(end int, style contract.Style, link, newline string) {
	for b.at < end {
		for b.mi < len(b.masks) && b.masks[b.mi].src.End <= b.at {
			b.mi++
		}
		limit := end
		if b.mi < len(b.masks) {
			m := b.masks[b.mi]
			if m.src.Start <= b.at {
				stop := min(end, m.src.End)
				if m.prefix {
					pad := b.padding(stop)
					if pad != "" {
						b.emit(stop, contract.SegReplaced, pad, style, link, nil)
					} else {
						b.emit(stop, contract.SegHidden, "", style, link, nil)
					}
				} else {
					b.marker(stop, style, link, m.show)
				}
				continue
			}
			limit = min(limit, m.src.Start)
		}
		s := b.sub(b.at, limit)
		j := strings.IndexAny(s, "\r\n")
		if j < 0 {
			b.emit(limit, contract.SegText, s, style, link, nil)
			continue
		}
		if j > 0 {
			b.emit(b.at+j, contract.SegText, s[:j], style, link, nil)
		}
		stop := b.at + 1
		if b.sub(b.at, stop) == "\r" && stop < limit && b.sub(stop, stop+1) == "\n" {
			stop++
		}
		if b.mi < len(b.masks) && b.masks[b.mi].prefix && b.masks[b.mi].src.Start == stop {
			stop = min(end, b.masks[b.mi].src.End)
		}
		b.emit(stop, contract.SegReplaced, newline+b.padding(stop), style, link, nil)
	}
}

func (b *builder) padding(at int) string {
	i, ok := slices.BinarySearchFunc(b.block.Lines, at, func(line contract.BlockLine, at int) int {
		return line.Source.Start - at
	})
	if ok && b.block.Lines[i].Padding > 0 {
		return strings.Repeat(" ", int(b.block.Lines[i].Padding))
	}
	return ""
}

func (b *builder) inlines(in []contract.Inline, start, end int, style contract.Style, link string) {
	in = in[:end]
	for i := start; i < end; i = max(i+1, in[i].Next) {
		n := in[i]
		if n.Range.Start < b.at || n.Range.End > b.block.Range.End {
			continue
		}
		// Break spaces and backslashes are omitted from the parser's text nodes.
		if n.Kind == contract.InlineLineBreak || n.Kind == contract.InlineSoftBreak {
			gap := b.sub(b.at, n.Range.Start)
			if strings.Trim(gap, " \\") == "" {
				withGap := n
				withGap.Range.Start = b.at
				b.marker(n.Range.Start, style, link, b.nodeRevealed(withGap))
			}
		}
		b.raw(n.Range.Start, style, link, "\n")
		rule, ok := inlineRules[n.Kind]
		if !ok {
			b.raw(n.Range.End, style, link, "\n")
			continue
		}
		rule.apply(b, in, i, style|rule.style, link, b.nodeRevealed(n))
	}
}

func footnoteInline(b *builder, in []contract.Inline, i int, style contract.Style, link string, show bool) {
	n := in[i]
	// Footnotes uses the same label normalization as the Markdown snapshot.
	label := strings.Join(strings.FieldsFunc(n.Text, func(r rune) bool {
		return r == ' ' || r == '\t' || r == '\r' || r == '\n'
	}), " ")
	label = strings.ReplaceAll(strings.ToLower(label), "ß", "ss")
	label = strings.ToLower(strings.ToUpper(label))
	if _, ok := b.ast.Footnotes()[label]; !ok || show {
		b.raw(n.Range.End, style|contract.StyleFootnoteRef|contract.StyleMarker, link, "\n")
		return
	}
	label = b.sub(n.Range.Start+2, n.Range.End-1)
	b.emit(n.Range.End, contract.SegObject, "\ufffc", style|contract.StyleFootnoteRef, link,
		&contract.ObjectRef{Kind: contract.ObjFootnoteRef, Source: strings.Clone(label)})
}

func linkInline(b *builder, in []contract.Inline, i int, style contract.Style, _ string, show bool) {
	n := in[i]
	if n.Auto {
		style |= contract.StyleAutolink
	}
	nestedInline(b, in, i, style, strings.Clone(n.Dest), show)
}

func textInline(b *builder, in []contract.Inline, i int, style contract.Style, link string, _ bool) {
	b.raw(in[i].Range.End, style, link, "\n")
}

func nestedInline(b *builder, in []contract.Inline, i int, style contract.Style, link string, show bool) {
	n := in[i]
	if !n.Open.Empty() {
		b.marker(n.Open.End, style, link, show)
	}
	b.inlines(in, i+1, n.Next, style, link)
	end := n.Range.End
	if !n.Close.Empty() {
		end = n.Close.Start
	}
	b.raw(end, style, link, "\n")
	b.marker(n.Range.End, style, link, show)
}

func breakInline(b *builder, in []contract.Inline, i int, style contract.Style, link string, show bool) {
	n := in[i]
	if n.Kind == contract.InlineLineBreak {
		if j := strings.IndexAny(b.sub(b.at, n.Range.End), "\r\n"); j > 0 {
			b.marker(b.at+j, style, link, show)
		}
	}
	display := "\n"
	if n.Kind == contract.InlineSoftBreak && b.reveal.SoftBreak == contract.SoftBreakSpace {
		display = " "
	}
	b.emit(n.Range.End, contract.SegReplaced, display+b.padding(n.Range.End), style, link, nil)
}

func replacedInline(b *builder, in []contract.Inline, i int, style contract.Style, link string, show bool) {
	n := in[i]
	if show {
		b.marker(n.Range.End, style, link, true)
	} else {
		b.emit(n.Range.End, contract.SegReplaced, strings.Clone(n.Text), style, link, nil)
	}
}

func codeInline(b *builder, in []contract.Inline, i int, style contract.Style, link string, show bool) {
	n := in[i]
	b.marker(n.Open.End, style, link, show)
	end := n.Close.Start
	if !show {
		logicalEnd := end
		for j := len(b.block.Prefixes) - 1; j >= 0; j-- {
			prefix := b.block.Prefixes[j]
			if prefix.End == logicalEnd {
				logicalEnd = prefix.Start
			}
		}
		s := b.sub(b.at, logicalEnd)
		if len(s) >= 2 && (s[0] == ' ' || s[0] == '\r' || s[0] == '\n') &&
			(s[len(s)-1] == ' ' || s[len(s)-1] == '\r' || s[len(s)-1] == '\n') && strings.Trim(n.Text, " ") != "" {
			trim := 1
			if strings.HasPrefix(s, "\r\n") {
				trim = 2
			}
			b.marker(b.at+trim, style, link, false)
			trim = 1
			if strings.HasSuffix(s, "\r\n") {
				trim = 2
			}
			end = logicalEnd - trim
		}
		b.codeText(end, style, link)
		b.marker(n.Close.Start, style, link, false)
	} else {
		b.raw(end, style, link, "\n")
	}
	b.marker(n.Range.End, style, link, show)
}

func (b *builder) codeText(end int, style contract.Style, link string) {
	if b.block.Kind == contract.BlockTableCell {
		for b.at < end {
			j := strings.Index(b.sub(b.at, end), `\|`)
			if j < 0 {
				break
			}
			b.raw(b.at+j, style, link, " ")
			b.emit(b.at+2, contract.SegReplaced, "|", style, link, nil)
		}
	}
	b.raw(end, style, link, " ")
}

func imageInline(b *builder, in []contract.Inline, i int, style contract.Style, link string, show bool) {
	n := in[i]
	if show {
		nestedInline(b, in, i, style, link, true)
		return
	}
	var alt strings.Builder
	for j := i + 1; j < n.Next; j++ {
		if in[j].Next == j+1 {
			if in[j].Kind == contract.InlineSoftBreak || in[j].Kind == contract.InlineLineBreak {
				alt.WriteByte(' ')
			} else {
				alt.WriteString(in[j].Text)
			}
		}
	}
	b.emit(n.Range.End, contract.SegObject, "\ufffc", style, link, &contract.ObjectRef{Kind: contract.ObjImage,
		Source: strings.Clone(n.Dest), Alt: alt.String(), Title: strings.Clone(n.Title)})
}
