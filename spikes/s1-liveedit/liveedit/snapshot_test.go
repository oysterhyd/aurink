package liveedit

import (
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSnapshot writes what the editor paints to the directory S1_SNAPSHOT names,
// for looking at; it asserts nothing and is skipped otherwise.
func TestSnapshot(t *testing.T) {
	dir := os.Getenv("S1_SNAPSHOT")
	if dir == "" {
		t.Skip("S1_SNAPSHOT not set")
	}
	src := "# Aurink 是什么\n\n轻量、即开即写的 Markdown 编辑器。**Markdown 文件就是你的文件**：打开、编辑、保存，都不会改动你没碰过的任何一个字节。\n\n## 写作时的感受\n\n光标进入一段文字时，它的**标记**才淡淡地出现；*离开之后*，页面又回到干净的排版，`code` 也一样。\n\nA Latin paragraph with **bold**, *italic* and `code`.\n\n" + GenerateBlocks(60)
	h := open(t, src)
	h.caretAt(strings.Index(src, "文件就是"))
	save(t, h, filepath.Join(dir, "revealed.png"))
	h.tt.Compose("wenjian", 7)
	save(t, h, filepath.Join(dir, "composing.png"))
	h.tt.Compose("", 0)
	a, z := strings.Index(src, "即写"), strings.Index(src, "干净")
	h.e.setSelection(a, z)
	h.tt.Frame()
	save(t, h, filepath.Join(dir, "selection.png"))
}

func save(t *testing.T, h *harness, path string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, h.tt.Image()); err != nil {
		t.Fatal(err)
	}
}
