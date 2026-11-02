package app

import (
	"testing"

	"aurink/internal/contract"
	"aurink/internal/core/commands"
)

// The menu holds registered commands alone: nothing the app cannot do.
func TestMenuComesFromTheRegistry(t *testing.T) {
	r := commands.New()
	if err := r.Register(contract.Command{ID: contract.CmdFileSave, Title: "save", Run: func() error { return nil }}); err != nil {
		t.Fatal(err)
	}
	var labels []string
	for _, top := range buildMenu(r).Items() {
		for _, it := range top.Submenu {
			// The roles' items of the app and edit menus are the system's.
			if it.Click != nil {
				labels = append(labels, it.Label)
			}
		}
	}
	if len(labels) != 1 {
		t.Fatalf("menu items %q", labels)
	}
}
