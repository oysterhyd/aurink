package ui_test

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// TestSlashTodoShowsACheckbox: the slash menu's To-do leaves "- [ ] " and,
// as in Typora, an empty checkbox with the caret after it, not the text
// "[ ]" (BE-FIX-TASK).
func TestSlashTodoShowsACheckbox(t *testing.T) {
	h := openRail(t, "")
	h.typed("/")
	h.tt.Frame()
	h.typed("task")
	h.tt.Frame()
	h.key(ui.KeyEnter)
	h.tt.Frame()
	h.wantSrc("- [ ] ")
	h.wantCaret(6)
	if d := h.e.Display(0); d != "" {
		t.Fatalf("the empty task shows %q", d)
	}
	h.typed("b")
	if d := h.e.Display(0); d != "b" {
		t.Fatalf("typed into the task, it shows %q", d)
	}
}
