package export

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"aurink/internal/contract"
	"aurink/internal/core/markdown"
	"aurink/internal/core/text"
	"github.com/egoist/mygo/ui/markdown/parser"
)

type fakeRenderer struct {
	available bool
	requests  []contract.RenderRequest
	render    func(context.Context, contract.RenderRequest) (contract.RenderResult, error)
}

func (f *fakeRenderer) Available() bool { return f.available }
func (f *fakeRenderer) Render(ctx context.Context, req contract.RenderRequest) (contract.RenderResult, error) {
	f.requests = append(f.requests, req)
	return f.render(ctx, req)
}

func TestRenderedObjectsAndFallback(t *testing.T) {
	src := "$x$ and $x$ and $bad$\n\n$$\nx+1\n$$\n\n```mermaid\ngraph LR\n A-->B\n```\n\n```dot\ndigraph{}\n```\n"
	txt, ast := parse(t, src)
	f := &fakeRenderer{available: true, render: func(_ context.Context, req contract.RenderRequest) (contract.RenderResult, error) {
		if req.Source == "bad" {
			return contract.RenderResult{}, errors.New("bad formula")
		}
		if req.Kind == contract.RenderDiagram {
			return contract.RenderResult{PNG: []byte("png"), Width: 40, Height: 30}, nil
		}
		return contract.RenderResult{SVG: []byte(`<svg xmlns="http://www.w3.org/2000/svg"><path d="M0 0"/></svg>`), Width: 20, Height: 10, Ascent: 7}, nil
	}}
	var out bytes.Buffer
	err := (&Exporter{ObjectLangs: []string{"dot"}}).Export(context.Background(), txt, ast, contract.ExportOptions{Format: contract.ExportHTML, Fragment: true, Dark: true, Renderer: f}, &out)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`<span class="math"><span style="display:inline-block;width:20px;height:10px;vertical-align:-3px;">`, `<svg xmlns=`, `<span class="math">bad</span>`, `<div class="math">`, `<figure class="diagram"><img`, `data:image/png;base64,cG5n`} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("missing %q in %s", want, out.String())
		}
	}
	if len(f.requests) != 5 {
		t.Fatalf("requests = %+v", f.requests)
	}
	for _, req := range f.requests {
		if !req.Dark || req.Color != "#ecedf2" || req.FontSize != 16 || req.Scale != 2 {
			t.Errorf("render settings: %+v", req)
		}
	}
	for _, unavailable := range []contract.Renderer{nil, &fakeRenderer{}} {
		out.Reset()
		if err := New().Export(context.Background(), txt, ast, contract.ExportOptions{Format: contract.ExportHTML, Fragment: true, Renderer: unavailable}, &out); err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{`<span class="math">x</span>`, `<div class="math">x+1</div>`, `<pre><code class="language-mermaid">`} {
			if !strings.Contains(out.String(), want) {
				t.Errorf("fallback missing %q in %s", want, out.String())
			}
		}
	}
}

func TestUnsafeRenderedSVGUsesFallback(t *testing.T) {
	for _, svg := range []string{
		`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`,
		`<svg xmlns="http://www.w3.org/2000/svg" onload="alert(1)"/>`,
		`<svg xmlns="http://www.w3.org/2000/svg"><use href="https://example.test/x"/></svg>`,
		`<svg xmlns="http://www.w3.org/2000/svg"><use href="java&#x73;cript:alert(1)"/></svg>`,
		`<svg xmlns="http://www.w3.org/2000/svg"><path style="fill:url(https://example.test/x)"/></svg>`,
		`<svg xmlns="http://www.w3.org/2000/svg"><g style="background-image:image-set('https://example.test/x')"/></svg>`,
		`<svg xmlns="http://www.w3.org/2000/svg"><path src="https://example.test/x"/></svg>`,
	} {
		f := &fakeRenderer{available: true, render: func(context.Context, contract.RenderRequest) (contract.RenderResult, error) {
			return contract.RenderResult{SVG: []byte(svg), Width: 10, Height: 10, Ascent: 8}, nil
		}}
		out := fragment(t, "$x$\n", contract.ExportOptions{Renderer: f})
		if out != "<p><span class=\"math\">x</span></p>\n" {
			t.Errorf("unsafe SVG emitted: %s", out)
		}
	}
}

func TestPNGMathAndWikiTargets(t *testing.T) {
	f := &fakeRenderer{available: true, render: func(context.Context, contract.RenderRequest) (contract.RenderResult, error) {
		return contract.RenderResult{PNG: []byte("png"), Width: 20, Height: 10, Ascent: 6}, nil
	}}
	out := fragment(t, "$x$ [[javascript:alert|safe]] [[中文 标题#某 一节]]\n", contract.ExportOptions{Renderer: f})
	for _, want := range []string{`vertical-align:-4px;`, `src="data:image/png;base64,cG5n"`, `href="javascript%3Aalert.md"`, `href="%E4%B8%AD%E6%96%87%20%E6%A0%87%E9%A2%98.md#某-一节"`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in %s", want, out)
		}
	}
}

func TestRenderCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f := &fakeRenderer{available: true, render: func(ctx context.Context, req contract.RenderRequest) (contract.RenderResult, error) {
		cancel()
		return contract.RenderResult{}, contract.E(contract.CodeCanceled, "fake", ctx.Err())
	}}
	r := renderer{ctx: ctx, opts: contract.ExportOptions{Renderer: f}, rendered: make(map[contract.RenderRequest]rendered)}
	r.renderObject(contract.RenderMathInline, "x", "")
	r.renderObject(contract.RenderMathInline, "y", "")
	if !errors.Is(r.err, context.Canceled) || len(f.requests) != 1 {
		t.Fatalf("error = %v, requests = %+v", r.err, f.requests)
	}
	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()
	f = &fakeRenderer{available: true, render: func(ctx context.Context, req contract.RenderRequest) (contract.RenderResult, error) {
		cancel()
		return contract.RenderResult{}, contract.E(contract.CodeCanceled, "fake", ctx.Err())
	}}
	txt, ast := parse(t, "$x$ $y$\n")
	err := New().Export(ctx, txt, ast, contract.ExportOptions{Format: contract.ExportHTML, Renderer: f}, &bytes.Buffer{})
	if contract.CodeOf(err) != contract.CodeCanceled || len(f.requests) != 1 {
		t.Fatalf("export error = %v, requests = %+v", err, f.requests)
	}
}

func TestExtensionsOnAndOff(t *testing.T) {
	tests := []struct {
		name, src, enabled string
		flag               contract.Extensions
	}{
		{"highlight", "==marked==\n", "<p><mark>marked</mark></p>\n", contract.ExtHighlight},
		{"sup", "^up^\n", "<p><sup>up</sup></p>\n", contract.ExtSupSub},
		{"sub", "~down~\n", "<p><sub>down</sub></p>\n", contract.ExtSupSub | contract.ExtTildeSub},
		{"emoji", ":smile:\n", "<p>😄</p>\n", contract.ExtEmoji},
		{"dollar math", "$x$\n", "<p><span class=\"math\">x</span></p>\n", contract.ExtDollarMath},
		{"double-dollar math", "$$x$$\n", "<p><span class=\"math\">x</span></p>\n", contract.ExtDollarMath},
		{"wiki", "[[Foo Bar#Hello, World|alias]]\n", "<p><a class=\"wikilink\" href=\"Foo%20Bar.md#hello-world\">alias</a></p>\n", contract.ExtWikiLink},
		{"toc", "# Head\n\n[TOC]\n", "<h1 id=\"head\">Head</h1>\n<nav class=\"toc\"><ul>\n<li class=\"toc-level-1\"><a href=\"#head\">Head</a></li>\n</ul></nav>\n", contract.ExtTOC},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for _, enabled := range []bool{false, true} {
				ext := contract.Extensions(0)
				if enabled {
					ext = tc.flag
				}
				p, err := markdown.New(tc.src, 1, ext)
				if err != nil {
					t.Fatal(err)
				}
				var out bytes.Buffer
				if err := New().Export(context.Background(), text.New(tc.src, 1), p.Snapshot(), contract.ExportOptions{Format: contract.ExportHTML, Fragment: true}, &out); err != nil {
					t.Fatal(err)
				}
				want := parser.Parse(tc.src).HTML()
				if enabled {
					want = tc.enabled
				}
				// Heading IDs are an export rule, independent of the extensions.
				if !enabled && tc.name == "toc" {
					want = strings.Replace(want, "<h1>", "<h1 id=\"head\">", 1)
				}
				if !enabled && tc.name == "double-dollar math" {
					want = "<div class=\"math\">x</div>\n"
				}
				if out.String() != want {
					t.Errorf("enabled=%v: got %q want %q", enabled, out.String(), want)
				}
			}
		})
	}
}

func TestFrontMatterDisabledMatchesGFM(t *testing.T) {
	src := "---\ntitle: Example\n---\n\nbody\n"
	p, err := markdown.New(src, 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := New().Export(context.Background(), text.New(src, 1), p.Snapshot(), contract.ExportOptions{Format: contract.ExportHTML, Fragment: true}, &out); err != nil {
		t.Fatal(err)
	}
	want := strings.Replace(parser.Parse(src).HTML(), "<h2>", `<h2 id="title-example">`, 1)
	if out.String() != want {
		t.Fatalf("got %q want %q", out.String(), want)
	}
}

func TestFrontMatterTitleAndTOC(t *testing.T) {
	src := "---\ntitle: \"A # 中文\"\ntags: [one, two]\n---\n\n# Head\n\n[TOC]\n"
	txt, ast := parse(t, src)
	var out bytes.Buffer
	if err := New().Export(context.Background(), txt, ast, contract.ExportOptions{Format: contract.ExportHTML, TOC: true}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "<title>A # 中文</title>") || strings.Count(out.String(), `<nav class="toc">`) != 2 || strings.Contains(out.String(), "tags: [one, two]") {
		t.Fatal(out.String())
	}
	out.Reset()
	if err := New().Export(context.Background(), txt, ast, contract.ExportOptions{Format: contract.ExportHTML, Title: "Override"}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "<title>Override</title>") {
		t.Fatal(out.String())
	}
}
