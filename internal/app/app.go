package app

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	// The languages of the catalogs become the app's.
	_ "aurink/locales"
)

// Run opens the main window and blocks until the app quits.
func Run() error {
	mygo.App.WhenReady(func() {
		mygo.NewWindow(mygo.WindowOptions{
			Title:     "Aurink",
			Width:     1100,
			Height:    720,
			MinWidth:  640,
			MinHeight: 480,
			StateKey:  "main",
			Content:   ui.View(view),
		})
	})
	return mygo.App.Run()
}

// view is the empty window until the shell is wired in (INT-1.1).
func view(c *ui.Context) {
	ui.Column(c).Fill()
}
