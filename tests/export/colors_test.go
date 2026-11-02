package export_test

import (
	"bytes"
	"context"
	"fmt"
	"regexp"
	"testing"

	"aurink/internal/contract"
	"aurink/internal/core/export"
	"aurink/internal/core/markdown"
	"aurink/internal/core/text"
	"aurink/internal/views/theme"
	"github.com/egoist/mygo/ui"
)

func TestCSSColorsMatchTheme(t *testing.T) {
	p, err := markdown.New("", 1, contract.ExtDefault)
	if err != nil {
		t.Fatal(err)
	}
	variables := regexp.MustCompile(`--([a-z-]+):\s*(#[0-9a-f]{8})`)
	for _, dark := range []bool{false, true} {
		var out bytes.Buffer
		err := export.New().Export(context.Background(), text.New("", 1), p.Snapshot(),
			contract.ExportOptions{Format: contract.ExportHTML, Dark: dark}, &out)
		if err != nil {
			t.Fatal(err)
		}
		got := map[string]string{}
		for _, match := range variables.FindAllStringSubmatch(out.String(), -1) {
			got[match[1]] = match[2]
		}
		tokens := theme.For(theme.Options{Dark: dark})
		want := map[string]ui.Color{
			"sheet": tokens.Sheet, "text": tokens.Text, "muted": tokens.Muted,
			"faint": tokens.Faint, "border": tokens.Border, "accent": tokens.Accent,
			"mark": tokens.Mark, "code-bg": tokens.CodeBg, "syntax-keyword": tokens.SynKeyword,
			"syntax-func": tokens.SynFunc, "syntax-comment": tokens.SynComment, "syntax-string": tokens.SynString,
		}
		if len(got) != len(want) {
			t.Errorf("dark=%v: found %d CSS colors, want %d", dark, len(got), len(want))
		}
		for name, c := range want {
			expected := fmt.Sprintf("#%02x%02x%02x%02x", c.R, c.G, c.B, c.A)
			if got[name] != expected {
				t.Errorf("dark=%v: --%s = %q, want %q", dark, name, got[name], expected)
			}
		}
	}
}
