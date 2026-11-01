// Command s4-look is the look and performance spike of Phase 0. Run it for the
// visual check in REPORT.md. With -stress it drives the real window by itself
// and exits; set MYGO_FRAME_STATS=all to log the frames.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func main() {
	a := &look{}
	flag.BoolVar(&a.dark, "dark", false, "start in Midnight Ink")
	flag.BoolVar(&a.low, "low", false, "start on the low-power path")
	flag.BoolVar(&a.welcome, "welcome", false, "start on the welcome page")
	flag.BoolVar(&a.palette, "palette", false, "start with the command panel open")
	flag.BoolVar(&a.find, "find", false, "start with the find panel open")
	stress := flag.String("stress", "", "drive the window: scroll, pill, panel, theme or idle")
	count := flag.Int("n", 300, "steps of -stress")
	fonts := flag.String("fonts", "", "register the .ttf files of this directory")
	flag.Parse()

	if *fonts != "" {
		files, _ := filepath.Glob(filepath.Join(*fonts, "*.ttf"))
		for _, f := range files {
			data, err := os.ReadFile(f)
			if err != nil {
				log.Fatal(err)
			}
			t0 := time.Now()
			if err := ui.RegisterFont(data, ""); err != nil {
				log.Fatal(err)
			}
			fmt.Printf("font %s: %d bytes, registered in %v\n", filepath.Base(f), len(data), time.Since(t0))
		}
	}

	mygo.App.WhenReady(func() {
		w := mygo.NewWindow(mygo.WindowOptions{
			Title:         "Aurink S4",
			Width:         1280,
			Height:        800,
			TitleBarStyle: mygo.TitleBarHidden,
			Content:       ui.View(a.view),
		})
		if *stress != "" {
			go drive(w, a, *stress, *count)
		}
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

// drive posts one step per refresh through Window.Update, the way real events
// reach the main thread. Each view build is one count.
func drive(w *mygo.Window, a *look, mode string, n int) {
	time.Sleep(time.Second)
	builds := 0
	a.onFrame = func() { builds++ }
	t0 := time.Now()
	for i := range n {
		w.Update(func() {
			switch mode {
			case "scroll":
				a.scroll.Y += 12
			case "pill":
				if i%30 == 0 {
					a.tab = (a.tab + 1) % 3
				}
			case "panel":
				if i%45 == 0 {
					a.palette = !a.palette
				}
			case "theme":
				if i%45 == 0 {
					a.toggleTheme(ui.Point{X: 1100, Y: 22})
				}
			}
		})
		time.Sleep(16 * time.Millisecond)
	}
	fmt.Printf("stress %s: steps=%d builds=%d elapsed=%v\n", mode, n, builds, time.Since(t0).Round(time.Millisecond))
	os.Exit(0)
}
