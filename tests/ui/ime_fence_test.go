package ui_test

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// A backtick from an input method that completes a fence in the middle of a
// document closes it as typing does, in the same undo step (BE-TYPORA-KEYS).
func TestIMEFenceClosesInOneUndoStep(t *testing.T) {
	h := open(t, "x\n\n``\nb")
	h.caret(5)
	h.tt.Compose("`", 1)
	h.tt.Type("`")
	h.tt.Frame()
	h.wantSrc("x\n\n```\n```\nb")
	h.mod(ui.Cmd, ui.KeyZ)
	h.wantSrc("x\n\n``\nb")
}
