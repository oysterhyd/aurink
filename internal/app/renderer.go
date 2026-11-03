package app

import (
	"context"
	"sync"

	"aurink/internal/contract"
)

// lateRenderer is the render service, which needs the window the shell is
// built for: it is set once the window exists, and until then is unavailable.
type lateRenderer struct {
	mu sync.Mutex
	r  contract.Renderer
}

var _ contract.Renderer = (*lateRenderer)(nil)

func (l *lateRenderer) set(r contract.Renderer) {
	l.mu.Lock()
	l.r = r
	l.mu.Unlock()
}

func (l *lateRenderer) get() contract.Renderer {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.r
}

func (l *lateRenderer) Available() bool {
	r := l.get()
	return r != nil && r.Available()
}

func (l *lateRenderer) Render(ctx context.Context, req contract.RenderRequest) (contract.RenderResult, error) {
	r := l.get()
	if r == nil {
		return contract.RenderResult{}, contract.E(contract.CodeUnavailable, "app.Render", nil)
	}
	return r.Render(ctx, req)
}
