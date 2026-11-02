package richpaste

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRealPasteGoldens(t *testing.T) {
	files, err := filepath.Glob("testdata/paste/*.html")
	if err != nil || len(files) == 0 {
		t.Fatalf("fixtures: %v", err)
	}
	for _, name := range files {
		t.Run(filepath.Base(name), func(t *testing.T) {
			input, err := os.ReadFile(name)
			if err != nil {
				t.Fatal(err)
			}
			want, err := os.ReadFile(strings.TrimSuffix(name, ".html") + ".md")
			if err != nil {
				t.Fatal(err)
			}
			got, err := New().HTMLToMarkdown(string(input))
			if err != nil {
				t.Fatal(err)
			}
			if got != strings.TrimSuffix(string(want), "\n") {
				t.Fatalf("got:\n%s\nwant:\n%s", got, want)
			}
		})
	}
}

func TestConversionEdges(t *testing.T) {
	for _, tt := range []struct{ html, want string }{
		{`<b style="font-weight:normal" id="docs-internal-guid-a"><p>plain <span style="font-weight:700">bold</span> <span style="font-style:italic">italic</span></p></b>`, "plain **bold** *italic*"},
		{`<ul><li>one</li><ul><li>nested</li></ul><li>two</li></ul>`, "- one\n  - nested\n- two"},
		{`<p><a href="javascript:alert(1)" onclick="x()">label</a><img src="data:image/png,a"><script>evil()</script></p>`, "label"},
		{`<p class="MsoListParagraph" style="mso-list:l0 level1 lfo1"><span style="mso-list:Ignore">1.&nbsp; </span>First</p><p style="mso-list:l0 level1 lfo1"><span style="mso-list:Ignore">2.&nbsp; </span>Next</p>`, "1. First\n2. Next"},
	} {
		got, err := New().HTMLToMarkdown(tt.html)
		if err != nil || got != tt.want {
			t.Errorf("%s: got %q, want %q, err %v", tt.html, got, tt.want, err)
		}
	}
	for _, tt := range []struct{ rtf, want string }{
		{`{\rtf1\ansi caf\'e9 \u-10179?\u-8704?}`, "café 😀"},
		{`{\rtf1\ansi\ansicpg1251 \'cf\'f0\'e8\'e2\'e5\'f2}`, "Привет"},
		{`{\rtf1\ansi a {\b strong {\i both} strong} end}`, "a **strong** ***both*** **strong** end"},
	} {
		got, err := New().RTFToMarkdown(tt.rtf)
		if err != nil || got != tt.want {
			t.Errorf("%s: got %q, want %q, err %v", tt.rtf, got, tt.want, err)
		}
	}
}

var benchmarkMarkdown string

func BenchmarkHTML1MB(b *testing.B) {
	unit := `<p>This is <strong>bold text</strong>, <em>emphasis</em> and <a href="https://example.org/a">a link</a>.</p>`
	input := strings.Repeat(unit, (1<<20)/len(unit)+1)[:1<<20]
	c := New()
	b.SetBytes(int64(len(input)))
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		var err error
		benchmarkMarkdown, err = c.HTMLToMarkdown(input)
		if err != nil {
			b.Fatal(err)
		}
	}
}
