package astspike

import (
	"fmt"
	"math/rand"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"aurink.local/spikes/contract-rc/contract"
	"github.com/egoist/mygo/ui/markdown/parser"
)

func equivalent(a, b *snapshot) error {
	if len(a.blocks) != len(b.blocks) {
		return fmt.Errorf("block count %d != %d", len(a.blocks), len(b.blocks))
	}
	for i := range a.blocks {
		x, y := a.blocks[i], b.blocks[i]
		x.ID, y.ID = 0, 0
		if !reflect.DeepEqual(x, y) || !reflect.DeepEqual(a.lines[i], b.lines[i]) {
			return fmt.Errorf("block %d\nincremental=%+v\nfull=%+v\nlines=%+v/%+v", i, x, y, a.lines[i], b.lines[i])
		}
		xs, err := a.inlines(i)
		if err != nil {
			return err
		}
		ys, err := b.inlines(i)
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(xs, ys) {
			return fmt.Errorf("inlines %d\nincremental=%+v\nfull=%+v", i, xs, ys)
		}
	}
	if a.doc.HTML() != b.doc.HTML() {
		return fmt.Errorf("HTML differs")
	}
	return nil
}

func TestRandomEdits(t *testing.T) {
	for _, config := range []struct {
		seed    int64
		guarded bool
	}{{0x533, true}, {20261101, true}, {73, true}, {20261101, false}, {73, false}} {
		seed := config.seed
		t.Run(fmt.Sprintf("%d/guarded=%t", seed, config.guarded), func(t *testing.T) {
			rng := rand.New(rand.NewSource(seed))
			src := "---\r\ntitle: 中文\r\n---\r\n\n" + strings.Repeat("## 标题\n\n段落 **粗体** &amp; \\* [ref]\n续行\n\n> - [ ] 任务\n>   内容\n\n```go\na := 1\n```\n\n", 40) + "[ref]: /old\n"
			e, err := newEngine(src, parser.Default, true)
			if err != nil {
				t.Fatal(err)
			}
			e.guarded = config.guarded
			fragments := []string{"中", "a", " ", "\n", "\r\n", "\r", "> ", "- ", "**", "```\n", "&amp;", "\\*", "[ref]: /new\n", "\n---\n", "🐈", "\t", "| a | b |\n| - | - |\n", "[^x]: 注\n"}
			var survivors, disappeared, changedKinds, insertions, deletions, replacements, fmChanges int
			retired := map[contract.BlockID]bool{}
			for step := 0; step < 10000; step++ {
				old := e.state
				// Select UTF-8 boundaries; CRLF may deliberately be split. The
				// sequence exercises valid source edits, not malformed UTF-8.
				boundaries := []int{0}
				for off := 0; off < len(src); {
					_, n := utf8.DecodeRuneInString(src[off:])
					off += n
					boundaries = append(boundaries, off)
				}
				p := rng.Intn(len(boundaries))
				q := p
				mode := rng.Intn(3)
				if mode != 0 {
					q = min(p+rng.Intn(9)+1, len(boundaries)-1)
				}
				text := fragments[rng.Intn(len(fragments))]
				if mode == 1 && len(src) > 512 {
					text = ""
				}
				edit := contract.TextEdit{Range: contract.Range{Start: boundaries[p], End: boundaries[q]}, Text: text}
				switch {
				case edit.Range.Empty():
					insertions++
				case text == "":
					deletions++
				default:
					replacements++
				}
				src = src[:edit.Range.Start] + text + src[edit.Range.End:]
				if err := e.edit(edit); err != nil {
					t.Fatalf("seed=%d step=%d edit=%+v: %v", seed, step, edit, err)
				}
				if e.state.bodyAt != old.bodyAt {
					fmChanges++
				}
				full, err := parse(src, parser.Default, true)
				if err != nil {
					t.Fatal(err)
				}
				if err := equivalent(e.state, full); err != nil {
					minimal, reduced, reproduced := minimizeEdit(old.source, edit)
					t.Fatalf("seed=%d step=%d edit=%+v: %v\nreproduced=%t reduced_old=%q reduced_edit=%+v", seed, step, edit, err, reproduced, minimal, reduced)
				}
				seen := map[contract.BlockID]bool{}
				for _, b := range e.state.blocks {
					if b.ID == 0 || seen[b.ID] || retired[b.ID] {
						t.Fatalf("invalid/duplicate/reused ID %d at step %d", b.ID, step)
					}
					seen[b.ID] = true
				}
				// Independent oracle: use full-parse bounds, not the engine's
				// matching function, to identify exact surviving source spans.
				delta := len(text) - edit.Range.Len()
				for _, b := range old.blocks {
					a, z := b.Range.Start, b.Range.End
					if z <= edit.Range.Start {
						// Before the edit: unchanged offsets.
					} else if a >= edit.Range.End {
						a, z = a+delta, z+delta
					} else {
						continue
					}
					found, sameKind := false, false
					for j, candidate := range full.blocks {
						if candidate.Kind == b.Kind && candidate.Range.Start == a && candidate.Range.End == z {
							// Empty table cells can have equal ranges. Match IDs as
							// a multiset rather than assigning an arbitrary sibling.
							sameKind = true
							if e.state.blocks[j].ID == b.ID {
								found = true
								break
							}
						}
					}
					if sameKind && !found {
						t.Fatalf("surviving ID changed: seed=%d step=%d block=%+v", seed, step, b)
					}
					if found {
						survivors++
					} else {
						disappeared++
						for _, candidate := range full.blocks {
							if candidate.Range.Start == a && candidate.Kind != b.Kind {
								changedKinds++
								break
							}
						}
					}
				}
				for _, b := range old.blocks {
					if !seen[b.ID] {
						retired[b.ID] = true
					}
				}
			}
			t.Logf("seed=%d steps=10000 full_structure_and_inlines=10000/10000 surviving_untouched_IDs=%d/%d source_disjoint_disappeared=%d changed_kind_or_start=%d insert=%d delete=%d replace=%d frontmatter_transitions=%d code_fallbacks=%d reference_fallbacks=%d parser_edits=%d frontmatter_reparses=%d", seed, survivors, survivors, disappeared, changedKinds, insertions, deletions, replacements, fmChanges, e.codeFallbacks, e.refFallbacks, e.incrementalEdits, e.frontFallbacks)
		})
	}
}

func TestParserReferenceInvalidationWitness(t *testing.T) {
	const src = "[^x]:[^x]\n\r[\n^x]: 注*"
	const from, to = 16, 22
	d := parser.Parse(src)
	for i := 1; i < d.Len(); i++ {
		d.Inlines(i)
	}
	d.Edit(from, to, "\t")
	full := parser.Parse(src[:from] + "\t" + src[to:])
	stale := false
	for i := 1; i < min(d.Len(), full.Len()); i++ {
		if !reflect.DeepEqual(d.Inlines(i), full.Inlines(i)) {
			stale = true
		}
	}
	if !stale {
		t.Fatal("locked-version reference invalidation witness no longer reproduces")
	}
	e, err := newEngine(src, parser.Default, false)
	if err != nil {
		t.Fatal(err)
	}
	for i := range e.state.blocks {
		_, _ = e.state.inlines(i)
	}
	if err := e.edit(contract.TextEdit{Range: contract.Range{Start: from, End: to}, Text: "\t"}); err != nil {
		t.Fatal(err)
	}
	want, err := parse(e.state.source, parser.Default, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := equivalent(e.state, want); err != nil || e.refFallbacks != 1 {
		t.Fatalf("guard fallback=%d equivalence=%v", e.refFallbacks, err)
	}
	t.Log("raw cached reference becomes stale; guarded adapter fallback=1 equivalent=true")
}

func TestParserEditKnownMismatch(t *testing.T) {
	const src = "    e\n容\n``\n---|"
	edit := contract.TextEdit{Range: contract.Range{Start: 6, End: 16}, Text: "\t"}
	d := parser.Parse(src)
	d.Edit(edit.Range.Start, edit.Range.End, edit.Text)
	full := parser.Parse(src[:edit.Range.Start] + edit.Text + src[edit.Range.End:])
	if d.Len() != 3 || full.Len() != 2 || !adjacentIndentedCode(d) {
		t.Fatalf("locked-version witness changed: incremental=%d full=%d", d.Len(), full.Len())
	}
	e, err := newEngine(src, parser.Default, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.edit(edit); err != nil {
		t.Fatal(err)
	}
	want, err := parse(e.state.source, parser.Default, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := equivalent(e.state, want); err != nil || e.codeFallbacks != 1 {
		t.Fatalf("guard fallback=%d equivalence=%v", e.codeFallbacks, err)
	}
	t.Logf("raw parser.Edit blocks=%d full=%d (root included); guarded adapter fallback=1 equivalent=true", d.Len(), full.Len())
}

// minimizeEdit removes source chunks around an edit while preserving a fresh
// parser.Edit/full-parse discrepancy. It runs only when a property fails.
func minimizeEdit(src string, edit contract.TextEdit) (string, contract.TextEdit, bool) {
	bad := func(s string, ed contract.TextEdit) bool {
		e, err := newEngine(s, parser.Default, true)
		if err != nil {
			return false
		}
		for i := range e.state.blocks {
			_, _ = e.state.inlines(i)
		}
		if e.edit(ed) != nil {
			return false
		}
		full, err := parse(e.state.source, parser.Default, true)
		return err == nil && equivalent(e.state, full) != nil
	}
	if !bad(src, edit) {
		return src, edit, false
	}
	for size := len(src) / 2; size > 0; size /= 2 {
		for start := 0; start+size <= len(src); {
			end := start + size
			if start < edit.Range.End && end > edit.Range.Start || !utf8.ValidString(src[:start]+src[end:]) {
				start++
				continue
			}
			candidate := src[:start] + src[end:]
			ed := edit
			if end <= ed.Range.Start {
				ed.Range.Start -= size
				ed.Range.End -= size
			}
			if bad(candidate, ed) {
				src, edit = candidate, ed
			} else {
				start++
			}
		}
	}
	return src, edit, true
}

func TestIdentityBoundariesAndGrammar(t *testing.T) {
	for _, tc := range []struct {
		name, src string
		edit      contract.TextEdit
	}{
		{"insert-before", "a\n\nb\n", contract.TextEdit{Range: contract.Range{Start: 3, End: 3}, Text: "# x\n\n"}},
		{"type-in-paragraph", "abc\n\n末\n", contract.TextEdit{Range: contract.Range{Start: 1, End: 1}, Text: "中"}},
		{"delete-gap-merge", "a\n\nb\n", contract.TextEdit{Range: contract.Range{Start: 1, End: 3}, Text: " "}},
		{"open-fence-swallow", "a\n\nb\n", contract.TextEdit{Range: contract.Range{}, Text: "```\n"}},
		{"remote-definition", "[x]\n\n[x]: /a\n", contract.TextEdit{Range: contract.Range{Start: 10, End: 11}, Text: "b"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e, err := newEngine(tc.src, parser.Default, false)
			if err != nil {
				t.Fatal(err)
			}
			oldID := e.state.blocks[1].ID
			if err := e.edit(tc.edit); err != nil {
				t.Fatal(err)
			}
			full, err := parse(e.state.source, parser.Default, false)
			if err != nil {
				t.Fatal(err)
			}
			if err := equivalent(e.state, full); err != nil {
				t.Fatal(err)
			}
			if tc.name == "type-in-paragraph" && e.state.blocks[1].ID != oldID {
				t.Fatal("typing changed paragraph ID")
			}
			if tc.name == "open-fence-swallow" {
				if len(e.state.blocks) != 2 || e.state.blocks[1].Kind != contract.BlockCode {
					t.Fatal("expected two source-disjoint paragraphs to disappear")
				}
				t.Log("source-disjoint paragraphs disappeared after inserting an open fence; literal contract ID rule is impossible")
			}
		})
	}
}
