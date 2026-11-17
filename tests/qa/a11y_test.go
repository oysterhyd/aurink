package qa

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"aurink/internal/contract"
	"aurink/locales"
	"github.com/egoist/mygo/ui"
)

func (a *app) mustFind(t *testing.T, labels ...string) {
	t.Helper()
	for _, label := range labels {
		if strings.TrimSpace(label) == "" {
			t.Fatal("empty expected accessibility label")
		}
		r, ok := a.tt.Find(label)
		if !ok {
			t.Fatalf("missing accessible element %q (texts=%v)", label, a.tt.Texts())
		}
		if r.W <= 0 || r.H <= 0 {
			t.Fatalf("zero-sized accessible element %q: %+v", label, r)
		}
	}
}

func assertTabFocusAllNamed(t *testing.T, tt *ui.Tester, steps int, candidates []string) int {
	t.Helper()
	named := 0
	for step := range steps {
		tt.Key(0, ui.KeyTab)
		tt.Frame()
		matched := false
		for _, lbl := range candidates {
			if lbl != "" && tt.Focused(lbl) {
				matched = true
				named++
				break
			}
		}
		if !matched {
			t.Fatalf("Tab step %d landed on an unnamed or unexpected focusable element (candidates=%v)", step+1, candidates)
		}
	}
	return named
}

// TestAccessibilityAllSurfacesAndLocales exercises every screen, sidebar panel,
// banner, toolbar, popover, modal, and editor surface in both English and
// Simplified Chinese, asserting localized accessible labels, focus reachability,
// and ActiveDescendant keyboard navigation (09 D13, FE-1.8).
func TestAccessibilityAllSurfacesAndLocales(t *testing.T) {
	for _, lang := range []string{"en", "zh-Hans"} {
		t.Run(lang, func(t *testing.T) {
			a := newApp(t)
			a.followLanguage()
			a.lang = lang
			a.tt.SetLocale(lang)
			a.tt.Frame()

			// 1. Welcome screen, titlebar controls, and Notice dismiss/action buttons.
			a.s.Notify(contract.Notice{
				Level:   contract.NoticeInfo,
				Message: "Sample notice",
				Action:  locales.ShellUndo(),
				Run:     func() {},
			})
			a.animate()

			welcomeControls := []string{
				locales.ShellFileMenu(),
				locales.ShellToggleSidebar(),
				locales.ShellNew(),
				locales.ShellOmnibox(),
				locales.ShellToggleTheme(),
				locales.SettingsOpen(),
				locales.ShellMinimize(),
				locales.ShellMaximize(),
				locales.ShellClose(),
				locales.ShellOpen(),
				locales.ShellUndo(),
				locales.KitDismiss(),
			}
			a.mustFind(t, welcomeControls...)
			assertTabFocusAllNamed(t, a.tt, 10, welcomeControls)
			a.click(locales.KitDismiss())

			// 2. Workspace folder + active document in Live mode.
			root := a.folder("guide.md", "notes.md")
			if err := os.WriteFile(filepath.Join(root, "guide.md"), []byte("# Intro\n\nHello **world** and [Aurink](https://example.com).\n\n| A | B |\n| --- | --- |\n| 1 | 2 |\n\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, "notes.md"), []byte("# Second\n\nSearchable keyword alpha.\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			a.s.Open(filepath.Join(root, "guide.md"))
			a.settle()
			a.animate()

			docLabel := locales.LiveeditEditorOf("guide.md")
			a.mustFind(t,
				docLabel,
				"guide.md",
				locales.ShellViewLive(),
				locales.ShellPanelFiles(),
				locales.ShellPanelOutline(),
				locales.ShellPanelSearch(),
			)
			if !a.tt.Focused(docLabel) {
				t.Fatalf("expected live editor %q to hold focus", docLabel)
			}

			// 3. Sidebar Outline & Search panels.
			a.click(locales.ShellPanelOutline())
			a.animate()
			a.mustFind(t, "Intro")

			a.click(locales.ShellPanelSearch())
			a.animate()
			a.s.SetSearch("alpha")
			a.until("search results arrive", func() bool { return len(a.s.SearchResults()) > 0 })
			a.mustFind(t,
				locales.ShellSearchPlaceholder(),
				locales.ShellSearchCase(),
				locales.ShellSearchWord(),
				locales.ShellSearchRegex(),
				"notes.md",
			)

			// 4. Find & Replace bar.
			a.run(contract.CmdEditReplace)
			a.animate()
			a.mustFind(t,
				locales.KitFind(),
				locales.KitToggleReplace(),
				locales.KitMatchCase(),
				locales.KitWholeWord(),
				locales.KitRegex(),
				locales.KitPrevious(),
				locales.KitNext(),
				locales.KitCloseFind(),
				locales.KitReplaceWith(),
				locales.KitReplace(),
				locales.KitReplaceAll(),
			)
			a.click(locales.KitCloseFind())
			a.animate()

			// 5. Format Bar (selection) + inline Link address field.
			src := a.text()
			helloOff := strings.Index(src, "Hello")
			a.s.ActiveLive().Focus()
			a.s.ActiveSession().Model().SetSelection(contract.Selection{
				Ranges: []contract.SelRange{{Anchor: helloOff, Head: helloOff + 5}},
			})
			a.tt.Frame()
			a.animate()
			a.mustFind(t,
				locales.LiveeditFormatBar(),
				locales.LiveeditTransform(),
				locales.LiveeditBold(),
				locales.LiveeditItalic(),
				locales.LiveeditUnderline(),
				locales.LiveeditStrike(),
				locales.LiveeditInlineCode(),
				locales.LiveeditLink(),
				locales.LiveeditHighlight(),
			)
			a.click(locales.LiveeditLink())
			a.animate()
			a.mustFind(t, locales.LiveeditLinkAddress())
			a.key(0, ui.KeyEscape)
			a.animate()

			// 6. Link Bar (caret inside [Aurink](https://example.com)).
			linkOff := strings.Index(src, "Aurink") + 2
			a.caret(linkOff)
			a.animate()
			a.mustFind(t,
				locales.LiveeditLink(),
				locales.LiveeditEditLink(),
				locales.LiveeditOpenLink(),
			)

			// 7. Table Bar + Size Grid (caret inside table cell).
			cellOff := strings.Index(src, "| 1 |") + 2
			a.caret(cellOff)
			a.animate()
			a.mustFind(t,
				locales.LiveeditTableBar(),
				locales.LiveeditTableSize(),
				locales.LiveeditAlignLeft(),
				locales.LiveeditAlignCenter(),
				locales.LiveeditAlignRight(),
				locales.LiveeditTableMore(),
				locales.LiveeditDeleteTable(),
			)
			a.click(locales.LiveeditTableSize())
			a.animate()
			a.mustFind(t, locales.LiveeditSlashSizeN("2", "2"))
			a.click(locales.LiveeditTableSize())
			a.animate()

			// 8. Slash Menu (with ActiveDescendant keeping editor focused).
			a.caret(len(src))
			a.typed("/")
			a.animate()
			a.mustFind(t,
				locales.LiveeditSlashMenu(),
				locales.LiveeditHeadingN("1"),
				locales.LiveeditSlashTable(),
			)
			if !a.tt.Focused(docLabel) {
				t.Fatal("slash menu stole keyboard focus from live editor")
			}
			a.key(0, ui.KeyDown)
			if !a.tt.Focused(docLabel) {
				t.Fatal("arrow navigation in slash menu lost editor focus")
			}
			a.key(0, ui.KeyEscape)
			a.animate()

			// 9. Source mode editor surface.
			a.run(contract.CmdViewSource)
			a.animate()
			a.mustFind(t, locales.SourceEditor())
			if !a.tt.Focused(locales.SourceEditor()) {
				t.Fatalf("expected source editor %q to hold focus", locales.SourceEditor())
			}
			a.run(contract.CmdViewLive)
			a.animate()

			// 10. Command Palette (Files, Commands, Headings) with ComboBox focus + ActiveDescendant.
			a.run(contract.CmdNavPalette)
			a.animate()
			a.mustFind(t, locales.ShellPalettePlaceholder())
			if !a.tt.Focused(locales.ShellPalettePlaceholder()) {
				t.Fatal("command palette query input is not focused")
			}
			a.key(0, ui.KeyDown)
			if !a.tt.Focused(locales.ShellPalettePlaceholder()) {
				t.Fatal("command palette arrow key moved focus off query combobox")
			}
			a.key(0, ui.KeyEscape)
			a.animate()

			// 11. Settings modal: all 5 categories + Tab traversal inside modal.
			a.openSettings()
			a.mustFind(t,
				locales.SettingsTitle(),
				locales.SettingsClose(),
				locales.SettingsAppearance(),
				locales.SettingsEditor(),
				locales.SettingsFiles(),
				locales.SettingsExtensions(),
				locales.SettingsShortcuts(),
				locales.SettingsThemeLight(),
				locales.SettingsThemeDark(),
				locales.SettingsThemeSystem(),
				locales.SettingsHueTeal(),
				locales.SettingsHueIris(),
				locales.SettingsHueEmber(),
				locales.SettingsHueMoss(),
				locales.SettingsFontSize(),
				locales.SettingsLineHeight(),
				locales.SettingsContentWidth(),
			)
			settingsFocusCandidates := []string{
				locales.SettingsClose(),
				locales.SettingsAppearance(),
				locales.SettingsEditor(),
				locales.SettingsFiles(),
				locales.SettingsExtensions(),
				locales.SettingsShortcuts(),
				locales.SettingsThemeLight(),
				locales.SettingsThemeDark(),
				locales.SettingsThemeSystem(),
				locales.SettingsHueTeal(),
				locales.SettingsHueIris(),
				locales.SettingsHueEmber(),
				locales.SettingsHueMoss(),
				locales.SettingsSans(),
				locales.SettingsSerif(),
				locales.SettingsFontSize(),
				locales.SettingsLineHeight(),
				locales.SettingsContentWidth(),
				locales.SettingsSystem(),
				locales.SettingsOn(),
				locales.SettingsOff(),
				locales.SettingsReduceTransparency(),
				"简体中文",
				"English",
			}
			assertTabFocusAllNamed(t, a.tt, 14, settingsFocusCandidates)

			a.category(1) // Editor
			a.mustFind(t, locales.SettingsReveal(), locales.SettingsAutoPair(), locales.SettingsSmoothCaret())
			a.category(2) // Files
			a.mustFind(t, locales.SettingsAutosave(), locales.SettingsRestoreSession(), locales.SettingsCopyImages())
			a.category(3) // Extensions
			a.mustFind(t, locales.SettingsExtFrontMatter(), locales.SettingsExtMath(), locales.SettingsExtToc())
			a.category(4) // Shortcuts
			a.mustFind(t, locales.SettingsShortcutsHint())
			a.click(locales.SettingsClose())
			a.animate()

			// 12. About dialog.
			a.run(contract.CmdAppAbout)
			a.animate()
			a.mustFind(t, locales.SettingsAbout())
			a.key(0, ui.KeyEscape)
			a.animate()
		})
	}
}

// TestAccessibilityViewsStaticAudit parses internal/views/... and enforces that
// every IconButton has a non-empty localized label (never "" or raw "×") and
// every SwitchBase, SliderBase, TextInputBase, SearchField, and DialogBase sets
// an accessible Label.
func TestAccessibilityViewsStaticAudit(t *testing.T) {
	root := filepath.Join("..", "..", "internal", "views")
	fset := token.NewFileSet()
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		srcHasLabel := map[int]bool{}
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if sel.Sel.Name == "Label" && len(call.Args) == 1 {
				line := fset.Position(call.Pos()).Line
				for d := -6; d <= 6; d++ {
					srcHasLabel[line+d] = true
				}
				if lit, ok := call.Args[0].(*ast.BasicLit); ok && lit.Kind == token.STRING {
					val, _ := strconv.Unquote(lit.Value)
					if strings.TrimSpace(val) == "" || val == "×" {
						t.Errorf("%s:%d: non-descriptive Label(%q)", path, line, val)
					}
				}
			}
			// Check IconButton(..., label, size, iconSize)
			if sel.Sel.Name == "IconButton" && len(call.Args) >= 4 {
				line := fset.Position(call.Pos()).Line
				if lit, ok := call.Args[3].(*ast.BasicLit); ok && lit.Kind == token.STRING {
					val, _ := strconv.Unquote(lit.Value)
					if strings.TrimSpace(val) == "" || val == "×" {
						t.Errorf("%s:%d: IconButton has non-localized/empty label %q", path, line, val)
					}
				}
			}
			return true
		})

		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			pkgIdent, ok := sel.X.(*ast.Ident)
			if !ok || pkgIdent.Name != "ui" {
				return true
			}
			switch sel.Sel.Name {
			case "SwitchBase", "SliderBase", "TextInputBase", "SearchField", "DialogBase":
				line := fset.Position(call.Pos()).Line
				if !srcHasLabel[line] {
					t.Errorf("%s:%d: ui.%s is missing a nearby .Label(...) call", path, line, sel.Sel.Name)
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
