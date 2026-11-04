// Package app is the composition root: it builds the real services and the window.
package app

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"aurink/internal/adapters/clipboard"
	"aurink/internal/adapters/richpaste"
	"aurink/internal/adapters/trash"
	"aurink/internal/adapters/watch"
	"aurink/internal/adapters/webviewhost"
	"aurink/internal/contract"
	"aurink/internal/core/commands"
	"aurink/internal/core/docsession"
	"aurink/internal/core/document"
	"aurink/internal/core/editing"
	"aurink/internal/core/fsstore"
	"aurink/internal/core/projection"
	"aurink/internal/core/render"
	"aurink/internal/core/search"
	"aurink/internal/core/settings"
	"aurink/internal/core/workspace"
	"aurink/internal/views/shell"
	"aurink/locales"
	"aurink/resources"
	renderpage "aurink/resources/render"
)

// FollowSettings returns what a settings change does to the services that
// read the settings when built: the file tree's filter, the image importer
// and the sessions (autosave). exts stays the parsing extensions until the
// app renders them all.
func FollowSettings(ws *workspace.Workspace, assets *workspace.Assets, sessions *docsession.Manager, exts contract.Extensions) func(contract.Settings) {
	return func(v contract.Settings) {
		ws.SetShowAll(v.Files.ShowAllFiles)
		assets.SetFiles(v.Files.ImageDir, v.Files.CopyImages)
		v.Editor.Exts = exts
		sessions.SetSettings(v)
	}
}

// licenses are the about dialog's lines: what ships inside the app.
var licenses = []string{
	"MyGo — MIT",
	"MathJax 3.2.2 — Apache-2.0",
	"Mermaid 10.9.3 — MIT",
	"gemoji — MIT",
	"Newsreader — OFL-1.1",
	"Noto Serif SC — OFL-1.1",
	"JetBrains Mono — OFL-1.1",
}

// objectLangs are the code languages shown as diagrams.
var objectLangs = []string{"mermaid"}

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
	mygo.App.OnSecondInstance(func(args []string, wd string) {
		if a != nil {
			a.window.Show()
			a.window.Focus()
			for _, dir := range folderArgs(args, wd) {
				a.open(dir)
			}
		}
	})
	wd, _ := os.Getwd()
	folders := folderArgs(os.Args[1:], wd)
	mygo.App.WhenReady(func() {
		var err error
		if a, err = newApplication(); err != nil {
			slog.Error("startup failed", "error", err)
			mygo.Dialog.Error(locales.AppName(), err.Error())
			mygo.App.Quit()
			return
		}
		for _, dir := range folders {
			a.open(dir)
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
	// The extensions apply to documents opened after they change (Typora's way).
	exts := base.Editor.Exts
	renderer := &lateRenderer{}
	sessions := docsession.New(files, document.Factory{}, dispatcher, filepath.Join(data, "recovery"), docsession.Options{Settings: &base})
	a.sessions = sessions
	registry := commands.New()
	registry.SetDefaults(commands.DefaultBindings())
	ws := workspace.New(watch.New(), trash.New(), dispatcher, workspace.Options{ShowAll: base.Files.ShowAllFiles})
	assets := workspace.NewAssets(base.Files.ImageDir, base.Files.CopyImages)
	index := newFileIndex(ws, dispatcher)
	a.shell = shell.New(shell.Options{Files: index.Files,
		Sessions: sessions, Settings: store, Projector: projection.New(), Editing: editing.New(),
		Commands: registry, Dispatcher: dispatcher, Platform: a, Exts: exts, ObjectLangs: objectLangs, Renderer: renderer, Fuzzy: search.NewFuzzy(),
		Workspace: ws, Search: search.NewWorkspaceSearch(), Finder: search.NewFinder(),
		Clip: clipHTML(exts, store), Converter: richpaste.New(), Clipboard: clipboard.New(), Assets: assets,
		WindowButtons: runtime.GOOS != "darwin",
		SessionStore:  docsession.NewSessionStore(filepath.Join(data, "session.json"), files),
		Recent:        systemRecent{workspace.NewRecent(filepath.Join(data, "recent.json"), dispatcher)},
		ExportHTML:    ExportHTML(files, dispatcher, exts, store),
		SetLanguage:   mygo.App.SetLocale,
		Version:       mygo.App.Version(),
		Licenses:      licenses,
	})
	// The settings store is the language's source; files settings reach the
	// sessions as they change.
	mygo.App.SetLocale(base.Language)
	store.Subscribe(FollowSettings(ws, assets, sessions, exts))
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
	renderer.set(render.New(webviewhost.New(a.window), render.Options{
		Page: renderpage.Page(), CacheDir: filepath.Join(data, "render-cache")}))
	a.window.OnClose(func(e *mygo.CloseEvent) {
		if a.closing {
			return
		}
		e.PreventDefault()
		// The session is saved before the tabs close one by one (D9).
		a.shell.SaveSession(true, func(err error) {
			if err != nil {
				slog.Error("session save failed", "error", err)
			}
			a.closeAll(sessions)
		})
	})
	if runtime.GOOS == "darwin" {
		mygo.App.SetMenu(buildMenu(registry))
	}
	// Recovered copies first, then the last session's folder and tabs.
	sessions.Restore(func(_ []contract.Session, err error) {
		if err != nil {
			a.shell.Notify(contract.Notice{Level: contract.NoticeError, Message: err.Error()})
		}
		a.shell.RestoreSession(func() {
			a.restored = true
			for _, p := range a.pending {
				a.openNow(p)
			}
			a.pending = nil
		})
	})
	return a, nil
}

// closeAll closes the tabs, asking about unsaved ones, and then the window.
func (a *application) closeAll(sessions contract.Sessions) {
	a.shell.CloseAll(func(ok bool) {
		if !ok {
			a.shell.ThawSession()
			return
		}
		// Close removes recovery copies in the session queue. Restore runs after
		// them in it, so the next start does not bring closed documents back.
		sessions.Restore(func([]contract.Session, error) {
			a.closing = true
			a.window.Close()
		})
	})
}

// systemRecent also gives opened files to the system's recent documents
// (the jump list on Windows, Open Recent on macOS).
type systemRecent struct{ contract.Recent }

func (r systemRecent) Add(path string, isDir bool) {
	r.Recent.Add(path, isDir)
	if !isDir {
		if err := mygo.App.AddRecentDocument(path); err != nil {
			slog.Warn("system recent documents", "error", err)
		}
	}
}

// folderArgs are the arguments naming folders, which open as the workspace
// ("aurink ."). MyGo hands over only files, through OnOpenFile.
func folderArgs(args []string, wd string) []string {
	var out []string
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			continue
		}
		p := arg
		if !filepath.IsAbs(p) {
			p = filepath.Join(wd, p)
		}
		if info, err := os.Stat(p); err == nil && info.IsDir() {
			out = append(out, filepath.Clean(p))
		}
	}
	return out
}

// open opens a file the system hands over, or a folder as the workspace.
func (a *application) open(path string) {
	a.window.Update(func() {
		if !a.restored {
			a.pending = append(a.pending, path)
			return
		}
		a.openNow(path)
	})
}

func (a *application) openNow(path string) {
	if info, err := os.Stat(path); err == nil && info.IsDir() {
		a.shell.OpenFolder(path)
		return
	}
	a.shell.Open(path)
}

// buildMenu is the application menu (macOS): the registered commands and nothing else.
func buildMenu(r contract.CommandRegistry) *mygo.Menu {
	items := []*mygo.MenuItem{{Role: mygo.RoleAppMenu}}
	groups := []struct {
		label string
		ids   []contract.CommandID
	}{
		{locales.ShellOpen(), []contract.CommandID{contract.CmdFileNew, contract.CmdFileOpen, contract.CmdFileOpenFolder, contract.CmdFileSave, contract.CmdFileSaveAs, contract.CmdFileExportHTML, contract.CmdFileClose}},
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

func (a *application) ExportPath(name string, done func(string)) {
	go func() {
		path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{Parent: a.window, DefaultPath: name + ".html",
			Filters: []mygo.FileFilter{{Name: locales.ShellHtmlFiles(), Extensions: []string{"html", "htm"}}}})
		a.window.Update(func() {
			if err != nil {
				a.shell.Notify(contract.Notice{Level: contract.NoticeError, Message: err.Error()})
				path = ""
			}
			done(path)
		})
	}()
}

func filters() []mygo.FileFilter {
	return []mygo.FileFilter{{Name: locales.ShellMarkdownFiles(), Extensions: []string{"md", "markdown"}}}
}
