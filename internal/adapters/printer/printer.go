// Package printer prints exported HTML in a dedicated hidden MyGo window.
package printer

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"

	"aurink/internal/contract"
	"github.com/egoist/mygo"
)

// Printer serializes jobs. HTMLToPDF must run off the main thread, after
// MyGo's application is ready. MyGo marshals window and page operations to
// the main thread; waiting for a load or print there would block its events.
type Printer struct {
	once sync.Once
	gate chan struct{}
}

const printTimeout = 60 * time.Second

var _ contract.Printer = (*Printer)(nil)

// New creates a printer; its first job creates the hidden window.
func New() *Printer { return &Printer{} }

// HTMLToPDF prints one page after navigation, fonts and image decoding finish.
func (p *Printer) HTMLToPDF(ctx context.Context, html, baseURL string, opts contract.PDFOptions) ([]byte, error) {
	const op = "printer.HTMLToPDF"
	if ctx == nil {
		return nil, contract.E(contract.CodeInvalid, op, nil)
	}
	if err := ctx.Err(); err != nil {
		return nil, contract.E(contract.CodeCanceled, op, err)
	}
	size := mygo.PageA4
	switch opts.PageSize {
	case "", "A4":
	case "Letter":
		size = mygo.PageLetter
	default:
		return nil, contract.E(contract.CodeInvalid, op, errors.New("unsupported page size"))
	}
	for _, margin := range opts.Margins {
		if margin < 0 || math.IsNaN(float64(margin)) || math.IsInf(float64(margin), 0) {
			return nil, contract.E(contract.CodeInvalid, op, errors.New("invalid margin"))
		}
	}
	p.once.Do(func() { p.gate = make(chan struct{}, 1); p.gate <- struct{}{} })
	select {
	case <-ctx.Done():
		return nil, contract.E(contract.CodeCanceled, op, ctx.Err())
	case <-p.gate:
	}
	defer func() { p.gate <- struct{}{} }()
	if ctx.Err() != nil {
		return nil, contract.E(contract.CodeCanceled, op, ctx.Err())
	}
	if !mygo.App.IsReady() || !mygo.WebViewAvailable() {
		return nil, contract.E(contract.CodeUnavailable, op, errors.New("page runtime unavailable"))
	}
	win := mygo.NewWindow(mygo.WindowOptions{Hidden: true, SkipTaskbar: true, Page: mygo.PageOptions{BackgroundThrottling: mygo.ThrottlingOff}})
	if win == nil || win.IsDestroyed() || win.Page() == nil {
		if win != nil {
			win.Destroy()
		}
		return nil, contract.E(contract.CodeUnavailable, op, errors.New("print window unavailable"))
	}
	owned := true
	defer func() {
		if owned {
			win.Destroy()
		}
	}()
	page := win.Page()
	loaded := make(chan error, 1)
	var once sync.Once
	page.OnDidFinishLoad(func() { once.Do(func() { loaded <- nil }) })
	page.OnDidFailLoad(func(e *mygo.LoadError) { once.Do(func() { loaded <- fmt.Errorf("load: %v", e) }) })
	page.OnRenderProcessGone(func(reason string) { once.Do(func() { loaded <- fmt.Errorf("render process gone: %s", reason) }) })
	loadCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	page.LoadHTML(html, baseURL)
	select {
	case err := <-loaded:
		if err != nil {
			return nil, contract.E(contract.CodeUnavailable, op, err)
		}
	case <-loadCtx.Done():
		return nil, loadError(ctx, loadCtx, op)
	}
	_, err := page.EvalContext(loadCtx, `(async () => { await document.fonts.ready; await Promise.all(Array.from(document.images, img => img.decode().catch(() => {}))); return true; })()`)
	if err != nil {
		if loadCtx.Err() != nil {
			return nil, loadError(ctx, loadCtx, op)
		}
		return nil, contract.E(contract.CodeUnavailable, op, err)
	}
	if ctx.Err() != nil {
		return nil, contract.E(contract.CodeCanceled, op, ctx.Err())
	}
	mo := mygo.PDFOptions{PageSize: size, Landscape: opts.Landscape, Background: opts.Background,
		Margins: &mygo.Margins{Top: float64(opts.Margins[0]), Right: float64(opts.Margins[1]), Bottom: float64(opts.Margins[2]), Left: float64(opts.Margins[3])}}
	timer := time.NewTimer(printTimeout)
	defer timer.Stop()
	owned = false
	return waitPrint(ctx, timer.C, func() ([]byte, error) { return page.PrintToPDF(mo) }, win.Destroy)
}

// MyGo's blocking PrintToPDF has no context. If its callback never arrives,
// this returns while the caller destroys the window and releases the slot.
func waitPrint(ctx context.Context, timeout <-chan time.Time, print func() ([]byte, error), closeWindow func()) ([]byte, error) {
	const op = "printer.HTMLToPDF"
	defer closeWindow()
	type result struct {
		data []byte
		err  error
	}
	finished := make(chan result, 1)
	go func() { data, err := print(); finished <- result{data, err} }()
	select {
	case <-ctx.Done():
		return nil, contract.E(contract.CodeCanceled, op, ctx.Err())
	case <-timeout:
		if ctx.Err() != nil {
			return nil, contract.E(contract.CodeCanceled, op, ctx.Err())
		}
		return nil, contract.E(contract.CodeTimeout, op, errors.New("printing timed out"))
	case result := <-finished:
		if ctx.Err() != nil {
			return nil, contract.E(contract.CodeCanceled, op, ctx.Err())
		}
		if result.err != nil {
			return nil, contract.E(contract.CodeUnavailable, op, result.err)
		}
		return result.data, nil
	}
}

func loadError(parent, bounded context.Context, op string) error {
	if parent.Err() != nil {
		return contract.E(contract.CodeCanceled, op, parent.Err())
	}
	return contract.E(contract.CodeTimeout, op, bounded.Err())
}
