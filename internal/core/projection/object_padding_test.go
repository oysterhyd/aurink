package projection

import (
	"strings"
	"testing"

	"aurink/internal/contract"
)

func TestObjectSourceIncludesPartialTabPadding(t *testing.T) {
	for _, tc := range []struct {
		name, src, want string
		block           contract.BlockKind
		object          contract.ObjectKind
	}{
		{"diagram", "- ```mermaid\n\tgraph TD\n\t  a-->b\n  ```\n", "  graph TD\n    a-->b", contract.BlockCode, contract.ObjDiagram},
		{"math", "- $$\n\tE=mc^2\n\tx+y\n  $$\n", "  E=mc^2\n  x+y", contract.BlockMath, contract.ObjMath},
	} {
		for _, ending := range []string{"\n", "\r\n"} {
			name := tc.name + "/LF"
			if ending == "\r\n" {
				name = tc.name + "/CRLF"
			}
			t.Run(name, func(t *testing.T) {
				text, ast := parse(t, strings.ReplaceAll(tc.src, "\n", ending))
				for leaf := range ast.Leaves() {
					b := ast.Block(leaf)
					if b.Kind != tc.block {
						continue
					}
					if len(b.Lines) != 2 || b.Lines[0].Padding != 2 || b.Lines[1].Padding != 2 {
						t.Fatalf("expected a line with two padding spaces: %+v", b.Lines)
					}
					p := New().Project(ast, text, leaf, contract.Reveal{Mode: contract.RevealNone, Caret: -1, ObjectLangs: []string{"mermaid"}})
					for _, segment := range p.Segments {
						if segment.Kind == contract.SegObject && segment.Object != nil && segment.Object.Kind == tc.object {
							if got := segment.Object.Source; got != tc.want {
								t.Errorf("source %q, want %q", got, tc.want)
							}
							return
						}
					}
					t.Fatalf("missing object in %+v", p.Segments)
				}
				t.Fatalf("missing block kind %d", tc.block)
			})
		}
	}
}
