package qa

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// Inline math is deleted as the characters it shows: from the end, Backspace
// takes the $ first, then c, b, a and the other $, one key each, the caret
// staying where the deleted character was. Delete from the start does the
// same forwards.
func TestDeletingInlineMathTakesWhatIsThere(t *testing.T) {
	for _, tc := range []struct {
		key   ui.Key
		caret int
		steps []string
		heads []int
	}{
		{ui.KeyBackspace, 7, []string{"x $abc y\n", "x $ab y\n", "x $a y\n", "x $ y\n", "x  y\n"}, []int{6, 5, 4, 3, 2}},
		{ui.KeyDelete, 2, []string{"x abc$ y\n", "x bc$ y\n", "x c$ y\n", "x $ y\n", "x  y\n"}, []int{2, 2, 2, 2, 2}},
	} {
		a := newApp(t)
		a.open("m.md", "x $abc$ y\n")
		a.caret(tc.caret)
		a.animate()
		for i, want := range tc.steps {
			a.key(0, tc.key)
			a.animate()
			head := a.s.ActiveSession().Model().Selection().Primary().Head
			if got := a.text(); got != want || head != tc.heads[i] {
				t.Fatalf("key %v, step %d: %q with the caret at %d, want %q at %d", tc.key, i+1, got, head, want, tc.heads[i])
			}
		}
	}
}
