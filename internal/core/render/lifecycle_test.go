package render

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"aurink/internal/contract"
)

type lifecycleHost struct {
	mu            sync.Mutex
	starts, evals int
	failStart     bool
	entered       chan struct{}
	release       chan struct{}
}

type slowStartHost struct {
	mu               sync.Mutex
	starts, evals    int
	entered, release chan struct{}
}

func (*slowStartHost) Available() bool { return true }
func (h *slowStartHost) Start(ctx context.Context, _, _ string) error {
	h.mu.Lock()
	h.starts++
	h.mu.Unlock()
	close(h.entered)
	select {
	case <-h.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (*slowStartHost) Stop() {}
func (h *slowStartHost) Eval(context.Context, string) ([]byte, error) {
	h.mu.Lock()
	h.evals++
	h.mu.Unlock()
	return []byte(`{"svg":"<svg/>","width":1,"height":1,"ascent":1,"scale":1}`), nil
}

func TestStartHasSeparateTimeout(t *testing.T) {
	h := &slowStartHost{entered: make(chan struct{}), release: make(chan struct{})}
	var release sync.Once
	defer release.Do(func() { close(h.release) })
	r := New(h, Options{Page: "page", Timeout: 100 * time.Millisecond, StartTimeout: 10 * time.Second, Idle: time.Hour})
	results := make(chan error, 2)
	call := func(source string) {
		_, err := r.Render(context.Background(), contract.RenderRequest{Kind: contract.RenderMathInline, Source: source})
		results <- err
	}
	go call("first")
	select {
	case <-h.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("start did not begin")
	}
	go call("queued")
	deadline := time.Now().Add(5 * time.Second)
	for {
		r.mu.Lock()
		pending := r.pending
		r.mu.Unlock()
		if pending == 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("second request did not queue")
		}
		runtime.Gosched()
	}
	// The page remains blocked longer than the evaluation timeout. Both the
	// starting request and the queued one must remain eligible to render.
	window, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	select {
	case err := <-results:
		t.Fatalf("request ended during start: %v", err)
	case <-window.Done():
	}
	release.Do(func() { close(h.release) })
	for range 2 {
		select {
		case err := <-results:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("render did not finish")
		}
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.starts != 1 || h.evals != 2 {
		t.Fatalf("starts=%d evals=%d", h.starts, h.evals)
	}
}

func TestStartTimeoutIsBounded(t *testing.T) {
	h := &slowStartHost{entered: make(chan struct{}), release: make(chan struct{})}
	r := New(h, Options{Page: "page", StartTimeout: 50 * time.Millisecond, Idle: time.Hour})
	results := make(chan error, 1)
	go func() {
		_, err := r.Render(context.Background(), contract.RenderRequest{Kind: contract.RenderMathInline, Source: "x"})
		results <- err
	}()
	select {
	case err := <-results:
		if contract.CodeOf(err) != contract.CodeUnavailable {
			t.Fatalf("start error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("start did not time out")
	}
	if r.opts.StartTimeout != 50*time.Millisecond || New(nil, Options{}).opts.StartTimeout != 10*time.Second {
		t.Fatal("incorrect start timeout")
	}
}

func TestCanceledWhileStartIsQueued(t *testing.T) {
	h := &slowStartHost{entered: make(chan struct{}), release: make(chan struct{})}
	var release sync.Once
	defer release.Do(func() { close(h.release) })
	r := New(h, Options{Page: "page", StartTimeout: 10 * time.Second, Idle: time.Hour})
	first := make(chan error, 1)
	go func() {
		_, err := r.Render(context.Background(), contract.RenderRequest{Kind: contract.RenderMathInline, Source: "first"})
		first <- err
	}()
	select {
	case <-h.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("start did not begin")
	}
	ctx, cancel := context.WithCancel(context.Background())
	queued := make(chan error, 1)
	go func() {
		_, err := r.Render(ctx, contract.RenderRequest{Kind: contract.RenderMathInline, Source: "queued"})
		queued <- err
	}()
	deadline := time.Now().Add(5 * time.Second)
	for {
		r.mu.Lock()
		pending := r.pending
		r.mu.Unlock()
		if pending == 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("second request did not queue")
		}
		runtime.Gosched()
	}
	cancel()
	select {
	case err := <-queued:
		if contract.CodeOf(err) != contract.CodeCanceled {
			t.Fatalf("queued error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("canceled request did not return")
	}
	release.Do(func() { close(h.release) })
	select {
	case err := <-first:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("first request did not return")
	}
}

func TestQueuedRequestTimeout(t *testing.T) {
	h := &lifecycleHost{}
	r := New(h, Options{Page: "page", Timeout: 50 * time.Millisecond, Idle: time.Hour})
	<-r.serial
	defer func() { r.serial <- struct{}{} }()
	result := make(chan error, 1)
	go func() {
		_, err := r.Render(context.Background(), contract.RenderRequest{Kind: contract.RenderMathInline, Source: "queued"})
		result <- err
	}()
	select {
	case err := <-result:
		if contract.CodeOf(err) != contract.CodeTimeout {
			t.Fatalf("queued error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("queued request did not time out")
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.starts != 0 {
		t.Fatalf("started a page for a timed out request")
	}
}

func TestPageValidation(t *testing.T) {
	for _, answer := range []string{
		`{"svg":"<svg><g></svg>","width":1,"height":1,"ascent":1,"scale":1}`,
		`{"svg":"<other/>","width":1,"height":1,"ascent":1,"scale":1}`,
		`{"svg":"<svg xmlns:s='urn:x'><s:text>x</s:text></svg>","width":1,"height":1,"ascent":1,"scale":1}`,
		`{"svg":"<svg/>","width":1,"height":1,"ascent":2,"scale":1}`,
	} {
		if _, err := parseAnswer([]byte(answer), contract.RenderMathInline); err == nil {
			t.Fatalf("accepted %s", answer)
		}
	}
}

func TestPageVersionInvalidatesDisk(t *testing.T) {
	dir := t.TempDir()
	req := contract.RenderRequest{Kind: contract.RenderMathInline, Source: "x"}
	for i, page := range []string{"first", "second", "first"} {
		h := &lifecycleHost{}
		r := New(h, Options{Page: page, CacheDir: dir, Idle: time.Hour})
		if _, err := r.Render(context.Background(), req); err != nil {
			t.Fatal(err)
		}
		h.mu.Lock()
		starts := h.starts
		h.mu.Unlock()
		want := 1
		if i == 2 {
			want = 0
		}
		if starts != want {
			t.Fatalf("page %q started %d times, want %d", page, starts, want)
		}
	}
}

func TestDiskEvictsOldest(t *testing.T) {
	dir := t.TempDir()
	c := newCache(dir)
	svg := append([]byte("<svg><!--"), bytes.Repeat([]byte("x"), 22<<20)...)
	svg = append(svg, []byte("--></svg>")...)
	result := contract.RenderResult{SVG: svg, Width: 1, Height: 1, Scale: 1}
	var written int64
	for i := range 3 {
		var key [32]byte
		key[31] = byte(i)
		path := filepath.Join(dir, fmt.Sprintf("%064x.json", i))
		c.put(key, result)
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("entry %d was not written: %v", i, err)
		}
		written += info.Size()
		if i < 2 {
			// The second write is older than the first, regardless of insertion order.
			stamp := time.Unix(int64(2-i), 0)
			if err = os.Chtimes(path, stamp, stamp); err != nil {
				t.Fatal(err)
			}
		}
	}
	if written <= diskBytes {
		t.Fatalf("test wrote only %d bytes, limit %d", written, diskBytes)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var total int64
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			t.Fatal(err)
		}
		total += info.Size()
	}
	if total > diskBytes {
		t.Errorf("disk cache has %d bytes, limit %d", total, diskBytes)
	}
	for i := range 3 {
		_, err := os.Stat(filepath.Join(dir, fmt.Sprintf("%064x.json", i)))
		if i == 1 && !os.IsNotExist(err) {
			t.Errorf("oldest file retained: %v", err)
		}
		if i != 1 && err != nil {
			t.Errorf("newer file %d removed: %v", i, err)
		}
	}
}

func (*lifecycleHost) Available() bool { return true }
func (h *lifecycleHost) Start(context.Context, string, string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.starts++
	if h.failStart {
		return errors.New("start failed")
	}
	return nil
}
func (*lifecycleHost) Stop() {}
func (h *lifecycleHost) Eval(ctx context.Context, _ string) ([]byte, error) {
	h.mu.Lock()
	h.evals++
	n := h.evals
	h.mu.Unlock()
	if n == 1 && h.entered != nil {
		close(h.entered)
		select {
		case <-h.release:
			return nil, contract.E(contract.CodeUnavailable, "gone", nil)
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return []byte(`{"svg":"<svg></svg>","width":1,"height":1,"ascent":1,"scale":1}`), nil
}

func TestGoneFailsQueuedRequests(t *testing.T) {
	h := &lifecycleHost{entered: make(chan struct{}), release: make(chan struct{})}
	r := New(h, Options{Page: "page", Idle: time.Hour, Timeout: 5 * time.Second})
	results := make(chan error, 3)
	call := func(i int) {
		_, err := r.Render(context.Background(), contract.RenderRequest{Kind: contract.RenderMathInline, Source: fmt.Sprint(i)})
		results <- err
	}
	go call(0)
	select {
	case <-h.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("first evaluation did not start")
	}
	go call(1)
	go call(2)
	deadline := time.Now().Add(5 * time.Second)
	for {
		r.mu.Lock()
		pending := r.pending
		r.mu.Unlock()
		if pending == 3 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("requests did not queue")
		}
		runtime.Gosched()
	}
	close(h.release)
	for range 3 {
		select {
		case err := <-results:
			if contract.CodeOf(err) != contract.CodeUnavailable {
				t.Fatalf("queued error: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("request did not finish")
		}
	}
	if _, err := r.Render(context.Background(), contract.RenderRequest{Kind: contract.RenderMathInline, Source: "new"}); err != nil {
		t.Fatal(err)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.starts != 2 || h.evals != 2 {
		t.Fatalf("starts=%d evals=%d", h.starts, h.evals)
	}
}

func TestStartFailureCooldown(t *testing.T) {
	h := &lifecycleHost{failStart: true}
	r := New(h, Options{Page: "page", Idle: time.Hour})
	req := contract.RenderRequest{Kind: contract.RenderMathInline, Source: "x"}
	for range 2 {
		if _, err := r.Render(context.Background(), req); contract.CodeOf(err) != contract.CodeUnavailable {
			t.Fatalf("error %v", err)
		}
	}
	h.mu.Lock()
	starts := h.starts
	h.failStart = false
	h.mu.Unlock()
	if starts != 1 {
		t.Fatalf("retried a failed start: %d", starts)
	}
	r.mu.Lock()
	r.retryAfter = time.Now().Add(-time.Second)
	r.mu.Unlock()
	if _, err := r.Render(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.starts != 2 {
		t.Fatalf("starts=%d", h.starts)
	}
}

func TestMemoryCacheLRUAndIsolation(t *testing.T) {
	c := newCache("")
	result := contract.RenderResult{SVG: []byte("original"), Width: 1, Height: 1, Scale: 1}
	var keys [257][32]byte
	for i := range keys {
		keys[i][0] = byte(i)
		keys[i][1] = byte(i >> 8)
	}
	for i := 0; i < 256; i++ {
		c.put(keys[i], result)
	}
	hit, _ := c.get(keys[0], contract.RenderMathInline)
	hit.SVG[0] = 'X'
	c.put(keys[256], result)
	if _, ok := c.get(keys[1], contract.RenderMathInline); ok {
		t.Fatal("least recently used entry was retained")
	}
	if hit, ok := c.get(keys[0], contract.RenderMathInline); !ok || string(hit.SVG) != "original" {
		t.Fatalf("cache changed through returned slice: %q", hit.SVG)
	}
}
