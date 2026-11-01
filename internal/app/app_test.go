package app

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// The empty window settles: it asks for no further frames while idle.
func TestViewIsIdle(t *testing.T) {
	tt := ui.NewTester(view, 1100, 720)
	tt.Frame()
	if tt.FramePending() {
		t.Error("the empty window keeps asking for frames")
	}
}
