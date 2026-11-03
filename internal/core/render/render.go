// Package render implements the blocking, cached rendering service.
package render

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"image/png"
	"io"
	"math"
	"strings"
	"sync"
	"time"

	"aurink/internal/contract"
)

// Options supplies the page and controls the service's cache and lifecycle.
type Options struct {
	Page, BaseURL string
	CacheDir      string
	// Timeout bounds queueing and evaluation, excluding page startup.
	Timeout time.Duration
	// StartTimeout separately bounds starting the page (default 10 seconds).
	StartTimeout time.Duration
	Idle         time.Duration
}

// Renderer serializes page calls. Render must be called off the UI thread.
type Renderer struct {
	host       contract.RenderHost
	opts       Options
	pageHash   [32]byte
	cache      *cache
	serial     chan struct{}
	mu         sync.Mutex
	pending    int
	generation uint64
	running    bool
	retryAfter time.Time
	idleTimer  *time.Timer
	starting   time.Time
	startTotal time.Duration
	startPhase chan struct{}
}

var _ contract.Renderer = (*Renderer)(nil)

// New constructs a lazy renderer. An empty BaseURL uses a nonempty local URL.
func New(host contract.RenderHost, opts Options) *Renderer {
	if opts.BaseURL == "" {
		opts.BaseURL = "https://aurink.invalid/render.html"
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 2 * time.Second
	}
	if opts.StartTimeout <= 0 {
		opts.StartTimeout = 10 * time.Second
	}
	if opts.Idle <= 0 {
		opts.Idle = 60 * time.Second
	}
	r := &Renderer{host: host, opts: opts, pageHash: sha256.Sum256([]byte(opts.Page)), cache: newCache(opts.CacheDir), serial: make(chan struct{}, 1), startPhase: make(chan struct{})}
	r.serial <- struct{}{}
	return r
}

// Available reports whether the host runtime is present.
func (r *Renderer) Available() bool { return r.host != nil && r.host.Available() }

type pageRequest struct {
	Kind     contract.RenderKind `json:"kind"`
	Source   string              `json:"source"`
	Lang     string              `json:"lang"`
	Dark     bool                `json:"dark"`
	FontSize float32             `json:"fontSize"`
	Color    string              `json:"color"`
	Scale    float32             `json:"scale"`
}

// Render returns a cached or freshly rendered result.
func (r *Renderer) Render(ctx context.Context, req contract.RenderRequest) (contract.RenderResult, error) {
	if err := contextError(ctx); err != nil {
		return contract.RenderResult{}, err
	}
	if req.Scale == 0 {
		req.Scale = 1
	}
	request := pageRequest{req.Kind, req.Source, req.Lang, req.Dark, req.FontSize, req.Color, req.Scale}
	b, err := json.Marshal(request)
	if err != nil {
		return contract.RenderResult{}, contract.E(contract.CodeRender, "render.Render", err)
	}
	h := sha256.New()
	_, _ = h.Write(r.pageHash[:])
	_, _ = h.Write(b)
	var key [32]byte
	copy(key[:], h.Sum(nil))
	if result, ok := r.cache.get(key, req.Kind); ok {
		return result, nil
	}

	r.mu.Lock()
	queuedAt := time.Now()
	startBase := r.startupTime(queuedAt)
	gen := r.generation
	r.pending++
	if r.idleTimer != nil {
		r.idleTimer.Stop()
		r.idleTimer = nil
	}
	r.mu.Unlock()
	defer r.leave()
	if err := r.acquire(ctx, queuedAt, startBase); err != nil {
		return contract.RenderResult{}, err
	}
	defer func() { r.serial <- struct{}{} }()
	if err := contextError(ctx); err != nil {
		return contract.RenderResult{}, err
	}
	r.mu.Lock()
	stale := gen != r.generation
	running := r.running
	retryAfter := r.retryAfter
	r.mu.Unlock()
	if stale {
		return contract.RenderResult{}, contract.E(contract.CodeUnavailable, "render.Render", errors.New("render page went away"))
	}
	if result, ok := r.cache.get(key, req.Kind); ok {
		return result, nil
	}
	if !r.Available() || time.Now().Before(retryAfter) {
		return contract.RenderResult{}, contract.E(contract.CodeUnavailable, "render.Render", nil)
	}
	if !running {
		r.mu.Lock()
		r.starting = time.Now()
		r.notifyStartPhase()
		r.mu.Unlock()
		startCtx, cancelStart := context.WithTimeout(ctx, r.opts.StartTimeout)
		err := r.host.Start(startCtx, r.opts.Page, r.opts.BaseURL)
		cancelStart()
		r.mu.Lock()
		r.startTotal += time.Since(r.starting)
		r.starting = time.Time{}
		r.notifyStartPhase()
		r.mu.Unlock()
		if err != nil {
			r.host.Stop()
			r.mu.Lock()
			r.retryAfter = time.Now().Add(time.Minute)
			r.mu.Unlock()
			return contract.RenderResult{}, contract.E(contract.CodeUnavailable, "render.Start", err)
		}
		r.mu.Lock()
		r.running = true
		r.mu.Unlock()
	}
	if err := contextError(ctx); err != nil {
		return contract.RenderResult{}, err
	}
	remaining := r.remaining(queuedAt, startBase)
	if remaining <= 0 {
		return contract.RenderResult{}, contract.E(contract.CodeTimeout, "render.Render", context.DeadlineExceeded)
	}
	evalCtx, cancelEval := context.WithTimeout(ctx, remaining)
	defer cancelEval()
	answer, err := r.host.Eval(evalCtx, "aurink.render("+string(b)+")")
	if err != nil {
		if contract.CodeOf(err) == contract.CodeUnavailable {
			r.mu.Lock()
			r.running = false
			r.generation++
			r.mu.Unlock()
			r.host.Stop()
			return contract.RenderResult{}, contract.E(contract.CodeUnavailable, "render.Eval", err)
		}
		if e := contextError(evalCtx); e != nil {
			return contract.RenderResult{}, e
		}
		switch contract.CodeOf(err) {
		case contract.CodeCanceled, contract.CodeTimeout:
			return contract.RenderResult{}, contract.E(contract.CodeOf(err), "render.Eval", err)
		default:
			return contract.RenderResult{}, contract.E(contract.CodeRender, "render.Eval", err)
		}
	}
	result, err := parseAnswer(answer, req.Kind)
	if err != nil {
		return contract.RenderResult{}, contract.E(contract.CodeRender, "render.Render", err)
	}
	r.cache.put(key, result)
	return cloneResult(result), nil
}

// startupTime is called under mu and includes an ongoing start.
func (r *Renderer) startupTime(now time.Time) time.Duration {
	total := r.startTotal
	if !r.starting.IsZero() {
		total += now.Sub(r.starting)
	}
	return total
}

// notifyStartPhase is called under mu to wake queued requests when a start
// begins or ends, so they can pause or resume their request timeout.
func (r *Renderer) notifyStartPhase() {
	close(r.startPhase)
	r.startPhase = make(chan struct{})
}

func (r *Renderer) remaining(queuedAt time.Time, startBase time.Duration) time.Duration {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	return r.opts.Timeout - (now.Sub(queuedAt) - (r.startupTime(now) - startBase))
}

func (r *Renderer) acquire(ctx context.Context, queuedAt time.Time, startBase time.Duration) error {
	for {
		if err := contextError(ctx); err != nil {
			return err
		}
		r.mu.Lock()
		now := time.Now()
		remaining := r.opts.Timeout - (now.Sub(queuedAt) - (r.startupTime(now) - startBase))
		starting, phase := !r.starting.IsZero(), r.startPhase
		r.mu.Unlock()
		if remaining <= 0 {
			return contract.E(contract.CodeTimeout, "render.Render", context.DeadlineExceeded)
		}
		if starting {
			select {
			case <-ctx.Done():
				return contextError(ctx)
			case <-r.serial:
				return nil
			case <-phase:
				continue
			}
		}
		timer := time.NewTimer(remaining)
		select {
		case <-ctx.Done():
			timer.Stop()
			return contextError(ctx)
		case <-r.serial:
			timer.Stop()
			return nil
		case <-phase:
		case <-timer.C:
		}
		timer.Stop()
	}
}

func contextError(ctx context.Context) error {
	switch ctx.Err() {
	case context.Canceled:
		return contract.E(contract.CodeCanceled, "render.Render", ctx.Err())
	case context.DeadlineExceeded:
		return contract.E(contract.CodeTimeout, "render.Render", ctx.Err())
	default:
		return nil
	}
}

func (r *Renderer) leave() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pending--
	if r.pending == 0 && r.running {
		r.idleTimer = time.AfterFunc(r.opts.Idle, r.stopIdle)
	}
}

func (r *Renderer) stopIdle() {
	select {
	case <-r.serial:
	default:
		return
	}
	defer func() { r.serial <- struct{}{} }()
	r.mu.Lock()
	stop := r.pending == 0 && r.running
	if stop {
		r.running = false
	}
	r.mu.Unlock()
	if stop {
		r.host.Stop()
	}
}

type pageAnswer struct {
	SVG    string  `json:"svg"`
	PNG    []byte  `json:"png"`
	Width  float32 `json:"width"`
	Height float32 `json:"height"`
	Ascent float32 `json:"ascent"`
	Scale  float32 `json:"scale"`
	Error  string  `json:"error"`
}

func parseAnswer(data []byte, kind contract.RenderKind) (contract.RenderResult, error) {
	var a pageAnswer
	if err := json.Unmarshal(data, &a); err != nil {
		return contract.RenderResult{}, fmt.Errorf("invalid page answer: %w", err)
	}
	if a.Error != "" {
		return contract.RenderResult{}, errors.New(a.Error)
	}
	if (a.SVG == "") == (len(a.PNG) == 0) {
		return contract.RenderResult{}, errors.New("page must return exactly one of SVG and PNG")
	}
	if a.Width <= 0 || a.Height <= 0 || a.Scale <= 0 || math.IsInf(float64(a.Width), 0) || math.IsInf(float64(a.Height), 0) {
		return contract.RenderResult{}, errors.New("invalid dimensions or scale")
	}
	if a.SVG != "" && (kind == contract.RenderDiagram || strings.Contains(strings.ToLower(a.SVG), "<text")) {
		return contract.RenderResult{}, errors.New("unsupported SVG content")
	}
	if kind == contract.RenderMathInline && (a.Ascent <= 0 || a.Ascent > a.Height) {
		return contract.RenderResult{}, errors.New("invalid inline ascent")
	}
	if a.SVG != "" {
		if err := validateSVG(a.SVG); err != nil {
			return contract.RenderResult{}, err
		}
	}
	if len(a.PNG) > 0 {
		if _, err := png.Decode(bytes.NewReader(a.PNG)); err != nil {
			return contract.RenderResult{}, fmt.Errorf("invalid PNG: %w", err)
		}
	}
	return contract.RenderResult{SVG: []byte(a.SVG), PNG: a.PNG, Width: a.Width, Height: a.Height, Ascent: a.Ascent, Scale: a.Scale}, nil
}

func validateSVG(source string) error {
	decoder := xml.NewDecoder(strings.NewReader(source))
	depth, roots := 0, 0
	for {
		tok, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("invalid SVG: %w", err)
		}
		switch elem := tok.(type) {
		case xml.StartElement:
			if depth == 0 {
				roots++
				if roots != 1 || elem.Name.Local != "svg" {
					return errors.New("invalid SVG root")
				}
			}
			if strings.EqualFold(elem.Name.Local, "text") {
				return errors.New("unsupported SVG text")
			}
			depth++
		case xml.EndElement:
			depth--
		}
	}
	if roots != 1 || depth != 0 {
		return errors.New("invalid SVG document")
	}
	return nil
}

func cloneResult(r contract.RenderResult) contract.RenderResult {
	r.SVG = bytes.Clone(r.SVG)
	r.PNG = bytes.Clone(r.PNG)
	return r
}
