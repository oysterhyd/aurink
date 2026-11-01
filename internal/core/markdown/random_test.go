package markdown

import (
	"fmt"
	"math/rand"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"aurink/internal/contract"
)

var fragments = []string{"a", "中", "🐈", " ", "\n", "\n\n", "\r\n", "\t", "**", "*", "`", "```\n", "> ", "- ", "1. ", "# ", "| a |\n|---|\n", "[r]: /u\n", "[^x]: y\n", "[r]", "---\n", "$$\n", "<div>\n", "\\*", "&amp;"}

func randomEdit(rng *rand.Rand, src string) contract.TextEdit {
	boundary := func(i int) int {
		for i > 0 && i < len(src) && !utf8.RuneStart(src[i]) {
			i--
		}
		return i
	}
	start := boundary(rng.Intn(len(src) + 1))
	end := start
	if rng.Intn(2) == 0 {
		end = max(start, boundary(min(len(src), start+rng.Intn(12))))
	}
	t := ""
	if start == end || rng.Intn(2) == 0 {
		t = fragments[rng.Intn(len(fragments))]
	}
	return contract.TextEdit{Range: contract.Range{Start: start, End: end}, Text: t}
}

func equivalent(a, b *Snapshot) error {
	if a.Len() != b.Len() {
		return fmt.Errorf("blocks %d != %d", a.Len(), b.Len())
	}
	for i := 0; i < a.Len(); i++ {
		x, y := *a.Block(i), *b.Block(i)
		x.ID, y.ID = 0, 0
		if !reflect.DeepEqual(x, y) {
			return fmt.Errorf("block %d: %+v != %+v", i, x, y)
		}
		if x, y := a.Inlines(i), b.Inlines(i); !reflect.DeepEqual(x, y) {
			return fmt.Errorf("inlines %d: %+v != %+v", i, x, y)
		}
	}
	return nil
}

func checkDirty(t testing.TB, old *Snapshot, u Update) {
	t.Helper()
	dirty := make(map[contract.BlockID]bool, len(u.Dirty))
	for _, id := range u.Dirty {
		dirty[id] = true
	}
	s := u.Snapshot
	for i := range s.entries {
		b := s.Block(i)
		j, ok := old.Index(b.ID)
		if !ok {
			if !dirty[b.ID] {
				t.Fatalf("new block %d missing from Dirty", b.ID)
			}
			continue
		}
		before := old.Block(j)
		x, y := moveBlock(*before, -before.Range.Start), moveBlock(*b, -b.Range.Start)
		x.ID, y.ID = 0, 0
		x.Parent, y.Parent = 0, 0
		x.Next -= j
		y.Next -= i
		parentChanged := before.Parent >= 0 && b.Parent >= 0 && old.entries[before.Parent].id != s.entries[b.Parent].id
		changed := parentChanged || !reflect.DeepEqual(x, y) || old.text.Slice(before.Range) != s.text.Slice(b.Range) || !reflect.DeepEqual(inlineDump(old, j), inlineDump(s, i))
		if changed && !dirty[b.ID] {

			t.Fatalf("changed block %d (old %d new %d) missing from Dirty: old=%+v new=%+v parent=%v oldSource=%q newSource=%q stats=%+v", b.ID, j, i, x, y, parentChanged, old.text.String(), s.text.String(), u.Stats)
		}
	}
}

func checkCoverage(t *testing.T, s *Snapshot) {
	t.Helper()
	at := 0
	for i := range s.Leaves() {
		b := s.Block(i)
		if b.Lead.Start != at || b.Lead.End != b.Range.Start || b.Trailer.Start != b.Range.End || b.Trailer.End < b.Range.End {
			t.Fatalf("coverage at %d: %+v source=%q", at, b, s.text.String())
		}
		for off := at; off < b.Trailer.End; off++ {
			if s.LeafAt(off) != i {
				t.Fatalf("LeafAt %d", off)
			}
		}
		for _, r := range []contract.Range{b.Lead, b.Trailer} {
			if !structuralRe.MatchString(footnoteRe.ReplaceAllString(s.text.Slice(r), "")) {
				t.Fatalf("hidden text %q kind=%d source=%q", s.text.Slice(r), b.Kind, s.text.String())
			}
		}
		at = b.Trailer.End
	}
	if at != s.text.Len() {
		t.Fatalf("coverage end %d", at)
	}
}

func TestRandomEdits(t *testing.T) {
	for _, seed := range []int64{1331, 20261101, 73} {
		t.Run(fmt.Sprint(seed), func(t *testing.T) {
			rng := rand.New(rand.NewSource(seed))
			p := mustNew(t, "# 标题\n\n正文 **粗** [r]\n\n> 引用\n\n- 项\n\n```\n码\n```\n\n[r]: /u\n", 1, contract.ExtFrontMatter)
			retired := map[contract.BlockID]bool{}
			var total Stats
			var survivorChecks, disappeared int
			for step := 0; step < longSteps(1000, 10000); step++ {
				old := p.Snapshot()
				src := old.text.String()
				ed := randomEdit(rng, src)
				u, err := p.Apply([]contract.TextEdit{ed})
				if err != nil {
					t.Fatal(err)
				}
				addStats(&total, u.Stats)
				s := u.Snapshot
				checkDirty(t, old, u)
				full := mustNew(t, s.text.String(), s.Version(), s.exts).Snapshot()
				if err := equivalent(s, full); err != nil {
					t.Fatalf("seed=%d step=%d old=%q edit=%+v: %v", seed, step, src, ed, err)
				}
				checkCoverage(t, s)
				seen := map[contract.BlockID]bool{}
				for i := range s.entries {
					b := s.Block(i)
					if b.ID == 0 || seen[b.ID] || retired[b.ID] {
						t.Fatalf("ID %d at %d", b.ID, step)
					}
					seen[b.ID] = true
					if j, ok := s.Index(b.ID); !ok || j != i {
						t.Fatal("index")
					}
				}
				slots := map[identityKey]int{}
				for j := range full.entries {
					b := full.Block(j)
					slots[identityKey{b.Kind, b.Range.Start, b.Range.End}]++
				}
				for i := range old.entries {
					b := old.Block(i)
					// Independent source-disjoint survivor oracle, including
					// duplicate empty-cell ranges matched as ordered candidates.
					a, z := b.Range.Start, b.Range.End
					untouched := true
					if z <= ed.Range.Start {
					} else if a >= ed.Range.End {
						d := len(ed.Text) - ed.Range.Len()
						a += d
						z += d
					} else {
						untouched = false
					}
					if untouched {
						key := identityKey{b.Kind, a, z}
						if slots[key] > 0 {
							slots[key]--
							j, ok := s.Index(b.ID)
							if !ok || s.Block(j).Kind != b.Kind || s.Block(j).Range != (contract.Range{Start: a, End: z}) {
								t.Fatalf("survivor lost ID %d at %d block=%+v old=%q edit=%+v", b.ID, step, b, src, ed)
							}
							survivorChecks++
						} else {
							disappeared++
						}
					}
					if !seen[b.ID] {
						retired[b.ID] = true
					}
				}
			}
			t.Logf("survivors=%d, disjoint disappearances=%d; counts: %+v", survivorChecks, disappeared, total)
		})
	}
}

func TestTypingPath(t *testing.T) {
	p := mustNew(t, strings.Repeat("paragraph **bold** words\n\n", 100), 1, 0)
	u, err := p.Apply([]contract.TextEdit{{Range: contract.Range{Start: 3, End: 3}, Text: "中"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := equivalent(u.Snapshot, mustNew(t, u.Snapshot.text.String(), 2, 0).Snapshot()); err != nil {
		t.Fatal(err)
	}
}

func inlineDump(s *Snapshot, i int) []contract.Inline {
	out := append([]contract.Inline(nil), s.Inlines(i)...)
	shiftInlines(out, -s.entries[i].start)
	return out
}

func addStats(total *Stats, s Stats) {
	total.AdaptedBlocks += s.AdaptedBlocks
	total.FullRebuilds += s.FullRebuilds
	total.BoundaryFallbacks += s.BoundaryFallbacks
	total.WindowLimitFallbacks += s.WindowLimitFallbacks
	total.ReferenceFallbacks += s.ReferenceFallbacks
	total.FrontMatterFallbacks += s.FrontMatterFallbacks
	total.LocalParses += s.LocalParses
}
