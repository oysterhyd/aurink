// Package shell is the window around the documents: title bar, tabs, sidebar, sheet,
// status capsule, notices and the welcome page (06 §2.3).
package shell

import (
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode"

	"aurink/internal/contract"
	"aurink/internal/views/editorapi"
	"aurink/internal/views/effects"
	"aurink/internal/views/kit"
	"aurink/internal/views/liveedit"
	"aurink/internal/views/motion"
	"aurink/internal/views/palette"
	"aurink/internal/views/source"
	"aurink/internal/views/theme"
	"aurink/locales"
	"github.com/egoist/mygo/ui"
)

// Choice is the answer to closing a document with unsaved changes.
type Choice uint8

const (
	ChoiceCancel Choice = iota
	ChoiceSave
	ChoiceDiscard
)

// Platform is what the shell asks of the app: dialogs and the window. Dialogs call
// back on the main thread; a canceled one gives "".
type Platform interface {
	OpenFile(done func(path string))
	SavePath(name string, done func(path string))
	// ExportPath asks where to export a page of a kind ("html" or "pdf"); ""
	// when canceled.
	ExportPath(name, kind string, done func(path string))
	ConfirmClose(name string, done func(Choice))
	// OpenFolder asks for a folder; "" when canceled.
	OpenFolder(done func(path string))
	// ConfirmTrash asks before trashing what holds unsaved changes (D8).
	ConfirmTrash(name string, done func(ok bool))
	Minimize()
	ToggleMaximize()
	IsMaximized() bool
	Close()
}

// Options is what a shell is built from.
type Options struct {
	Sessions  contract.Sessions
	Settings  contract.SettingsStore
	Projector contract.Projector
	Editing   contract.Editing
	Commands  contract.CommandRegistry
	// Dispatcher brings images back to the main thread.
	Dispatcher contract.Dispatcher
	Platform   Platform
	// WindowButtons draws minimize, maximize and close; the system draws them on macOS.
	WindowButtons bool
	// Exts are the syntax extensions M1 renders.
	Exts contract.Extensions
	// ObjectLangs are the code languages shown as diagrams; Renderer renders
	// them and math (nil shows their source).
	ObjectLangs []string
	Renderer    contract.Renderer
	// Fuzzy ranks the palette's rows; nil matches by substring.
	Fuzzy contract.Fuzzy
	// Files lists the workspace's files for quick open; nil lists open and closed tabs only.
	Files func() []string
	// Workspace backs the file tree; nil shows the open documents instead.
	Workspace contract.Workspace
	// Finder finds in the active document (the find bar); nil leaves the live
	// view's find bar without matches.
	Finder contract.Finder
	// Search searches the workspace folder; nil leaves the search panel empty.
	Search contract.WorkspaceSearch
	// Clip, Converter and Assets serve the clipboard and dropped images; see
	// liveedit.Options.
	Clip      func(markdown string) string
	Converter contract.PasteConverter
	Clipboard contract.Clipboard
	Assets    contract.Assets
	// SessionStore keeps the tabs and folder for the next start; nil keeps
	// nothing (D9).
	SessionStore contract.SessionStore
	// Recent records opened files and folders for the welcome page and quick
	// open; nil records nothing.
	Recent contract.Recent
	// ExportHTML writes markdown as an HTML page at path off the main thread
	// and calls done on it; nil leaves file.exportHTML out.
	ExportHTML func(markdown, path string, opts contract.ExportOptions, done func(error))
	// ExportPDF writes markdown as a PDF at path off the main thread; it calls
	// opts.OnProgress and done on the main thread. nil leaves file.exportPDF out.
	ExportPDF func(markdown, path string, opts contract.ExportOptions, done func(error))
	// Print shows markdown as a page in a print preview and opens the system's
	// print dialog; done comes on the main thread. nil leaves file.print out.
	Print func(markdown string, opts contract.ExportOptions, done func(error))
	// SetLanguage shows the interface in a language ("" for the system's);
	// nil leaves the language setting stored only.
	SetLanguage func(tag string)
	// Version and Licenses fill the about dialog: the app's version, and a
	// line for each bundled library with its license.
	Version  string
	Licenses []string
}

const maxNotices = 4

type doc struct {
	s      contract.Session
	live   *liveedit.Editor
	source *source.Editor
	mode   editorapi.Mode
	offs   []func()
	// The word count, recounted at most this often while typing.
	find      lfind
	counted   contract.Version
	countedAt time.Time
	words     int
	// readFrom is the view the tab had before reading mode (D17).
	readFrom editorapi.Mode
}

// Shell is one window's interface.
type Shell struct {
	o Options
	// renderWarned is set once the notice that math and diagrams cannot
	// render has shown.
	renderWarned bool
	// opening is the document on its way, for the skeleton.
	opening opening
	// edgeShown is whether the scroll edge showed in the last frame.
	edgeShown bool
	// welcomeAt and emptyAt are when the welcome page and the empty document
	// showed, for their auroras; auroraShown is whether one showed.
	welcomeAt, emptyAt time.Time
	auroraShown        bool
	// still is reduced motion this frame, the app's setting or the system's.
	still   bool
	docs    []*doc
	active  *doc
	sidebar bool
	off     func()
	offSet  func()

	dark      bool
	themeSet  bool
	themeGen  int
	origin    ui.Point
	live      bool
	cpu       bool
	tokens    map[theme.Options]*theme.Tokens
	notices   []*notice
	services  ui.Services
	hasServ   bool
	focusNext bool
	// reading is reading mode, for the window (D17); zoom is the document's
	// zoom in steps of zoomStep.
	reading bool
	zoom    int
	set     settingsView

	// drag is the tab the pointer drags, and how far it is from its slot.
	drag struct {
		d *doc
		x float32
	}
	// widths are the tabs' widths in the last frame, for reordering.
	widths map[*doc]float32
	// closed are the paths of closed tabs, the latest last (file.reopenClosed).
	closed []string
	// reveal scrolls the active tab into view in the next frame.
	reveal bool
	pal    *palette.Palette
	// panel is the sidebar's panel: files, outline or search.
	panel   int
	files   files
	outline outline
	search  wsearch
	sess    session
}

const (
	panelFiles = iota
	panelOutline
	panelSearch
)

// maxClosed is how many closed tabs file.reopenClosed remembers.
const maxClosed = 20

// tabGap is the gap between tabs in the title bar.
const tabGap = 6

// New returns a shell over the sessions, with a tab for each already open.
func New(o Options) *Shell {
	s := &Shell{o: o, tokens: map[theme.Options]*theme.Tokens{}, widths: map[*doc]float32{}}
	s.pal = palette.New(o.Fuzzy, paletteSource{s})
	s.watchWorkspace()
	s.off = o.Sessions.Subscribe(s.sessionsChanged)
	s.offSet = o.Settings.Subscribe(func(contract.Settings) { s.applyTheme() })
	for _, x := range o.Sessions.All() {
		s.attach(x)
	}
	return s
}

// Dispose stops following the sessions and settings.
func (s *Shell) Dispose() {
	s.search.stop()
	if s.files.off != nil {
		s.files.off()
	}
	s.off()
	s.offSet()
	for _, d := range s.docs {
		s.detach(d)
	}
}

func (s *Shell) sessionsChanged(ev contract.SessionsEvent) {
	switch ev.Kind {
	case contract.SessionOpened, contract.SessionRestored:
		if x, ok := s.o.Sessions.Get(ev.ID); ok {
			s.activate(s.attach(x))
		}
	case contract.SessionClosed:
		i := slices.IndexFunc(s.docs, func(d *doc) bool { return d.s.ID() == ev.ID })
		if i < 0 {
			return
		}
		d := s.docs[i]
		s.docs = slices.Delete(s.docs, i, i+1)
		if p := d.s.State().Path; p != "" {
			s.closed = append(slices.DeleteFunc(s.closed, func(q string) bool { return q == p }), p)
			if len(s.closed) > maxClosed {
				s.closed = s.closed[1:]
			}
		}
		delete(s.widths, d)
		if s.drag.d == d {
			s.drag.d = nil
		}
		s.detach(d)
		if s.active == d {
			s.active = nil
			if len(s.docs) > 0 {
				s.activate(s.docs[min(i, len(s.docs)-1)])
			}
		}
		s.sessionChanged()
	}
	s.invalidate()
}

func (s *Shell) attach(x contract.Session) *doc {
	for _, d := range s.docs {
		if d.s.ID() == x.ID() {
			return d
		}
	}
	d := &doc{s: x, mode: editorapi.ModeLive, counted: ^contract.Version(0)}
	d.live = liveedit.New(liveedit.Options{Session: x, Projector: s.o.Projector, Editing: s.o.Editing,
		Prefs: s.prefs, Exts: s.o.Exts, ObjectLangs: s.o.ObjectLangs, Renderer: s.o.Renderer, Theme: s.palette(s.dark), Dispatcher: s.o.Dispatcher,
		DocDir: func() string {
			if p := x.State().Path; p != "" {
				return filepath.Dir(p)
			}
			return ""
		}, KeyLabel: s.keyLabel, OpenFile: s.Open, Clip: s.o.Clip, Converter: s.o.Converter, Clipboard: s.o.Clipboard, Assets: s.o.Assets,
		DocPath: func() string { return x.State().Path },
		SaveFirst: func(done func(bool)) {
			if d := s.docOf(x); d != nil {
				s.saveAs(d, func(err error) { done(err == nil) })
			}
		},
		OnError: s.fail})
	d.offs = append(d.offs, x.OnState(func(contract.DocState) { s.invalidate() }))
	s.docs = append(s.docs, d)
	if s.active == nil {
		s.activate(d)
	}
	return d
}

func (s *Shell) detach(d *doc) {
	for _, off := range d.offs {
		off()
	}
	d.live.Close()
	if d.source != nil {
		d.source.Close()
	}
}

func (s *Shell) activate(d *doc) {
	s.active = d
	s.focusNext = true
	s.reveal = true
	s.invalidate()
	s.sessionChanged()
}

// activateAt activates the tab at index i, if there is one.
func (s *Shell) activateAt(i int) {
	if i >= 0 && i < len(s.docs) {
		s.activate(s.docs[i])
	}
}

// step activates the tab by of the active one, wrapping around.
func (s *Shell) step(by int) {
	if n := len(s.docs); n > 0 {
		s.activateAt((slices.Index(s.docs, s.active) + by + n) % n)
	}
}

// reopenClosed opens the latest closed tab whose file is not open again.
func (s *Shell) reopenClosed() {
	for len(s.closed) > 0 {
		p := s.closed[len(s.closed)-1]
		s.closed = s.closed[:len(s.closed)-1]
		if _, open := s.o.Sessions.FindByPath(p); !open {
			s.Open(p)
			return
		}
	}
}

// dragTab moves the dragged tab by dx, and past a neighbor once it is over half of it.
func (s *Shell) dragTab(d *doc, dx float32) {
	if s.drag.d != d {
		s.drag.d, s.drag.x = d, 0
	}
	s.drag.x += dx
	// Only in the direction of this move, so that a tab never swaps back and forth.
	for {
		i := slices.Index(s.docs, d)
		switch {
		case dx > 0 && i+1 < len(s.docs) && s.widths[s.docs[i+1]] > 0 && s.drag.x > s.widths[s.docs[i+1]]/2:
			s.drag.x -= s.widths[s.docs[i+1]] + tabGap
			s.docs[i], s.docs[i+1] = s.docs[i+1], s.docs[i]
			s.sessionChanged()
		case dx < 0 && i > 0 && s.widths[s.docs[i-1]] > 0 && -s.drag.x > s.widths[s.docs[i-1]]/2:
			s.drag.x += s.widths[s.docs[i-1]] + tabGap
			s.docs[i], s.docs[i-1] = s.docs[i-1], s.docs[i]
			s.sessionChanged()
		default:
			return
		}
	}
}

func (s *Shell) invalidate() {
	if s.hasServ {
		s.services.Invalidate()
	}
}

func (s *Shell) prefs() contract.EditPrefs {
	e := s.o.Settings.Get().Editor
	p := contract.EditPrefs{StrongDelim: e.StrongDelim, EmphasisDelim: e.EmphasisDelim, TabSize: e.TabSize, AutoPair: e.AutoPair}
	if e.BulletMarker != "" {
		p.BulletMarker = e.BulletMarker[0]
	}
	return p
}

// palette returns the tokens of the settings, in the same value for the same options.
func (s *Shell) palette(dark bool) *theme.Tokens {
	a := s.o.Settings.Get().Appearance
	o := theme.Options{Dark: dark, LowPower: a.ReduceTransparency, Still: s.still, Hue: float32(a.AccentHue)}
	t := s.tokens[o]
	if t == nil {
		t = theme.For(o)
		s.tokens[o] = t
	}
	return t
}

func (s *Shell) applyTheme() {
	t := s.palette(s.dark)
	for _, d := range s.docs {
		d.live.SetTheme(t)
		if d.source != nil {
			d.source.SetTheme(t)
		}
	}
	s.invalidate()
}

// ToggleTheme flips light and dark, dissolving out from a point of the window.
func (s *Shell) ToggleTheme(at ui.Point) {
	s.dark = !s.dark
	s.themeGen++
	s.origin = at
	mode := "light"
	if s.dark {
		mode = "dark"
	}
	if err := s.o.Settings.Update(func(v *contract.Settings) { v.Appearance.Theme = mode }); err != nil {
		s.fail(err)
	}
	s.applyTheme()
}

// followSystem settles the theme once the window's preference is known.
func (s *Shell) followSystem(c *ui.Context) {
	mode := s.o.Settings.Get().Appearance.Theme
	want := s.dark
	switch mode {
	case "dark":
		want = true
	case "light":
		want = false
	default:
		want = c.Theme().Dark
	}
	if want != s.dark || !s.themeSet {
		s.themeSet = true
		s.dark = want
		s.applyTheme()
	}
}

// Dark reports the current theme.
func (s *Shell) Dark() bool { return s.dark }

// View builds the window.
func (s *Shell) View(c *ui.Context) {
	s.services, s.hasServ = c.Services(), true
	s.followSystem(c)
	// The toolkit draws the view menu: it follows the app's theme, not only the system's.
	if own := c.Theme(); own.Dark != s.dark {
		base := ui.LightTheme()
		if s.dark {
			base = ui.DarkTheme()
		}
		base.Font, base.FontSize = own.Font, own.FontSize
		c.SetTheme(base)
	}
	s.bindCommands(c)
	s.escapeReading(c)
	s.still = s.reduceMotion(c)
	s.live = true
	root := ui.Box(c).Fill().Clip()
	v := root.Animate("theme", float32(s.themeGen), 400*time.Millisecond)
	progress := v - float32(s.themeGen-1)
	root.Children(func() {
		if progress >= 1 || s.themeGen == 0 {
			s.window(c.Key("shell"), s.palette(s.dark))
			return
		}
		// The old theme stays underneath while the new one dissolves in (ADR-004).
		s.live = false
		s.window(c.Key("old"), s.palette(!s.dark))
		s.live = true
		over := ui.Box(c.Key("new")).Absolute().Fill()
		a := s.o.Settings.Get().Appearance
		if a.ReduceTransparency || s.cpu || s.reduceMotion(c) {
			over = over.Opacity(progress)
		} else {
			w, h := c.Size()
			over = effects.Dissolve(over, s.origin, progress*(w+h))
		}
		over.Children(func() { s.window(c.Key("shell"), s.palette(s.dark)) })
	})
}

func (s *Shell) matched(a *ui.Animation, id any) ui.ElementTransition {
	if !s.live {
		return ui.ElementTransition{}
	}
	return motion.Still(s.still, ui.ElementTransition{Animation: a, Match: id})
}

func (s *Shell) window(c *ui.Context, t *theme.Tokens) {
	col := ui.Column(c).Fill().Font(theme.Sans).TextColor(t.Text).Draw(func(p *ui.Painter, r ui.Rect) {
		s.cpu = !effects.PaintWindow(p, r, t)
	})
	if s.reading && s.active == nil {
		s.reading = false
	}
	// Reading mode folds the title bar away; focus mode fades it (02 §5.10).
	var read, dim float32
	if s.reading {
		read = 1
	}
	if s.focused() {
		dim = 1
	}
	read = col.AnimateTo("reading", read, motion.Smooth)
	dim = col.AnimateTo("focus", dim, motion.Smooth)
	col.Children(func() {
		if read < 0.999 {
			h := max(c.TitleBar().Height, theme.TitleBar)
			ui.Column(c.Key("titleBar")).Height(h * (1 - read)).Clip().Opacity((1 - read) * (1 - dim*(1-focusBar))).Children(func() {
				s.titleBar(c, t)
			})
		}
		ui.Row(c).Grow(1).AlignItems(ui.Stretch).Children(func() {
			s.sideBar(c, t, s.sidebar && !s.reading && !s.focused())
			s.sheet(c, t, read, dim)
		})
		s.paletteView(c, t)
		s.settingsView(c, t)
		if d := s.active; d != nil && s.live && d.mode != editorapi.ModeSource {
			d.live.Lightbox(c.Key("lightbox"))
		}
	})
}

type pillMatch struct{}

type tabMatch struct{ d *doc }

func (s *Shell) titleBar(c *ui.Context, t *theme.Tokens) {
	bar := c.TitleBar()
	right := bar.Right + 6
	if s.o.WindowButtons {
		right = 0
	}
	ui.Row(c).Height(max(bar.Height, theme.TitleBar)).Padding(0, right, 0, bar.Left+12).Gap(6).AlignItems(ui.Center).DragWindow().Children(func() {
		ui.Image(c, kit.Brand(t.Aurora1, t.Aurora2, t.Aurora3)).Size(22, 22).TextColor(t.Text)
		if kit.IconButton(c.Key("toggleSidebar"), t, kit.IconSidebar, locales.ShellToggleSidebar(), 30, 16).Clicked() {
			s.sidebar = !s.sidebar
		}
		// The tabs scroll when they do not fit; the new button stays beside them.
		ui.ScrollHorizontal(c.Key("tabs")).Shrink(1).MinWidth(0).Gap(tabGap).AlignItems(ui.Center).Children(func() {
			// A copy: dragging reorders s.docs while the tabs are built.
			for _, d := range slices.Clone(s.docs) {
				s.tab(c.Key(d.s.ID()), t, d)
			}
		})
		s.reveal = false
		if kit.IconButton(c.Key("new"), t, kit.IconPlus, locales.ShellNew(), 26, 14).Clicked() {
			s.run(contract.CmdFileNew)
		}
		ui.Spacer(c)
		s.omnibox(c, t)
		ui.Spacer(c)
		if s.active != nil {
			s.viewMenu(c, t)
		}
		theme := kit.IconButton(c.Key("theme"), t, kit.IconContrast, locales.ShellToggleTheme(), 30, 16)
		if theme.Clicked() {
			r := theme.Bounds()
			s.ToggleTheme(ui.Point{X: r.X + r.W/2, Y: r.Y + r.H/2})
		}
		if kit.IconButton(c.Key("settings"), t, kit.IconSliders, locales.SettingsOpen(), 30, 16).Clicked() {
			s.OpenSettings()
		}
		if s.o.WindowButtons {
			s.windowButtons(c, t)
		}
	})
}

func (s *Shell) tab(c *ui.Context, t *theme.Tokens, d *doc) {
	st := d.s.State()
	active := d == s.active
	tab := ui.Box(c).Height(30).Role(ui.RoleTab).Label(st.Name)
	clicked := tab.Clicked()
	hover := tab.Hovered()
	if dx, _, ok := tab.Dragged(); ok && dx != 0 {
		s.dragTab(d, dx)
	}
	if _, _, ended := tab.DragEnded(); ended && s.drag.d == d {
		s.drag.d = nil
		clicked = false
	}
	if s.drag.d == d {
		tab.Translate(s.drag.x, 0)
	} else {
		// The others glide aside as the dragged tab passes them.
		tab.Transition(s.matched(&motion.Smooth, tabMatch{d}))
	}
	if r := tab.Bounds(); r.W > 0 {
		s.widths[d] = r.W
	}
	if active && s.reveal {
		tab.ScrollIntoView()
	}
	tab.Children(func() {
		if active {
			ui.Box(c.Key("pill")).Absolute().Fill().Radius(theme.RadiusButton).Background(t.Sheet).
				Shadow(0, 1, 3, 0, t.ShadowColor).PassThrough().Transition(s.matched(&motion.Smooth, pillMatch{}))
		}
		// Absolute children draw above those in flow, and the pill is absolute, so an
		// unseen copy of the name sizes the tab and the label lies over the pill.
		ui.Row(c).Height(30).Padding(0, 8, 0, 12).Gap(6).Opacity(0).PassThrough().Children(func() {
			ui.Text(c.Key("size"), st.Name).FontSize(theme.UIText).FontWeight(600)
			ui.Box(c).Size(16, 16)
		})
		ui.Row(c).Absolute().Fill().Padding(0, 8, 0, 12).Gap(6).AlignItems(ui.Center).Children(func() {
			text := ui.Text(c, st.Name).FontSize(theme.UIText).Grow(1)
			if active {
				text.FontWeight(600)
			} else {
				text.TextColor(t.Muted)
			}
			// The same box holds the unsaved dot and the close button, so tabs keep
			// their width when hovered.
			ui.Box(c).Size(16, 16).Children(func() {
				switch {
				case hover || active:
					if kit.IconButton(c.Key("close"), t, kit.IconClose, locales.ShellClose(), 16, 10).Clicked() {
						s.closeDoc(d)
					}
				case st.Edited:
					ui.Box(c).Absolute().Top(5).Left(5).Size(6, 6).Radius(3).Background(t.Accent)
				}
			})
		})
	})
	if clicked {
		s.activate(d)
	}
}

func (s *Shell) omnibox(c *ui.Context, t *theme.Tokens) {
	om := ui.Row(c).Size(280, 30).Padding(0, 10).Gap(8).AlignItems(ui.Center).Radius(theme.RadiusButton).
		Background(t.Surface).Border(1, t.Border).Role(ui.RoleButton).Label(locales.ShellOmnibox())
	// The palette grows out of the omnibox and goes back into it (02 §5.4).
	if !s.pal.IsOpen() && !s.still {
		om.Transition(s.matched(&motion.Gentle, palette.Match))
	}
	om.Children(func() {
		ui.Icon(c, kit.IconSearch).Size(14, 14).TextColor(t.Muted)
		ui.Text(c, locales.ShellOmnibox()).FontSize(theme.UIText).TextColor(t.Muted).Grow(1).SingleLine()
		kit.KeyHint(c, t, s.keyLabel(contract.CmdNavQuickOpen))
	})
	if om.Clicked() {
		s.openPalette(palette.ModeFiles)
	}
}

func (s *Shell) viewMenu(c *ui.Context, t *theme.Tokens) {
	// The label names the mode the tab is in, as Focus.png has it.
	label := locales.ShellViewLive()
	switch {
	case s.active.mode == editorapi.ModeSource:
		label = locales.ShellViewSource()
	case s.active.live.FocusMode():
		label = locales.ShellViewFocus()
	case s.active.live.Typewriter():
		label = locales.ShellViewTypewriter()
	}
	// The tabs give way to it: it keeps its one line however many are open.
	b := ui.Row(c.Key("view")).Shrink(0).Height(30).Padding(0, 8).Gap(4).AlignItems(ui.Center).Radius(theme.RadiusButton).Role(ui.RoleButton).Label(label)
	if b.Hovered() {
		b.Background(t.Hover)
	}
	b.Children(func() {
		ui.Text(c, label).FontSize(theme.UIText).TextColor(t.Muted)
		ui.Icon(c, kit.IconChevron).Size(12, 12).TextColor(t.Muted)
	})
	// Drawn in the window: at 150% scale the native Windows menu clips its widest item.
	b.PopoverMenuButton(func(m *ui.Menu) {
		if m.Item(locales.ShellViewLive()).Checked(s.active.mode == editorapi.ModeLive).Chosen() {
			s.setMode(s.active, editorapi.ModeLive)
		}
		if m.Item(locales.ShellViewSource()).Checked(s.active.mode == editorapi.ModeSource).Chosen() {
			s.setMode(s.active, editorapi.ModeSource)
		}
		// The writing modes and the zoom, as Typora's View menu has them.
		m.Separator()
		writing := s.active.mode == editorapi.ModeLive
		if m.Item(locales.ShellReading()).Checked(s.reading).Chosen() {
			s.run(contract.CmdViewRead)
		}
		if m.Item(locales.ShellFocus()).Checked(writing && s.active.live.FocusMode()).Disabled(!writing).Chosen() {
			s.run(contract.CmdViewFocus)
		}
		if m.Item(locales.ShellTypewriter()).Checked(writing && s.active.live.Typewriter()).Disabled(!writing).Chosen() {
			s.run(contract.CmdViewTypewriter)
		}
		m.Separator()
		for _, z := range []struct {
			label string
			id    contract.CommandID
		}{
			{locales.ShellZoomIn(), contract.CmdViewZoomIn},
			{locales.ShellZoomOut(), contract.CmdViewZoomOut},
			{locales.ShellZoomReset(), contract.CmdViewZoomReset},
		} {
			if m.Item(z.label).Chosen() {
				s.run(z.id)
			}
		}
	})
}

func (s *Shell) windowButtons(c *ui.Context, t *theme.Tokens) {
	button := func(key string, icon *ui.SVG, label string, closing bool) ui.Element {
		b := ui.Row(c.Key(key)).Size(theme.WindowButton, theme.TitleBar).Justify(ui.Center).AlignItems(ui.Center).Role(ui.RoleButton).Label(label)
		tone := t.Muted
		if b.Hovered() {
			if closing {
				b.Background(t.Danger)
				tone = ui.RGB(255, 255, 255)
			} else {
				b.Background(t.Hover)
				tone = t.Text
			}
		}
		b.Children(func() { ui.Icon(c, icon).Size(12, 12).TextColor(tone) })
		return b
	}
	if button("min", kit.IconMinimize, locales.ShellMinimize(), false).Clicked() {
		s.o.Platform.Minimize()
	}
	if b := button("max", kit.IconMaximize, locales.ShellMaximize(), false).MaximizeButton(); b.Clicked() {
		s.o.Platform.ToggleMaximize()
	}
	if button("close", kit.IconClose, locales.ShellClose(), true).Clicked() {
		s.o.Platform.Close()
	}
}

func (s *Shell) sideBar(c *ui.Context, t *theme.Tokens, open bool) {
	var target float32
	if open {
		target = theme.SidebarWidth
	}
	box := ui.Box(c.Key("sidebar")).Clip()
	w := box.AnimateTo("width", target, motion.Smooth)
	box.Width(w)
	if w < 1 {
		return
	}
	box.Children(func() {
		ui.Column(c).Width(theme.SidebarWidth).FillHeight().Padding(4, 8, 8, 12).Gap(6).Children(func() {
			labels := []string{locales.ShellPanelFiles(), locales.ShellPanelOutline(), locales.ShellPanelSearch()}
			if i := kit.Segmented(c.Key("panels"), t, "panels", labels, s.panel, s.live); i >= 0 {
				s.panel = i
			}
			ui.Column(c.Key(s.panel)).Grow(1).MinHeight(0).Margin(0, -8, 0, -12).Children(func() {
				switch s.panel {
				case panelOutline:
					s.outlinePanel(c, t)
				case panelSearch:
					s.searchPanel(c, t)
				default:
					s.filesPanel(c, t)
				}
			})
		})
	})
}

// openDocuments lists the open tabs, when no folder is open.
func (s *Shell) openDocuments(c *ui.Context, t *theme.Tokens) {
	ui.Column(c).Padding(0, 8, 0, 12).Gap(2).Children(func() {
		ui.Text(c, locales.ShellOpenDocuments()).FontSize(theme.UIBadge).TextColor(t.Muted).Padding(8, 8, 6)
		for _, d := range s.docs {
			st := d.s.State()
			row := ui.Row(c.Key(d.s.ID())).Padding(6, 8).Gap(8).AlignItems(ui.Center).Radius(theme.RadiusRow).Role(ui.RoleButton).Label(st.Name)
			if d == s.active {
				row.Background(t.SurfaceHover)
			} else if row.Hovered() {
				row.Background(t.Hover)
			}
			row.Children(func() {
				ui.Text(c, st.Name).FontSize(theme.UIText).Grow(1)
				if st.Edited {
					ui.Box(c).Size(6, 6).Radius(3).Background(t.Accent)
				}
			})
			if row.Clicked() {
				s.activate(d)
			}
		}
	})
}

// sheet draws the paper; in reading mode (read 1) it fills the window with
// square corners, in focus mode (dim 1) it fades at the top and bottom.
func (s *Shell) sheet(c *ui.Context, t *theme.Tokens, read, dim float32) {
	m := 8 * (1 - read)
	ui.Box(c).Grow(1).Margin(0, m, m, 0).Radius(theme.RadiusSheet * (1 - read)).Background(t.Sheet).Clip().Children(func() {
		s.auroraShown = false
		if s.active == nil {
			s.auroraLayer(c.Key("welcomeAurora"), t, &s.welcomeAt, welcomePools(t), t.Sheet, 0)
			s.welcome(c, t)
		} else {
			s.welcomeAt = time.Time{}
			d := s.active
			s.document(c.Key(d.s.ID()), t, d)
			if dim > 0.01 {
				s.focusFades(c, t, dim)
			}
			if read > 0.01 && s.active == d {
				s.readingTop(c, t, d, read)
			}
			// A banner's button can close the tab as it builds.
			if s.active == d && !s.reading {
				s.capsule(c, t, d)
			}
		}
		s.loading(c, t)
		s.noticeStack(c, t)
	})
}

func (s *Shell) document(c *ui.Context, t *theme.Tokens, d *doc) {
	ui.Column(c).Absolute().Fill().Children(func() {
		s.banner(c, t, d)
		ui.Box(c.Key("editor")).Grow(1).Children(func() {
			s.editor(c, t, d)
			// An empty document glows at its foot until there is text.
			if s.live && d.mode != editorapi.ModeSource && !s.reading && d.s.Model().Text().Len() == 0 {
				s.auroraLayer(c.Key("emptyAurora"), t, &s.emptyAt, emptyPools(t), ui.Color{}, .45)
			} else {
				s.emptyAt = time.Time{}
			}
			s.scrollEdge(c, t, d)
		})
	})
}

// scrollEdge fades the text into the sheet at its top edge once the
// document is scrolled (02 §6, glass.ScrollEdge's soft edge); in low power
// a hairline marks the edge instead.
func (s *Shell) scrollEdge(c *ui.Context, t *theme.Tokens, d *doc) {
	if !s.live || d.mode == editorapi.ModeSource || d.live.ScrollOffset() <= 0 {
		s.edgeShown = false
		return
	}
	s.edgeShown = true
	e := ui.Box(c.Key("scrollEdge")).Absolute().Top(0).Left(0).Right(0).PassThrough()
	if t.LowPower {
		e.Height(1).Background(t.Border)
		return
	}
	e.Height(28).Draw(func(p *ui.Painter, r ui.Rect) {
		p.FillGradient(r, ui.LinearGradient{From: t.Sheet.Alpha(.85), To: t.Sheet.Alpha(0), Angle: 180}, 0)
	})
}

// ScrollEdgeShown reports whether the sheet's top edge shows, for tests.
func (s *Shell) ScrollEdgeShown() bool { return s.edgeShown }

func (s *Shell) editor(c *ui.Context, t *theme.Tokens, d *doc) {
	switch {
	case !s.live && d.mode == editorapi.ModeSource:
		// One code editor state cannot build twice; the sheet stands in for it.
	case !s.live:
		d.live.Ghost(c, t)
	case d.mode == editorapi.ModeSource:
		e := d.source.View(c.Key("source"))
		if s.focusNext {
			s.focusNext = false
			e.Focus()
		}
	default:
		if s.reading {
			s.readDoc(d)
		}
		d.live.SetTypography(s.typography())
		ed := s.o.Settings.Get().Editor
		d.live.SetReveal(ed.Reveal, ed.SoftBreak)
		d.live.SetReduceMotion(s.reduceMotion(c))
		d.live.SetSmoothCaret(ed.SmoothCaret)
		if s.focusNext {
			s.focusNext = false
			d.live.Focus()
		}
		d.live.View(c.Key("live"))
		if !s.renderWarned && d.live.RenderUnavailable() {
			// Once a run: math and diagrams show as source (02 §5.9).
			s.renderWarned = true
			s.Notify(contract.Notice{Level: contract.NoticeWarning, Message: locales.ShellRenderUnavailable()})
		}
		// The rail stays in reading mode, as Reading.png has it.
		if !d.live.FocusMode() || s.reading {
			d.live.Rail(c.Key("rail"))
		}
		d.live.TableBar(c.Key("tableBar"))
		d.live.LinkBar(c.Key("linkBar"))
		d.live.FormatBar(c.Key("formatBar"))
		d.live.SlashMenu(c.Key("slashMenu"))
		s.findBar(c, t, d)
	}
}

func (s *Shell) capsule(c *ui.Context, t *theme.Tokens, d *doc) {
	label, tone := s.status(d, t)
	// Not pass-through: hovered, it opens to the reading time and the block
	// the caret is in (02 §3).
	capsule := kit.Capsule(c.Key("capsule"), t).Absolute().Bottom(16).Right(16)
	edited := d.s.State().Edited && tone == t.Warning
	hovered := capsule.Hovered()
	capsule.Children(func() {
		// The scroll progress ring (02 §3): a faint circle, the part read in
		// the accent from the top. It reads the fraction when it paints, so
		// it follows the scroll without a rebuild.
		if d.mode == editorapi.ModeLive {
			live := d.live
			ui.Box(c.Key("ring")).Size(12, 12).Draw(func(p *ui.Painter, r ui.Rect) {
				cx, cy, rad := r.X+r.W/2, r.Y+r.H/2, r.W/2-1
				p.StrokePath(new(ui.Path).Circle(cx, cy, rad), 1.5, t.Border)
				if f := live.ScrollFraction(); f > 0 {
					p.StrokePath(ringFromTop(cx, cy, rad).Trim(0, min(f, 1)).Cap(ui.RoundCap), 1.5, t.Accent)
				}
			})
		}
		// Unsaved is a hollow dot (02 §5.5); the other states a filled one.
		if edited {
			ui.Box(c).Size(7, 7).Radius(4).Border(1.5, tone)
		} else {
			ui.Box(c).Size(7, 7).Radius(4).Background(tone)
		}
		ui.Text(c, label).FontSize(theme.UIStatus)
		words := s.wordCount(c, d)
		ui.Text(c, locales.ShellWords(words)).FontSize(theme.UIStatus).TextColor(t.Muted)
		if hovered && d.mode == editorapi.ModeLive {
			n, total := d.live.CaretBlock()
			ui.Text(c, locales.ShellCapsuleDetail(readingMinutes(words), n, total)).FontSize(theme.UIStatus).TextColor(t.Muted)
		}
	})
}

// readingMinutes is the time to read a document of so many words, at 300 a
// minute (CJK characters count as words), never less than a minute.
func readingMinutes(words int) int { return max(1, (words+299)/300) }

// ringFromTop is a circle drawn clockwise from twelve o'clock, so a trimmed
// part of it grows the way a clock's hand goes.
func ringFromTop(cx, cy, r float32) *ui.Path {
	const k = 0.5522847498
	p := new(ui.Path).MoveTo(cx, cy-r)
	p.CubeTo(cx+r*k, cy-r, cx+r, cy-r*k, cx+r, cy)
	p.CubeTo(cx+r, cy+r*k, cx+r*k, cy+r, cx, cy+r)
	p.CubeTo(cx-r*k, cy+r, cx-r, cy+r*k, cx-r, cy)
	p.CubeTo(cx-r, cy-r*k, cx-r*k, cy-r, cx, cy-r)
	return p
}

func (s *Shell) status(d *doc, t *theme.Tokens) (string, ui.Color) {
	st := d.s.State()
	switch {
	case st.Conflict:
		return locales.ShellConflict(), t.Warning
	case st.Deleted:
		return locales.ShellDeletedStatus(), t.Danger
	case st.Err != nil:
		return locales.ShellSaveFailed(), t.Danger
	case st.Saving:
		return locales.ShellSaving(), t.Muted
	case st.Edited:
		return locales.ShellUnsaved(), t.Warning
	}
	return locales.ShellSaved(), t.Success
}

// wordCount counts at most every 300 ms, so a large document types as fast as a
// small one. A frame comes along with the last keystroke's count.
func (s *Shell) wordCount(c *ui.Context, d *doc) int {
	const every = 300 * time.Millisecond
	v := d.s.Model().Version()
	if v == d.counted {
		return d.words
	}
	if wait := every - c.Now().Sub(d.countedAt); wait > 0 && !d.countedAt.IsZero() {
		c.After(wait)
		return d.words
	}
	d.counted, d.countedAt = v, c.Now()
	d.words = CountWords(d.s.Model().Text().String())
	return d.words
}

// CountWords counts Latin words and CJK characters, each CJK character a word.
func CountWords(text string) int {
	n := 0
	in := false
	for _, r := range text {
		switch {
		case unicode.Is(unicode.Han, r) || unicode.Is(unicode.Hiragana, r) || unicode.Is(unicode.Katakana, r) || unicode.Is(unicode.Hangul, r):
			n++
			in = false
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			if !in {
				n++
			}
			in = true
		case r == '\'' || r == '-' || r == '_':
		default:
			in = false
		}
	}
	return n
}

func (s *Shell) welcome(c *ui.Context, t *theme.Tokens) {
	ui.Column(c).Absolute().Fill().Justify(ui.Center).AlignItems(ui.Center).Gap(8).Children(func() {
		ui.Text(c, locales.AppName()).Font(theme.Serif).FontSize(68).FontWeight(500).LetterSpacing(-2)
		ui.Text(c, locales.ShellWelcome()).FontSize(16).TextColor(t.Muted)
		ui.Row(c).Gap(10).Padding(20, 0, 0).Children(func() {
			if kit.Button(c.Key("new"), t, locales.ShellNew(), s.keyLabel(contract.CmdFileNew), false).Clicked() {
				s.run(contract.CmdFileNew)
			}
			if kit.Button(c.Key("open"), t, locales.ShellOpen(), s.keyLabel(contract.CmdFileOpen), true).Clicked() {
				s.run(contract.CmdFileOpen)
			}
		})
		s.recentList(c, t)
	})
}

// Up to this many recent folders and files show on the welcome page.
const (
	recentFolders = 3
	recentFiles   = 5
)

// recentList lists the recent folders and files under the welcome page's
// buttons; a click opens one again.
func (s *Shell) recentList(c *ui.Context, t *theme.Tokens) {
	r := s.o.Recent
	if r == nil {
		return
	}
	folders, files := r.Folders(), r.Files()
	folders, files = folders[:min(len(folders), recentFolders)], files[:min(len(files), recentFiles)]
	if len(folders)+len(files) == 0 {
		return
	}
	ui.Column(c).Width(420).Padding(28, 0, 0).Gap(2).Children(func() {
		ui.Text(c, locales.ShellRecent()).FontSize(theme.UIBadge).TextColor(t.Muted).Padding(0, 10, 6)
		row := func(path string, dir bool) {
			icon := kit.IconFile
			if dir {
				icon = kit.IconFolder
			}
			e := ui.Row(c.Key(path)).Height(30).Padding(0, 10).Gap(10).AlignItems(ui.Center).Radius(8).
				Role(ui.RoleButton).Label(filepath.Base(path)).Focusable()
			if e.Hovered() {
				e.Background(t.Hover)
			}
			e.Children(func() {
				ui.Icon(c, icon).Size(14, 14).TextColor(t.Muted)
				// The name whole; the folder gives way.
				ui.Text(c, filepath.Base(path)).FontSize(theme.UIText).SingleLine().Shrink(0)
				ui.Text(c, filepath.Dir(path)).FontSize(theme.UIStatus).TextColor(t.Faint).SingleLine().Shrink(1).Grow(1).MinWidth(0)
			})
			if e.Clicked() {
				if dir {
					s.OpenFolder(path)
				} else {
					s.Open(path)
				}
			}
		}
		for _, p := range folders {
			row(p, true)
		}
		for _, p := range files {
			row(p, false)
		}
	})
}

// run runs a command, reporting what it returns.
func (s *Shell) run(id contract.CommandID) {
	// A shortcut for a command that is off right now (no tab, source mode) does
	// nothing, as a greyed-out menu item would.
	if cmd, ok := s.o.Commands.Lookup(id); ok && cmd.Enabled != nil && !cmd.Enabled() {
		return
	}
	if err := s.o.Commands.Run(id); err != nil {
		s.fail(err)
	}
}

func (s *Shell) fail(err error) {
	s.Notify(contract.Notice{Level: contract.NoticeError, Message: errorText(err)})
}

// errorText says what went wrong in words.
func errorText(err error) string {
	var e *contract.Error
	if errors.As(err, &e) {
		switch e.Code {
		case contract.CodeTooLarge:
			return locales.ShellTooLarge()
		case contract.CodeNotFound:
			return locales.ShellNotFound()
		case contract.CodeConflict:
			return locales.ShellConflict()
		case contract.CodeCanceled:
			return ""
		}
	}
	return locales.ShellFailed()
}

// Open opens a file, or shows its tab if it is open.
func (s *Shell) Open(path string) {
	s.startOpening(path)
	s.o.Sessions.Open(path, func(x contract.Session, err error) {
		s.doneOpening(path)
		if err != nil {
			s.fail(err)
			return
		}
		s.activate(s.attach(x))
		if s.o.Recent != nil {
			s.o.Recent.Add(path, false)
		}
	})
}

// Save saves the active document, asking where when it has no file yet.
func (s *Shell) save(d *doc, done func(error)) {
	if d.source != nil {
		d.source.Sync()
	}
	if done == nil {
		done = func(err error) {
			// A conflict has its banner, with what to do about it.
			if err != nil && contract.CodeOf(err) != contract.CodeConflict {
				s.fail(err)
			}
		}
	}
	if d.s.State().Path == "" {
		s.saveAs(d, done)
		return
	}
	d.s.Save(done)
}

func (s *Shell) saveAs(d *doc, done func(error)) {
	if d.source != nil {
		d.source.Sync()
	}
	s.o.Platform.SavePath(d.s.State().Name, func(path string) {
		if path == "" {
			done(&contract.Error{Code: contract.CodeCanceled, Op: "shell.SaveAs"})
			return
		}
		if filepath.Ext(path) == "" {
			path += ".md"
		}
		d.s.SaveAs(path, done)
	})
}

// closeDoc closes a tab, asking first about unsaved changes. done, if any, hears
// whether it closed.
func (s *Shell) closeDoc(d *doc) { s.closeThen(d, func(bool) {}) }

func (s *Shell) closeThen(d *doc, done func(bool)) {
	if d.source != nil {
		d.source.Sync()
	}
	if d.s.Close(false) == contract.CloseDone {
		done(true)
		return
	}
	s.o.Platform.ConfirmClose(d.s.State().Name, func(ch Choice) {
		switch ch {
		case ChoiceDiscard:
			d.s.Close(true)
			done(true)
		case ChoiceSave:
			s.save(d, func(err error) {
				if err != nil {
					s.fail(err)
					done(false)
					return
				}
				d.s.Close(true)
				done(true)
			})
		default:
			done(false)
		}
	})
}

// CloseAll closes every document, asking about each with unsaved changes, and then
// calls done with whether all closed. The window closes on true.
func (s *Shell) CloseAll(done func(bool)) {
	if len(s.docs) == 0 {
		done(true)
		return
	}
	s.closeThen(s.docs[0], func(ok bool) {
		if !ok {
			done(false)
			return
		}
		s.CloseAll(done)
	})
}

func (s *Shell) setMode(d *doc, m editorapi.Mode) {
	// Choosing a view leaves reading mode first.
	if s.reading {
		s.setReading(false)
	}
	s.switchMode(d, m)
}

// switchMode shows a tab in the live or the source view.
func (s *Shell) switchMode(d *doc, m editorapi.Mode) {
	if d.mode == m {
		return
	}
	switch m {
	case editorapi.ModeSource:
		if d.source == nil {
			d.source = source.New(d.s.Model(), s.palette(s.dark))
			d.source.SetMotion(s.live)
		}
		d.source.Enter(d.live.TopOffset())
	default:
		top := d.source.Leave()
		d.live.ScrollToOffset(top)
	}
	d.mode = m
	s.focusNext = true
	s.invalidate()
}

// keyLabel writes a command's first shortcut for this platform.
func (s *Shell) keyLabel(id contract.CommandID) string {
	keys := s.o.Commands.KeysFor(id)
	if len(keys) == 0 {
		return ""
	}
	return chordLabel(keys[0])
}

func chordLabel(ch contract.Chord) string {
	var b strings.Builder
	if ch.Mods&contract.ModPrimary != 0 {
		if ui.Cmd == ui.Ctrl {
			b.WriteString("Ctrl ")
		} else {
			b.WriteString("⌘ ")
		}
	}
	if ch.Mods&contract.ModShift != 0 {
		b.WriteString("Shift ")
	}
	if ch.Mods&contract.ModAlt != 0 {
		b.WriteString("Alt ")
	}
	b.WriteString(ch.Key)
	return b.String()
}

// What tests read.

// Sessions returns the sessions with tabs, in order.
func (s *Shell) Sessions() []contract.Session {
	out := make([]contract.Session, len(s.docs))
	for i, d := range s.docs {
		out[i] = d.s
	}
	return out
}

// ActiveSession returns the shown document's session, nil on the welcome page.
func (s *Shell) ActiveSession() contract.Session {
	if s.active == nil {
		return nil
	}
	return s.active.s
}

// ActiveLive returns the shown document's live editor.
func (s *Shell) ActiveLive() *liveedit.Editor { return s.active.live }

// ActiveSource returns the shown document's source view, nil if never shown.
func (s *Shell) ActiveSource() *source.Editor { return s.active.source }

// ActiveMode returns the shown document's view.
func (s *Shell) ActiveMode() editorapi.Mode { return s.active.mode }

// SidebarOpen reports the sidebar.
func (s *Shell) SidebarOpen() bool { return s.sidebar }

// SheetLeft returns the sheet's left edge in the window.
func (s *Shell) SheetLeft() float32 {
	if s.sidebar && !s.reading && !s.focused() {
		return theme.SidebarWidth
	}
	return 0
}

// Notices returns the messages shown.
func (s *Shell) Notices() []string {
	out := make([]string, len(s.notices))
	for i, h := range s.notices {
		out[i] = h.n.Message
	}
	return out
}

// Status returns what the capsule says.
func (s *Shell) Status() string {
	if s.active == nil {
		return ""
	}
	label, _ := s.status(s.active, s.palette(s.dark))
	return label
}

// Check verifies each document's rows against its tree.
func (s *Shell) Check() error {
	for _, d := range s.docs {
		if err := d.live.CheckRows(); err != nil {
			return err
		}
	}
	return nil
}
