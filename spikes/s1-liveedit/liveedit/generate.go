package liveedit

import (
	"fmt"
	"strings"
)

var sentences = []string{
	"Markdown 文件就是你的文件：打开、编辑、保存，都不会改动你没碰过的任何一个字节。",
	"The **source string** is the only truth, and every edit is a *byte replacement* in it.",
	"光标进入一段文字时，它的**标记**才淡淡地出现；*离开之后*，页面又回到干净的排版。",
	"Layouts are cached per block, so a keystroke shapes `one block` and scrolling shapes only what comes into view.",
	"输入法的预编辑不写入源码，只是显示层的 `虚拟片段`，提交时一次写入。",
	"A long paragraph wraps over several lines, which is what moving up and down across a block has to survive with its *sticky x*.",
	"中英文 mixed 排版 with **粗体 bold** and *斜体 italic* and `code 代码` in one line.",
}

// GenerateBlocks returns a deterministic document of n blocks: a heading every
// 12 blocks, a first-level one every 60, and paragraphs of one to three
// sentences mixing Latin, CJK and inline syntax.
func GenerateBlocks(n int) string {
	var s strings.Builder
	for i := range n {
		writeBlock(&s, i)
	}
	return strings.TrimSuffix(s.String(), "\n") // one terminator, as files end
}

// GenerateBytes returns such a document of at least size bytes.
func GenerateBytes(size int) string {
	var s strings.Builder
	for i := 0; s.Len() < size; i++ {
		writeBlock(&s, i)
	}
	return strings.TrimSuffix(s.String(), "\n")
}

func writeBlock(s *strings.Builder, i int) {
	switch {
	case i%60 == 0:
		fmt.Fprintf(s, "# 第 %d 章 Chapter\n\n", i/60+1)
	case i%12 == 0:
		fmt.Fprintf(s, "## Section %d 小节\n\n", i)
	default:
		for k := 0; k <= i%3; k++ {
			if k > 0 {
				s.WriteByte(' ')
			}
			s.WriteString(sentences[(i+k*3)%len(sentences)])
		}
		s.WriteString("\n\n")
	}
}
