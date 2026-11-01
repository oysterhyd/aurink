package markdown

import (
	"reflect"
	"strings"
	"sync"
	"testing"

	"aurink/internal/contract"
	"aurink/internal/core/text"
	"github.com/egoist/mygo/ui/markdown/parser"
)

func TestParserEditWitnesses(t *testing.T) {
	t.Run("table boundary", func(t *testing.T) {
		src := "i\n-|\n |\n *🐈u\n-|"
		d := parser.Parse(src)
		d.Edit(5, 14, "")
		next := src[:5] + src[14:]
		a, err := adapt(next, d, 0)
		if err != nil {
			t.Fatal(err)
		}
		full, err := parseAdapted(next, parser.Default, false)
		if err != nil {
			t.Fatal(err)
		}
		if reflect.DeepEqual(a.blocks, full.blocks) {
			t.Fatal("locked parser table witness changed")
		}
		p := mustNew(t, src, 1, 0)
		u, err := p.Apply([]contract.TextEdit{{Range: contract.Range{Start: 5, End: 14}}})
		if err != nil {
			t.Fatal(err)
		}
		if u.Stats.LocalParses != 1 || u.Stats.FullRebuilds != 0 {
			t.Fatalf("fallback %+v", u.Stats)
		}
		if err := equivalent(u.Snapshot, mustNew(t, next, 2, 0).Snapshot()); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("indented code", func(t *testing.T) {
		src := "    e\n容\n``\n---|"
		d := parser.Parse(src)
		d.Edit(6, 16, "\t")
		full := parser.Parse(src[:6] + "\t" + src[16:])
		if d.Len() != 3 || full.Len() != 2 || !adjacentIndentedCode(d) {
			t.Fatal("locked parser witness changed")
		}
		p := mustNew(t, src, 1, 0)
		u, err := p.Apply([]contract.TextEdit{{Range: contract.Range{Start: 6, End: 16}, Text: "\t"}})
		if err != nil {
			t.Fatal(err)
		}
		if u.Stats.LocalParses != 1 || u.Stats.FullRebuilds != 0 {
			t.Fatalf("fallback %+v", u.Stats)
		}
		if err := equivalent(u.Snapshot, mustNew(t, u.Snapshot.text.String(), 2, 0).Snapshot()); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("reference cache", func(t *testing.T) {
		src := "[^x]:[^x]\n\r[\n^x]: 注*"
		d := parser.Parse(src)
		for i := 1; i < d.Len(); i++ {
			d.Inlines(i)
		}
		d.Edit(16, 22, "\t")
		full := parser.Parse(src[:16] + "\t" + src[22:])
		stale := false
		for i := 1; i < min(d.Len(), full.Len()); i++ {
			if !reflect.DeepEqual(d.Inlines(i), full.Inlines(i)) {
				stale = true
			}
		}
		if !stale {
			t.Fatal("locked parser cache witness changed")
		}
		p := mustNew(t, src, 1, 0)
		old := p.Snapshot()
		for i := range old.entries {
			old.Inlines(i)
		}
		u, err := p.Apply([]contract.TextEdit{{Range: contract.Range{Start: 16, End: 22}, Text: "\t"}})
		if err != nil {
			t.Fatal(err)
		}
		if u.Stats.ReferenceFallbacks != 1 {
			t.Fatalf("fallback %+v", u.Stats)
		}
		if err := equivalent(u.Snapshot, mustNew(t, u.Snapshot.text.String(), 2, 0).Snapshot()); err != nil {
			t.Fatal(err)
		}
	})
}

func TestDirtyReferencesAndMovement(t *testing.T) {
	t.Run("unrelated definition users only move", func(t *testing.T) {
		src := "[r]: /old\n[x]: /kept\n\nuse [x]\n\nuse [r]\n"
		p := mustNew(t, src, 1, 0)
		old := p.Snapshot()
		x := old.Block(old.LeafAt(strings.Index(src, "use [x]"))).ID
		r := old.Block(old.LeafAt(strings.Index(src, "use [r]"))).ID
		at := strings.Index(src, "old")
		u, err := p.Apply([]contract.TextEdit{{Range: contract.Range{Start: at, End: at + 3}, Text: "changed"}})
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, id := range u.Dirty {
			if id == x {
				t.Fatal("unrelated reference user marked dirty")
			}
			found = found || id == r
		}
		if !found {
			t.Fatal("changed reference user absent")
		}
	})
	for _, src := range []string{"before words\n\nuse [r]\n\nend words\n\n[r]: /old\n", "before words\n\nuse [^n]\n\nend words\n\n[^n]: old\n"} {
		p := mustNew(t, src, 1, 0)
		old := p.Snapshot()
		use := old.Block(old.LeafAt(strings.Index(src, "use"))).ID
		at := strings.Index(src, "old")
		u, err := p.Apply([]contract.TextEdit{{Range: contract.Range{Start: at, End: at + 3}, Text: "new"}})
		if err != nil {
			t.Fatal(err)
		}
		dirty := false
		for _, id := range u.Dirty {
			dirty = dirty || id == use
		}
		if !dirty {
			t.Fatalf("reference user missing from Dirty %v", u.Dirty)
		}
		if _, ok := u.Snapshot.Index(use); !ok {
			t.Fatal("reference user lost identity")
		}
	}
	p := mustNew(t, "first words\n\nsecond words\n", 1, 0)
	old := p.Snapshot()
	id := old.Block(old.LeafAt(15)).ID
	if old.lazy != nil {
		t.Fatal("eager inline parser")
	}
	u, err := p.Apply([]contract.TextEdit{{Range: contract.Range{Start: 3, End: 3}, Text: "abc"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, dirty := range u.Dirty {
		if dirty == id {
			t.Fatal("shift-only block is dirty")
		}
	}
	for i := 2; i < old.Len(); i++ {
		if old.entries[i].data != u.Snapshot.entries[i].data {
			t.Fatal("unaffected block not shared")
		}
	}
}

func TestOldSnapshotConcurrentReaders(t *testing.T) {
	src := strings.Repeat("paragraph **bold** [r] &amp;\n\n", 40) + "[r]: /old\n"
	p := mustNew(t, src, 1, 0)
	old := p.Snapshot()
	want := mustNew(t, src, 1, 0).Snapshot()
	var wg sync.WaitGroup
	for range 6 {
		wg.Go(func() {
			for range 10 {
				for i := range old.entries {
					_ = old.Block(i)
					_ = old.Inlines(i)
					_, _ = old.Index(old.Block(i).ID)
				}
				_ = old.Outline()
				_ = old.LinkDefs()
				_ = old.Footnotes()
				_ = old.Path(20)
			}
		})
	}
	for i := 0; i < 100; i++ {
		_, err := p.Apply([]contract.TextEdit{{Range: contract.Range{Start: 3, End: 4}, Text: string(byte('a' + i%2))}})
		if err != nil {
			t.Fatal(err)
		}
	}
	wg.Wait()
	if err := equivalent(old, want); err != nil {
		t.Fatal(err)
	}
}

func TestUpdateWorkDoesNotScaleWithUntouchedBlocks(t *testing.T) {
	var small int
	for _, n := range []int{10, 10000} {
		p := mustNew(t, strings.Repeat("paragraph **bold** words\n\n", n), 1, 0)
		// Populate inline caches first: editing must not copy their nodes.
		for i := range p.Snapshot().Leaves() {
			p.Snapshot().Inlines(i)
		}
		u, err := p.Apply([]contract.TextEdit{{Range: contract.Range{Start: 3, End: 4}, Text: "x"}})
		if err != nil {
			t.Fatal(err)
		}
		if n == 10 {
			small = u.Stats.AdaptedBlocks
		} else if u.Stats.AdaptedBlocks != small {
			t.Fatalf("adapted blocks grew from %d to %d", small, u.Stats.AdaptedBlocks)
		}
		if u.Stats.AdaptedBlocks > 2 {
			t.Fatalf("adapted %d blocks", u.Stats.AdaptedBlocks)
		}
		if u.Snapshot.lazy != nil {
			t.Fatal("inline parser rebuilt on update")
		}
	}
}

func TestLocalStructuralReparse(t *testing.T) {
	p := mustNew(t, strings.Repeat("paragraph **bold** words\n\n", 50), 1, 0)
	for _, edit := range []contract.TextEdit{{Range: contract.Range{Start: 8, End: 8}, Text: "\n\n"}, {Range: contract.Range{Start: 8, End: 10}, Text: ""}, {Range: contract.Range{}, Text: "# "}} {
		u, err := p.Apply([]contract.TextEdit{edit})
		if err != nil {
			t.Fatal(err)
		}
		if u.Stats.LocalParses != 1 {
			t.Fatalf("expected local parse: %+v", u.Stats)
		}
		if err := equivalent(u.Snapshot, mustNew(t, u.Snapshot.text.String(), u.Snapshot.Version(), 0).Snapshot()); err != nil {
			t.Fatal(err)
		}
		checkCoverage(t, u.Snapshot)
	}
}

func TestValidationAndBatchChanges(t *testing.T) {
	t.Run("same bytes keep all records clean", func(t *testing.T) {
		p := mustNew(t, "alpha words\n", 1, 0)
		old := p.Snapshot()
		u, err := p.Apply([]contract.TextEdit{{Range: contract.Range{Start: 1, End: 2}, Text: "l"}})
		if err != nil {
			t.Fatal(err)
		}
		if len(u.Dirty) != 0 || u.Snapshot.Version() != 2 || u.Stats.AdaptedBlocks != 0 {
			t.Fatal(u)
		}
		for i := range old.entries {
			if old.entries[i].data != u.Snapshot.entries[i].data {
				t.Fatal("equal-byte edit rebuilt data")
			}
		}
	})
	if _, err := New("", 0, 0); contract.CodeOf(err) != contract.CodeInvalid {
		t.Fatal(err)
	}
	p := mustNew(t, "alpha words\n\nbeta words\n\ngamma words\n", 5, 0)
	old := p.Snapshot()
	for _, edits := range [][]contract.TextEdit{{{Range: contract.Range{Start: -1}}}, {{Range: contract.Range{End: 999}}}, {{Range: contract.Range{Start: 4, End: 3}}}} {
		if _, err := p.Apply(edits); contract.CodeOf(err) != contract.CodeInvalid {
			t.Fatal(err)
		}
	}
	if u, err := p.Apply(nil); err != nil || u.Snapshot != old {
		t.Fatal("empty apply")
	}
	for _, cs := range [][]contract.Change{{{OldStart: -1}}, {{OldStart: 1, OldEnd: 0}}, {{OldEnd: 999}}, {{NewStart: -1}}, {{NewEnd: 999}}} {
		if _, err := p.Update(text.New("x", 6), cs); contract.CodeOf(err) != contract.CodeInvalid {
			t.Fatal(err)
		}
	}
	if _, err := p.Update(nil, nil); contract.CodeOf(err) != contract.CodeInvalid {
		t.Fatal(err)
	}
	if _, err := p.Update(text.New("x", 5), nil); contract.CodeOf(err) != contract.CodeInvalid {
		t.Fatal(err)
	}
	if p.Snapshot() != old {
		t.Fatal("invalid input changed state")
	}
	next, cs, err := old.text.Apply([]contract.TextEdit{{Range: contract.Range{Start: 1, End: 2}, Text: "XX"}, {Range: contract.Range{Start: 16, End: 19}, Text: ""}})
	if err != nil {
		t.Fatal(err)
	}
	u, err := p.Update(next, cs)
	if err != nil {
		t.Fatal(err)
	}
	if err := equivalent(u.Snapshot, mustNew(t, next.String(), 6, 0).Snapshot()); err != nil {
		t.Fatal(err)
	}
	if old.Block(-1) != nil || old.Block(old.Len()) != nil || old.Inlines(-1) != nil {
		t.Fatal("invalid index")
	}
	for range old.Children(-1) {
		t.Fatal("invalid children")
	}
	for range old.Children(0) {
		break
	}
	for range old.Leaves() {
		break
	}
	if _, ok := old.Index(0); ok {
		t.Fatal("zero ID")
	}
}

func TestDefinitionChangesAcrossWindows(t *testing.T) {
	src := "use [r] [^n]\n\n| [r] | [^n] |\n|---|---|\n\n" + strings.Repeat("padding words\n\n", 20) + "[r]: /old\n\n[^n]:\n\n" + strings.Repeat("after words\n\n", 20) + "[r]: /duplicate\n"
	link, note := strings.Index(src, "[r]: /old"), strings.Index(src, "[^n]:")
	for name, ed := range map[string]contract.TextEdit{
		"replace first link": {Range: contract.Range{Start: link + 6, End: link + 9}, Text: "new"},
		"remove first link":  {Range: contract.Range{Start: link, End: link + len("[r]: /old\n\n")}},
		"add first link":     {Text: "[r]: /first\n\n"},
		"rename empty note":  {Range: contract.Range{Start: note + 2, End: note + 3}, Text: "x"},
		"remove empty note":  {Range: contract.Range{Start: note, End: note + len("[^n]:\n\n")}},
	} {
		t.Run(name, func(t *testing.T) {
			p := mustNew(t, src, 1, 0)
			old := p.Snapshot()
			u, err := p.Apply([]contract.TextEdit{ed})
			if err != nil {
				t.Fatal(err)
			}
			if u.Stats.FullRebuilds != 0 {
				t.Fatalf("definition change rebuilt document: %+v", u.Stats)
			}
			if err := equivalent(u.Snapshot, mustNew(t, u.Snapshot.text.String(), 2, 0).Snapshot()); err != nil {
				t.Fatal(err)
			}
			checkDirty(t, old, u)
		})
	}
}
