package document

import (
	"math/rand"
	"testing"

	"aurink/internal/contract"
)

func referenceMapPosition(off int, right bool, changes []contract.Change) int {
	delta := 0
	for _, c := range changes {
		if off < c.OldStart || off == c.OldStart && !right {
			break
		}
		if off < c.OldEnd {
			if right {
				return c.NewEnd
			}
			return c.NewStart
		}
		delta = c.NewEnd - c.OldEnd
	}
	return off + delta
}

func TestMapPositionMatchesLinearReference(t *testing.T) {
	rng := rand.New(rand.NewSource(1607))
	for trial := 0; trial < 3000; trial++ {
		var changes []contract.Change
		oldEnd, delta := 0, 0
		for count := rng.Intn(25); count > 0; count-- {
			start := oldEnd + rng.Intn(3)
			end := start + rng.Intn(4)
			newStart := start + delta
			newEnd := newStart + rng.Intn(5)
			changes = append(changes, contract.Change{OldStart: start, OldEnd: end, NewStart: newStart, NewEnd: newEnd})
			oldEnd, delta = end, delta+newEnd-newStart-(end-start)
		}
		for off := 0; off <= oldEnd+2; off++ {
			for _, right := range []bool{false, true} {
				got := mapPosition(off, right, changes)
				want := referenceMapPosition(off, right, changes)
				if got != want {
					t.Fatalf("trial %d mapPosition(%d, %v) = %d; want %d; changes=%+v", trial, off, right, got, want, changes)
				}
			}
		}
	}
}
