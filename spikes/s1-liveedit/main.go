// Command s1-liveedit is the demo window of Spike S1. Run it with go run . for the
// IME check in REPORT.md. With -stress it drives the real window by itself, prints
// the engine's timings and exits; set MYGO_FRAME_STATS for the framework's own.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"slices"
	"strings"
	"time"

	"aurink.local/spikes/s1-liveedit/liveedit"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

const demoHead = `# Aurink 编辑引擎原型

这是 S1 的演示文档。光标所在的块显示 **Markdown 标记**，其他块隐藏标记。试试 *斜体* 和 ` + "`行内代码`" + `，然后用微软拼音输入中文。

## 在隐藏标记的块中组字

把光标点进这一段的**粗体文字**中间再组字：点击的瞬间标记出现，候选框应跟随光标。

`

func main() {
	blocks := flag.Int("blocks", 200, "generated blocks after the demo text")
	size := flag.Int("bytes", 0, "generate at least this many bytes instead of -blocks")
	stress := flag.String("stress", "", "drive the window by itself and exit: type or scroll")
	count := flag.Int("n", 300, "steps of -stress")
	flag.Parse()

	src := demoHead + liveedit.GenerateBlocks(*blocks)
	if *size > 0 {
		src = demoHead + liveedit.GenerateBytes(*size)
	}
	ed := liveedit.New(src)
	fmt.Printf("source %d bytes, %d blocks\n", len(src), ed.BlockCount())

	mygo.App.WhenReady(func() {
		w := mygo.NewWindow(mygo.WindowOptions{
			Title:   "Aurink S1 · Live Markdown",
			Width:   1100,
			Height:  800,
			Content: ui.View(func(c *ui.Context) { ed.View(c) }),
		})
		if *stress != "" {
			go drive(w, ed, *stress, *count)
		}
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

// drive posts one step per refresh through Window.Update, the way real events
// reach the main thread, and times each from the step to the end of the frame's
// painting: edit, parse, repartition, reshape, view, paint commands. Presenting is
// in MYGO_FRAME_STATS, not here.
func drive(w *mygo.Window, ed *liveedit.Editor, mode string, n int) {
	time.Sleep(time.Second)
	var times []time.Duration
	var t0 time.Time
	ed.OnPainted = func() {
		if !t0.IsZero() {
			times = append(times, time.Since(t0))
			t0 = time.Time{}
		}
	}
	w.Update(func() {
		s := ed.Source()
		mid := strings.Index(s[len(s)/2:], "\n\n") + len(s)/2 + 2
		ed.SetCaret(mid)
		ed.ResetStats()
	})
	time.Sleep(100 * time.Millisecond)
	for i := range n {
		w.Update(func() {
			t0 = time.Now()
			switch {
			case mode == "scroll":
				ed.ScrollBy(120)
			case i%60 == 59:
				ed.ReplaceText(nil, "\n\n")
			default:
				ed.ReplaceText(nil, string(rune('a'+i%26)))
			}
		})
		time.Sleep(16 * time.Millisecond)
	}
	time.Sleep(500 * time.Millisecond)
	done := make(chan struct{})
	w.Update(func() {
		s := ed.Stats
		sorted := slices.Clone(times)
		slices.Sort(sorted)
		p := func(q float64) time.Duration { return sorted[min(int(float64(len(sorted))*q), len(sorted)-1)] }
		fmt.Printf("stress %s: steps=%d timed=%d p50=%v p95=%v max=%v frames=%d shapes=%d maxFrameShapes=%d\n",
			mode, n, len(sorted), p(.5), p(.95), sorted[len(sorted)-1], s.Frames, s.Shapes, s.MaxFrameShapes)
		close(done)
	})
	<-done
	os.Exit(0)
}
