// Command windowcheck drives the real interface in a real window on this machine: it
// times keystrokes to the painted frame and saves the screens compared with the design
// baselines (QA-1.3). It opens a window for about a minute and writes only under -out.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

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
	"aurink/resources"
	"aurink/tests/corpus"
)

type dispatcher struct{ w *mygo.Window }

func (d *dispatcher) Post(fn func()) {
	if d.w != nil {
		d.w.Update(fn)
	}
}

type platform struct{ w *mygo.Window }

// buildStart is when the window's last frame began building: a frame's work
// runs from it to the end of the paint, without the wait for the display.
var buildStart time.Time

func (platform) OpenFile(done func(string))                     { done("") }
func (platform) OpenFolder(done func(string))                   { done("") }
func (platform) ConfirmTrash(_ string, done func(bool))         { done(true) }
func (platform) SavePath(_ string, done func(string))           { done("") }
func (platform) ExportPath(_ string, done func(string))         { done("") }
func (platform) ConfirmClose(_ string, done func(shell.Choice)) { done(shell.ChoiceDiscard) }
func (p platform) Minimize()                                    { p.w.Minimize() }
func (p platform) ToggleMaximize()                              { p.w.ToggleMaximize() }
func (p platform) IsMaximized() bool                            { return p.w.IsMaximized() }
func (p platform) Close()                                       { p.w.Close() }

const demo = `# Aurink 是什么

轻量、即开即写的 Markdown 编辑器。**Markdown 文件就是你的文件**：打开、编辑、保存，都不会改动你没碰过的任何一个字节。规划细节见 [可行性报告](https://example.com)。[^1]

## 写作时的感受

光标进入一段文字时，它的 ` + "`标记`" + ` 才淡淡地出现；*离开之后*，页面又回到<mark>干净的排版</mark>。这只是显示上的变化，源码本身从未被改写；按 <kbd>Ctrl</kbd> <kbd>/</kbd> 可以看源码。

> 内容优先：编辑区之外的一切默认安静，需要时才出现。

- 键盘即一切：所有功能都能通过快捷键到达。
- 动效服务于理解：说明东西从哪来、到哪去。
- 无损与可信：保存状态永远可见、可恢复。

### 上线前要确认的事

- [x] 冻结接口契约 contract-v1.0
- [x] 通过微软拼音真机候选框验证
- [ ] 点击复选框时，只改动源码中的 1 个字节

` + "```go\nfunc main() {\n\tfmt.Println(\"hello\") // 代码\n}\n```" + `

| 项 | 值 |
|---|---|
| 行距 | 1.75 |

[^1]: 见 docs/01-feasibility.md。
`

func main() {
	out := flag.String("out", "reports/parity/screens", "where the screens go")
	design := flag.Bool("design", false, "save only the design system screen")
	locale := flag.String("locale", "", "show the interface in this language, such as zh-Hans")
	flag.Parse()
	if *locale != "" {
		// For this run only: App.SetLocale would keep the language for later runs.
		os.Setenv("MYGO_LOCALE", *locale)
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		log.Fatal(err)
	}
	tmp, err := os.MkdirTemp("", "aurink-windowcheck")
	if err != nil {
		log.Fatal(err)
	}
	mygo.App.WhenReady(func() {
		if err := resources.RegisterFonts(); err != nil {
			log.Fatal(err)
		}
		if *design {
			designSystem(*out)
			return
		}
		d := &dispatcher{}
		files := fsstore.New()
		store, err := settings.New(filepath.Join(tmp, "settings.json"), files, d, nil)
		if err != nil {
			log.Fatal(err)
		}
		if err := store.Update(func(v *contract.Settings) { v.Appearance.Theme = "light" }); err != nil {
			log.Fatal(err)
		}
		base := store.Get()
		base.Editor.Exts = contract.ExtFrontMatter
		// Autosave stays out of the measured frames and off the disk.
		base.Files.AutosaveDelayMs = 3600000
		sessions := docsession.New(files, document.Factory{}, d, filepath.Join(tmp, "recovery"), docsession.Options{Settings: &base})
		reg := commands.New()
		reg.SetDefaults(commands.DefaultBindings())
		var w *mygo.Window
		var s *shell.Shell
		p := &platform{}
		s = shell.New(shell.Options{Finder: search.NewFinder(), Sessions: sessions, Settings: store, Projector: projection.New(), Editing: editing.New(),
			Commands: reg, Dispatcher: d, Platform: p, WindowButtons: true, Exts: contract.ExtFrontMatter, Fuzzy: search.NewFuzzy(),
			Workspace: workspace.New(watch.New(), trash.New(), d, workspace.Options{}), Search: search.NewWorkspaceSearch()})
		if err := reg.Register(s.Commands()...); err != nil {
			log.Fatal(err)
		}
		w = mygo.NewWindow(mygo.WindowOptions{Title: "Aurink", Width: 1280, Height: 800, TitleBarStyle: mygo.TitleBarHidden,
			HideWindowButtons: true, Content: ui.View(func(c *ui.Context) { buildStart = time.Now(); s.View(c) })})
		d.w, p.w = w, w
		go script(w, s, reg, store, tmp, *out)
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

// on runs fn on the window's thread and waits for it.
func on(w *mygo.Window, fn func()) {
	done := make(chan struct{})
	w.Update(func() { fn(); close(done) })
	<-done
}

func script(w *mygo.Window, s *shell.Shell, reg *commands.Registry, store *settings.Store, tmp, out string) {
	shot := func(name string) {
		time.Sleep(900 * time.Millisecond)
		data, err := w.CapturePage()
		if err != nil {
			log.Fatal(name, ": ", err)
		}
		if err := os.WriteFile(filepath.Join(out, name+".png"), data, 0o644); err != nil {
			log.Fatal(err)
		}
		fmt.Println("saved", name)
	}
	run := func(id contract.CommandID) {
		on(w, func() {
			if err := reg.Run(id); err != nil {
				log.Fatal(err)
			}
		})
	}
	open := func(name, text string) {
		path := filepath.Join(tmp, name)
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			log.Fatal(err)
		}
		on(w, func() { s.Open(path) })
		time.Sleep(500 * time.Millisecond)
	}
	shot("Welcome")
	open("README.md", demo)
	on(w, func() { s.ActiveSession().Model().SetSelection(contract.Caret(60)) })
	shot("Main")
	run(contract.CmdViewThemeDark)
	shot("Dark")
	run(contract.CmdViewThemeLight)
	run(contract.CmdViewSource)
	shot("Source")
	run(contract.CmdViewLive)
	run(contract.CmdNavPalette)
	time.Sleep(1600 * time.Millisecond) // past the one-time glow
	shot("Palette")
	on(w, func() { s.Palette().SetQuery(">切换") })
	shot("PaletteQuery")
	on(w, func() { s.Palette().Close() })
	time.Sleep(300 * time.Millisecond)
	var shown []contract.NoticeHandle
	on(w, func() {
		shown = append(shown, s.Notify(contract.Notice{Level: contract.NoticeInfo, Message: "已保存到磁盘", Detail: "README.md", Sticky: true}))
	})
	on(w, func() {
		shown = append(shown, s.Notify(contract.Notice{Level: contract.NoticeError, Message: "保存失败", Detail: "磁盘已满", Sticky: true}))
	})
	shot("Notify")
	on(w, func() {
		for _, h := range shown {
			h.Dismiss()
		}
	})
	on(w, func() {
		if err := store.Update(func(v *contract.Settings) { v.Appearance.ReduceTransparency = true }); err != nil {
			log.Fatal(err)
		}
	})
	run(contract.CmdViewToggleSidebar)
	shot("LowPower")
	on(w, func() {
		if err := store.Update(func(v *contract.Settings) { v.Appearance.ReduceTransparency = false }); err != nil {
			log.Fatal(err)
		}
	})
	// The sidebar over a folder, as in the baselines' sidebar (FE-2.3).
	notes := filepath.Join(tmp, "aurink-notes")
	for _, f := range []string{"docs/笔记.md", "docs/路线图.md", "docs/会议记录.md", "assets/logo.png", "README.md"} {
		p := filepath.Join(notes, filepath.FromSlash(f))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			log.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(demo), 0o644); err != nil {
			log.Fatal(err)
		}
	}
	on(w, func() { s.OpenFolder(notes) })
	time.Sleep(500 * time.Millisecond)
	on(w, func() { s.ExpandFolder(filepath.Join(notes, "docs")) })
	open("aurink-notes/README.md", demo)
	shot("Files")
	run(contract.CmdViewOutline)
	shot("Outline")
	run(contract.CmdNavSearchWorkspace)
	on(w, func() { s.SetSearch("Markdown") })
	time.Sleep(800 * time.Millisecond)
	shot("Search")
	run(contract.CmdViewFiles)
	on(w, func() { s.GoTo(strings.Index(demo, "| 项") + 2) })
	shot("Table")
	on(w, func() { s.GoTo(strings.Index(demo, "可行性报告") + 3) })
	shot("Link")
	on(w, func() { s.GoTo(0) })
	run(contract.CmdEditReplace)
	on(w, func() { s.SetFind("Markdown") })
	shot("Find")
	run(contract.CmdViewSource)
	run(contract.CmdEditReplace)
	shot("SourceFind")
	run(contract.CmdViewLive)
	on(w, s.CloseFind)
	// A conflict: an edit, a change on disk, a save.
	on(w, func() {
		m := s.ActiveSession().Model()
		eol := strings.IndexByte(demo, '\n')
		if _, err := m.Apply(contract.Transaction{Edits: []contract.TextEdit{{Range: contract.Range{Start: eol, End: eol}, Text: "（草稿）"}},
			Origin: contract.OriginUser}); err != nil {
			log.Fatal(err)
		}
	})
	if err := os.WriteFile(s.ActiveSession().State().Path, []byte(demo+"\n改动\n"), 0o644); err != nil {
		log.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	run(contract.CmdFileSave)
	time.Sleep(500 * time.Millisecond)
	shot("Conflict")
	for _, size := range []int{200 << 10, 1 << 20} {
		open(fmt.Sprintf("stress-%d.md", size), corpus.Generate(size, 1))
		stress(w, s, size)
	}
	on(w, func() { w.Destroy(); mygo.App.Quit() })
}

// stress times 500 keystrokes from the edit to the end of the frame that paints
// it, and that frame's own work from its build to its paint (07 §7).
func stress(w *mygo.Window, s *shell.Shell, size int) {
	const n = 500
	times := make([]time.Duration, 0, n)
	works := make([]time.Duration, 0, n)
	painted := make(chan time.Duration, 1)
	on(w, func() {
		e := s.ActiveLive()
		e.OnPainted = func() {
			select {
			case painted <- time.Since(buildStart):
			default:
			}
		}
		e.Model().SetSelection(contract.Caret(size / 2))
	})
	time.Sleep(300 * time.Millisecond)
	for range n {
		var t0 time.Time
		select {
		case <-painted:
		default:
		}
		on(w, func() {
			t0 = time.Now()
			m := s.ActiveSession().Model()
			at := m.Selection().Primary().Head
			if _, err := m.Apply(contract.Transaction{Edits: []contract.TextEdit{{Range: contract.Range{Start: at, End: at}, Text: "x"}}, Origin: contract.OriginUser}); err != nil {
				log.Fatal(err)
			}
		})
		select {
		case work := <-painted:
			times = append(times, time.Since(t0))
			works = append(works, work)
		case <-time.After(2 * time.Second):
			log.Fatal("no frame painted the edit")
		}
	}
	slices.Sort(times)
	slices.Sort(works)
	fmt.Printf("window %d KB: edit to painted p50 %v p95 %v max %v; frame work p50 %v p95 %v max %v\n", size>>10,
		times[n/2], times[n*95/100], times[n-1], works[n/2], works[n*95/100], works[n-1])
}
