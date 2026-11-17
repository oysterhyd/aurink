package document

import (
	"iter"

	"aurink/internal/contract"
)

// plainAST is the syntax-free contract.AST snapshot used in large-file mode
// (docs/09-phase4.md D2). It satisfies all AST invariants in O(1) time and
// space with one root block and one leaf covering the entire source.
type plainAST struct {
	version contract.Version
	blocks  [2]contract.Block
	inlines []contract.Inline
}

var _ contract.AST = (*plainAST)(nil)

func newPlainAST(length int, version contract.Version) *plainAST {
	kind := contract.BlockParagraph
	var inlines []contract.Inline
	if length == 0 {
		kind = contract.BlockBlank
	} else {
		inlines = []contract.Inline{{
			Kind:  contract.InlineText,
			Range: contract.Range{Start: 0, End: length},
			Next:  1,
		}}
	}
	return &plainAST{
		version: version,
		blocks: [2]contract.Block{
			{
				ID:     1,
				Kind:   contract.BlockRoot,
				Parent: -1,
				Next:   2,
				Range:  contract.Range{Start: 0, End: length},
			},
			{
				ID:      2,
				Kind:    kind,
				Parent:  0,
				Next:    2,
				Range:   contract.Range{Start: 0, End: length},
				Content: contract.Range{Start: 0, End: length},
				Lead:    contract.Range{Start: 0, End: 0},
				Trailer: contract.Range{Start: length, End: length},
			},
		},
		inlines: inlines,
	}
}

func (a *plainAST) Version() contract.Version { return a.version }
func (a *plainAST) Len() int                  { return 2 }
func (a *plainAST) Block(i int) *contract.Block {
	return &a.blocks[i]
}

func (a *plainAST) Index(id contract.BlockID) (int, bool) {
	switch id {
	case 1:
		return 0, true
	case 2:
		return 1, true
	default:
		return 0, false
	}
}

func (a *plainAST) Children(i int) iter.Seq[int] {
	return func(yield func(int) bool) {
		if i == 0 {
			yield(1)
		}
	}
}

func (a *plainAST) Inlines(i int) []contract.Inline {
	if i == 1 {
		return a.inlines
	}
	return nil
}

func (a *plainAST) LeafAt(int) int                        { return 1 }
func (a *plainAST) Path(int) []int                        { return []int{0, 1} }
func (a *plainAST) LeafCount() int                        { return 1 }
func (a *plainAST) Outline() []contract.Heading           { return nil }
func (a *plainAST) FrontMatter() (*contract.Block, bool)  { return nil, false }
func (a *plainAST) Footnotes() map[string]int             { return nil }
func (a *plainAST) LinkDefs() map[string]contract.LinkDef { return nil }

func (a *plainAST) Leaves() iter.Seq[int] {
	return func(yield func(int) bool) {
		yield(1)
	}
}
