package contracttest

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"aurink/internal/contract"
	"aurink/tests/corpus"
)

// ExportFixture builds an HTML exporter.
type ExportFixture struct {
	New func() contract.Exporter
	// Parse returns the text and AST of src with the default extensions.
	Parse func(src string) (contract.Text, contract.AST)
}

var tagPattern = regexp.MustCompile(`<(/?)([a-zA-Z][a-zA-Z0-9-]*)[^>]*?(/?)>`)

var voidTags = map[string]bool{"area": true, "base": true, "br": true, "col": true, "embed": true, "hr": true,
	"img": true, "input": true, "link": true, "meta": true, "source": true, "track": true, "wbr": true}

// unclosed returns the first tag that does not nest, or "".
func unclosed(html string) string {
	// Text inside <style> is not markup.
	html = regexp.MustCompile(`(?s)<style>.*?</style>`).ReplaceAllString(html, "")
	html = regexp.MustCompile(`(?s)<!--.*?-->|<!DOCTYPE[^>]*>`).ReplaceAllString(html, "")
	var stack []string
	for _, m := range tagPattern.FindAllStringSubmatch(html, -1) {
		name := strings.ToLower(m[2])
		if voidTags[name] || m[3] == "/" {
			continue
		}
		if m[1] == "" {
			stack = append(stack, name)
			continue
		}
		if len(stack) == 0 || stack[len(stack)-1] != name {
			return "</" + name + ">"
		}
		stack = stack[:len(stack)-1]
	}
	if len(stack) != 0 {
		return "<" + stack[len(stack)-1] + ">"
	}
	return ""
}

// TestExporter checks BE-2.8 and the export rules of docs/04 for exporters.
func TestExporter(t *testing.T, f ExportFixture) {
	export := func(t *testing.T, src string, opts contract.ExportOptions) string {
		t.Helper()
		text, ast := f.Parse(src)
		opts.Format = contract.ExportHTML
		var b bytes.Buffer
		if err := f.New().Export(context.Background(), text, ast, opts, &b); err != nil {
			t.Fatal(err)
		}
		out := b.String()
		if tag := unclosed(out); tag != "" {
			t.Fatalf("%s does not nest in:\n%s", tag, out)
		}
		return out
	}
	has := func(t *testing.T, out string, parts ...string) {
		t.Helper()
		for _, p := range parts {
			if !strings.Contains(out, p) {
				t.Errorf("no %q in:\n%s", p, out)
			}
		}
	}
	hasNot := func(t *testing.T, out string, parts ...string) {
		t.Helper()
		for _, p := range parts {
			if strings.Contains(strings.ToLower(out), strings.ToLower(p)) {
				t.Errorf("%q in:\n%s", p, out)
			}
		}
	}
	t.Run("document and fragment", func(t *testing.T) {
		out := export(t, "# T\n\nbody\n", contract.ExportOptions{Title: "a<b"})
		if !strings.HasPrefix(out, "<!DOCTYPE html>") {
			t.Fatalf("starts %q", out[:min(len(out), 40)])
		}
		has(t, out, "<title>a&lt;b</title>", "<style>", "<p>body</p>")
		frag := export(t, "# T\n\nbody\n", contract.ExportOptions{Fragment: true})
		hasNot(t, frag, "<!DOCTYPE", "<style", "<html")
		has(t, frag, "<p>body</p>")
	})
	t.Run("dark theme differs", func(t *testing.T) {
		if export(t, "x\n", contract.ExportOptions{}) == export(t, "x\n", contract.ExportOptions{Dark: true}) {
			t.Fatal("Dark changed nothing")
		}
	})
	t.Run("heading ids and toc", func(t *testing.T) {
		out := export(t, "# Hello World\n\n## 你好，世界\n\n# Hello World\n\n# !!!\n", contract.ExportOptions{TOC: true, Fragment: true})
		has(t, out, `id="hello-world"`, `id="你好世界"`, `id="hello-world-1"`, `id="section"`,
			`<nav class="toc">`, `href="#hello-world"`, `href="#hello-world-1"`)
		if strings.Index(out, `<nav class="toc">`) > strings.Index(out, `id="hello-world"`) {
			t.Error("the toc is not before the body")
		}
	})
	t.Run("code", func(t *testing.T) {
		hl := func(lang, code string) []contract.CodeToken {
			if lang != "go" {
				return nil
			}
			return []contract.CodeToken{{Range: contract.Range{Start: 0, End: 2}, Kind: "keyword"}}
		}
		out := export(t, "```go\nif a<b {}\n```\n\n```\nplain <x>\n```\n", contract.ExportOptions{Fragment: true, Highlight: hl})
		has(t, out, `<pre><code class="language-go"><span class="tok-keyword">if</span> a&lt;b {}`, "plain &lt;x&gt;")
		plain := export(t, "```go\nif a<b {}\n```\n", contract.ExportOptions{Fragment: true})
		has(t, plain, `<code class="language-go">if a&lt;b {}`)
	})
	t.Run("footnotes", func(t *testing.T) {
		out := export(t, "a[^n1]\n\n[^n1]: the note\n", contract.ExportOptions{Fragment: true})
		has(t, out, `<sup class="footnote-ref"><a href="#fn-n1"`, `<section class="footnotes">`, `id="fn-n1"`, "the note")
		if strings.Index(out, `<section class="footnotes">`) < strings.Index(out, "footnote-ref") {
			t.Error("footnotes are not at the end")
		}
	})
	t.Run("raw html is made safe", func(t *testing.T) {
		out := export(t, "<script>alert(1)</script>\n\n<iframe src=x></iframe>\n\n"+
			"a <img src=x onerror=\"alert(1)\"> <kbd>Ctrl</kbd> [l](javascript:alert(1)) <a href=\"JavaScript:x\">b</a>\n",
			contract.ExportOptions{Fragment: true})
		hasNot(t, out, "<script", "<iframe", "onerror", "javascript:")
		has(t, out, "<kbd>Ctrl</kbd>")
	})
	t.Run("embedded images", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "pic.png"), []byte("\x89PNG\r\n\x1a\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		src := "![a](pic.png) ![b](missing.png) ![c](https://example.com/c.png)\n"
		out := export(t, src, contract.ExportOptions{Fragment: true, EmbedImages: true, BaseDir: dir})
		has(t, out, "data:image/png;base64,", `src="missing.png"`, `src="https://example.com/c.png"`)
		hasNot(t, out, `src="pic.png"`)
		has(t, export(t, src, contract.ExportOptions{Fragment: true, BaseDir: dir}), `src="pic.png"`)
	})
	t.Run("progress", func(t *testing.T) {
		var got []float32
		export(t, corpus.Generate(200_000, 1), contract.ExportOptions{OnProgress: func(p float32) { got = append(got, p) }})
		if len(got) == 0 || got[len(got)-1] != 1 {
			t.Fatalf("progress %v", got)
		}
		for i := 1; i < len(got); i++ {
			if got[i] < got[i-1] {
				t.Fatalf("progress went back: %v", got)
			}
		}
	})
	t.Run("canceled", func(t *testing.T) {
		text, ast := f.Parse(corpus.Generate(200_000, 1))
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		err := f.New().Export(ctx, text, ast, contract.ExportOptions{Format: contract.ExportHTML}, &bytes.Buffer{})
		wantCode(t, err, contract.CodeCanceled)
	})
	t.Run("corpus nests", func(t *testing.T) {
		entries, err := corpus.Index()
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range entries {
			b, err := corpus.Read(e)
			if err != nil {
				t.Fatal(err)
			}
			text, ast := f.Parse(string(b))
			var out bytes.Buffer
			if err := f.New().Export(context.Background(), text, ast, contract.ExportOptions{Format: contract.ExportHTML}, &out); err != nil {
				t.Errorf("%s: %v", e.Path, err)
				continue
			}
			if tag := unclosed(out.String()); tag != "" {
				t.Errorf("%s: %s does not nest", e.Path, tag)
			}
		}
	})
}
