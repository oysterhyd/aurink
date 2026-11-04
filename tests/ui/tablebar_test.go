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

// TestTableBarAddsARowAndAColumn shows Typora's toolbar while the caret is in
// a table: its alignment buttons set the caret's column, the lit one again
// clears it, and its More menu runs the row and column commands.
func TestTableBarAddsARowAndAColumn(t *testing.T) {
	h := openRail(t, tableDoc)
	if _, ok := h.tt.Find(locales.LiveeditTableMore()); ok {
		t.Fatal("the toolbar shows with the caret outside the table")
	}
	h.caret(strings.Index(tableDoc, "1 |"))
	for _, label := range []string{locales.LiveeditTableSize(), locales.LiveeditAlignLeft(), locales.LiveeditAlignCenter(),
		locales.LiveeditAlignRight(), locales.LiveeditTableMore(), locales.LiveeditDeleteTable()} {
		if _, ok := h.tt.Find(label); !ok {
			t.Fatalf("no %q on the toolbar with the caret in a cell", label)
		}
	}
	if err := h.tt.Click(locales.LiveeditAlignCenter()); err != nil {
		t.Fatal(err)
	}
	h.tt.Frame()
	if got := strings.Split(h.m.Text().String(), "\n")[3]; got != "|:---:|---|" {
		t.Fatalf("the column was not centred: %q", got)
	}
	if err := h.tt.Click(locales.LiveeditAlignCenter()); err != nil {
		t.Fatal(err)
	}
	h.tt.Frame()
	if got := strings.Split(h.m.Text().String(), "\n")[3]; strings.Contains(got, ":") {
		t.Fatalf("centre clicked again did not clear the alignment: %q", got)
	}
	if !h.e.Focused() {
		t.Fatal("the button took the focus from the document")
	}
	if err := h.tt.Click(locales.LiveeditTableMore()); err != nil {
		t.Fatal(err)
	}
	h.tt.Frame()
	h.tt.Frame()
	if err := h.tt.Click(locales.LiveeditRowBelow()); err != nil {
		t.Fatal(err)
	}
	h.tt.Frame()
	src := h.m.Text().String()
	if strings.Count(src, "\n|") != 4 {
		t.Fatalf("no row added:\n%s", src)
	}
	if err := h.tt.Click(locales.LiveeditTableMore()); err != nil {
		t.Fatal(err)
	}
	h.tt.Frame()
	h.tt.Frame()
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

// TestTableMoreMenuKeepsTheToolbar: the More menu takes the focus while it
// shows, and the toolbar holding it stays; Escape gives the document its
// focus back, the caret in its cell.
func TestTableMoreMenuKeepsTheToolbar(t *testing.T) {
	h := openRail(t, tableDoc)
	h.caret(strings.Index(tableDoc, "1 |"))
	if err := h.tt.Click(locales.LiveeditTableMore()); err != nil {
		t.Fatal(err)
	}
	h.tt.Frame()
	h.tt.Frame()
	if !h.tt.HasText(locales.LiveeditRowBelow()) {
		t.Fatal("the More menu closed as it took the focus")
	}
	h.key(ui.KeyEscape)
	h.tt.Frame()
	if h.tt.HasText(locales.LiveeditRowBelow()) || !h.e.Focused() {
		t.Fatal("Escape left the menu up or the document without the focus")
	}
	if _, ok := h.tt.Find(locales.LiveeditTableMore()); !ok {
		t.Fatal("the toolbar went with the menu")
	}
	if c := h.m.Selection().Primary().Head; c != strings.Index(tableDoc, "1 |") {
		t.Fatalf("the caret moved to %d", c)
	}
}

// TestTableSizeGridResizesAndTrashDeletes: the size grid shows the table's
// size and a click on a cell resizes the table to it, the header row counted;
// the trash deletes the table, the text around it kept (Typora's toolbar).
func TestTableSizeGridResizesAndTrashDeletes(t *testing.T) {
	h := openRail(t, tableDoc)
	h.caret(strings.Index(tableDoc, "1 |"))
	if err := h.tt.Click(locales.LiveeditTableSize()); err != nil {
		t.Fatal(err)
	}
	h.tt.Frame()
	if !h.tt.HasText(locales.LiveeditSlashSizeN("2", "2")) {
		t.Fatal("the grid does not show the table's size, 2 rows by 2 columns")
	}
	if err := h.tt.Click(locales.LiveeditSlashSizeN("4", "3")); err != nil {
		t.Fatal(err)
	}
	h.tt.Frame()
	src := h.m.Text().String()
	lines := strings.Split(strings.TrimSuffix(strings.TrimPrefix(src, "Before\n\n"), "\n\nAfter\n"), "\n")
	if len(lines) != 5 || strings.Count(lines[0], "|") != 4 {
		t.Fatalf("not resized to a header and 3 rows of 3 columns:\n%s", src)
	}
	if _, ok := h.tt.Find(locales.LiveeditSlashSizeN("4", "3")); ok {
		t.Fatal("the grid stayed up after the click")
	}
	if !h.e.Focused() {
		t.Fatal("the grid took the focus from the document")
	}
	if err := h.tt.Click(locales.LiveeditDeleteTable()); err != nil {
		t.Fatal(err)
	}
	h.tt.Frame()
	got := h.m.Text().String()
	if strings.Contains(got, "|") || !strings.HasPrefix(got, "Before\n\n") || !strings.HasSuffix(got, "\n\nAfter\n") {
		t.Fatalf("the table was not deleted alone:\n%q", got)
	}
	if _, ok := h.tt.Find(locales.LiveeditTableMore()); ok {
		t.Fatal("the toolbar stayed with no table")
	}
}
