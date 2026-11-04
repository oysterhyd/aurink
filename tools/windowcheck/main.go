// Command windowcheck drives the real interface in a real window on this machine: it
// times keystrokes to the painted frame and saves the screens compared with the design
// baselines (QA-1.3). It opens a window for about a minute and writes only under -out.
package main

import (
	"context"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

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
	"aurink/resources"
	renderpage "aurink/resources/render"
	"aurink/tests/corpus"
)

type dispatcher struct{ w *mygo.Window }

func (d *dispatcher) Post(fn func()) {
	if d.w != nil {
		d.w.Update(fn)
	}
}

type platform struct{ w *mygo.Window }

// slowStore reads files whose name starts with "slow-" a second and a half
// late, so the document skeleton shows long enough to be captured.
type slowStore struct{ contract.FileStore }

func (s slowStore) Read(path string, opts contract.ReadOptions) (contract.FileContent, error) {
	if strings.HasPrefix(filepath.Base(path), "slow-") {
		time.Sleep(1500 * time.Millisecond)
	}
	return s.FileStore.Read(path, opts)
}

// lateRenderer is the render service, set once the window exists (as in app).
type lateRenderer struct {
	mu sync.Mutex
	r  contract.Renderer
}

func (l *lateRenderer) set(r contract.Renderer) { l.mu.Lock(); l.r = r; l.mu.Unlock() }

func (l *lateRenderer) get() contract.Renderer { l.mu.Lock(); defer l.mu.Unlock(); return l.r }

func (l *lateRenderer) Available() bool { r := l.get(); return r != nil && r.Available() }

func (l *lateRenderer) Render(ctx context.Context, req contract.RenderRequest) (contract.RenderResult, error) {
	r := l.get()
	if r == nil {
		return contract.RenderResult{}, contract.E(contract.CodeUnavailable, "windowcheck.Render", nil)
	}
	return r.Render(ctx, req)
}

// cover writes a 640×360 gradient PNG for the lightbox screen.
func cover(path string) {
	img := image.NewRGBA(image.Rect(0, 0, 640, 360))
	for y := range 360 {
		for x := range 640 {
			img.Set(x, y, color.RGBA{uint8(60 + x*120/640), uint8(90 + y*100/360), 200, 255})
		}
	}
	f, err := os.Create(path)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		log.Fatal(err)
	}
}

// rendered is a document of math and a diagram, for the render screen.
const rendered = `# 公式与图表

质能方程 $E = mc^2$ 写在行内。

$$
\int_0^1 x^2\,dx = \frac{1}{3}
$$

` + "```mermaid\ngraph LR\n  A[打开] --> B[编辑] --> C[保存]\n```\n"

// buildStart is when the window's last frame began building: a frame's work
// runs from it to the end of the paint, without the wait for the display.
var buildStart time.Time

func (platform) OpenFile(done func(string))                     { done("") }
func (platform) OpenFolder(done func(string))                   { done("") }
func (platform) ConfirmTrash(_ string, done func(bool))         { done(true) }
func (platform) SavePath(_ string, done func(string))           { done("") }
func (platform) ExportPath(_, _ string, done func(string))      { done("") }
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
		files := slowStore{fsstore.New()}
		store, err := settings.New(filepath.Join(tmp, "settings.json"), files, d, nil)
		if err != nil {
			log.Fatal(err)
		}
		if err := store.Update(func(v *contract.Settings) { v.Appearance.Theme = "light" }); err != nil {
			log.Fatal(err)
		}
		base := store.Get()
		// All extensions, as the app has them by default (Phase 3).
		base.Editor.Exts = contract.ExtDefault
		// Autosave stays out of the measured frames and off the disk.
		base.Files.AutosaveDelayMs = 3600000
		sessions := docsession.New(files, document.Factory{}, d, filepath.Join(tmp, "recovery"), docsession.Options{Settings: &base})
		reg := commands.New()
		reg.SetDefaults(commands.DefaultBindings())
		var w *mygo.Window
		var s *shell.Shell
		renderer := &lateRenderer{}
		p := &platform{}
		s = shell.New(shell.Options{Finder: search.NewFinder(), Sessions: sessions, Settings: store, Projector: projection.New(), Editing: editing.New(),
			Commands: reg, Dispatcher: d, Platform: p, WindowButtons: true, Exts: contract.ExtDefault, ObjectLangs: []string{"mermaid"}, Renderer: renderer, Fuzzy: search.NewFuzzy(),
			Workspace: workspace.New(watch.New(), trash.New(), d, workspace.Options{}), Search: search.NewWorkspaceSearch()})
		if err := reg.Register(s.Commands()...); err != nil {
			log.Fatal(err)
		}
		w = mygo.NewWindow(mygo.WindowOptions{Title: "Aurink", Width: 1280, Height: 800, TitleBarStyle: mygo.TitleBarHidden,
			HideWindowButtons: true, Content: ui.View(func(c *ui.Context) { buildStart = time.Now(); s.View(c) })})
		d.w, p.w = w, w
		renderer.set(render.New(webviewhost.New(w), render.Options{Page: renderpage.Page(), CacheDir: filepath.Join(tmp, "render-cache")}))
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
	phase3(w, s, run, open, shot, tmp)
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

// phase3 saves the screens of the Phase 3 baselines: reading, focus, settings,
// the format bar, the slash menu, the lightbox, the document skeleton, and
// rendered math and diagrams.
func phase3(w *mygo.Window, s *shell.Shell, run func(contract.CommandID), open func(name, text string), shot func(string), tmp string) {
	on(w, func() { s.GoTo(strings.Index(demo, "## 写作")) })
	run(contract.CmdViewRead)
	time.Sleep(600 * time.Millisecond)
	shot("Reading")
	run(contract.CmdViewRead)
	time.Sleep(600 * time.Millisecond)
	run(contract.CmdViewFocus)
	on(w, func() { s.GoTo(strings.Index(demo, "光标进入") + 3) })
	shot("Focus")
	run(contract.CmdViewFocus)
	run(contract.CmdAppSettings)
	shot("Settings")
	on(w, s.CloseSettings)
	time.Sleep(300 * time.Millisecond)
	on(w, func() {
		e := s.ActiveLive()
		at := strings.Index(demo, "Markdown 文件就是你的文件")
		e.Model().SetSelection(contract.Selection{Ranges: []contract.SelRange{{Anchor: at, Head: at + len("Markdown 文件就是你的文件")}}})
		e.Focus()
	})
	shot("Format")
	open("slash.md", "# 周会记录\n\n本周完成了导出。\n\n")
	on(w, func() {
		e := s.ActiveLive()
		e.Model().SetSelection(contract.Caret(e.Model().Text().Len()))
		e.Focus()
		e.ReplaceText(nil, "/")
	})
	shot("Slash")
	on(w, func() { s.ActiveLive().ReplaceText(nil, " ") })
	cover(filepath.Join(tmp, "cover.png"))
	open("image.md", "# 封面\n\n![封面图](cover.png)\n")
	time.Sleep(700 * time.Millisecond)
	on(w, func() {
		e := s.ActiveLive()
		for r := range e.RowCount() {
			if e.OpenLightbox(r) {
				return
			}
		}
		log.Fatal("no image to open in the lightbox")
	})
	shot("Lightbox")
	open("render.md", rendered)
	time.Sleep(2500 * time.Millisecond) // the render service starts
	shot("Render")
	if err := os.WriteFile(filepath.Join(tmp, "slow-doc.md"), []byte(demo), 0o644); err != nil {
		log.Fatal(err)
	}
	on(w, func() { s.Open(filepath.Join(tmp, "slow-doc.md")) })
	time.Sleep(100 * time.Millisecond) // shot waits 0.9 s more: past 100 ms, before 1.5 s
	shot("Loading")
	time.Sleep(time.Second)
}
