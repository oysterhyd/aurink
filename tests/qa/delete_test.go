package qa

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// Select all, then Backspace or Delete, empties a document that has a table.
func TestSelectAllDeletesATable(t *testing.T) {
	for _, k := range []ui.Key{ui.KeyBackspace, ui.KeyDelete} {
		for name, doc := range map[string]string{
			"table only":   "| a | b |\n| - | - |\n| 1 | 2 |\n",
			"text + table": "text\n\n| a | b |\n| - | - |\n| 1 | 2 |\n",
		} {
			a := newApp(t)
			a.open("t.md", doc)
			a.caret(strings.Index(doc, "1"))
			a.key(ui.Cmd, ui.KeyA)
			a.key(0, k)
			if got := a.text(); strings.TrimSpace(got) != "" {
				t.Errorf("%s, key %v: the document is %q", name, k, got)
			}
		}
	}
}
