package text

import (
	"fmt"
	"math/rand"
	"slices"
	"strings"
	"sync"
	"testing"

	"aurink/internal/contract"
)

func TestApply(t *testing.T) {
	for _, tt := range []struct {
		name, before, after string
		edits               []contract.TextEdit
		changes             []contract.Change
	}{
		{
			name: "insert into empty", after: "中文🙂\r\n",
			edits:   []contract.TextEdit{{Text: "中文🙂\r\n"}},
			changes: []contract.Change{{NewEnd: 12}},
		},
		{
			name: "delete all", before: "中文🙂\r\n",
			edits:   []contract.TextEdit{{Range: contract.Range{End: 12}}},
			changes: []contract.Change{{OldEnd: 12}},
		},
		{
			name: "mixed newlines", before: "甲\r\n乙\n🙂\r丙", after: "甲\n乙\r\n🐈\r丙!",
			edits: []contract.TextEdit{
				{Range: contract.Range{Start: 3, End: 5}, Text: "\n"},
				{Range: contract.Range{Start: 8, End: 9}, Text: "\r\n"},
				{Range: contract.Range{Start: 9, End: 13}, Text: "🐈"},
				{Range: contract.Range{Start: 17, End: 17}, Text: "!"},
			},
			changes: []contract.Change{
				{OldStart: 3, OldEnd: 5, NewStart: 3, NewEnd: 4},
				{OldStart: 8, OldEnd: 9, NewStart: 7, NewEnd: 9},
				{OldStart: 9, OldEnd: 13, NewStart: 9, NewEnd: 13},
				{OldStart: 17, OldEnd: 17, NewStart: 17, NewEnd: 18},
			},
		},
		{
			name: "adjacent and repeated insertions", before: "abcd", after: "AB12CD!",
			edits: []contract.TextEdit{
				{Range: contract.Range{End: 1}, Text: "A"},
				{Range: contract.Range{Start: 1, End: 2}, Text: "B"},
				{Range: contract.Range{Start: 2, End: 2}, Text: "1"},
				{Range: contract.Range{Start: 2, End: 2}, Text: "2"},
				{Range: contract.Range{Start: 2, End: 4}, Text: "CD"},
				{Range: contract.Range{Start: 4, End: 4}, Text: "!"},
			},
			changes: []contract.Change{
				{OldStart: 0, OldEnd: 1, NewStart: 0, NewEnd: 1},
				{OldStart: 1, OldEnd: 2, NewStart: 1, NewEnd: 2},
				{OldStart: 2, OldEnd: 2, NewStart: 2, NewEnd: 3},
				{OldStart: 2, OldEnd: 2, NewStart: 3, NewEnd: 4},
				{OldStart: 2, OldEnd: 4, NewStart: 4, NewEnd: 6},
				{OldStart: 4, OldEnd: 4, NewStart: 6, NewEnd: 7},
			},
		},
		{
			name: "split UTF-8 and CRLF", before: "🙂\r\n", after: "\xf0X\x99\x82\n",
			edits: []contract.TextEdit{
				{Range: contract.Range{Start: 1, End: 2}, Text: "X"},
				{Range: contract.Range{Start: 4, End: 5}},
			},
			changes: []contract.Change{
				{OldStart: 1, OldEnd: 2, NewStart: 1, NewEnd: 2},
				{OldStart: 4, OldEnd: 5, NewStart: 4, NewEnd: 4},
			},
		},
		{
			name: "unchanged bytes still advance version", before: "a\n", after: "a\n",
			edits:   []contract.TextEdit{{Range: contract.Range{End: 1}, Text: "a"}},
			changes: []contract.Change{{OldStart: 0, OldEnd: 1, NewStart: 0, NewEnd: 1}},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			old := New(tt.before, 8)
			checkText(t, old, tt.before)
			oldLines := slices.Clone(old.lines)
			edits := slices.Clone(tt.edits)
			next, changes, err := old.Apply(tt.edits)
			if err != nil {
				t.Fatal(err)
			}
			if next == old || next.Version() != 9 {
				t.Fatal("Apply did not return a new snapshot at the next version")
			}
			if next.lines != nil || next.lineBuilds != 0 {
				t.Fatal("new snapshot retained or eagerly built a line index")
			}
			if !slices.Equal(changes, tt.changes) {
				t.Fatalf("Changes = %v, want %v", changes, tt.changes)
			}
			checkText(t, next, tt.after)
			checkText(t, old, tt.before)
			if old.Version() != 8 || !slices.Equal(old.lines, oldLines) || !slices.Equal(tt.edits, edits) {
				t.Fatal("Apply changed the source snapshot or edits")
			}
		})
	}
}

func TestApplyInvalid(t *testing.T) {
	for _, tt := range []struct {
		name  string
		edits []contract.TextEdit
	}{
		{"negative start", []contract.TextEdit{{Range: contract.Range{Start: -1}}}},
		{"negative end", []contract.TextEdit{{Range: contract.Range{End: -1}}}},
		{"reversed range", []contract.TextEdit{{Range: contract.Range{Start: 3, End: 2}}}},
		{"end past EOF", []contract.TextEdit{{Range: contract.Range{End: 7}}}},
		{"insertion past EOF", []contract.TextEdit{{Range: contract.Range{Start: 7, End: 7}}}},
		{"unsorted insertions", []contract.TextEdit{{Range: contract.Range{Start: 4, End: 4}}, {Range: contract.Range{Start: 1, End: 1}}}},
		{"overlap", []contract.TextEdit{{Range: contract.Range{End: 3}}, {Range: contract.Range{Start: 2, End: 4}}}},
		{"insertion inside replacement", []contract.TextEdit{{Range: contract.Range{End: 3}}, {Range: contract.Range{Start: 1, End: 1}}}},
		{"invalid after valid", []contract.TextEdit{{Text: "valid\n"}, {Range: contract.Range{Start: 4, End: 9}}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			old := New("a\r\nb\nc", 5)
			edits := slices.Clone(tt.edits)
			next, changes, err := old.Apply(tt.edits)
			ce, ok := err.(*contract.Error)
			if !ok || ce.Code != contract.CodeInvalid {
				t.Fatalf("error = %#v, want *contract.Error with CodeInvalid", err)
			}
			if next != nil || changes != nil {
				t.Fatal("invalid edits returned a partial result")
			}
			if old.lineBuilds != 0 || old.lines != nil || !slices.Equal(tt.edits, edits) || old.Version() != 5 {
				t.Fatal("invalid edits changed the snapshot or input")
			}
			checkText(t, old, "a\r\nb\nc")
		})
	}
}

func TestApplyEmptyAndVersionLimit(t *testing.T) {
	old := New("a\n", ^contract.Version(0))
	for _, edits := range [][]contract.TextEdit{nil, {}} {
		next, changes, err := old.Apply(edits)
		if next != old || changes != nil || err != nil || old.lineBuilds != 0 {
			t.Fatal("empty edits did not preserve the snapshot")
		}
	}
	next, changes, err := old.Apply([]contract.TextEdit{{Text: "x"}})
	ce, ok := err.(*contract.Error)
	if next != nil || changes != nil || !ok || ce.Code != contract.CodeInvalid {
		t.Fatalf("version exhaustion: next = %v, changes = %v, error = %v", next, changes, err)
	}
}

func TestApplyStringModel(t *testing.T) {
	for _, seed := range []int64{1, 20261101, 0x5eed} {
		t.Run(fmt.Sprint(seed), func(t *testing.T) {
			rng := rand.New(rand.NewSource(seed))
			model := "中文🙂\r\nfirst\nsecond\rthird"
			x := New(model, 1)
			pieces := []string{"", "a", "abc", "中文", "🙂", "👩‍💻", "\n", "\r\n", "\r", "\x00\xff"}
			for step := range 10000 {
				edits := make([]contract.TextEdit, rng.Intn(6))
				end := 0
				for i := range edits {
					start := end + rng.Intn(len(model)-end+1)
					end = start + rng.Intn(len(model)-start+1)
					edits[i] = contract.TextEdit{
						Range: contract.Range{Start: start, End: end},
						Text:  pieces[rng.Intn(len(pieces))],
					}
				}
				want := model
				for i := len(edits) - 1; i >= 0; i-- {
					e := edits[i]
					want = want[:e.Range.Start] + e.Text + want[e.Range.End:]
				}
				next, changes, err := x.Apply(edits)
				if err != nil {
					t.Fatalf("step %d: %v", step, err)
				}
				wantVersion := x.Version()
				if len(edits) > 0 {
					wantVersion++
				}
				if next.Version() != wantVersion || len(changes) != len(edits) {
					t.Fatalf("step %d: version = %d, changes = %v", step, next.Version(), changes)
				}
				shift := 0
				for i, e := range edits {
					start := e.Range.Start + shift
					wantChange := contract.Change{OldStart: e.Range.Start, OldEnd: e.Range.End, NewStart: start, NewEnd: start + len(e.Text)}
					if changes[i] != wantChange {
						t.Fatalf("step %d: Change = %v, want %v", step, changes[i], wantChange)
					}
					shift += len(e.Text) - e.Range.Len()
				}
				checkText(t, next, want)
				checkText(t, x, model)
				x, model = next, want
			}
		})
	}
}

func TestConcurrentSnapshotsAndEdits(t *testing.T) {
	s := strings.Repeat("中文🙂\r\n", 1000)
	old := New(s, 1)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for worker := range 16 {
		wg.Go(func() {
			<-start
			for j := range 100 {
				off := (worker*97 + j) % len(s)
				if got := old.LineOf(off); got != strings.Count(s[:off], "\n") {
					t.Errorf("old LineOf(%d) = %d", off, got)
					return
				}
				next, _, err := old.Apply([]contract.TextEdit{{Range: contract.Range{End: 12}, Text: "new\n"}})
				if err != nil {
					t.Error(err)
					return
				}
				if next.String() != "new\n"+s[12:] || next.LineStart(1) != 4 || next.Version() != 2 {
					t.Error("new snapshot has incorrect bytes, lines, or version")
					return
				}
			}
		})
	}
	close(start)
	wg.Wait()
	checkText(t, old, s)
	if old.lineBuilds != 1 {
		t.Fatalf("old line index built %d times, want 1", old.lineBuilds)
	}
}
