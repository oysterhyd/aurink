package export

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"aurink/internal/contract"
)

func TestWikiTargets(t *testing.T) {
	for _, tc := range []struct{ target, want string }{
		{"notes/Plan", "notes/Plan.md"},
		{"Plan.md", "Plan.md"},
		{" Plan ", "Plan.md"},
		{"#Part", "#part"},
		{"a b/c#d e", "a%20b/c.md#d-e"},
		{"javascript:alert", "javascript%3Aalert.md"},
		{"//example.test/page", "%2F/example.test/page.md"},
	} {
		t.Run(tc.target, func(t *testing.T) {
			if got := wikiURL(tc.target); got != tc.want {
				t.Fatalf("wikiURL = %q, want %q", got, tc.want)
			}
			out := fragment(t, "[["+tc.target+"|label]]\n", contract.ExportOptions{})
			if !strings.Contains(out, `href="`+tc.want+`"`) {
				t.Fatalf("output = %q", out)
			}
		})
	}
}

func TestFrontMatterOnlyTitle(t *testing.T) {
	for _, tc := range []struct{ front, want string }{
		{"kicker: Heading", ""},
		{"title: Real title\nkicker: Heading", "Real title"},
		{"title: \"A # B\"", "A # B"},
	} {
		txt, ast := parse(t, "---\n"+tc.front+"\n---\n\nbody\n")
		var out bytes.Buffer
		if err := New().Export(context.Background(), txt, ast, contract.ExportOptions{Format: contract.ExportHTML}, &out); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out.String(), "<title>"+tc.want+"</title>") {
			t.Fatalf("front matter %q: %s", tc.front, out.String())
		}
	}
}

func TestSoftBreakModes(t *testing.T) {
	for _, tc := range []struct {
		mode contract.SoftBreakMode
		want string
	}{
		{contract.SoftBreakNewline, "<p>one<br />\ntwo</p>\n"},
		{contract.SoftBreakSpace, "<p>one\ntwo</p>\n"},
	} {
		if got := fragment(t, "one\ntwo\n", contract.ExportOptions{SoftBreak: tc.mode}); got != tc.want {
			t.Errorf("mode %v: %q, want %q", tc.mode, got, tc.want)
		}
	}
}

func TestEmptyBaseDoesNotEmbedFromWorkingDirectory(t *testing.T) {
	path := filepath.Join("..", "..", "..", "testdata", "pdf", "aurora.png")
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	dest := filepath.ToSlash(path)
	out := fragment(t, "![image]("+dest+")\n", contract.ExportOptions{EmbedImages: true})
	if strings.Contains(out, "data:image/") || !strings.Contains(out, `src="`+dest+`"`) {
		t.Fatalf("empty base embedded local file: %s", out)
	}
}

func TestNestedTOCOptionAndBlock(t *testing.T) {
	src := "# A\n\n### C\n\n## B\n\n# D\n\n[TOC]\n"
	out := fragment(t, src, contract.ExportOptions{TOC: true})
	want := "<nav class=\"toc\"><ul>\n" +
		"<li class=\"toc-level-1\"><a href=\"#a\">A</a><ul>\n" +
		"<li class=\"toc-level-3\"><a href=\"#c\">C</a></li>\n" +
		"<li class=\"toc-level-2\"><a href=\"#b\">B</a></li>\n" +
		"</ul></li>\n<li class=\"toc-level-1\"><a href=\"#d\">D</a></li>\n</ul></nav>\n"
	if strings.Count(out, want) != 2 || strings.Contains(out, "<ol>") {
		t.Fatalf("TOCs do not match nested outline:\n%s", out)
	}
	if strings.Index(out, want) > strings.Index(out, `id="a"`) || strings.LastIndex(out, want) < strings.Index(out, `id="d"`) {
		t.Fatalf("TOC positions: %s", out)
	}
}

func TestPrintCSS(t *testing.T) {
	txt, ast := parse(t, "text\n")
	for _, dark := range []bool{false, true} {
		var out bytes.Buffer
		if err := New().Export(context.Background(), txt, ast, contract.ExportOptions{Format: contract.ExportHTML, Dark: dark}, &out); err != nil {
			t.Fatal(err)
		}
		css := out.String()
		for _, want := range []string{"Microsoft YaHei", "PingFang SC", "Noto Sans CJK SC", "monospace", "font-variant-ligatures: none", "@media print", "--sheet: #fff", "html, body { background: #fff", "white-space: pre-wrap", "overflow-wrap: anywhere", "pre, img, figure, tr, div.math", "break-inside: avoid", "break-after: avoid", "width: calc(100% - 2px)", ".toc ul { list-style: none"} {
			if !strings.Contains(css, want) {
				t.Errorf("dark=%v missing %q", dark, want)
			}
		}
		filter := `.math img, .math svg, .diagram img { filter: invert(1) hue-rotate(180deg); }`
		if strings.Contains(css, filter) != dark {
			t.Errorf("dark=%v print filter: %s", dark, css)
		}
	}
}
