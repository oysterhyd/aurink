// Package editorapi 是视图层内部契约：shell、命令面板、查找栏等前端模块通过它
// 操作编辑器（views/liveedit 实现）。与 contract 同步冻结。
//
// 全部方法只能在主线程（视图构建或输入回调中）调用。
package editorapi

import "aurink.local/spikes/contract-rc/contract"

// Mode 是编辑器的显示模式。
type Mode uint8

const (
	ModeLive   Mode = iota // 所见即所得
	ModeSource             // 源码模式（ui/editor）
	ModeRead               // 阅读模式（只读、全部隐藏标记）
)

// ScrollAlign 是滚动定位方式。
type ScrollAlign uint8

const (
	AlignNearest ScrollAlign = iota // 尽量少滚动
	AlignTop                        // 置于视口顶部（留出标题栏高度）
	AlignCenter
)

// Rect 是编辑器元素坐标系中的矩形（DIP）。
type Rect struct{ X, Y, W, H float32 }

// Controller 是一个编辑器实例对外暴露的操作。
type Controller interface {
	Session() contract.Session
	Focus()
	Focused() bool

	Mode() Mode
	SetMode(m Mode)
	FocusMode() bool
	SetFocusMode(on bool)
	Typewriter() bool
	SetTypewriter(on bool)

	// RevealOffset 滚动使源码偏移可见并把光标放在那里（大纲跳转、搜索结果）。
	RevealOffset(off int, align ScrollAlign)
	// SetFindHighlights 设置查找高亮；current 为当前项下标，-1 表示无。
	SetFindHighlights(ranges []contract.Range, current int)
	ClearFindHighlights()

	// VisibleHeading 返回视口顶部所在章节的标题块（大纲 scroll spy）。
	VisibleHeading() (contract.BlockID, bool)
	// ScrollFraction 返回阅读进度 0–1。
	ScrollFraction() float32
	// ScrollOffset 返回滚动偏移（DIP），用于滚动联动标题。
	ScrollOffset() float32
	// CaretBlock 返回光标所在块的序号（从 1 开始）与块总数，用于状态胶囊。
	CaretBlock() (index, total int)
	// SelectionRect 返回主选区（或光标）在窗口坐标中的包围盒，用于浮层定位。
	SelectionRect() (Rect, bool)

	// RunEdit 以当前选区执行编辑命令（菜单、命令面板、格式条调用）。
	RunEdit(cmd contract.EditCommand) error
	// Formatting 返回当前选区的格式状态。
	Formatting() contract.FormatState
}

// Registry 跟踪打开的编辑器实例（标签页）。
type Registry interface {
	Active() (Controller, bool)
	Get(id contract.DocID) (Controller, bool)
}
