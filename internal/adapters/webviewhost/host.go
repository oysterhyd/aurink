package webviewhost

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"aurink/internal/contract"
	"github.com/egoist/mygo"
)

// Host owns one unmounted WebView in a native window. Its blocking methods
// must run off the UI thread. MyGo marshals NewWebView, LoadHTML, EvalContext
// and Destroy to the window's main thread; waiting there would deadlock.
type Host struct {
	window *mygo.Window
	mu     sync.Mutex
	view   *mygo.WebView
	gone   chan struct{}
	reason string
}

var _ contract.RenderHost = (*Host)(nil)

// New binds a host to a window showing native Content.
func New(window *mygo.Window) *Host { return &Host{window: window} }

// Available reports whether the system's WebView runtime is available.
func (h *Host) Available() bool {
	return h.window != nil && !h.window.IsDestroyed() && mygo.WebViewAvailable()
}

// Start creates and loads an offscreen page, waiting for its libraries.
func (h *Host) Start(ctx context.Context, html, baseURL string) error {
	h.Stop()
	if !h.Available() || baseURL == "" {
		return contract.E(contract.CodeUnavailable, "webviewhost.Start", errors.New("WebView unavailable or empty base URL"))
	}
	if err := ctx.Err(); err != nil {
		return contextErr("webviewhost.Start", err)
	}
	view, err := h.window.NewWebView(mygo.WebViewOptions{Page: mygo.PageOptions{BackgroundThrottling: mygo.ThrottlingOff}})
	if err != nil {
		return contract.E(contract.CodeUnavailable, "webviewhost.Start", err)
	}
	gone := make(chan struct{})
	h.mu.Lock()
	h.view, h.gone, h.reason = view, gone, ""
	h.mu.Unlock()
	page := view.Page()
	loaded := make(chan error, 1)
	var once sync.Once
	page.OnDidFinishLoad(func() { once.Do(func() { loaded <- nil }) })
	page.OnDidFailLoad(func(e *mygo.LoadError) { once.Do(func() { loaded <- fmt.Errorf("load: %v", e) }) })
	page.OnRenderProcessGone(func(reason string) {
		h.mu.Lock()
		if h.view == view && h.reason == "" {
			h.reason = reason
			if h.reason == "" {
				h.reason = "render process gone"
			}
			close(gone)
		}
		h.mu.Unlock()
	})
	page.LoadHTML(html, baseURL)
	select {
	case err := <-loaded:
		if err != nil {
			h.Stop()
			return contract.E(contract.CodeUnavailable, "webviewhost.Start", err)
		}
	case <-gone:
		h.Stop()
		return contract.E(contract.CodeUnavailable, "webviewhost.Start", errors.New("render process gone"))
	case <-ctx.Done():
		h.Stop()
		return contextErr("webviewhost.Start", ctx.Err())
	}
	if _, err := h.Eval(ctx, "aurink.ready"); err != nil {
		h.Stop()
		return err
	}
	return nil
}

// Eval evaluates one expression and returns its JSON encoding.
func (h *Host) Eval(ctx context.Context, script string) ([]byte, error) {
	h.mu.Lock()
	view, gone, reason := h.view, h.gone, h.reason
	h.mu.Unlock()
	if view == nil || reason != "" || view.IsDestroyed() {
		return nil, contract.E(contract.CodeUnavailable, "webviewhost.Eval", errors.New("render page unavailable"))
	}
	if err := ctx.Err(); err != nil {
		return nil, contextErr("webviewhost.Eval", err)
	}
	evalCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	finished := make(chan struct{})
	go func() {
		select {
		case <-gone:
			cancel()
		case <-finished:
		}
	}()
	value, err := view.Page().EvalContext(evalCtx, script)
	close(finished)
	select {
	case <-gone:
		return nil, contract.E(contract.CodeUnavailable, "webviewhost.Eval", errors.New("render process gone"))
	default:
	}
	if err != nil {
		if ctx.Err() != nil {
			return nil, contextErr("webviewhost.Eval", ctx.Err())
		}
		if view.IsDestroyed() {
			return nil, contract.E(contract.CodeUnavailable, "webviewhost.Eval", err)
		}
		return nil, contract.E(contract.CodeRender, "webviewhost.Eval", err)
	}
	b, err := json.Marshal(value)
	if err != nil {
		return nil, contract.E(contract.CodeRender, "webviewhost.Eval", err)
	}
	return b, nil
}

// Stop releases the page. It is safe to call when no page is running.
func (h *Host) Stop() {
	h.mu.Lock()
	view := h.view
	h.view = nil
	h.mu.Unlock()
	if view != nil {
		view.Destroy()
	}
}

// ProcessID returns the browser process ID for diagnostics, or zero when idle.
func (h *Host) ProcessID() int {
	h.mu.Lock()
	view := h.view
	h.mu.Unlock()
	if view == nil || view.IsDestroyed() {
		return 0
	}
	return view.Page().ProcessID()
}

func contextErr(op string, err error) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return contract.E(contract.CodeTimeout, op, err)
	}
	return contract.E(contract.CodeCanceled, op, err)
}
