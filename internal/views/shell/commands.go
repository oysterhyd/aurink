package shell

import (
	"aurink/internal/contract"
	"aurink/internal/views/editorapi"
	"aurink/locales"
	"github.com/egoist/mygo/ui"
)

// Commands returns the commands M1 implements (06 INT-1.1): menus and shortcuts come
// from these alone. Title holds a key of Title.
func (s *Shell) Commands() []contract.Command {
	hasDoc := func() bool { return s.active != nil }
	return []contract.Command{
		{ID: contract.CmdFileNew, Title: "new", Run: func() error { s.o.Sessions.New(); return nil }},
		{ID: contract.CmdFileOpen, Title: "open", Run: func() error {
			s.o.Platform.OpenFile(func(path string) {
				if path != "" {
					s.Open(path)
				}
			})
			return nil
		}},
		{ID: contract.CmdFileSave, Title: "save", Enabled: hasDoc, Run: func() error { s.save(s.active, nil); return nil }},
		{ID: contract.CmdFileSaveAs, Title: "saveAs", Enabled: hasDoc, Run: func() error {
			s.saveAs(s.active, func(err error) {
				if err != nil {
					s.fail(err)
				}
			})
			return nil
		}},
		{ID: contract.CmdFileClose, Title: "close", Enabled: hasDoc, Run: func() error { s.closeDoc(s.active); return nil }},
		{ID: contract.CmdViewToggleSidebar, Title: "sidebar", Checked: func() bool { return s.sidebar }, Run: func() error {
			s.sidebar = !s.sidebar
			s.invalidate()
			return nil
		}},
		{ID: contract.CmdViewLive, Title: "live", Enabled: hasDoc, Checked: func() bool { return s.active != nil && s.active.mode == editorapi.ModeLive },
			Run: func() error { s.setMode(s.active, editorapi.ModeLive); return nil }},
		{ID: contract.CmdViewSource, Title: "source", Enabled: hasDoc, Checked: func() bool { return s.active != nil && s.active.mode == editorapi.ModeSource },
			Run: func() error {
				// The shortcut toggles, as there is no key for going back otherwise.
				if s.active.mode == editorapi.ModeSource {
					s.setMode(s.active, editorapi.ModeLive)
				} else {
					s.setMode(s.active, editorapi.ModeSource)
				}
				return nil
			}},
		{ID: contract.CmdViewThemeLight, Title: "light", Checked: func() bool { return !s.dark }, Run: func() error {
			if s.dark {
				s.ToggleTheme(ui.Point{})
			}
			return nil
		}},
		{ID: contract.CmdViewThemeDark, Title: "dark", Checked: func() bool { return s.dark }, Run: func() error {
			if !s.dark {
				s.ToggleTheme(ui.Point{})
			}
			return nil
		}},
	}
}

// Title says a command's title in the app's language.
func Title(key string) string {
	switch key {
	case "new":
		return locales.ShellNew()
	case "open":
		return locales.ShellOpen()
	case "save":
		return locales.ShellSave()
	case "saveAs":
		return locales.ShellSaveAs()
	case "close":
		return locales.ShellClose()
	case "sidebar":
		return locales.ShellToggleSidebar()
	case "live":
		return locales.ShellViewLive()
	case "source":
		return locales.ShellViewSource()
	case "light":
		return locales.ShellThemeLight()
	case "dark":
		return locales.ShellThemeDark()
	}
	return key
}

// bindCommands gives every registered command its shortcuts.
func (s *Shell) bindCommands(c *ui.Context) {
	for _, cmd := range s.o.Commands.All() {
		for _, ch := range s.o.Commands.KeysFor(cmd.ID) {
			key, ok := keyOf(ch.Key)
			if !ok {
				continue
			}
			id := cmd.ID
			c.OnShortcut(modsOf(ch.Mods), key, func() { s.run(id) })
		}
	}
}

func modsOf(m contract.Mods) ui.Modifiers {
	var out ui.Modifiers
	if m&contract.ModPrimary != 0 {
		out |= ui.Cmd
	}
	if m&contract.ModShift != 0 {
		out |= ui.Shift
	}
	if m&contract.ModAlt != 0 {
		out |= ui.Alt
	}
	return out
}

var namedKeys = map[string]ui.Key{
	"A": ui.KeyA, "B": ui.KeyB, "C": ui.KeyC, "D": ui.KeyD, "E": ui.KeyE, "F": ui.KeyF, "G": ui.KeyG,
	"H": ui.KeyH, "I": ui.KeyI, "J": ui.KeyJ, "K": ui.KeyK, "L": ui.KeyL, "M": ui.KeyM, "N": ui.KeyN,
	"O": ui.KeyO, "P": ui.KeyP, "Q": ui.KeyQ, "R": ui.KeyR, "S": ui.KeyS, "T": ui.KeyT, "U": ui.KeyU,
	"V": ui.KeyV, "W": ui.KeyW, "X": ui.KeyX, "Y": ui.KeyY, "Z": ui.KeyZ, "/": ui.KeySlash,
}

func keyOf(name string) (ui.Key, bool) {
	k, ok := namedKeys[name]
	return k, ok
}
