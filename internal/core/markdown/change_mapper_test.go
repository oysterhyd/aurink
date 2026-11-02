package markdown

import (
	"math/rand"
	"testing"

	"aurink/internal/contract"
)

// These are the original linear mappings, retained as the equivalence oracle.
func referenceMapped(r contract.Range, cs []contract.Change) (contract.Range, bool) {
	delta := 0
	for _, c := range cs {
		if r.End <= c.OldStart {
			break
		}
		if r.Start >= c.OldEnd {
			delta += (c.NewEnd - c.NewStart) - (c.OldEnd - c.OldStart)
			continue
		}
		return contract.Range{}, false
	}
	return moveRange(r, delta), true
}

func referenceMappedStart(at int, cs []contract.Change) (int, bool) {
	delta := 0
	for _, c := range cs {
		if at > c.OldStart && at < c.OldEnd {
			return 0, false
		}
		if at >= c.OldEnd && at > c.OldStart {
			delta += (c.NewEnd - c.NewStart) - (c.OldEnd - c.OldStart)
		}
	}
	return at + delta, true
}

func TestChangeMapperBoundaries(t *testing.T) {
	replacement := []contract.Change{{OldStart: 3, OldEnd: 6, NewStart: 3, NewEnd: 7}}
	insertion := []contract.Change{{OldStart: 3, OldEnd: 3, NewStart: 3, NewEnd: 5}}
	cases := []struct {
		name      string
		changes   []contract.Change
		rangeIn   contract.Range
		rangeWant contract.Range
		rangeOK   bool
		at        int
		startWant int
		startOK   bool
	}{
		{"touch replacement start", replacement, contract.Range{Start: 0, End: 3}, contract.Range{Start: 0, End: 3}, true, 3, 3, true},
		{"touch replacement end", replacement, contract.Range{Start: 6, End: 9}, contract.Range{Start: 7, End: 10}, true, 6, 7, true},
		{"inside replacement", replacement, contract.Range{Start: 4, End: 5}, contract.Range{}, false, 4, 0, false},
		{"empty at start", replacement, contract.Range{Start: 3, End: 3}, contract.Range{Start: 3, End: 3}, true, 3, 3, true},
		{"empty inside", replacement, contract.Range{Start: 4, End: 4}, contract.Range{}, false, 4, 0, false},
		{"empty at end", replacement, contract.Range{Start: 6, End: 6}, contract.Range{Start: 7, End: 7}, true, 6, 7, true},
		{"insertion at block start", insertion, contract.Range{Start: 3, End: 5}, contract.Range{Start: 5, End: 7}, true, 3, 3, true},
		{"insertion at block end", insertion, contract.Range{Start: 1, End: 3}, contract.Range{Start: 1, End: 3}, true, 4, 6, true},
		{"insertion inside block", insertion, contract.Range{Start: 2, End: 4}, contract.Range{}, false, 4, 6, true},
		{"repeated insertions", []contract.Change{{OldStart: 3, OldEnd: 3, NewStart: 3, NewEnd: 5}, {OldStart: 3, OldEnd: 3, NewStart: 5, NewEnd: 6}}, contract.Range{Start: 3, End: 5}, contract.Range{Start: 6, End: 8}, true, 3, 3, true},
		{"deletion then insertion", []contract.Change{{OldStart: 0, OldEnd: 3, NewStart: 0, NewEnd: 0}, {OldStart: 3, OldEnd: 3, NewStart: 0, NewEnd: 2}}, contract.Range{Start: 3, End: 5}, contract.Range{Start: 2, End: 4}, true, 3, 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := newChangeMapper(tc.changes)
			if got, ok := m.mapped(tc.rangeIn); got != tc.rangeWant || ok != tc.rangeOK {
				t.Fatalf("mapped(%v) = %v, %v; want %v, %v", tc.rangeIn, got, ok, tc.rangeWant, tc.rangeOK)
			}
			if got, ok := m.mappedStart(tc.at); got != tc.startWant || ok != tc.startOK {
				t.Fatalf("mappedStart(%d) = %d, %v; want %d, %v", tc.at, got, ok, tc.startWant, tc.startOK)
			}
		})
	}
}

func TestChangeMapperMatchesLinearReference(t *testing.T) {
	rng := rand.New(rand.NewSource(1606))
	for trial := 0; trial < 1000; trial++ {
		var changes []contract.Change
		oldEnd, delta := 0, 0
		for count := rng.Intn(16); count > 0; count-- {
			start := oldEnd + rng.Intn(3)
			end := start + rng.Intn(4)
			newStart := start + delta
			newEnd := newStart + rng.Intn(5)
			changes = append(changes, contract.Change{OldStart: start, OldEnd: end, NewStart: newStart, NewEnd: newEnd})
			oldEnd, delta = end, delta+newEnd-newStart-(end-start)
		}
		m := newChangeMapper(changes)
		for start := 0; start <= oldEnd+2; start++ {
			gotStart, ok := m.mappedStart(start)
			wantStart, wantOK := referenceMappedStart(start, changes)
			if gotStart != wantStart || ok != wantOK {
				t.Fatalf("trial %d mappedStart(%d): got %d, %v; want %d, %v; changes=%+v", trial, start, gotStart, ok, wantStart, wantOK, changes)
			}
			for end := start; end <= oldEnd+2; end++ {
				r := contract.Range{Start: start, End: end}
				got, ok := m.mapped(r)
				want, wantOK := referenceMapped(r, changes)
				if got != want || ok != wantOK {
					t.Fatalf("trial %d mapped(%v): got %v, %v; want %v, %v; changes=%+v", trial, r, got, ok, want, wantOK, changes)
				}
			}
		}
	}
}
