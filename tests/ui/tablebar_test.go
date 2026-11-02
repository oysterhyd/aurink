package ui_test

import (
	"strings"
	"testing"

	"aurink/locales"
	"github.com/egoist/mygo/ui"
)

// rowShowing returns where the text of the row displaying text starts.
func (h *harness) rowShowing(text string) ui.Point {
	h.t.Helper()
	for r := range h.e.RowCount() {
		if strings.TrimSpace(h.e.Display(r)) == text {
			return h.e.RowText(r)
		}
	}
	h.t.Fatalf("no row shows %q", text)
	return ui.Point{}
}

const tableDoc = "Before\n\n| a | b |\n|---|---|\n| 1 | 2 |\n\nAfter\n"

// TestTableBarAddsARowAndAColumn shows the toolbar while the caret is in a
// table, and its buttons run the table commands (02 §5.8).
func TestTableBarAddsARowAndAColumn(t *testing.T) {
	h := openRail(t, tableDoc)
	if _, ok := h.tt.Find(locales.LiveeditRowBelow()); ok {
		t.Fatal("the toolbar shows with the caret outside the table")
	}
	h.caret(strings.Index(tableDoc, "1 |"))
	if _, ok := h.tt.Find(locales.LiveeditRowBelow()); !ok {
		t.Fatal("no toolbar with the caret in a cell")
	}
	if err := h.tt.Click(locales.LiveeditRowBelow()); err != nil {
		t.Fatal(err)
	}
	h.tt.Frame()
	src := h.m.Text().String()
	if strings.Count(src, "\n|") != 4 {
		t.Fatalf("no row added:\n%s", src)
	}
	if !h.e.Focused() {
		t.Fatal("the button took the focus from the document")
	}
	if err := h.tt.Click(locales.LiveeditColRight()); err != nil {
		t.Fatal(err)
	}
	h.tt.Frame()
	if first := strings.Split(h.m.Text().String(), "\n")[2]; strings.Count(first, "|") != 4 {
		t.Fatalf("no column added: %q", first)
	}
	// Only the table changed.
	got := h.m.Text().String()
	if !strings.HasPrefix(got, "Before\n\n") || !strings.HasSuffix(got, "\n\nAfter\n") {
		t.Fatalf("text outside the table changed:\n%s", got)
	}
	if err := h.e.CheckRows(); err != nil {
		t.Fatal(err)
	}
}

// TestTableMenuDeletesTheRowClicked opens the context menu on a cell: the caret
// moves there first, so the menu acts on that row.
func TestTableMenuDeletesTheRowClicked(t *testing.T) {
	h := openRail(t, tableDoc)
	h.caret(0)
	at := h.rowShowing("2")
	h.tt.RightClickAt(at.X+4, at.Y+10)
	h.tt.Frame()
	if err := h.tt.Click(locales.LiveeditDeleteRow()); err != nil {
		t.Fatal(err)
	}
	h.tt.Frame()
	h.wantSrc("Before\n\n| a | b |\n|---|---|\n\nAfter\n")
}

// TestTableKeys: Tab from the last cell adds a row and goes to it, and
// Primary+Enter adds a row below (02 §5.8).
func TestTableKeys(t *testing.T) {
	h := openRail(t, tableDoc)
	h.caret(strings.Index(tableDoc, "2 |") + 1)
	h.key(ui.KeyTab)
	src := h.m.Text().String()
	if strings.Count(src, "\n|") != 4 {
		t.Fatalf("Tab in the last cell added no row:\n%s", src)
	}
	if c := h.m.Selection().Primary().Head; c < strings.Index(src, "2 |") {
		t.Fatalf("the caret stayed at %d", c)
	}
	h.caret(strings.Index(src, "a |"))
	h.mod(ui.Cmd, ui.KeyEnter)
	got := h.m.Text().String()
	if strings.Count(got, "\n|") != 5 {
		t.Fatalf("Primary+Enter added no row:\n%s", got)
	}
	if !strings.HasPrefix(got, "Before\n\n") || !strings.HasSuffix(got, "\n\nAfter\n") {
		t.Fatalf("text outside the table changed:\n%s", got)
	}
}

// TestTableMenuIsOnlyInTables: a right click in a paragraph offers no table
// items.
func TestTableMenuIsOnlyInTables(t *testing.T) {
	h := openRail(t, tableDoc)
	at := h.rowShowing("Before")
	h.tt.RightClickAt(at.X+4, at.Y+10)
	h.tt.Frame()
	if h.tt.HasText(locales.LiveeditDeleteRow()) {
		t.Fatal("table items in a paragraph's menu")
	}
}
