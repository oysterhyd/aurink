package qa

import (
	"strings"
	"testing"
	"time"

	"aurink/internal/contract"
	"aurink/locales"
	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"
)

// The settings dialog (FE-2.7, 02 §5.15): every change goes to the store and
// shows at once.

func (a *app) openSettings() {
	a.t.Helper()
	a.click(locales.SettingsOpen())
	a.animate()
	if !a.s.SettingsOpen() {
		a.t.Fatal("the settings button did not open settings")
	}
}

// followLanguage makes the catalogs say their messages in the language the
// shell sets, as mygo.App does in the app.
func (a *app) followLanguage() {
	i18n.SetLocaleFunc(func() string { return a.lang })
	a.t.Cleanup(func() { i18n.SetLocaleFunc(nil) })
}

func (a *app) settings() contract.Settings { return a.store.Get() }

// category clicks a category in the dialog's sidebar, by its place: its
// name can also name other things in the window ("Editor").
func (a *app) category(i int) {
	a.tt.ClickAt((1280-820)/2+40, (800-580)/2+18+16+float32(i)*34)
	a.settle()
}

// scrollSettings scrolls the dialog's page down.
func (a *app) scrollSettings() {
	a.tt.Scroll(800, 500, 0, 600)
	a.animate()
}

// control returns the middle of the control at the right of a setting's row:
// the dialog is 820 wide in the middle of the window, with a 26 margin. The
// control is in the middle of the row's text: the label, and the hint under
// it when hint is not "".
func (a *app) control(label, hint string, width float32) (x, y float32) {
	a.t.Helper()
	r, ok := a.tt.Find(label)
	if !ok {
		a.t.Fatalf("no setting %q", label)
	}
	bottom := r.Y + r.H
	if hint != "" {
		h, ok := a.tt.Find(hint)
		if !ok {
			a.t.Fatalf("no hint %q", hint)
		}
		bottom = h.Y + h.H
	}
	return (1280+820)/2 - 26 - width/2, (r.Y + bottom) / 2
}

func TestSettingsOpensAndCloses(t *testing.T) {
	a := newApp(t)
	a.openSettings()
	for _, s := range []string{locales.SettingsThemeLight(), locales.SettingsThemeDark(), locales.SettingsThemeSystem(), locales.SettingsAccent()} {
		if !a.shows(s) {
			t.Fatalf("the appearance page lacks %q", s)
		}
	}
	// Reduce Transparency and its hint must be unclipped inside the 580px dialog (Y: 110..690) on initial open.
	for _, s := range []string{locales.SettingsReduceTransparency(), locales.SettingsReduceTransparencyHint()} {
		r, ok := a.tt.Find(s)
		if !ok || r.Y+r.H > 682 {
			t.Fatalf("Reduce Transparency text %q is clipped at the bottom of the dialog: rect=%+v", s, r)
		}
	}
	a.key(0, ui.KeyEscape)
	if a.s.SettingsOpen() {
		t.Fatal("Escape did not close settings")
	}
	a.run(contract.CmdAppSettings)
	a.click(locales.SettingsClose())
	if a.s.SettingsOpen() {
		t.Fatal("the close button did not close settings")
	}
}

func TestSettingsAppearanceAppliesAtOnce(t *testing.T) {
	a := newApp(t)
	a.open("look.md", "# Look\n\ntext\n")
	a.openSettings()
	a.click(locales.SettingsThemeDark())
	if a.settings().Appearance.Theme != "dark" || !a.s.Dark() {
		t.Fatalf("theme %q, dark %v", a.settings().Appearance.Theme, a.s.Dark())
	}
	a.click(locales.SettingsHueIris())
	if a.settings().Appearance.AccentHue != 278 {
		t.Fatalf("hue %d", a.settings().Appearance.AccentHue)
	}
	a.click(locales.SettingsSerif())
	if !a.s.ActiveLive().Typography().Serif {
		t.Fatal("the serif body did not apply")
	}
	// Dragging the size slider to its right end makes it 24.
	x, y := a.control(locales.SettingsFontSize(), "13–24", 40+8+150)
	a.tt.Press(x, y)
	a.tt.Move(x+200, y)
	a.tt.Release(x+200, y)
	a.settle()
	if got := a.settings().Appearance.FontSize; got != 24 || a.s.ActiveLive().Typography().Body != 24 {
		t.Fatalf("size %v, body %v", got, a.s.ActiveLive().Typography().Body)
	}
	// Keys step it by one.
	a.key(0, ui.KeyLeft)
	if got := a.settings().Appearance.FontSize; got != 23 {
		t.Fatalf("Left made the size %v", got)
	}
	a.scrollSettings()
	x, y = a.control(locales.SettingsReduceTransparency(), locales.SettingsReduceTransparencyHint(), 38)
	a.tt.ClickAt(x, y)
	a.settle()
	if !a.settings().Appearance.ReduceTransparency {
		t.Fatal("the switch did not turn reduce transparency on")
	}
	a.tt.ClickAt(x, y)
	a.settle()
	if a.settings().Appearance.ReduceTransparency {
		t.Fatal("the switch did not turn reduce transparency off")
	}
}

func TestSettingsEditorAppliesAtOnce(t *testing.T) {
	a := newApp(t)
	a.open("marks.md", "# Marks\n\na **b**\nc\n")
	a.openSettings()
	a.category(1)
	a.click(locales.SettingsRevealAlways())
	if m, _ := a.s.ActiveLive().Reveal(); a.settings().Editor.Reveal != contract.RevealAlways || m != contract.RevealAlways {
		t.Fatalf("reveal %v, editor %v", a.settings().Editor.Reveal, m)
	}
	a.click(locales.SettingsSoftBreakSpace())
	if _, sb := a.s.ActiveLive().Reveal(); sb != contract.SoftBreakSpace {
		t.Fatal("soft breaks did not turn to spaces")
	}
	a.click("__")
	if a.settings().Editor.StrongDelim != "__" {
		t.Fatalf("bold marker %q", a.settings().Editor.StrongDelim)
	}
	a.click("4")
	if a.settings().Editor.TabSize != 4 {
		t.Fatalf("indent %d", a.settings().Editor.TabSize)
	}
}

func TestSettingsFilesAndExtensions(t *testing.T) {
	a := newApp(t)
	a.openSettings()
	a.category(2)
	x, y := a.control(locales.SettingsAutosave(), locales.SettingsAutosaveHint(), 38)
	a.tt.ClickAt(x, y)
	a.settle()
	if a.settings().Files.Autosave {
		t.Fatal("the switch did not turn autosave off")
	}
	a.click("./${filename}.assets")
	if a.settings().Files.ImageDir != "./${filename}.assets" {
		t.Fatalf("image folder %q", a.settings().Files.ImageDir)
	}
	a.category(3)
	if !a.shows(locales.SettingsExtHint()) {
		t.Fatal("the extensions page does not say when they apply")
	}
	before := a.settings().Editor.Exts
	x, y = a.control(locales.SettingsExtWiki(), "", 38)
	a.tt.ClickAt(x, y)
	a.settle()
	if got := a.settings().Editor.Exts; got != before^contract.ExtWikiLink {
		t.Fatalf("extensions %b, were %b", got, before)
	}
}

func TestSettingsListsTheShortcuts(t *testing.T) {
	a := newApp(t)
	a.openSettings()
	a.category(4)
	for _, s := range []string{locales.ShellReading(), "F7", locales.ShellZoomIn()} {
		if !a.shows(s) {
			t.Fatalf("the shortcuts page lacks %q", s)
		}
	}
}

// Changing the language changes the interface without a restart.
func TestSettingsLanguage(t *testing.T) {
	a := newApp(t)
	a.followLanguage()
	a.openSettings()
	a.scrollSettings()
	a.click("简体中文")
	if a.settings().Language != "zh-Hans" || !a.shows("外观") {
		t.Fatalf("language %q, texts %v", a.settings().Language, a.tt.Texts())
	}
	a.scrollSettings()
	a.click("English")
	if !a.shows("Appearance") {
		t.Fatal("the interface did not go back to English")
	}
}

// In the pseudo locale, longer by a third, every text of every page stays
// inside the dialog.
func TestSettingsFitInPseudoLocale(t *testing.T) {
	a := newApp(t)
	a.followLanguage()
	a.lang = "en-XA"
	a.tt.SetLocale("en-XA")
	if locales.SettingsAppearance() == "Appearance" {
		t.Fatal("the pseudo locale is not in effect")
	}
	a.openSettings()
	left, right := float32(1280-820)/2, float32(1280+820)/2
	for i, cat := range []func() string{locales.SettingsAppearance, locales.SettingsEditor, locales.SettingsFiles,
		locales.SettingsExtensions, locales.SettingsShortcuts} {
		a.category(i)
		a.animate()
		for _, s := range a.tt.Texts() {
			r, ok := a.tt.Find(s)
			if !ok || r.Y < 110 || r.Y > 690 {
				continue // outside the dialog, or scrolled away
			}
			if r.X < left || r.X+r.W > right {
				t.Fatalf("%s: %q spans %v–%v, past the dialog's %v–%v", cat(), s, r.X, r.X+r.W, left, right)
			}
		}
	}
}

func TestAboutShowsTheVersion(t *testing.T) {
	a := newApp(t)
	a.run(contract.CmdAppAbout)
	if !a.shows(locales.SettingsVersion("9.9.9")) || !a.shows("MyGo — MIT") {
		t.Fatalf("about: %v", a.tt.Texts())
	}
}

// File settings reach the services built from them: the tree's filter and
// autosave change while the app runs.
func TestSettingsFilesApplyAtOnce(t *testing.T) {
	a := newApp(t)
	a.folder("b.md", "notes.txt")
	if a.shows("notes.txt") {
		t.Fatal("the tree shows a text file before the setting")
	}
	a.openSettings()
	a.category(2)
	x, y := a.control(locales.SettingsShowAllFiles(), "", 38)
	a.tt.ClickAt(x, y)
	a.settle()
	a.key(0, ui.KeyEscape)
	a.until("the tree lists everything", func() bool { return a.tt.HasText("notes.txt") })

	if err := a.store.Update(func(v *contract.Settings) { v.Files.Autosave = false }); err != nil {
		t.Fatal(err)
	}
	p := a.open("auto.md", "text\n")
	a.typed("x")
	a.advance(10 * time.Second)
	if got := a.disk(p); got != "text\n" {
		t.Fatalf("autosave off still saved %q", got)
	}
	// The image folder: a pasted image goes to the one set now.
	if err := a.store.Update(func(v *contract.Settings) { v.Files.ImageDir = "./pics" }); err != nil {
		t.Fatal(err)
	}
	md, err := a.assets.ImportImage(p, []byte("\x89PNG\r\n\x1a\n"), "shot.png")
	if err != nil || !strings.HasPrefix(md, "pics/") {
		t.Fatalf("pasted to %q, %v", md, err)
	}
}
