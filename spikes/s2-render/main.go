package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

type app struct {
	left, right *ui.Bitmap
	sample      *ui.SVG
	caption     string
}

func (a *app) view(c *ui.Context) {
	ui.Column(c).Fill().Padding(16).Gap(10).Background(ui.RGB(255, 255, 255)).TextColor(ui.RGB(17, 17, 17)).Children(func() {
		ui.Text(c, "Aurink S2 · MathJax → ParseSVG").FontSize(22).Bold()
		ui.Text(c, "LEFT: browser SVG/canvas reference     RIGHT: MyGo Native SVG raster").FontSize(14)
		ui.Text(c, a.caption).FontSize(13)
		if a.sample != nil {
			ui.Image(c, a.sample).Size(80, 48).Label("Direct Native SVG matrix")
		}
		ui.Row(c).Gap(20).Children(func() {
			if a.left != nil {
				ui.Image(c, a.left).Width(540).Height(450).Fit(ui.Contain)
			}
			if a.right != nil {
				ui.Image(c, a.right).Width(540).Height(450).Fit(ui.Contain)
			}
		})
		ui.Text(c, "Renderer WebView has never been placed in a ui.WebView element.").FontSize(13)
	})
}

func main() {
	output := flag.String("out", "evidence", "evidence directory")
	forceUnavailable := flag.Bool("force-unavailable", false, "exercise source fallback without creating WebView")
	flag.Parse()
	if err := os.MkdirAll(*output, 0755); err != nil {
		log.Fatal(err)
	}
	a := &app{caption: "Starting offscreen renderer…"}
	var runErr error
	mygo.App.WhenReady(func() {
		win := mygo.NewWindow(mygo.WindowOptions{Title: "Aurink S2 render probe", Width: 1180, Height: 820,
			Hidden: true, Content: ui.View(a.view)})
		win.ShowInactive()
		start := time.Now()
		if *forceUnavailable || !mygo.WebViewAvailable() {
			a.caption = `WebView unavailable · source fallback: \frac{a}{b} and Mermaid source`
			go func() {
				defer mygo.App.Quit()
				_, err := newRenderer(nil).render(context.Background(), `\frac{a}{b}`)
				runErr = saveJSON(filepath.Join(*output, "fallback.json"), map[string]any{
					"forced": *forceUnavailable, "error": fmt.Sprint(err), "webview_created": false})
				if png, err := win.CapturePage(); err == nil {
					runErr = os.WriteFile(filepath.Join(*output, "fallback-window.png"), png, 0644)
				} else {
					runErr = err
				}
			}()
			return
		}
		view, err := win.NewWebView(mygo.WebViewOptions{Page: mygo.PageOptions{BackgroundThrottling: mygo.ThrottlingOff}})
		if err != nil {
			runErr = fmt.Errorf("NewWebView: %w", err)
			mygo.App.Quit()
			return
		}
		page := view.Page()
		loaded := make(chan error, 1)
		var once sync.Once
		page.OnDidFinishLoad(func() { once.Do(func() { loaded <- nil }) })
		page.OnDidFailLoad(func(e *mygo.LoadError) { once.Do(func() { loaded <- fmt.Errorf("load: %v", e) }) })
		page.OnRenderProcessGone(func(reason string) { log.Printf("render process gone: %s", reason) })
		html, err := localHTML()
		if err != nil {
			runErr = err
			mygo.App.Quit()
			return
		}
		// A nonempty base uses MyGo's in-memory document interception on
		// WebView2, avoiding NavigateToString's document-size limit.
		page.LoadHTML(html, "https://aurink-s2.invalid/render.html")
		go func() {
			defer mygo.App.Quit()
			defer view.Destroy()
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			select {
			case err := <-loaded:
				if err != nil {
					runErr = err
					return
				}
			case <-ctx.Done():
				runErr = fmt.Errorf("LoadHTML: %w", ctx.Err())
				return
			}
			ready, err := page.EvalContext(ctx, "return await window.s2.ready")
			if err != nil {
				runErr = fmt.Errorf("page ready: %w", err)
				return
			}
			startup := time.Since(start)
			runErr = runProbe(*output, a, win, view, ready, startup)
		}()
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
	if runErr != nil {
		log.Fatal(runErr)
	}
}
