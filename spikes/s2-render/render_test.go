package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

const testSVG = `<svg xmlns="http://www.w3.org/2000/svg" width="20" height="20" viewBox="0 0 20 20"><path d="M2 2H18V18H2Z"/></svg>`

func TestCacheAvoidsBridgeAndCanceledRequests(t *testing.T) {
	calls := 0
	source := "\\text{中文}`); throw new Error('injected'); //\n\\frac{a}{b}"
	r := newRenderer(func(ctx context.Context, code string) (any, error) {
		calls++
		argument := strings.TrimSuffix(strings.TrimPrefix(code, "return await window.s2.math("), ")")
		var got string
		if err := json.Unmarshal([]byte(argument), &got); err != nil || got != source {
			t.Fatalf("source was not encoded as a single JSON argument: %s", code)
		}
		return mathResult{SVG: testSVG, Width: 20, Height: 20}, nil
	})
	for range 1000 {
		if _, err := r.render(context.Background(), source); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 1 {
		t.Fatalf("1000 identical requests made %d bridge calls, want 1", calls)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := r.render(ctx, source); !errors.Is(err, context.Canceled) {
		t.Fatalf("cached result bypassed cancellation: %v", err)
	}
}

func TestUnavailableAndFailuresDoNotPoisonCache(t *testing.T) {
	if _, err := newRenderer(nil).render(context.Background(), "x"); !errors.Is(err, errUnavailable) {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		result mathResult
		err    error
	}{
		{"bridge failure", mathResult{}, errors.New("crashed")},
		{"invalid SVG", mathResult{SVG: "<html/>", Width: 20, Height: 20}, nil},
		{"empty result", mathResult{}, nil},
		{"unsupported text", mathResult{SVG: testSVG, Width: 20, Height: 20, TextElements: 1}, nil},
		{"TeX error", mathResult{SVG: testSVG, Width: 20, Height: 20, ErrorElements: 1}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			r := newRenderer(func(context.Context, string) (any, error) { calls++; return tc.result, tc.err })
			for range 2 {
				r.render(context.Background(), "x")
			}
			if calls != 2 || len(r.cache) != 0 {
				t.Fatalf("failed/incomplete output cached: calls=%d entries=%d", calls, len(r.cache))
			}
		})
	}
}

func TestMeasuredCorpusParseAndNativeDrawing(t *testing.T) {
	formulas, err := corpus()
	if err != nil || len(formulas) != 50 {
		t.Fatalf("corpus: count=%d err=%v", len(formulas), err)
	}
	ids := make(map[string]bool)
	for _, f := range formulas {
		if ids[f.ID] {
			t.Fatalf("duplicate ID: %s", f.ID)
		}
		ids[f.ID] = true
		b, err := os.ReadFile(filepath.Join("evidence", "svg", f.ID+".svg"))
		if err != nil {
			t.Fatal(err)
		}
		svg, err := ui.ParseSVG(b)
		if err != nil {
			t.Fatalf("%s: %v", f.ID, err)
		}
		w, h := svg.Size()
		if w <= 0 || h <= 0 {
			t.Fatalf("%s: invalid dimensions %vx%v", f.ID, w, h)
		}
		if f.ID == "F20" && !strings.Contains(string(b), "stroke-width: 70px") {
			t.Fatal("array separator lost MathJax stylesheet when exported")
		}
		if f.ID != "F03" {
			continue
		}
		tt := ui.NewTester(func(c *ui.Context) {
			ui.Box(c).Fill().Background(ui.RGB(255, 255, 255)).Children(func() {
				ui.Image(c, svg).Size(w, h).Label("formula")
			})
		}, 400, 160)
		if _, ok := tt.Find("formula"); !ok {
			t.Fatal("formula missing")
		}
		ink := 0
		img := tt.Image()
		for y := 0; y < 160; y++ {
			for x := 0; x < 400; x++ {
				if img.RGBAAt(x, y).R < 128 {
					ink++
				}
			}
		}
		if ink < 50 {
			t.Fatalf("formula was not drawn: %d dark pixels", ink)
		}
		tt.Advance(time.Hour)
		if tt.FramePending() {
			t.Fatal("static formula keeps scheduling frames")
		}
	}
}

func TestFallbackStaysNativeAndIdle(t *testing.T) {
	a := &app{caption: `WebView unavailable · source fallback: \frac{a}{b}`}
	tt := ui.NewTester(a.view, 1180, 820)
	if !tt.HasText(`\frac{a}{b}`) {
		t.Fatal("source fallback missing")
	}
	tt.Advance(time.Hour)
	if tt.FramePending() {
		t.Fatal("fallback schedules frames")
	}
}
