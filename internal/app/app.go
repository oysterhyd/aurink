// Package app is the composition root: it builds the real services and the window.
package app

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"aurink/internal/adapters/trash"
	"aurink/internal/adapters/watch"
	"aurink/internal/contract"
	"aurink/internal/core/commands"
	"aurink/internal/core/docsession"
	"aurink/internal/core/document"
	"aurink/internal/core/editing"
	"aurink/internal/core/fsstore"
	"aurink/internal/core/projection"
	"aurink/internal/core/search"
	"aurink/internal/core/settings"
	"aurink/internal/core/workspace"
	"aurink/internal/views/shell"
	"aurink/locales"
	"aurink/resources"
)

// m1Exts is what M1 renders (06 §2.1): the other extensions are off until their cards.
const m1Exts = contract.ExtFrontMatter

// windowDispatcher brings work back to the window's thread and draws afterwards.
type windowDispatcher struct{ w *mygo.Window }

func (d *windowDispatcher) Post(fn func()) {
	if d.w == nil || d.w.IsDestroyed() {
		return
	}
	d.w.Update(fn)
}

// Run opens the main window and blocks until the app quits.
func Run() error {
	if !mygo.App.RequestSingleInstanceLock() {
		return nil // The first instance got the arguments and opens them.
	}
	var a *application
	mygo.App.OnOpenFile(func(path string) {
		if a != nil {
			a.open(path)
		}
	})
	mygo.App.OnSecondInstance(func([]string, string) {
		if a != nil {
			a.window.Show()
			a.window.Focus()
		}
	})
	mygo.App.WhenReady(func() {
		var err error
		if a, err = newApplication(); err != nil {
			slog.Error("startup failed", "error", err)
			mygo.Dialog.Error(locales.AppName(), err.Error())
			mygo.App.Quit()
		}
	})
	return mygo.App.Run()
}

type application struct {
	window   *mygo.Window
	shell    *shell.Shell
	sessions contract.Sessions
	// Files asked for before the last session's recovery finished wait here.
	restored bool
	pending  []string
	closing  bool
}

func newApplication() (*application, error) {
	if err := resources.RegisterFonts(); err != nil {
		return nil, err
	}
	data, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return nil, err
	}
	a := &application{}
	dispatcher := &windowDispatcher{}
	files := fsstore.New()
	store, err := settings.New(filepath.Join(data, "settings.json"), files, dispatcher, func(err error) {
		slog.Error("settings write failed", "error", err)
	})
	if err != nil {
		return nil, err
	}
	base := store.Get()
	base.Editor.Exts = m1Exts
	sessions := docsession.New(files, document.Factory{}, dispatcher, filepath.Join(data, "recovery"), docsession.Options{Settings: &base})
	a.sessions = sessions
	registry := commands.New()
	registry.SetDefaults(commands.DefaultBindings())
	a.shell = shell.New(shell.Options{
		Sessions: sessions, Settings: store, Projector: projection.New(), Editing: editing.New(),
		Commands: registry, Dispatcher: dispatcher, Platform: a, Exts: m1Exts, Fuzzy: search.NewFuzzy(),
		Workspace: workspace.New(watch.New(), trash.New(), dispatcher, workspace.Options{}), Search: search.NewWorkspaceSearch(),
		WindowButtons: runtime.GOOS != "darwin",
	})
	if err := registry.Register(a.shell.Commands()...); err != nil {
		return nil, err
	}
	style := mygo.WindowOptions{
		Title: locales.AppName(), Width: 1280, Height: 800, MinWidth: 640, MinHeight: 480, StateKey: "main",
		TitleBarStyle: mygo.TitleBarHidden, HideWindowButtons: runtime.GOOS != "darwin",
		Content: ui.View(a.shell.View),
	}
	a.window = mygo.NewWindow(style)
	dispatcher.w = a.window
	a.window.OnClose(func(e *mygo.CloseEvent) {
		if a.closing {
			return
		}
		e.PreventDefault()
		a.shell.CloseAll(func(ok bool) {
			if !ok {
				return
			}
			// Close removes recovery copies in the session queue. Restore runs after
			// them in it, so the next start does not bring closed documents back.
			sessions.Restore(func([]contract.Session, error) {
				a.closing = true
				a.window.Close()
			})
		})
	})
	if runtime.GOOS == "darwin" {
		mygo.App.SetMenu(buildMenu(registry))
	}
	sessions.Restore(func(_ []contract.Session, err error) {
		if err != nil {
			a.shell.Notify(contract.Notice{Level: contract.NoticeError, Message: err.Error()})
		}
		a.restored = true
		for _, p := range a.pending {
			a.shell.Open(p)
		}
		a.pending = nil
	})
	return a, nil
}

func (a *application) open(path string) {
	a.window.Update(func() {
		if !a.restored {
			a.pending = append(a.pending, path)
			return
		}
		a.shell.Open(path)
	})
}

// buildMenu is the application menu (macOS): the registered commands and nothing else.
func buildMenu(r contract.CommandRegistry) *mygo.Menu {
	items := []*mygo.MenuItem{{Role: mygo.RoleAppMenu}}
	groups := []struct {
		label string
		ids   []contract.CommandID
	}{
		{locales.ShellOpen(), []contract.CommandID{contract.CmdFileNew, contract.CmdFileOpen, contract.CmdFileSave, contract.CmdFileSaveAs, contract.CmdFileClose}},
		{locales.ShellViewLive(), []contract.CommandID{contract.CmdViewLive, contract.CmdViewSource, contract.CmdViewToggleSidebar, contract.CmdViewThemeLight, contract.CmdViewThemeDark}},
	}
	items = append(items, &mygo.MenuItem{Role: mygo.RoleEditMenu})
	for _, g := range groups {
		top := &mygo.MenuItem{Label: g.label}
		for _, id := range g.ids {
			cmd, ok := r.Lookup(id)
			if !ok {
				continue
			}
			top.Submenu = append(top.Submenu, &mygo.MenuItem{Label: shell.Title(cmd.Title), Click: func(*mygo.MenuItem, *mygo.Window) {
				if err := r.Run(id); err != nil {
					slog.Error("menu command failed", "command", id, "error", err)
				}
			}})
		}
		items = append(items, top)
	}
	return mygo.NewMenu(items)
}

// The dialogs block, so they run off the main thread and answer on it.

func (a *application) OpenFile(done func(string)) {
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: a.window, Filters: filters()})
		a.window.Update(func() {
			if err != nil {
				a.shell.Notify(contract.Notice{Level: contract.NoticeError, Message: err.Error()})
			}
			if len(paths) > 0 {
				done(paths[0])
			} else {
				done("")
			}
		})
	}()
}

func (a *application) SavePath(name string, done func(string)) {
	go func() {
		path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{Parent: a.window, DefaultPath: name + ".md", Filters: filters()})
		a.window.Update(func() {
			if err != nil {
				a.shell.Notify(contract.Notice{Level: contract.NoticeError, Message: err.Error()})
				path = ""
			}
			done(path)
		})
	}()
}

func (a *application) ConfirmClose(name string, done func(shell.Choice)) {
	go func() {
		res, err := mygo.Dialog.Message(mygo.MessageOptions{Parent: a.window, Type: mygo.MessageWarning, Title: locales.AppName(),
			Message: locales.ShellConfirmTitle(name), Buttons: []string{locales.ShellSave(), locales.ShellDiscard(), locales.ShellCancel()},
			CancelButton: 2})
		a.window.Update(func() {
			switch {
			case err != nil:
				fmt.Fprintln(os.Stderr, err)
				done(shell.ChoiceCancel)
			case res.Button == 0:
				done(shell.ChoiceSave)
			case res.Button == 1:
				done(shell.ChoiceDiscard)
			default:
				done(shell.ChoiceCancel)
			}
		})
	}()
}

func (a *application) OpenFolder(done func(string)) {
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: a.window, Directory: true})
		a.window.Update(func() {
			if err != nil {
				a.shell.Notify(contract.Notice{Level: contract.NoticeError, Message: err.Error()})
			}
			if len(paths) > 0 {
				done(paths[0])
			} else {
				done("")
			}
		})
	}()
}

func (a *application) ConfirmTrash(name string, done func(bool)) {
	go func() {
		res, err := mygo.Dialog.Message(mygo.MessageOptions{Parent: a.window, Type: mygo.MessageWarning, Title: locales.AppName(),
			Message: locales.ShellTrashUnsaved(name), Buttons: []string{locales.ShellTrash(), locales.ShellCancel()}, CancelButton: 1})
		a.window.Update(func() { done(err == nil && res.Button == 0) })
	}()
}

func (a *application) Minimize()         { a.window.Minimize() }
func (a *application) ToggleMaximize()   { a.window.ToggleMaximize() }
func (a *application) IsMaximized() bool { return a.window.IsMaximized() }
func (a *application) Close()            { a.window.Close() }

func filters() []mygo.FileFilter {
	return []mygo.FileFilter{{Name: locales.ShellMarkdownFiles(), Extensions: []string{"md", "markdown"}}}
}
