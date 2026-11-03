package contracttest

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"strings"
	"sync"
	"testing"
	"time"

	"aurink/internal/contract"
)

// FakeRenderHost answers the render page's protocol (contract.RenderHost)
// without a browser. Reply decides each answer; by default a formula is a
// small SVG and a diagram a 2×1 PNG.
type FakeRenderHost struct {
	Unavailable bool
	// Reply returns the page's JSON answer to a request; nil uses the default.
	Reply func(req map[string]any) string
	// Block makes Eval wait until its context ends, as a page that never answers.
	Block bool
	// Gone makes the next Eval fail as if the page's process ended.
	Gone bool

	mu                  sync.Mutex
	started, stopped    int
	running             bool
	evals, inEval, most int
	pages, scripts      []string
}

// Started, Stopped and Evals count the calls.
func (h *FakeRenderHost) Started() int { h.mu.Lock(); defer h.mu.Unlock(); return h.started }
func (h *FakeRenderHost) Stopped() int { h.mu.Lock(); defer h.mu.Unlock(); return h.stopped }
func (h *FakeRenderHost) Evals() int   { h.mu.Lock(); defer h.mu.Unlock(); return h.evals }

// MostAtOnce is the most Evals that ran at the same time.
func (h *FakeRenderHost) MostAtOnce() int { h.mu.Lock(); defer h.mu.Unlock(); return h.most }

func (h *FakeRenderHost) Available() bool { return !h.Unavailable }

func (h *FakeRenderHost) Start(ctx context.Context, page, baseURL string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.Unavailable {
		return contract.E(contract.CodeUnavailable, "host.Start", nil)
	}
	if baseURL == "" {
		return fmt.Errorf("an empty base URL")
	}
	h.started++
	h.running = true
	h.pages = append(h.pages, page)
	return nil
}

func (h *FakeRenderHost) Stop() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.running {
		h.stopped++
		h.running = false
	}
}

func (h *FakeRenderHost) Eval(ctx context.Context, script string) ([]byte, error) {
	h.mu.Lock()
	if !h.running {
		h.mu.Unlock()
		return nil, fmt.Errorf("Eval before Start")
	}
	h.evals++
	h.inEval++
	h.most = max(h.most, h.inEval)
	h.scripts = append(h.scripts, script)
	gone, block := h.Gone, h.Block
	h.Gone = false
	h.mu.Unlock()
	defer func() { h.mu.Lock(); h.inEval--; h.mu.Unlock() }()

	if gone {
		h.mu.Lock()
		h.running = false
		h.mu.Unlock()
		return nil, contract.E(contract.CodeUnavailable, "host.Eval", fmt.Errorf("render process gone"))
	}
	if block {
		<-ctx.Done()
		if ctx.Err() == context.DeadlineExceeded {
			return nil, contract.E(contract.CodeTimeout, "host.Eval", ctx.Err())
		}
		return nil, contract.E(contract.CodeCanceled, "host.Eval", ctx.Err())
	}
	const prefix, suffix = "aurink.render(", ")"
	if !strings.HasPrefix(script, prefix) || !strings.HasSuffix(script, suffix) {
		return nil, fmt.Errorf("not the page's protocol: %q", script)
	}
	var req map[string]any
	if err := json.Unmarshal([]byte(script[len(prefix):len(script)-len(suffix)]), &req); err != nil {
		return nil, fmt.Errorf("the request is not JSON: %v", err)
	}
	if h.Reply != nil {
		return []byte(h.Reply(req)), nil
	}
	return []byte(defaultReply(req)), nil
}

const fakeSVG = `<svg xmlns="http://www.w3.org/2000/svg" width="20" height="10" viewBox="0 0 20 10"><path d="M0 0H20V10H0Z"/></svg>`

func fakePNG() string {
	img := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	img.Set(0, 0, color.Black)
	var b bytes.Buffer
	_ = png.Encode(&b, img)
	return base64.StdEncoding.EncodeToString(b.Bytes())
}

func defaultReply(req map[string]any) string {
	if req["kind"] == float64(contract.RenderDiagram) {
		return fmt.Sprintf(`{"png":%q,"width":2,"height":1,"scale":1}`, fakePNG())
	}
	return fmt.Sprintf(`{"svg":%q,"width":20,"height":10,"ascent":8,"scale":1}`, fakeSVG)
}

// RendererOptions configures a renderer under test.
type RendererOptions struct {
	CacheDir string        // disk cache; "" for none
	Timeout  time.Duration // per request
	Idle     time.Duration // the page stops after this long without requests
}

// RendererFixture makes the renderer of core/render over a host.
type RendererFixture struct {
	New func(h contract.RenderHost, opts RendererOptions) contract.Renderer
}

// TestRenderer checks BE-2.9 (08 D7–D11) against FakeRenderHost. Waits are
// bounded polls of what the host saw, not sleeps.
func TestRenderer(t *testing.T, f RendererFixture) {
	ctx := context.Background()
	math := contract.RenderRequest{Kind: contract.RenderMathBlock, Source: "x^2", FontSize: 16, Color: "#111111", Scale: 1}
	opts := func(t *testing.T) RendererOptions {
		return RendererOptions{CacheDir: t.TempDir(), Timeout: 2 * time.Second, Idle: time.Hour}
	}
	code := contract.CodeOf

	t.Run("lazy start, one page", func(t *testing.T) {
		h := &FakeRenderHost{}
		r := f.New(h, opts(t))
		if !r.Available() || h.Started() != 0 {
			t.Fatalf("available %v, started %d before any request", r.Available(), h.Started())
		}
		for i := range 3 {
			req := math
			req.Source = fmt.Sprint("x^", i)
			if _, err := r.Render(ctx, req); err != nil {
				t.Fatal(err)
			}
		}
		if h.Started() != 1 || h.Evals() != 3 {
			t.Errorf("started %d, evals %d", h.Started(), h.Evals())
		}
	})
	t.Run("results", func(t *testing.T) {
		h := &FakeRenderHost{}
		r := f.New(h, opts(t))
		res, err := r.Render(ctx, math)
		if err != nil || len(res.SVG) == 0 || len(res.PNG) != 0 || res.Width != 20 || res.Height != 10 || res.Ascent != 8 {
			t.Fatalf("formula %+v, %v", res, err)
		}
		res, err = r.Render(ctx, contract.RenderRequest{Kind: contract.RenderDiagram, Lang: "mermaid", Source: "graph TD", Scale: 2})
		if err != nil || len(res.PNG) == 0 || len(res.SVG) != 0 {
			t.Fatalf("diagram %+v, %v", res, err)
		}
		if _, err := png.Decode(bytes.NewReader(res.PNG)); err != nil {
			t.Errorf("the PNG does not decode: %v", err)
		}
	})
	t.Run("the request reaches the page as JSON", func(t *testing.T) {
		var got map[string]any
		h := &FakeRenderHost{Reply: func(req map[string]any) string { got = req; return defaultReply(req) }}
		r := f.New(h, opts(t))
		req := contract.RenderRequest{Kind: contract.RenderMathInline, Source: `a"b\c</script>`, Dark: true, FontSize: 15, Color: "#abcdef", Scale: 1.5}
		if _, err := r.Render(ctx, req); err != nil {
			t.Fatal(err)
		}
		want := map[string]any{"kind": float64(1), "source": `a"b\c</script>`, "dark": true, "fontSize": float64(15), "color": "#abcdef", "scale": 1.5}
		for k, v := range want {
			if got[k] != v {
				t.Errorf("%s = %v, want %v", k, got[k], v)
			}
		}
	})
	t.Run("cache", func(t *testing.T) {
		h := &FakeRenderHost{}
		o := opts(t)
		r := f.New(h, o)
		for range 1000 {
			if _, err := r.Render(ctx, math); err != nil {
				t.Fatal(err)
			}
		}
		dark := math
		dark.Dark = true
		if _, err := r.Render(ctx, dark); err != nil {
			t.Fatal(err)
		}
		if h.Evals() != 2 {
			t.Errorf("%d evals for 1000 same requests and one in the dark", h.Evals())
		}
		// A new renderer over the same directory finds the result on disk.
		h2 := &FakeRenderHost{}
		r2 := f.New(h2, o)
		if res, err := r2.Render(ctx, math); err != nil || len(res.SVG) == 0 {
			t.Fatalf("%+v, %v", res, err)
		}
		if h2.Started() != 0 || h2.Evals() != 0 {
			t.Errorf("a disk hit started the page: started %d, evals %d", h2.Started(), h2.Evals())
		}
	})
	t.Run("bad answers are errors and are not cached", func(t *testing.T) {
		for _, tt := range []struct{ name, reply string }{
			{"an error", `{"error":"Undefined control sequence \\foo"}`},
			{"text in the SVG", `{"svg":"<svg xmlns=\"http://www.w3.org/2000/svg\"><text>中</text></svg>","width":1,"height":1,"scale":1}`},
			{"both", fmt.Sprintf(`{"svg":%q,"png":%q,"width":1,"height":1,"scale":1}`, fakeSVG, fakePNG())},
			{"neither", `{"width":1,"height":1,"scale":1}`},
			{"a PNG that does not decode", `{"png":"bm90IGEgcG5n","width":1,"height":1,"scale":1}`},
			{"no size", fmt.Sprintf(`{"svg":%q,"width":0,"height":10,"scale":1}`, fakeSVG)},
			{"not JSON", `undefined`},
		} {
			t.Run(tt.name, func(t *testing.T) {
				h := &FakeRenderHost{Reply: func(map[string]any) string { return tt.reply }}
				r := f.New(h, opts(t))
				for range 2 {
					if _, err := r.Render(ctx, math); code(err) != contract.CodeRender {
						t.Fatalf("error %v, want code render", err)
					}
				}
				if h.Evals() != 2 {
					t.Errorf("%d evals: a bad answer was cached", h.Evals())
				}
			})
		}
		h := &FakeRenderHost{Reply: func(map[string]any) string { return `{"error":"Undefined control sequence \\foo"}` }}
		_, err := f.New(h, opts(t)).Render(ctx, math)
		if err == nil || !strings.Contains(err.Error(), "Undefined control sequence") {
			t.Errorf("the page's message is lost: %v", err)
		}
	})
	t.Run("unavailable", func(t *testing.T) {
		h := &FakeRenderHost{Unavailable: true}
		r := f.New(h, opts(t))
		if r.Available() {
			t.Error("available without a host")
		}
		if _, err := r.Render(ctx, math); code(err) != contract.CodeUnavailable {
			t.Errorf("error %v", err)
		}
		if h.Started() != 0 {
			t.Error("started without a host")
		}
	})
	t.Run("canceled before it starts", func(t *testing.T) {
		h := &FakeRenderHost{}
		c, cancel := context.WithCancel(ctx)
		cancel()
		if _, err := f.New(h, opts(t)).Render(c, math); code(err) != contract.CodeCanceled {
			t.Errorf("error %v", err)
		}
		if h.Evals() != 0 {
			t.Error("a canceled request reached the page")
		}
	})
	t.Run("timeout", func(t *testing.T) {
		h := &FakeRenderHost{Block: true}
		o := opts(t)
		o.Timeout = 50 * time.Millisecond
		r := f.New(h, o)
		if _, err := r.Render(ctx, math); code(err) != contract.CodeTimeout {
			t.Errorf("error %v", err)
		}
		// The page answers again afterwards.
		h.mu.Lock()
		h.Block = false
		h.mu.Unlock()
		if _, err := r.Render(ctx, math); err != nil {
			t.Errorf("after a timeout: %v", err)
		}
	})
	t.Run("requests are serial", func(t *testing.T) {
		h := &FakeRenderHost{}
		r := f.New(h, opts(t))
		var wg sync.WaitGroup
		for i := range 32 {
			wg.Go(func() {
				req := math
				req.Source = fmt.Sprint("y_", i)
				if _, err := r.Render(ctx, req); err != nil {
					t.Error(err)
				}
			})
		}
		wg.Wait()
		if h.MostAtOnce() != 1 || h.Started() != 1 {
			t.Errorf("%d evals at once, %d starts", h.MostAtOnce(), h.Started())
		}
	})
	t.Run("a gone page restarts on the next request", func(t *testing.T) {
		h := &FakeRenderHost{}
		r := f.New(h, opts(t))
		if _, err := r.Render(ctx, math); err != nil {
			t.Fatal(err)
		}
		h.mu.Lock()
		h.Gone = true
		h.mu.Unlock()
		other := math
		other.Source = "z"
		if _, err := r.Render(ctx, other); code(err) != contract.CodeUnavailable {
			t.Errorf("error %v", err)
		}
		third := math
		third.Source = "w"
		if _, err := r.Render(ctx, third); err != nil {
			t.Errorf("after the page went away: %v", err)
		}
		if h.Started() != 2 {
			t.Errorf("%d starts", h.Started())
		}
	})
	t.Run("idle stop and restart", func(t *testing.T) {
		h := &FakeRenderHost{}
		o := opts(t)
		o.Idle = 20 * time.Millisecond
		r := f.New(h, o)
		if _, err := r.Render(ctx, math); err != nil {
			t.Fatal(err)
		}
		deadline := time.Now().Add(5 * time.Second)
		for h.Stopped() == 0 {
			if time.Now().After(deadline) {
				t.Fatal("the page never stopped while idle")
			}
			time.Sleep(5 * time.Millisecond)
		}
		other := math
		other.Source = "z"
		if _, err := r.Render(ctx, other); err != nil {
			t.Fatal(err)
		}
		if h.Started() != 2 {
			t.Errorf("%d starts", h.Started())
		}
	})
}
