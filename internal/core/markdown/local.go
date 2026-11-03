package markdown

import (
	"reflect"

	"aurink/internal/contract"
	"aurink/internal/core/text"
)

// localUpdate reparses complete top-level subtrees, including their source
// prefixes. Two unchanged subtrees certify the suffix before it is shared.
func (p *Parser) localUpdate(source string, v contract.Version, cs []contract.Change) (Update, bool) {
	old := p.current
	if len(cs) == 0 {
		return Update{}, false
	}
	tops := make([]int, 0, old.LeafCount())
	for i := 1; i < old.Len(); i = old.entries[i].next {
		tops = append(tops, i)
	}
	if len(tops) == 0 {
		return Update{}, false
	}
	first, last := cs[0].OldStart, cs[len(cs)-1].OldEnd
	lo := 0
	for j, i := range tops {
		if old.entries[i].start > first {
			break
		}
		lo = j
	}
	lo = max(0, lo-1)
	for lo > 0 && (synthetic(old.entries[tops[lo]].data.block.Kind) || windowStart(old, tops[lo]) < old.Block(tops[lo-1]).Range.End) {
		lo--
	}
	hi := lo
	for hi < len(tops) && old.entries[tops[hi]].start <= last {
		hi++
	}
	start := windowStart(old, tops[lo])
	delta := len(source) - old.text.Len()
	var a *adapted
	var blocks []contract.Block
	var anchorOld, anchorNew, cutOld, cutNew int
	stats := Stats{}
	for look := 4; ; look *= 2 {
		stop := hi
		for remaining := look; stop < len(tops) && remaining > 0; stop++ {
			if old.entries[tops[stop]].data.block.Kind != contract.BlockBlank {
				remaining--
			}
		}
		for stop < len(tops) && windowStart(old, tops[stop]) < old.Block(tops[stop-1]).Range.End {
			stop++
		}
		end := old.text.Len()
		if stop < len(tops) {
			end = windowStart(old, tops[stop])
		}
		newEnd := end + delta
		// Coverage adds one row per blank line, without adding parser subtrees.
		windowTops := 0
		for _, i := range tops[lo:stop] {
			if old.entries[i].data.block.Kind != contract.BlockBlank {
				windowTops++
			}
		}
		if end-start > maxWindowBytes || newEnd-start > maxWindowBytes || windowTops > maxWindowTops {
			stats.WindowLimitFallbacks++
			return Update{Stats: stats}, false
		}
		if first < start || last > end || newEnd < start || newEnd > len(source) {
			stats.BoundaryFallbacks++
			return Update{Stats: stats}, false
		}
		ext := old.exts&contract.ExtFrontMatter != 0 && start == 0
		var err error
		a, err = parseWithExtensions(source[start:newEnd], old.exts, ext)
		if err != nil {
			stats.BoundaryFallbacks++
			return Update{Stats: stats}, false
		}
		blocks = a.coveredBlocks()
		stats.AdaptedBlocks += len(blocks)
		stats.LocalParses++
		// Indented text may be a lazy paragraph continuation at this boundary.
		// Certify the left context too; otherwise restart one subtree earlier.
		if start > 0 && (old.Block(tops[lo]).Range.End >= first || len(blocks) < 2 || !sameSubtree(old, tops[lo], blocks, 1, start, 0)) {
			lo--
			for lo > 0 && (synthetic(old.entries[tops[lo]].data.block.Kind) || windowStart(old, tops[lo]) < old.Block(tops[lo-1]).Range.End) {
				lo--
			}
			start = windowStart(old, tops[lo])
			look = 2
			continue
		}
		var localTops []int
		for i := 1; i < len(blocks); i = blocks[i].Next {
			localTops = append(localTops, i)
		}
		anchorOld, anchorNew, cutOld, cutNew = -1, -1, 0, 0
		if end == old.text.Len() {
			anchorOld, anchorNew, cutOld, cutNew = old.Len(), len(blocks), end, len(source)
		}
		for j := max(hi, lo+1); anchorOld < 0 && j+1 < stop; j++ {
			oi := tops[j]
			ob := old.Block(oi)
			if synthetic(ob.Kind) || synthetic(old.Block(tops[j+1]).Kind) || ob.Range.Start < last {
				continue
			}
			boundary := windowStart(old, oi)
			if boundary < old.Block(tops[j-1]).Range.End {
				continue
			}
			for k, ni := range localTops {
				if k+1 >= len(localTops) {
					break
				}
				nb := blocks[ni]
				if synthetic(nb.Kind) || synthetic(blocks[localTops[k+1]].Kind) || ob.Kind != nb.Kind || ob.Range.Start+delta != nb.Range.Start+start {
					continue
				}
				if sameSubtree(old, oi, blocks, ni, start, delta) && sameSubtree(old, tops[j+1], blocks, localTops[k+1], start, delta) {
					leaf := ni
					for leaf < len(blocks) && !isLeaf(blocks[leaf].Kind) {
						leaf++
					}
					if leaf < len(blocks) && blocks[leaf].Lead.Start+start == boundary+delta {
						anchorOld, anchorNew, cutOld, cutNew = oi, ni, boundary, boundary+delta
						break
					}
				}
			}
		}
		if anchorOld >= 0 {
			break
		}
		if look >= maxWindowTops/2 {
			stats.BoundaryFallbacks++
			return Update{Stats: stats}, false
		}
	}
	begin := tops[lo]
	region := &Snapshot{text: text.New(old.text.String()[start:cutOld], old.Version()), exts: old.exts, entries: make([]placement, anchorOld-begin+1)}
	root := contract.Block{Kind: contract.BlockRoot, Range: contract.Range{End: cutOld - start}, Content: contract.Range{End: cutOld - start}}
	region.entries[0] = placement{data: &blockData{block: root}, parent: -1, next: len(region.entries), id: old.entries[0].id}
	for i := begin; i < anchorOld; i++ {
		e := &old.entries[i]
		x := &region.entries[i-begin+1]
		copyPlacement(x, e)
		x.start -= start
		x.next -= begin - 1
		if x.parent > 0 {
			x.parent -= begin - 1
		}
		if isLeaf(x.data.block.Kind) {
			region.leaves = append(region.leaves, i-begin+1)
		}
	}
	blocks = blocks[:anchorNew]
	blocks[0].Range.End, blocks[0].Content.End, blocks[0].Next = cutNew-start, cutNew-start, len(blocks)
	changes := make([]contract.Change, len(cs))
	for i, c := range cs {
		changes[i] = contract.Change{OldStart: c.OldStart - start, OldEnd: c.OldEnd - start, NewStart: c.NewStart - start, NewEnd: c.NewEnd - start}
	}
	writer := &Parser{current: region, next: p.next, skipReferences: true}
	middle, dirty := writer.freezeBlocks(source[start:cutNew], a.bodyAt, blocks, v, old.exts, changes)
	p.next = writer.next
	count := begin + middle.Len() - 1 + old.Len() - anchorOld
	s := &Snapshot{text: text.New(source, v), exts: old.exts, bodyAt: old.bodyAt, highest: p.next, entries: make([]placement, count)}
	s.leaves = make([]int, 0, old.LeafCount()+middle.LeafCount()-region.LeafCount())
	if start == 0 {
		s.bodyAt = a.bodyAt
	}
	root.Range.End, root.Content.End = len(source), len(source)
	s.entries[0] = placement{data: &blockData{block: root}, parent: -1, next: count, id: old.entries[0].id}
	for i := 1; i < begin; i++ {
		copyPlacement(&s.entries[i], &old.entries[i])
	}
	for i := 1; i < middle.Len(); i++ {
		x := &s.entries[begin+i-1]
		copyPlacement(x, &middle.entries[i])
		x.start += start
		x.next += begin - 1
		if x.parent > 0 {
			x.parent += begin - 1
		}
	}
	shift := begin + middle.Len() - 1 - anchorOld
	for i := anchorOld; i < old.Len(); i++ {
		x := &s.entries[i+shift]
		copyPlacement(x, &old.entries[i])
		x.start += delta
		x.next += shift
		if x.parent > 0 {
			x.parent += shift
		}
	}
	for i := range s.entries {
		kind := s.entries[i].data.block.Kind
		if isLeaf(kind) {
			s.leaves = append(s.leaves, i)
		}
		if kind == contract.BlockLinkDef || kind == contract.BlockFootnoteDef {
			s.hasDefinitions = true
		}
	}
	if definitionBytes(region) != definitionBytes(middle) {
		dirty = append(dirty, referenceDirty(old, s, dirty)...)
	}
	stats.AdaptedBlocks++ // The document root is also replaced.
	return Update{Snapshot: s, Dirty: dirty, Stats: stats}, true
}

const (
	maxWindowBytes = 24 << 10
	maxWindowTops  = 64
)

func windowStart(s *Snapshot, top int) int {
	return min(s.Block(top).Range.Start, s.Block(firstLeaf(s, top)).Lead.Start)
}

func synthetic(k contract.BlockKind) bool {
	return k == contract.BlockBlank || k == contract.BlockLinkDef
}

func copyPlacement(dst, src *placement) {
	dst.data, dst.start, dst.parent, dst.next, dst.id = src.data, src.start, src.parent, src.next, src.id
}
func firstLeaf(s *Snapshot, i int) int {
	for !isLeaf(s.entries[i].data.block.Kind) {
		i++
	}
	return i
}

func sameSubtree(old *Snapshot, oi int, blocks []contract.Block, ni, start, delta int) bool {
	if old.entries[oi].next-oi != blocks[ni].Next-ni {
		return false
	}
	for j := 0; j < old.entries[oi].next-oi; j++ {
		a, b := *old.Block(oi + j), blocks[ni+j]
		a = moveBlock(a, delta-start)
		a.ID, b.ID = 0, 0
		if a.Parent > 0 {
			a.Parent -= oi
		}
		if b.Parent > 0 {
			b.Parent -= ni
		}
		a.Next -= oi
		b.Next -= ni
		if !reflect.DeepEqual(a, b) {
			return false
		}
	}
	return true
}
