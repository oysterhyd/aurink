package markdown

import (
	"iter"
	"sort"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"aurink/internal/contract"
	"aurink/internal/core/text"
	"github.com/egoist/mygo/ui/markdown/parser"
)

// Parser owns the single-writer incremental parser. Its snapshots are independent
// immutable values; only Parser's methods require serialization by the caller.
type Parser struct {
	current        *Snapshot
	next           contract.BlockID
	skipReferences bool
}

// Stats counts work in one update. AdaptedBlocks includes window lookahead and
// failed attempts. FullRebuilds is the sum of the four fallback counters.
// BoundaryFallbacks means no suffix anchor survived bounded window growth;
// WindowLimitFallbacks means the edit span or a complete subtree exceeded it.
type Stats struct {
	AdaptedBlocks        int
	FullRebuilds         int
	BoundaryFallbacks    int
	WindowLimitFallbacks int
	ReferenceFallbacks   int
	FrontMatterFallbacks int
	LocalParses          int
	// These parser.Edit counters remain zero; updates now use fresh windows.
	CodeFallbacks  int
	TableFallbacks int
	Resyncs        int
	ParserEdits    int
}

// Update is the result consumed by core/document. Dirty contains new or changed
// IDs, including reference users; it excludes blocks that only moved. Removed
// IDs can be detected with Snapshot.Index.
type Update struct {
	Snapshot *Snapshot
	Dirty    []contract.BlockID
	Stats    Stats
}

// New parses source at version with the requested extension settings.
func New(source string, version contract.Version, exts contract.Extensions) (*Parser, error) {
	if version == 0 {
		return nil, &contract.Error{Code: contract.CodeInvalid, Op: "markdown.New", Detail: "zero version"}
	}
	p := &Parser{}
	a, err := parseWithExtensions(source, exts, exts&contract.ExtFrontMatter != 0)
	if err != nil {
		return nil, &contract.Error{Code: contract.CodeInternal, Op: "markdown.New", Err: err}
	} // Public Line offsets must map to their source.
	p.current, _ = p.freeze(a, version, exts, nil)
	return p, nil
}

// Snapshot returns the current AST. Later updates never modify this value.
func (p *Parser) Snapshot() *Snapshot { return p.current }

// Apply applies ordered edits using core/text's validation and byte-preserving
// change mapping. It is a convenience for callers that do not own a Text yet.
func (p *Parser) Apply(edits []contract.TextEdit) (Update, error) {
	next, changes, err := p.current.text.Apply(edits)
	if err != nil {
		return Update{}, err
	}
	if len(changes) == 0 {
		return Update{Snapshot: p.current}, nil
	}
	return p.Update(next, changes)
}

// Update advances from Snapshot to next. Changes must describe the ordered,
// non-overlapping replacements from the previous source to next's source, using
// both coordinate spaces as returned by core/text.Apply. Invalid input leaves
// the writer and all snapshots unchanged. Versions must increase.
func (p *Parser) Update(next contract.Text, changes []contract.Change) (Update, error) {
	old := p.current
	if next == nil || next.Version() <= old.Version() || !validChanges(old.text.String(), next.String(), changes) {
		return Update{}, &contract.Error{Code: contract.CodeInvalid, Op: "markdown.Update", Detail: "invalid version or change mapping"}
	}
	source := next.String()
	if source == old.text.String() {
		s := &Snapshot{text: text.New(source, next.Version()), exts: old.exts, bodyAt: old.bodyAt, highest: old.highest, hasDefinitions: old.hasDefinitions, entries: make([]placement, old.Len()), leaves: old.leaves}
		for i := range old.entries {
			copyPlacement(&s.entries[i], &old.entries[i])
		}
		p.current = s
		return Update{Snapshot: s}, nil
	}
	dollarEdit := touchesDollarLine(old.text.String(), source, changes)
	if s, dirty, ok := old.typing(source, next.Version(), changes); ok && !dollarEdit {
		p.current = s
		return Update{Snapshot: s, Dirty: dirty, Stats: Stats{AdaptedBlocks: len(dirty)}}, nil
	}
	var stats Stats
	at, _ := front(source, old.exts&contract.ExtFrontMatter != 0)
	switch {
	case at != old.bodyAt || touchesFront(changes, old.bodyAt):
		stats.FrontMatterFallbacks++
	case dollarEdit:
		// A closer can make a distant, formerly literal opener into a block.
		stats.BoundaryFallbacks++
	case ambiguousCaretLabel(old.text.String()) || ambiguousCaretLabel(source):
		stats.ReferenceFallbacks++
	default:
		u, ok := p.localUpdate(source, next.Version(), changes)
		if ok {
			p.current = u.Snapshot
			return u, nil
		}
		stats = u.Stats
	}
	stats.FullRebuilds++
	a, err := parseWithExtensions(source, old.exts, old.exts&contract.ExtFrontMatter != 0)
	if err != nil {
		return Update{}, &contract.Error{Code: contract.CodeInternal, Op: "markdown.Update", Err: err}
	}
	s, dirty := p.freeze(a, next.Version(), old.exts, changes)
	stats.AdaptedBlocks += s.Len()
	p.current = s
	return Update{Snapshot: s, Dirty: dirty, Stats: stats}, nil
}

func touchesFront(cs []contract.Change, at int) bool {
	return at > 0 && len(cs) > 0 && cs[0].OldStart < at
}

func validChanges(old, next string, cs []contract.Change) bool {
	a, b := 0, 0
	for _, c := range cs {
		if c.OldStart < a || c.OldEnd < c.OldStart || c.OldEnd > len(old) || c.NewStart < b || c.NewEnd < c.NewStart || c.NewEnd > len(next) || old[a:c.OldStart] != next[b:c.NewStart] {
			return false
		}
		a, b = c.OldEnd, c.NewEnd
	}
	return old[a:] == next[b:]
}

type blockData struct {
	block      contract.Block
	inlineOnce sync.Once
	inline     []contract.Inline
	local      bool
}

type placement struct {
	data                *blockData
	start, parent, next int
	id                  contract.BlockID
	once                sync.Once
	block               *contract.Block
	inOnce              sync.Once
	in                  []contract.Inline
}

// Snapshot is an immutable AST with shared relative block records. Do not copy
// it. Blocks, inline slices and outline slices returned by it are read-only.
type Snapshot struct {
	text           *text.Text
	exts           contract.Extensions
	bodyAt         int
	entries        []placement
	leaves         []int
	adapted        int
	highest        contract.BlockID
	hasDefinitions bool
	indexOnce      sync.Once
	index          map[contract.BlockID]int
	lazyOnce       sync.Once
	lazyMu         sync.Mutex
	lazy           *parser.Document
	lazyInput      parserInput
	derivedOnce    sync.Once
	outline        []contract.Heading
	notes          map[string]int
	defs           map[string]contract.LinkDef
}

var _ contract.AST = (*Snapshot)(nil)

// Version returns the source version.
func (s *Snapshot) Version() contract.Version { return s.text.Version() }

// Len returns the block count, including the root.
func (s *Snapshot) Len() int { return len(s.entries) }

// Block returns a read-only block, or nil for an invalid index.
func (s *Snapshot) Block(i int) *contract.Block {
	if i < 0 || i >= s.Len() {
		return nil
	}
	e := &s.entries[i]
	e.once.Do(func() {
		b := moveBlock(e.data.block, e.start)
		b.Parent, b.Next, b.ID = e.parent, e.next, e.id
		e.block = &b
	})
	return e.block
}

// Index looks up a live ID in this snapshot.
func (s *Snapshot) Index(id contract.BlockID) (int, bool) {
	s.indexOnce.Do(func() {
		s.index = make(map[contract.BlockID]int, s.Len())
		for i := range s.entries {
			s.index[s.entries[i].id] = i
		}
	})
	i, ok := s.index[id]
	return i, ok
}

// Children iterates immediate children in document order.
func (s *Snapshot) Children(i int) iter.Seq[int] {
	return func(yield func(int) bool) {
		if i < 0 || i >= s.Len() {
			return
		}
		for j := i + 1; j < s.entries[i].next; j = s.entries[j].next {
			if !yield(j) {
				return
			}
		}
	}
}

// Leaves iterates all editor rows in document order.
func (s *Snapshot) Leaves() iter.Seq[int] {
	return func(yield func(int) bool) {
		for _, i := range s.leaves {
			if !yield(i) {
				return
			}
		}
	}
}

// LeafCount returns the number of editor rows.
func (s *Snapshot) LeafCount() int { return len(s.leaves) }

// LeafAt returns the owner of offset, clamped to the source boundaries. A
// separator without its own row belongs to the preceding leaf's trailer.
func (s *Snapshot) LeafAt(offset int) int {
	j := sort.Search(len(s.leaves), func(j int) bool { e := &s.entries[s.leaves[j]]; return e.start+e.data.block.Trailer.End > offset })
	return s.leaves[min(j, len(s.leaves)-1)]
}

// Path returns root-to-leaf ancestry for offset.
func (s *Snapshot) Path(offset int) []int {
	var out []int
	for i := s.LeafAt(offset); i >= 0; i = s.entries[i].parent {
		out = append(out, i)
	}
	for a, b := 0, len(out)-1; a < b; a, b = a+1, b-1 {
		out[a], out[b] = out[b], out[a]
	}
	return out
}

// FrontMatter returns the recognized envelope, if present.
func (s *Snapshot) FrontMatter() (*contract.Block, bool) {
	if s.bodyAt > 0 {
		return s.Block(1), true
	}
	return nil, false
}

func moveRange(r contract.Range, delta int) contract.Range {
	return contract.Range{Start: r.Start + delta, End: r.End + delta}
}
func moveBlock(b contract.Block, delta int) contract.Block {
	b.Range = moveRange(b.Range, delta)
	b.Content = moveRange(b.Content, delta)
	if isLeaf(b.Kind) {
		b.Lead = moveRange(b.Lead, delta)
		b.Trailer = moveRange(b.Trailer, delta)
	}
	if b.Task != contract.TaskNone {
		b.TaskMarker = moveRange(b.TaskMarker, delta)
	}
	if len(b.Markers) > 0 {
		b.Markers = append([]contract.Range(nil), b.Markers...)
		for i := range b.Markers {
			b.Markers[i] = moveRange(b.Markers[i], delta)
		}
	}
	if len(b.Prefixes) > 0 {
		b.Prefixes = append([]contract.Range(nil), b.Prefixes...)
		for i := range b.Prefixes {
			b.Prefixes[i] = moveRange(b.Prefixes[i], delta)
		}
	}
	if len(b.Lines) > 0 {
		b.Lines = append([]contract.BlockLine(nil), b.Lines...)
		for i := range b.Lines {
			b.Lines[i].Source = moveRange(b.Lines[i].Source, delta)
		}
	}
	return b
}

// typing proves a replacement cannot change block grammar: the block and the
// affected line retain a leading letter; line endings and pipes cannot change.
func (s *Snapshot) typing(source string, v contract.Version, cs []contract.Change) (*Snapshot, []contract.BlockID, bool) {
	if len(cs) != 1 {
		return nil, nil, false
	}
	c := cs[0]
	if strings.ContainsAny(source[c.NewStart:c.NewEnd], "\r\n|") || strings.ContainsAny(s.text.String()[c.OldStart:c.OldEnd], "\r\n|") {
		return nil, nil, false
	}
	i := s.LeafAt(c.OldStart)
	b := s.Block(i)
	for j := s.entries[i].parent; j > 0; j = s.entries[j].parent {
		if s.entries[j].data.block.Kind == contract.BlockFootnoteDef {
			return nil, nil, false
		}
	}
	if b.Kind != contract.BlockParagraph && b.Kind != contract.BlockHeading && b.Kind != contract.BlockTableCell {
		return nil, nil, false
	}
	if len(b.Lines) == 0 {
		return nil, nil, false
	}
	initial, _ := utf8.DecodeRuneInString(s.text.Slice(b.Lines[0].Source))
	if !unicode.IsLetter(initial) {
		return nil, nil, false
	}
	safe := false
	for _, l := range b.Lines {
		first, width := utf8.DecodeRuneInString(s.text.Slice(l.Source))
		inside := c.OldEnd < l.Source.End || b.Kind == contract.BlockParagraph && c.OldEnd == l.Source.End
		if unicode.IsLetter(first) && c.OldStart >= l.Source.Start+width && inside {
			safe = true
			break
		}
	}
	if !safe {
		return nil, nil, false
	}
	n := &Snapshot{text: text.New(source, v), exts: s.exts, bodyAt: s.bodyAt, entries: make([]placement, s.Len()), leaves: s.leaves}
	n.highest = s.highest
	n.hasDefinitions = s.hasDefinitions
	delta := (c.NewEnd - c.NewStart) - (c.OldEnd - c.OldStart)
	var dirty []contract.BlockID
	for j := range s.entries {
		e := &s.entries[j]
		x := &n.entries[j]
		x.data, x.start, x.parent, x.next, x.id = e.data, e.start, e.parent, e.next, e.id
		end := e.start + e.data.block.Range.End
		affected := j == i || j < i && e.next > i
		if !affected && end <= c.OldStart {
			continue
		}
		if !affected && e.start >= c.OldEnd {
			x.start += delta
			continue
		}
		z := moveBlock(e.data.block, e.start)
		mapEnd := func(r contract.Range) contract.Range {
			if r.Start >= c.OldEnd {
				r.Start += delta
			}
			if r.End >= c.OldEnd {
				r.End += delta
			}
			return r
		}
		z.Range = mapEnd(z.Range)
		z.Content = mapEnd(z.Content)
		if isLeaf(z.Kind) {
			z.Lead = mapEnd(z.Lead)
			z.Trailer = mapEnd(z.Trailer)
		}
		for k := range z.Lines {
			z.Lines[k].Source = mapEnd(z.Lines[k].Source)
		}
		for k := range z.Markers {
			z.Markers[k] = mapEnd(z.Markers[k])
		}
		for k := range z.Prefixes {
			z.Prefixes[k] = mapEnd(z.Prefixes[k])
		}
		x.data = &blockData{block: moveBlock(z, -x.start)}
		dirty = append(dirty, x.id)
	}
	return n, dirty, true
}

func normalizeLabel(s string) string {
	s = strings.Join(strings.FieldsFunc(s, func(r rune) bool { return r == ' ' || r == '\t' || r == '\r' || r == '\n' }), " ")
	s = strings.ReplaceAll(strings.ToLower(s), "ß", "ss")
	return strings.ToLower(strings.ToUpper(s))
}
