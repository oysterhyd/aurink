package export

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"aurink/internal/contract"
	"aurink/internal/core/text"
	"aurink/tests/corpus"
)

func fragment(t *testing.T, src string, opts contract.ExportOptions) string {
	t.Helper()
	txt, ast := parse(t, src)
	opts.Format, opts.Fragment = contract.ExportHTML, true
	var out bytes.Buffer
	if err := New().Export(context.Background(), txt, ast, opts, &out); err != nil {
		t.Fatal(err)
	}
	return out.String()
}

func TestSafeHTMLAndURLs(t *testing.T) {
	src := "Text <a href=\"&#x6a;Ava&#x73;cript&#58;go\" onclick=\"bad()\">link</a> " +
		"<img src=\"java&#x09;script:go\" onerror=\"bad()\" title=\"x > y\"> " +
		"[bad](JaVaScRiPt:alert(1)) <kbd>Ctrl</kbd> <script>steal()</script>.\n\n" +
		"<iframe>keep <b>text</b> <script>discard()</script></iframe>\n\n" +
		"<details>\n\n<summary>Read</summary>\n\nA paragraph.\n\n</details>\n"
	out := fragment(t, src, contract.ExportOptions{})
	lower := strings.ToLower(out)
	for _, bad := range []string{"javascript:", "<script", "<iframe", "onclick", "onerror", "steal()", "discard()"} {
		if strings.Contains(lower, bad) {
			t.Errorf("unsafe %q in %s", bad, out)
		}
	}
	for _, want := range []string{"<kbd>Ctrl</kbd>", `title="x &gt; y"`, "keep text", "<details>", "<summary>Read</summary>", "<p>A paragraph.</p>", "</details>"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in %s", want, out)
		}
	}
	if strings.Index(out, "<details>") > strings.Index(out, "<p>A paragraph.</p>") ||
		strings.Index(out, "</details>") < strings.Index(out, "<p>A paragraph.</p>") {
		t.Errorf("details did not surround the paragraph: %s", out)
	}
}

func TestRawImageAndImagePaths(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a b.png"), []byte("\x89PNG\r\n\x1a\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out := fragment(t, "<img src=\"a%20b.png\" alt=\"picture\">\n", contract.ExportOptions{EmbedImages: true, BaseDir: dir})
	if !strings.Contains(out, `src="data:image/png;base64,`) || !strings.Contains(out, `alt="picture"`) {
		t.Fatalf("raw image was not embedded: %s", out)
	}
	absolute := strings.ReplaceAll(filepath.ToSlash(filepath.Join(dir, "a b.png")), " ", "%20")
	out = fragment(t, "<img src=\""+absolute+"\">\n", contract.ExportOptions{EmbedImages: true, BaseDir: dir})
	if !strings.Contains(out, `src="data:image/png;base64,`) {
		t.Fatalf("encoded absolute image was not embedded: %s", out)
	}
	fileURL := (&url.URL{Scheme: "file", Path: "/" + strings.TrimPrefix(filepath.ToSlash(filepath.Join(dir, "a b.png")), "/")}).String()
	out = fragment(t, "<img src=\""+fileURL+"\">\n", contract.ExportOptions{EmbedImages: true, BaseDir: dir})
	if !strings.Contains(out, `src="data:image/png;base64,`) {
		t.Fatalf("file URL was not embedded: %s", out)
	}
	r := renderer{opts: contract.ExportOptions{EmbedImages: true, BaseDir: dir}}
	for _, remote := range []string{"//example.test/a.png", `\\server\share\a.png`, "https://example.test/a.png"} {
		if got := r.imageURL(remote); got != remote {
			t.Errorf("network image %q changed to %q", remote, got)
		}
	}
	other := filepath.Join(t.TempDir(), "outside.png")
	if err := os.WriteFile(other, []byte("\x89PNG\r\n\x1a\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := fragment(t, "![outside](<"+filepath.ToSlash(other)+">)\n", contract.ExportOptions{EmbedImages: true, BaseDir: dir}); strings.Contains(got, "data:image/") {
		t.Fatalf("embedded an image outside BaseDir: %s", got)
	}
}

func TestFootnotesAndExtensions(t *testing.T) {
	src := "one[^a] twice[^a]\n\n[^a]: first [^b]\n\n[^b]: second [^a]\n\n" +
		"| A | B |\n| --- | --- |\n| x | y | overflow |\n\n" +
		"```mermaid\ngraph A-->B\n```\n\n$$\nx < y\n$$\n"
	out := fragment(t, src, contract.ExportOptions{})
	for _, want := range []string{`id="fnref-a"`, `id="fnref-a-2"`, `id="fnref-a-3"`,
		`href="#fnref-a-3"`, `id="fn-b"`, `<pre class="mermaid">graph A--&gt;B`,
		`<code class="math">x &lt; y</code>`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in %s", want, out)
		}
	}
	if strings.Contains(out, "overflow") || strings.Count(out, "<td>") != 2 {
		t.Errorf("GFM table did not limit row width: %s", out)
	}
}

type extensionAST struct {
	contract.AST
	toc, paragraph int
}

func (a extensionAST) Block(i int) *contract.Block {
	b := *a.AST.Block(i)
	if i == a.toc {
		b.Kind = contract.BlockTOC
	}
	return &b
}

func (a extensionAST) Inlines(i int) []contract.Inline {
	if i != a.paragraph {
		return a.AST.Inlines(i)
	}
	return []contract.Inline{
		{Kind: contract.InlineHighlight, Next: 2}, {Kind: contract.InlineText, Text: "marked", Next: 2},
		{Kind: contract.InlineSup, Next: 4}, {Kind: contract.InlineText, Text: "up", Next: 4},
		{Kind: contract.InlineSub, Next: 6}, {Kind: contract.InlineText, Text: "down", Next: 6},
		{Kind: contract.InlineEmoji, Text: "😀", Next: 7},
		{Kind: contract.InlineWikiLink, Dest: "Page", Next: 9}, {Kind: contract.InlineText, Text: "alias", Next: 9},
		{Kind: contract.InlineMath, Text: "x < y", Next: 10},
	}
}

func TestOptionalASTNodes(t *testing.T) {
	txt, base := parse(t, "# Head\n\n[TOC]\n\nplaceholder\n")
	var paragraphs []int
	for i := 1; i < base.Len(); i++ {
		if base.Block(i).Kind == contract.BlockParagraph {
			paragraphs = append(paragraphs, i)
		}
	}
	if len(paragraphs) != 2 {
		t.Fatalf("expected two paragraphs, got %v", paragraphs)
	}
	ast := extensionAST{AST: base, toc: paragraphs[0], paragraph: paragraphs[1]}
	var out bytes.Buffer
	err := New().Export(context.Background(), txt, ast, contract.ExportOptions{Format: contract.ExportHTML, Fragment: true}, &out)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`<nav class="toc">`, `href="#head"`, "<mark>marked</mark>",
		"<sup>up</sup>", "<sub>down</sub>", "😀", `<a href="Page">alias</a>`, `<code class="math">x &lt; y</code>`} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("missing %q in %s", want, out.String())
		}
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

type cancelWriter struct{ cancel context.CancelFunc }

func (w cancelWriter) Write(p []byte) (int, error) {
	w.cancel()
	return len(p), nil
}

func TestErrorsAndCancellation(t *testing.T) {
	txt, ast := parse(t, corpus.Generate(200_000, 7))
	var calls []float32
	ctx, cancel := context.WithCancel(context.Background())
	err := New().Export(ctx, txt, ast, contract.ExportOptions{Format: contract.ExportHTML, OnProgress: func(p float32) {
		calls = append(calls, p)
		if p >= .2 {
			cancel()
		}
	}}, io.Discard)
	if contract.CodeOf(err) != contract.CodeCanceled || len(calls) == 0 || calls[len(calls)-1] == 1 {
		t.Fatalf("cancellation: err=%v progress=%v", err, calls)
	}
	var progress []float32
	err = New().Export(context.Background(), txt, ast, contract.ExportOptions{Format: contract.ExportHTML, OnProgress: func(p float32) {
		progress = append(progress, p)
	}}, io.Discard)
	if err != nil || len(progress) > 41 || len(progress) == 0 || progress[len(progress)-1] != 1 {
		t.Fatalf("bounded progress: err=%v progress=%v", err, progress)
	}
	err = New().Export(context.Background(), txt, ast, contract.ExportOptions{Format: contract.ExportHTML}, failingWriter{})
	if contract.CodeOf(err) != contract.CodeIO || !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("writer error: %v", err)
	}
	smallText, smallAST := parse(t, "hello\n")
	flushCtx, flushCancel := context.WithCancel(context.Background())
	err = New().Export(flushCtx, smallText, smallAST, contract.ExportOptions{Format: contract.ExportHTML}, cancelWriter{flushCancel})
	if contract.CodeOf(err) != contract.CodeCanceled {
		t.Fatalf("cancellation during flush: %v", err)
	}
	err = New().Export(context.Background(), txt, ast, contract.ExportOptions{Format: contract.ExportPDF}, io.Discard)
	if contract.CodeOf(err) != contract.CodeUnavailable {
		t.Fatalf("PDF error: %v", err)
	}
	err = New().Export(context.Background(), text.New("different", 2), ast, contract.ExportOptions{Format: contract.ExportHTML}, io.Discard)
	if contract.CodeOf(err) != contract.CodeInvalid {
		t.Fatalf("snapshot mismatch: %v", err)
	}
}
