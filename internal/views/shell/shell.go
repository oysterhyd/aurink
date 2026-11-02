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
	ConfirmClose(name string, done func(Choice))
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
	// Fuzzy ranks the palette's rows; nil matches by substring.
	Fuzzy contract.Fuzzy
	// Files lists the workspace's files for quick open; nil lists open and closed tabs only.
	Files func() []string
}

const maxNotices = 4

type doc struct {
	s      contract.Session
	live   *liveedit.Editor
	source *source.Editor
	mode   editorapi.Mode
	offs   []func()
	// The word count, recounted at most this often while typing.
	counted   contract.Version
	countedAt time.Time
	words     int
}

// Shell is one window's interface.
type Shell struct {
	o       Options
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
}

// maxClosed is how many closed tabs file.reopenClosed remembers.
const maxClosed = 20

// tabGap is the gap between tabs in the title bar.
const tabGap = 6

// New returns a shell over the sessions, with a tab for each already open.
func New(o Options) *Shell {
	s := &Shell{o: o, tokens: map[theme.Options]*theme.Tokens{}, widths: map[*doc]float32{}}
	s.pal = palette.New(o.Fuzzy, paletteSource{s})
	s.off = o.Sessions.Subscribe(s.sessionsChanged)
	s.offSet = o.Settings.Subscribe(func(contract.Settings) { s.applyTheme() })
	for _, x := range o.Sessions.All() {
		s.attach(x)
	}
	return s
}

// Dispose stops following the sessions and settings.
func (s *Shell) Dispose() {
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
		Prefs: s.prefs, Exts: s.o.Exts, Theme: s.palette(s.dark), Dispatcher: s.o.Dispatcher,
		DocDir: func() string {
			if p := x.State().Path; p != "" {
				return filepath.Dir(p)
			}
			return ""
		}})
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
		case dx < 0 && i > 0 && s.widths[s.docs[i-1]] > 0 && -s.drag.x > s.widths[s.docs[i-1]]/2:
			s.drag.x += s.widths[s.docs[i-1]] + tabGap
			s.docs[i], s.docs[i-1] = s.docs[i-1], s.docs[i]
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
	o := theme.Options{Dark: dark, LowPower: a.ReduceTransparency, Hue: float32(a.AccentHue)}
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
		if a.ReduceTransparency || s.cpu || c.Preferences().ReduceMotion {
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
	return ui.ElementTransition{Animation: a, Match: id}
}

func (s *Shell) window(c *ui.Context, t *theme.Tokens) {
	ui.Column(c).Fill().Font(theme.Sans).TextColor(t.Text).Draw(func(p *ui.Painter, r ui.Rect) {
		s.cpu = !effects.PaintWindow(p, r, t)
	}).Children(func() {
		s.titleBar(c, t)
		ui.Row(c).Grow(1).AlignItems(ui.Stretch).Children(func() {
			s.sideBar(c, t)
			s.sheet(c, t)
		})
		s.paletteView(c, t)
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
		ui.Icon(c, kit.Brand).Size(22, 22).TextColor(t.Accent)
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
	if !s.pal.IsOpen() && !c.Preferences().ReduceMotion {
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
	label := locales.ShellViewLive()
	if s.active.mode == editorapi.ModeSource {
		label = locales.ShellViewSource()
	}
	b := ui.Row(c.Key("view")).Height(30).Padding(0, 8).Gap(4).AlignItems(ui.Center).Radius(theme.RadiusButton).Role(ui.RoleButton).Label(label)
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

func (s *Shell) sideBar(c *ui.Context, t *theme.Tokens) {
	var target float32
	if s.sidebar {
		target = theme.SidebarWidth
	}
	box := ui.Box(c.Key("sidebar")).Clip()
	w := box.AnimateTo("width", target, motion.Smooth)
	box.Width(w)
	if w < 1 {
		return
	}
	box.Children(func() {
		ui.Column(c).Width(theme.SidebarWidth).Padding(4, 8, 8, 12).Gap(2).Children(func() {
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
	})
}

func (s *Shell) sheet(c *ui.Context, t *theme.Tokens) {
	ui.Box(c).Grow(1).Margin(0, 8, 8, 0).Radius(theme.RadiusSheet).Background(t.Sheet).Clip().Children(func() {
		if s.active == nil {
			s.welcome(c, t)
		} else {
			s.document(c.Key(s.active.s.ID()), t, s.active)
			s.capsule(c, t, s.active)
		}
		s.noticeStack(c, t)
	})
}

func (s *Shell) document(c *ui.Context, t *theme.Tokens, d *doc) {
	host := ui.Box(c).Absolute().Fill()
	host.Children(func() {
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
			if s.focusNext {
				s.focusNext = false
				d.live.Focus()
			}
			d.live.View(c.Key("live"))
		}
	})
}

func (s *Shell) capsule(c *ui.Context, t *theme.Tokens, d *doc) {
	label, tone := s.status(d, t)
	capsule := kit.Capsule(c, t).Absolute().Bottom(16).Right(16).PassThrough()
	capsule.Children(func() {
		ui.Box(c).Size(7, 7).Radius(4).Background(tone)
		ui.Text(c, label).FontSize(theme.UIStatus)
		ui.Text(c, locales.ShellWords(s.wordCount(c, d))).FontSize(theme.UIStatus).TextColor(t.Muted)
	})
}

func (s *Shell) status(d *doc, t *theme.Tokens) (string, ui.Color) {
	st := d.s.State()
	switch {
	case st.Conflict:
		return locales.ShellConflict(), t.Danger
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
	s.o.Sessions.Open(path, func(x contract.Session, err error) {
		if err != nil {
			s.fail(err)
			return
		}
		s.activate(s.attach(x))
	})
}

// Save saves the active document, asking where when it has no file yet.
func (s *Shell) save(d *doc, done func(error)) {
	if d.source != nil {
		d.source.Sync()
	}
	if done == nil {
		done = func(err error) {
			if err != nil {
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
	if d.mode == m {
		return
	}
	switch m {
	case editorapi.ModeSource:
		if d.source == nil {
			d.source = source.New(d.s.Model(), s.palette(s.dark))
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
	if s.sidebar {
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
