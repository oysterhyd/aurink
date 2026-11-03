package render

import (
	"testing"

	"aurink/internal/contract"
	"aurink/internal/contracttest"
)

func TestRenderer(t *testing.T) {
	contracttest.TestRenderer(t, contracttest.RendererFixture{New: func(h contract.RenderHost, o contracttest.RendererOptions) contract.Renderer {
		return New(h, Options{Page: "test page", CacheDir: o.CacheDir, Timeout: o.Timeout, Idle: o.Idle})
	}})
}

func BenchmarkCacheHit(b *testing.B) {
	r := New(&contracttest.FakeRenderHost{}, Options{Page: "test page"})
	req := contract.RenderRequest{Kind: contract.RenderMathInline, Source: "x^2", FontSize: 16, Scale: 1}
	if _, err := r.Render(b.Context(), req); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for b.Loop() {
		if _, err := r.Render(b.Context(), req); err != nil {
			b.Fatal(err)
		}
	}
}
