package markdown

import (
	"reflect"
	"sort"
	"strings"

	"aurink/internal/contract"
	"aurink/internal/core/text"
)

func (a *adapted) coveredBlocks() []contract.Block {
	// Keep overflow cells visible, rather than letting a row's tail hide them.
	var extras []contract.Block
	for i, b := range a.blocks {
		if b.Kind != contract.BlockTableRow {
			continue
		}
		end, _ := lineEnd(a.source, b.Range.Start)
		cells := splitCells(a.source[b.Range.Start:end])
		n := b.Next - i - 1
		for _, r := range cells[min(n, len(cells)):] {
			r = moveRange(r, b.Range.Start)
			extras = append(extras, contract.Block{Kind: contract.BlockTableCell, Parent: i, Range: r, Content: r, Lines: []contract.BlockLine{{Source: r}}})
		}
	}
	if len(extras) > 0 {
		a.blocks = insertBlocks(a.blocks, extras)
	}
	c := a.cover()
	a.coverQuoteEnds(c)
	c.revealGaps()
	byOrigin := make(map[int]coverLeaf, len(c.leaves))
	var additions []contract.Block
	for _, l := range c.leaves {
		if l.origin > 0 {
			byOrigin[l.origin] = l
			continue
		}
		parent := l.parent
		if parent == 0 {
			j := sort.Search(len(a.blocks), func(i int) bool { return a.blocks[i].Range.Start > l.Range.Start }) - 1
			for j > 0 {
				b := a.blocks[j]
				if !isLeaf(b.Kind) && b.Range.End >= l.Range.End {
					parent = j
					break
				}
				j = b.Parent
			}
		}
		additions = append(additions, contract.Block{Kind: l.Kind, Parent: parent, Range: l.Range, Content: l.Range, Lead: l.Lead, Trailer: l.Trailer})
	}
	for i, l := range byOrigin {
		b := &a.blocks[i]
		b.Kind, b.Range, b.Lead, b.Trailer = l.Kind, l.Range, l.Lead, l.Trailer
	}
	return insertBlocks(a.blocks, additions)
}

func (c *coverage) revealGaps() {
	var out []coverLeaf
	visible := func(r contract.Range) bool {
		return visibleGap(c.src[r.Start:r.End])
	}
	definition := func(r contract.Range) coverLeaf {
		start, end := r.Start, r.End
		for start < end && strings.ContainsRune(" \t>\r\n", rune(c.src[start])) {
			start++
		}
		for end > start && (c.src[end-1] == '\r' || c.src[end-1] == '\n') {
			end--
		}
		return coverLeaf{Kind: kindLinkDef, Lead: contract.Range{Start: r.Start, End: start}, Range: contract.Range{Start: start, End: end}, Trailer: contract.Range{Start: end, End: r.End}}
	}
	for _, l := range c.leaves {
		if visible(l.Lead) {
			out = append(out, definition(l.Lead))
			l.Lead.Start = l.Lead.End
		}
		tail := l.Trailer
		if visible(tail) {
			l.Trailer.End = l.Trailer.Start
		}
		out = append(out, l)
		if visible(tail) {
			out = append(out, definition(tail))
		}
	}
	var merged []coverLeaf
	for _, l := range out {
		if len(merged) > 0 {
			prev := &merged[len(merged)-1]
			if prev.Kind == kindLinkDef && l.Kind == kindLinkDef && !strings.Contains(c.src[prev.Range.Start:prev.Range.End], "]:") && strings.Contains(c.src[l.Range.Start:l.Range.End], "]:") {
				prev.Range.End = l.Range.End
				prev.Trailer = l.Trailer
				continue
			}
		}
		merged = append(merged, l)
	}
	c.leaves = merged
}

func insertBlocks(blocks, added []contract.Block) []contract.Block {
	children := make([][]int, len(blocks))
	all := append(append([]contract.Block(nil), blocks...), added...)
	for i := 1; i < len(all); i++ {
		p := all[i].Parent
		children[p] = append(children[p], i)
	}
	for i := range children {
		sort.SliceStable(children[i], func(x, y int) bool { return all[children[i][x]].Range.Start < all[children[i][y]].Range.Start })
	}
	out := make([]contract.Block, 0, len(all))
	var emit func(int, int)
	emit = func(i, parent int) {
		j := len(out)
		b := all[i]
		b.Parent = parent
		out = append(out, b)
		if i < len(children) {
			for _, k := range children[i] {
				emit(k, j)
			}
		}
		out[j].Next = len(out)
		if parent >= 0 {
			out[parent].Range.Start = min(out[parent].Range.Start, out[j].Range.Start)
			out[parent].Range.End = max(out[parent].Range.End, out[j].Range.End)
		}
	}
	emit(0, -1)
	return out
}

// splitCells follows the pinned parser's escaped-pipe rule, including cells
// beyond the header's column count which it intentionally does not materialize.
func splitCells(line string) []contract.Range {
	var cells []contract.Range
	i := 0
	if strings.HasPrefix(line, "|") {
		i++
	}
	for i < len(line) {
		j := i
		for j < len(line) && line[j] != '|' {
			if line[j] == '\\' && j+1 < len(line) && punctuation(line[j+1]) {
				j++
			}
			j++
		}
		a, b := i, j
		for a < b && (line[a] == ' ' || line[a] == '\t') {
			a++
		}
		for b > a && (line[b-1] == ' ' || line[b-1] == '\t') {
			b--
		}
		if j > i || j < len(line) {
			cells = append(cells, contract.Range{Start: a, End: b})
		}
		if j == len(line) {
			break
		}
		i = j + 1
		for i < len(line) && (line[i] == ' ' || line[i] == '\t') {
			i++
		}
	}
	return cells
}

type identityKey struct {
	kind       contract.BlockKind
	start, end int
}

// changeMapper indexes ordered, non-overlapping changes. Both starts and ends
// are nondecreasing, so the last change before a boundary determines whether
// any preceding change overlaps it. The prefix sums retain insertion order at
// shared boundaries, including multiple insertions at one offset.
type changeMapper struct {
	changes []contract.Change
	deltas  []int
}

func newChangeMapper(cs []contract.Change) changeMapper {
	m := changeMapper{changes: cs, deltas: make([]int, len(cs)+1)}
	for i, c := range cs {
		m.deltas[i+1] = m.deltas[i] + (c.NewEnd - c.NewStart) - (c.OldEnd - c.OldStart)
	}
	return m
}

func (m changeMapper) mapped(r contract.Range) (contract.Range, bool) {
	i := sort.Search(len(m.changes), func(i int) bool { return m.changes[i].OldStart >= r.End })
	if i > 0 && m.changes[i-1].OldEnd > r.Start {
		return contract.Range{}, false
	}
	return moveRange(r, m.deltas[i]), true
}

func (m changeMapper) mappedStart(at int) (int, bool) {
	i := sort.Search(len(m.changes), func(i int) bool { return m.changes[i].OldStart >= at })
	if i > 0 && m.changes[i-1].OldEnd > at {
		return 0, false
	}
	return at + m.deltas[i], true
}

func (p *Parser) freeze(a *adapted, v contract.Version, exts contract.Extensions, cs []contract.Change) (*Snapshot, []contract.BlockID) {
	blocks := a.coveredBlocks()
	return p.freezeBlocks(a.source, a.bodyAt, blocks, v, exts, cs)
}

func (p *Parser) freezeBlocks(source string, bodyAt int, blocks []contract.Block, v contract.Version, exts contract.Extensions, cs []contract.Change) (*Snapshot, []contract.BlockID) {
	s := &Snapshot{text: text.New(source, v), exts: exts, bodyAt: bodyAt, entries: make([]placement, len(blocks))}
	s.hasDefinitions = definitionBlocks(source, blocks) != ""
	old := p.current
	var oldFor []int
	if old != nil {
		mapper := newChangeMapper(cs)
		oldFor = make([]int, len(blocks))
		for i := range oldFor {
			oldFor[i] = -1
		}
		byRange := map[identityKey][]int{}
		for i, b := range blocks {
			key := identityKey{b.Kind, b.Range.Start, b.Range.End}
			byRange[key] = append(byRange[key], i)
		}
		used := make([]bool, old.Len())
		for i := range old.entries {
			b := old.Block(i)
			r, ok := mapper.mapped(b.Range)
			if !ok {
				continue
			}
			key := identityKey{b.Kind, r.Start, r.End}
			candidates := byRange[key]
			if len(candidates) > 0 {
				j := candidates[0]
				oldFor[j] = i
				used[i] = true
				byRange[key] = candidates[1:]
			}
		}
		starts := map[identityKey][]int{}
		for i := range old.entries {
			if used[i] {
				continue
			}
			b := old.Block(i)
			at, ok := mapper.mappedStart(b.Range.Start)
			if !ok {
				continue
			}
			key := identityKey{kind: b.Kind, start: at}
			starts[key] = append(starts[key], i)
		}
		for i, b := range blocks {
			if oldFor[i] >= 0 {
				continue
			}
			key := identityKey{kind: b.Kind, start: b.Range.Start}
			ids := starts[key]
			if len(ids) > 0 {
				oldFor[i] = ids[0]
				starts[key] = ids[1:]
			}
		}
	}
	var dirty []contract.BlockID
	var referenceChecks [][2]int
	globalChanged := false
	if old != nil && !p.skipReferences {
		globalChanged = definitionBytes(old) != definitionBlocks(source, blocks)
	}
	for i, b := range blocks {
		e := &s.entries[i]
		e.start, e.parent, e.next = b.Range.Start, b.Parent, b.Next
		rel := moveBlock(b, -e.start)
		rel.Parent, rel.Next, rel.ID = 0, 0, 0
		previous := -1
		if old != nil {
			previous = oldFor[i]
		}
		changed := true
		if previous >= 0 {
			o := &old.entries[previous]
			e.id = o.id
			if reflect.DeepEqual(rel, o.data.block) && sameBytes(old, previous, source, b) {
				e.data = o.data
				changed = false
			}
		}
		if e.id == 0 {
			p.next++
			e.id = p.next
		}
		if e.data == nil {
			e.data = &blockData{block: rel}
			s.adapted++
		}
		if previous >= 0 {
			o := &old.entries[previous]
			if b.Next-i != o.next-previous || b.Parent >= 0 && o.parent >= 0 && s.entries[b.Parent].id != old.entries[o.parent].id {
				changed = true
			}
		}
		if isLeaf(b.Kind) {
			s.leaves = append(s.leaves, i)
		}
		if changed {
			dirty = append(dirty, e.id)
		} else if globalChanged && strings.Contains(source[b.Range.Start:b.Range.End], "[") {
			referenceChecks = append(referenceChecks, [2]int{previous, i})
		}
	}
	for _, pair := range referenceChecks {
		if !sameInlineMeaning(old, pair[0], s, pair[1]) {
			dirty = append(dirty, s.entries[pair[1]].id)
		}
	}
	s.highest = p.next
	return s, dirty
}

func sameInlineMeaning(a *Snapshot, i int, b *Snapshot, j int) bool {
	x, y := a.Inlines(i), b.Inlines(j)
	if len(x) != len(y) {
		return false
	}
	for k := range x {
		n, m := x[k], y[k]
		if n.Kind == contract.InlineFootnoteRef && m.Kind == contract.InlineFootnoteRef && footnoteSource(a, n.Text) != footnoteSource(b, m.Text) {
			return false
		}
		n.Range = moveRange(n.Range, -a.entries[i].start)
		m.Range = moveRange(m.Range, -b.entries[j].start)
		if !n.Open.Empty() {
			n.Open = moveRange(n.Open, -a.entries[i].start)
		}
		if !m.Open.Empty() {
			m.Open = moveRange(m.Open, -b.entries[j].start)
		}
		if !n.Close.Empty() {
			n.Close = moveRange(n.Close, -a.entries[i].start)
		}
		if !m.Close.Empty() {
			m.Close = moveRange(m.Close, -b.entries[j].start)
		}
		if n != m {
			return false
		}
	}
	return true
}

func referenceDirty(old, next *Snapshot, dirty []contract.BlockID) []contract.BlockID {
	seen := make(map[contract.BlockID]bool, len(dirty))
	for _, id := range dirty {
		seen[id] = true
	}
	var out []contract.BlockID
	for i := range next.Leaves() {
		e := &next.entries[i]
		if seen[e.id] || !strings.Contains(next.text.Slice(moveRange(e.data.block.Range, e.start)), "[") {
			continue
		}
		if j, ok := old.Index(e.id); ok && !sameInlineMeaning(old, j, next, i) {
			out = append(out, e.id)
		}
	}
	return out
}

func footnoteSource(s *Snapshot, label string) string {
	key := normalizeLabel(label)
	for i := range s.entries {
		e := &s.entries[i]
		if e.data.block.Kind == contract.BlockFootnoteDef && normalizeLabel(e.data.block.Label) == key {
			return s.text.Slice(moveRange(e.data.block.Range, e.start))
		}
	}
	return ""
}

func sameBytes(old *Snapshot, i int, src string, b contract.Block) bool {
	o := old.Block(i)
	if isLeaf(b.Kind) {
		return old.text.Slice(contract.Range{Start: o.Lead.Start, End: o.Trailer.End}) == src[b.Lead.Start:b.Trailer.End]
	}
	return old.text.Slice(o.Range) == src[b.Range.Start:b.Range.End]
}

func definitionBytes(s *Snapshot) string {
	var out strings.Builder
	for i := range s.entries {
		b := s.Block(i)
		if b.Kind == contract.BlockLinkDef || b.Kind == contract.BlockFootnoteDef {
			out.WriteString(b.Label)
			out.WriteByte(0)
			out.WriteString(s.text.Slice(b.Range))
			out.WriteByte(0)
		}
	}
	return out.String()
}
func definitionBlocks(src string, blocks []contract.Block) string {
	var out strings.Builder
	for _, b := range blocks {
		if b.Kind == contract.BlockLinkDef || b.Kind == contract.BlockFootnoteDef {
			out.WriteString(b.Label)
			out.WriteByte(0)
			out.WriteString(src[b.Range.Start:b.Range.End])
			out.WriteByte(0)
		}
	}
	return out.String()
}
