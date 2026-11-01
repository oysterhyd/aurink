package document

import (
	"slices"
	"unicode/utf8"

	"aurink/internal/contract"
)

func cloneSelection(s contract.Selection) contract.Selection {
	return contract.Selection{Ranges: slices.Clone(s.Ranges)}
}

func sameSelection(a, b contract.Selection) bool { return slices.Equal(a.Ranges, b.Ranges) }

func validSelection(s contract.Selection, size int) bool {
	for _, r := range s.Ranges {
		if r.Anchor < 0 || r.Head < 0 || r.Anchor > size || r.Head > size {
			return false
		}
	}
	return true
}

func mapSelection(s contract.Selection, changes []contract.Change) contract.Selection {
	out := cloneSelection(s)
	for i, r := range out.Ranges {
		a := mapPosition(r.Anchor, r.Anchor <= r.Head, changes)
		h := mapPosition(r.Head, r.Head <= r.Anchor, changes)
		// A range inside a replaced span collapses instead of changing direction.
		if (r.Anchor < r.Head && a > h) || (r.Anchor > r.Head && a < h) {
			a = max(a, h)
			h = a
		}
		out.Ranges[i] = contract.SelRange{Anchor: a, Head: h}
	}
	return out
}

// keepSelection returns s with its offsets clamped to source, on rune starts.
func keepSelection(s contract.Selection, source string) contract.Selection {
	clamp := func(off int) int {
		off = min(off, len(source))
		for off > 0 && off < len(source) && !utf8.RuneStart(source[off]) {
			off--
		}
		return off
	}
	out := cloneSelection(s)
	for i, r := range out.Ranges {
		out.Ranges[i] = contract.SelRange{Anchor: clamp(r.Anchor), Head: clamp(r.Head)}
	}
	return out
}

func mapPosition(off int, right bool, changes []contract.Change) int {
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
