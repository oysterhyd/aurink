package qa

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"aurink/internal/contract"
	"aurink/internal/views/editorapi"
	"aurink/locales"
	"aurink/tests/corpus"
	"github.com/egoist/mygo/ui"
	"golang.org/x/text/encoding/simplifiedchinese"
)

// TestLargeFileMode verifies that documents >= LargeFileBytes open directly in
// Source mode without running the renderer, lock out Live/Reading views, show
// the large-file banner and outline notice, and edit/save/undo losslessly
// (09 D1-D3, FE-1.8).
func TestLargeFileMode(t *testing.T) {
	a := newApp(t)
	body := corpus.Generate((5<<20)+(256<<10), 11)
	raw := "# Large Document\r\n\r\n$$\r\nE = mc^2\r\n$$\r\n\r\n```mermaid\r\ngraph TD; A-->B;\r\n```\r\n\r\n" +
		strings.ReplaceAll(body, "\n", "\r\n")
	p := a.open("huge.md", raw)

	st := a.s.ActiveSession().State()
	if !st.LargeFile || !a.s.ActiveSession().Model().Meta().Large {
		t.Fatalf("LargeFile = %v, Meta.Large = %v, want both true", st.LargeFile, a.s.ActiveSession().Model().Meta().Large)
	}
	if a.s.ActiveMode() != editorapi.ModeSource {
		t.Fatalf("ActiveMode = %v, want ModeSource", a.s.ActiveMode())
	}
	if !a.shows(locales.ShellBannerLarge()) {
		t.Fatal("missing large file banner")
	}
	if n := a.render.count(); n != 0 {
		t.Fatalf("renderer ran %d times on a large file, want 0", n)
	}

	// Live, Source-toggle, Reading (F7) and Go to Heading must be disabled.
	for _, id := range []contract.CommandID{
		contract.CmdViewLive,
		contract.CmdViewSource,
		contract.CmdViewRead,
		contract.CmdNavGoToHeading,
	} {
		cmd, ok := a.reg.Lookup(id)
		if !ok {
			t.Fatalf("missing command %s", id)
		}
		if cmd.Enabled != nil && cmd.Enabled() {
			t.Fatalf("command %s is enabled on a large file", id)
		}
	}
	a.tt.Key(0, ui.KeyF7)
	a.tt.Key(ui.Cmd, ui.KeySlash)
	a.settle()
	if a.s.ActiveMode() != editorapi.ModeSource || a.s.Reading() {
		t.Fatalf("shortcuts switched mode: mode=%v reading=%v", a.s.ActiveMode(), a.s.Reading())
	}

	// Outline sidebar shows the large-file notice.
	a.run(contract.CmdViewOutline)
	if !a.shows(locales.ShellLargeNoOutline()) {
		t.Fatal("outline panel did not show large-file notice")
	}

	// Edit, save, undo, redo and verify exact CRLF byte preservation.
	src := a.s.ActiveSource()
	if src == nil {
		t.Fatal("ActiveSource is nil")
	}
	src.State().Select(0, 0)
	a.tt.Frame()
	src.State().Insert("Preface: ")
	a.tt.Frame()
	a.run(contract.CmdFileSave)
	wantEdited := "Preface: " + raw
	if got := a.disk(p); got != wantEdited {
		t.Fatalf("saved bytes mismatch: len %d vs %d", len(got), len(wantEdited))
	}

	a.tt.Key(ui.Cmd, ui.KeyZ)
	a.tt.Frame()
	if a.text() != raw {
		t.Fatal("Undo did not restore original bytes")
	}
	a.tt.Key(ui.Cmd, ui.KeyY)
	a.tt.Frame()
	if a.text() != wantEdited {
		t.Fatal("Redo did not reapply edit")
	}
	if err := a.s.Check(); err != nil {
		t.Fatal(err)
	}
}

// TestLegacyGB18030BannerAndConvert verifies that GB18030/GBK files open
// read-only with the encoding banner, block edits until explicitly converted to
// UTF-8 (or saved as a copy), and preserve CRLF line endings upon save
// (09 D6-D7, BE-2.11, FE-1.8).
func TestLegacyGB18030BannerAndConvert(t *testing.T) {
	a := newApp(t)
	md := "# 旧编码标题\r\n\r\n中文段落与 **粗体** 内容。\r\n"
	gbBytes, err := simplifiedchinese.GB18030.NewEncoder().Bytes([]byte(md))
	if err != nil {
		t.Fatal(err)
	}
	if utf8.Valid(gbBytes) {
		t.Fatal("expected non-UTF-8 GB18030 bytes")
	}
	p := filepath.Join(a.dir, "legacy.md")
	if err := os.WriteFile(p, gbBytes, 0o644); err != nil {
		t.Fatal(err)
	}

	a.s.Open(p)
	a.settle()

	sess := a.s.ActiveSession()
	if !sess.State().ReadOnly || sess.Model().Meta().Encoding != "gb18030" {
		t.Fatalf("ReadOnly=%v Encoding=%q, want true/gb18030", sess.State().ReadOnly, sess.Model().Meta().Encoding)
	}
	if !a.shows(locales.ShellBannerEncoding()) {
		t.Fatal("missing legacy encoding banner")
	}
	a.want(a.text(), md)

	// Attempting to type in Source mode while read-only leaves document & disk untouched.
	a.run(contract.CmdViewSource)
	if !a.s.ActiveSource().State().ReadOnly {
		t.Fatal("source editor should be ReadOnly before conversion")
	}
	a.typed("NOPE")
	a.want(a.text(), md)
	onDisk, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(onDisk, gbBytes) {
		t.Fatal("legacy GB18030 bytes on disk were modified while read-only")
	}

	// Click "Convert to UTF-8 and Edit" in the banner.
	a.click(locales.ShellConvertUtf8())
	if sess.State().ReadOnly || sess.Model().Meta().Encoding != "utf-8" {
		t.Fatalf("after convert: ReadOnly=%v Encoding=%q", sess.State().ReadOnly, sess.Model().Meta().Encoding)
	}
	if a.s.ActiveSource().State().ReadOnly {
		t.Fatal("source editor remained ReadOnly after conversion")
	}
	if a.shows(locales.ShellBannerEncoding()) {
		t.Fatal("encoding banner stayed after conversion")
	}

	// Edit in Source view and save: disk becomes valid UTF-8 with CRLF preserved.
	a.s.ActiveSource().State().Select(0, 0)
	a.tt.Frame()
	a.s.ActiveSource().State().Insert("已转换 ")
	a.tt.Frame()
	a.run(contract.CmdFileSave)

	saved, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if !utf8.Valid(saved) {
		t.Fatal("saved file is not valid UTF-8")
	}
	a.want(string(saved), "已转换 "+md)
}

func TestLegacyGB18030SaveAsCopy(t *testing.T) {
	a := newApp(t)
	md := "# 副本测试\n\n保持原盘不变。\n"
	gbBytes, err := simplifiedchinese.GB18030.NewEncoder().Bytes([]byte(md))
	if err != nil {
		t.Fatal(err)
	}
	orig := filepath.Join(a.dir, "orig.md")
	if err := os.WriteFile(orig, gbBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	a.s.Open(orig)
	a.settle()
	if !a.shows(locales.ShellBannerEncoding()) {
		t.Fatal("missing legacy encoding banner")
	}

	copyPath := filepath.Join(a.dir, "utf8-copy.md")
	a.p.save = copyPath
	a.click(locales.ShellSaveAs())

	// Original file keeps its GB18030 bytes; copy is UTF-8 and unlocked.
	origAfter, err := os.ReadFile(orig)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(origAfter, gbBytes) {
		t.Fatal("SaveAs mutated original GB18030 file")
	}
	a.want(a.disk(copyPath), md)
	if a.s.ActiveSession().State().ReadOnly || a.s.ActiveSession().Model().Meta().Encoding != "utf-8" {
		t.Fatal("SaveAs did not unlock session as UTF-8")
	}
}

// TestCustomThemesLoadSwitchAndCorruptFallback verifies loading custom JSON
// themes from PathUserData/themes, warning once on corrupt files without
// crashing, switching & persisting custom themes across restarts, and opening
// the themes folder (09 D10, FE-2.8).
func TestCustomThemesLoadSwitchAndCorruptFallback(t *testing.T) {
	a := newApp(t)
	themesDir := filepath.Join(a.data, "themes")
	if err := os.MkdirAll(themesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(themesDir, "solarized.json"), []byte(`{
		"name": "Solarized Paper",
		"dark": false,
		"accentHue": 175,
		"colors": {"window": "#EEE8D5", "sheet": "#FDF6E3", "text": "#586E75", "accent": "#2AA198"}
	}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(themesDir, "nord.json"), []byte(`{
		"name": "Nord Night",
		"dark": true,
		"accentHue": 210,
		"colors": {"window": "#2E3440", "sheet": "#3B4252", "text": "#ECEFF4", "accent": "#88C0D0"}
	}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(themesDir, "broken.json"), []byte(`{corrupt`), 0o644); err != nil {
		t.Fatal(err)
	}

	a.openSettings()
	if !a.shows(locales.SettingsThemeBad()) {
		t.Fatal("missing warning notice for corrupt custom theme JSON")
	}
	if len(a.s.CustomThemes()) != 2 {
		t.Fatalf("CustomThemes len = %d, want 2", len(a.s.CustomThemes()))
	}
	a.click(locales.SettingsOpenThemesFolder())
	if len(a.p.openedPaths) != 1 || a.p.openedPaths[0] != themesDir {
		t.Fatalf("openedPaths = %v, want [%s]", a.p.openedPaths, themesDir)
	}

	a.click("Nord Night")
	a.animate()
	if a.settings().Appearance.Theme != "custom:nord" || !a.s.Dark() {
		t.Fatalf("after Nord Night: theme=%q dark=%v", a.settings().Appearance.Theme, a.s.Dark())
	}

	a.click("Solarized Paper")
	a.animate()
	if a.settings().Appearance.Theme != "custom:solarized" || a.s.Dark() {
		t.Fatalf("after Solarized Paper: theme=%q dark=%v", a.settings().Appearance.Theme, a.s.Dark())
	}

	// Select Nord Night, reboot, and confirm persistence across restarts.
	a.click("Nord Night")
	a.animate()
	a.click(locales.SettingsClose())
	a.boot()
	if a.settings().Appearance.Theme != "custom:nord" || !a.s.Dark() {
		t.Fatalf("after reboot: theme=%q dark=%v, want custom:nord / true", a.settings().Appearance.Theme, a.s.Dark())
	}
}

// TestKeybindingEditorRecordConflictPersistAndReset verifies shortcut search,
// bare-key rejection, Escape cancel, conflict prompt & override, live execution,
// persistence across restarts, and Reset All (09 D11, FE-2.8).
func TestKeybindingEditorRecordConflictPersistAndReset(t *testing.T) {
	a := newApp(t)
	a.open("doc.md", "# Keymap\n\nBody\n")
	a.openSettings()
	a.category(4) // Shortcuts

	// 1. Click New's shortcut pill to start recording; bare key 'K' must be rejected.
	newPill := locales.ShellNew() + ": " + "Ctrl N"
	if ui.Cmd != ui.Ctrl {
		newPill = locales.ShellNew() + ": " + "⌘ N"
	}
	a.click(newPill)
	a.animate()
	if !a.shows(locales.SettingsShortcutsRecord()) {
		t.Fatal("clicking shortcut row did not start recording")
	}
	a.key(0, ui.KeyK)
	if !a.shows(locales.SettingsShortcutsNeedMod()) {
		t.Fatal("bare key was not rejected with modifier hint")
	}

	// 2. Escape cancels recording without closing Settings.
	a.key(0, ui.KeyEscape)
	if !a.s.SettingsOpen() || a.shows(locales.SettingsShortcutsRecord()) {
		t.Fatalf("Escape while recording: settingsOpen=%v stillRecording=%v", a.s.SettingsOpen(), a.shows(locales.SettingsShortcutsRecord()))
	}

	// 3. Record Ctrl+O onto New: conflicts with Open!
	a.click(newPill)
	a.animate()
	a.key(ui.Cmd, ui.KeyO)
	a.animate()
	wantConflict := locales.SettingsShortcutsConflict(locales.ShellOpen())
	if !a.shows(wantConflict) || !a.shows(locales.SettingsShortcutsOverride()) {
		t.Fatalf("missing conflict prompt %q (texts=%v)", wantConflict, a.tt.Texts())
	}

	// 4. Confirm Override: Ctrl+O resolves to CmdFileNew, CmdFileOpen is unbound, and Ctrl+N no longer resolves to CmdFileNew.
	a.click(locales.SettingsShortcutsOverride())
	a.animate()
	ctrlO := contract.Chord{Mods: contract.ModPrimary, Key: "O"}
	ctrlN := contract.Chord{Mods: contract.ModPrimary, Key: "N"}
	if got, ok := a.reg.Resolve(ctrlO); !ok || got != contract.CmdFileNew {
		t.Fatalf("Resolve(Ctrl+O) = (%q, %v), want CmdFileNew", got, ok)
	}
	if _, ok := a.reg.Resolve(ctrlN); ok {
		t.Fatal("old default Ctrl+N still resolved after rebinding")
	}
	if len(a.reg.KeysFor(contract.CmdFileOpen)) != 0 {
		t.Fatalf("overridden CmdFileOpen still has keys: %v", a.reg.KeysFor(contract.CmdFileOpen))
	}

	// Close Settings and press Ctrl+O in the window: creates a 2nd tab!
	a.click(locales.SettingsClose())
	a.animate()
	tabsBefore := len(a.s.Sessions())
	a.tt.Key(ui.Cmd, ui.KeyO)
	a.settle()
	if len(a.s.Sessions()) != tabsBefore+1 {
		t.Fatalf("pressing rebound Ctrl+O resulted in %d tabs, want %d", len(a.s.Sessions()), tabsBefore+1)
	}

	// 5. Reboot: custom binding persists in settings.json and loads into registry.
	a.boot()
	if got, ok := a.reg.Resolve(ctrlO); !ok || got != contract.CmdFileNew {
		t.Fatalf("after reboot: Resolve(Ctrl+O) = (%q, %v), want CmdFileNew", got, ok)
	}

	// 6. Open Settings -> Shortcuts -> Reset All restores DefaultBindings().
	a.openSettings()
	a.category(4)
	a.click(locales.SettingsShortcutsResetAll())
	a.animate()
	if got, ok := a.reg.Resolve(ctrlN); !ok || got != contract.CmdFileNew {
		t.Fatalf("after Reset All: Resolve(Ctrl+N) = (%q, %v), want CmdFileNew", got, ok)
	}
	if got, ok := a.reg.Resolve(ctrlO); !ok || got != contract.CmdFileOpen {
		t.Fatalf("after Reset All: Resolve(Ctrl+O) = (%q, %v), want CmdFileOpen", got, ok)
	}
}

// TestTabBarOverflowEdgeFades verifies that horizontal tab overflow draws
// smooth pass-through edge fades on the overflowing side(s) without blocking
// tab clicks (09 §3.1, §6 Batch 5, FE-2.8).
func TestTabBarOverflowEdgeFades(t *testing.T) {
	a := newApp(t)
	a.open("tab-01.md", "# 1\n")
	a.open("tab-02.md", "# 2\n")
	a.animate()
	if left, right := a.s.TabFades(); left || right {
		t.Fatalf("2 tabs showed overflow fades: left=%v right=%v", left, right)
	}

	for i := 3; i <= 14; i++ {
		a.open(fmt.Sprintf("tab-%02d.md", i), fmt.Sprintf("# %d\n", i))
	}
	a.animate()
	// Last tab is scrolled into view on open -> left side overflows!
	if left, _ := a.s.TabFades(); !left {
		t.Fatal("14 tabs scrolled to end did not show left overflow fade")
	}

	// Jump to tab 1 -> right side overflows!
	a.run(contract.CmdNavGoToTab1)
	a.animate()
	if left, right := a.s.TabFades(); left || !right {
		t.Fatalf("14 tabs scrolled to start: left=%v right=%v, want false/true", left, right)
	}

	// Jump to middle tab 6 -> both sides overflow, and clicking a visible tab still works!
	a.run(contract.CmdNavGoToTab6)
	a.animate()
	if left, right := a.s.TabFades(); !left || !right {
		t.Fatalf("14 tabs at middle tab 6: left=%v right=%v, want true/true", left, right)
	}
	a.click("tab-05.md")
	if a.s.ActiveSession().State().Name != "tab-05.md" {
		t.Fatalf("active tab = %q, want tab-05.md", a.s.ActiveSession().State().Name)
	}
}

// TestUpdateCheckMenuAboutPaletteAndToggle verifies that Check for Updates is
// wired through the Command Registry/Palette, File menu, and About dialog, and
// that the automatic update check toggle updates platform state (09 D12, INT-4.1).
func TestUpdateCheckMenuAboutPaletteAndToggle(t *testing.T) {
	a := newApp(t)

	// 1. Command Registry / Palette command triggers CheckUpdates.
	a.run(contract.CmdAppCheckUpdates)
	if a.p.updateChecks != 1 {
		t.Fatalf("updateChecks after CmdAppCheckUpdates = %d, want 1", a.p.updateChecks)
	}

	// 2. File menu under brand mark contains Check for Updates… and invokes it.
	a.click(locales.ShellFileMenu())
	a.animate()
	a.click(locales.ShellCheckUpdates())
	if a.p.updateChecks != 2 {
		t.Fatalf("updateChecks after File menu = %d, want 2", a.p.updateChecks)
	}

	// 3. About dialog shows Check for Updates… button and automatic updates toggle.
	a.run(contract.CmdAppAbout)
	a.animate()
	if !a.shows(locales.ShellCheckUpdates()) || !a.shows(locales.SettingsAutoUpdate()) {
		t.Fatalf("About dialog missing update controls: %v", a.tt.Texts())
	}
	a.click(locales.ShellCheckUpdates())
	a.animate()
	if a.p.updateChecks != 3 {
		t.Fatalf("updateChecks after About button = %d, want 3", a.p.updateChecks)
	}

	// 4. Toggle automatic updates in Settings -> Files.
	if !a.p.autoUpdate {
		t.Fatal("expected initial autoUpdate = true")
	}
	a.openSettings()
	a.category(2) // Files
	x, y := a.control(locales.SettingsAutoUpdate(), "", 38)
	a.tt.ClickAt(x, y)
	a.settle()
	if a.p.autoUpdate {
		t.Fatal("clicking autoUpdate switch did not turn automatic checks off")
	}
}
