package astspike

import (
	"fmt"

	"aurink.local/spikes/contract-rc/contract"
	"github.com/egoist/mygo/ui/markdown/parser"
)

type engine struct {
	ext              parser.Extensions
	fm               bool
	state            *snapshot
	next             contract.BlockID
	codeFallbacks    int
	refFallbacks     int
	guarded          bool
	incrementalEdits int
	frontFallbacks   int
}

func newEngine(src string, ext parser.Extensions, fm bool) (*engine, error) {
	s, err := parse(src, ext, fm)
	if err != nil {
		return nil, err
	}
	e := &engine{ext: ext, fm: fm, state: s, guarded: true}
	for i := range s.blocks {
		s.blocks[i].ID = e.allocate()
	}
	return e, nil
}

func (e *engine) allocate() contract.BlockID {
	e.next++
	return e.next
}

func (e *engine) edit(edit contract.TextEdit) error {
	old := e.state
	r := edit.Range
	if r.Start < 0 || r.End < r.Start || r.End > len(old.source) {
		return fmt.Errorf("invalid edit: %+v", edit)
	}
	src := old.source[:r.Start] + edit.Text + old.source[r.End:]
	at, _ := front(src, e.fm)
	d := old.doc
	if e.guarded && (ambiguousCaretLabel(old.source) || ambiguousCaretLabel(src)) {
		// Ordinary reference labels such as "[\n^x]" collide with the
		// parser's internal "^x" footnote invalidation key. Public APIs do
		// not expose a cache invalidator. Rebuild while that syntax exists.
		e.refFallbacks++
		d = parser.New(e.ext)
		d.Set(src[at:])
	} else if at == old.bodyAt && r.Start >= old.bodyAt {
		e.incrementalEdits++
		d.Edit(r.Start-old.bodyAt, r.End-old.bodyAt, edit.Text)
		// v0.7.3 can resynchronize between adjacent indented-code siblings
		// that a full parse merges. This syntax cannot have two consecutive
		// indented-code siblings; reparse conservatively, without an oracle.
		if e.guarded && adjacentIndentedCode(d) {
			e.codeFallbacks++
			d = parser.New(e.ext)
			d.Set(src[at:])
		}
	} else {
		e.frontFallbacks++
		d = parser.New(e.ext)
		d.Set(src[at:])
	}
	next, err := adapt(src, d, at)
	if err != nil {
		return err
	}
	e.inherit(old, next, edit)
	e.state = next
	return nil
}

func ambiguousCaretLabel(src string) bool {
	for i := 0; i < len(src); i++ {
		if src[i] != '[' {
			continue
		}
		j := i + 1
		for j < len(src) && (src[j] == ' ' || src[j] == '\t' || src[j] == '\n' || src[j] == '\r') {
			j++
		}
		if j < len(src) && src[j] == '^' {
			space := j > i+1
			for k := j + 1; k < len(src) && k-i <= 1000 && src[k] != '['; k++ {
				if src[k] == ']' {
					if space {
						return true
					}
					break
				}
				space = space || src[k] == ' ' || src[k] == '\t' || src[k] == '\n' || src[k] == '\r'
			}
		}
	}
	return false
}

func adjacentIndentedCode(d *parser.Document) bool {
	previous := map[int]int{}
	for i := 1; i < d.Len(); i++ {
		b := d.Block(i)
		if prev, ok := previous[b.Parent]; ok && b.Kind == parser.CodeBlock && !b.Fenced {
			p := d.Block(prev)
			if p.Kind == parser.CodeBlock && !p.Fenced {
				return true
			}
		}
		previous[b.Parent] = i
	}
	return false
}

type identityKey struct {
	kind       contract.BlockKind
	start, end int
}

// mappedUnchanged uses after affinity at the start and before affinity at the
// end. It describes source-disjoint blocks, not blocks guaranteed to survive a
// Markdown grammar change (see CR-004).
func mappedUnchanged(r contract.Range, edit contract.TextEdit) (contract.Range, bool) {
	delta := len(edit.Text) - edit.Range.Len()
	if r.End <= edit.Range.Start {
		return r, true
	}
	if r.Start >= edit.Range.End {
		return contract.Range{Start: r.Start + delta, End: r.End + delta}, true
	}
	return contract.Range{}, false
}

func (e *engine) inherit(old, next *snapshot, edit contract.TextEdit) {
	byRange := map[identityKey][]int{}
	for i, b := range next.blocks {
		key := identityKey{b.Kind, b.Range.Start, b.Range.End}
		byRange[key] = append(byRange[key], i)
	}
	used := map[contract.BlockID]bool{}
	// Reserve all exact survivors before the same-start typing heuristic, so
	// that a changed predecessor cannot steal an unaffected block's identity.
	for _, b := range old.blocks {
		r, ok := mappedUnchanged(b.Range, edit)
		if !ok {
			continue
		}
		key := identityKey{b.Kind, r.Start, r.End}
		for _, i := range byRange[key] {
			if next.blocks[i].ID == 0 {
				next.blocks[i].ID = b.ID
				used[b.ID] = true
				break
			}
		}
	}
	type startKey struct {
		kind contract.BlockKind
		at   int
	}
	starts := map[startKey][]contract.BlockID{}
	for _, b := range old.blocks {
		if used[b.ID] {
			continue
		}
		at := b.Range.Start
		if at > edit.Range.Start && at < edit.Range.End {
			continue // The old start was deleted.
		}
		if at >= edit.Range.End && at > edit.Range.Start {
			at += len(edit.Text) - edit.Range.Len()
		}
		key := startKey{b.Kind, at}
		starts[key] = append(starts[key], b.ID)
	}
	for i := range next.blocks {
		b := &next.blocks[i]
		if b.ID != 0 {
			continue
		}
		key := startKey{b.Kind, b.Range.Start}
		ids := starts[key]
		if len(ids) > 0 {
			b.ID = ids[0]
			starts[key] = ids[1:]
		} else {
			b.ID = e.allocate()
		}
	}
}
