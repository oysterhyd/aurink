package contract

import "iter"

// BlockID 是块的稳定标识：块在编辑中存活时保持不变，用作排版缓存键与视图 Key。
type BlockID uint64

// BlockKind 是块的种类。
type BlockKind uint8

const (
	BlockRoot BlockKind = iota
	BlockParagraph
	BlockHeading
	BlockThematicBreak
	BlockCode
	BlockHTML
	BlockQuote
	BlockList
	BlockItem
	BlockTable
	BlockTableRow
	BlockTableCell
	BlockFootnoteDef
	BlockMath
	// Aurink 扩展
	BlockFrontMatter
	BlockTOC
)

// TaskState 是任务列表项的状态。
type TaskState uint8

const (
	TaskNone TaskState = iota
	TaskOpen
	TaskDone
)

// Align 是表格列对齐。
type Align uint8

const (
	AlignNone Align = iota
	AlignLeft
	AlignCenter
	AlignRight
)

// Extensions 是 CommonMark+GFM+脚注+数学之外启用的扩展语法。
type Extensions uint32

const (
	ExtFrontMatter Extensions = 1 << iota
	ExtHighlight              // ==高亮==
	ExtSupSub                 // ^上标^，以及开启 ExtTildeSub 时的 ~下标~
	ExtTildeSub               // 单波浪线为下标（否则遵循 GFM 为删除线）
	ExtEmoji                  // :shortcode:
	ExtWikiLink               // [[目标|别名]]
	ExtTOC                    // 独占段落的 [TOC]
	ExtDollarMath             // $行内公式$

	// ExtDefault 是默认启用的扩展。
	ExtDefault = ExtFrontMatter | ExtHighlight | ExtSupSub | ExtEmoji | ExtTOC | ExtDollarMath
)

// Block 是一个块。块按文档顺序存放，子块紧随父块，直到 Next。只读，不得修改。
type Block struct {
	ID     BlockID
	Kind   BlockKind
	Parent int // 父块索引，根为 -1
	Next   int // 本块及其子孙之后的索引
	// Range 是块的完整源码（含标记）。
	Range Range
	// Content 是去掉标记后的内容（标题文字、代码正文、数学 TeX、Front Matter YAML）。
	Content Range
	// Markers 是块自身的语法标记，按源码顺序：如 "## "、闭合 "##"、围栏行、"$$" 行、"---" 行。
	Markers []Range
	// Prefixes 是叶子块内每一续行的容器前缀（"> "、列表缩进），WYSIWYG 中隐藏。
	Prefixes []Range

	Level   int  // 标题级别 1–6
	Ordered bool // 列表
	Number  int  // 有序列表起始编号
	Marker  byte // '-' '*' '+' 或 '.' ')'
	Tight   bool
	Task    TaskState
	// TaskMarker 是 "[ ]" / "[x]" 的源码区间。
	TaskMarker Range
	Lang       string // 代码块语言（信息串首词）
	Info       string // 完整信息串
	Fenced     bool
	Align      []Align // 表格列对齐
	Header     bool    // 表头行
	Label      string  // 脚注标签
	// Open 表示块在文档末尾未闭合（如缺少闭合围栏）。
	Open bool
}

// InlineKind 是内联节点种类。
type InlineKind uint8

const (
	InlineText InlineKind = iota
	InlineSoftBreak
	InlineLineBreak
	InlineCode
	InlineEmphasis
	InlineStrong
	InlineStrike
	InlineLink
	InlineImage
	InlineHTML
	InlineMath
	InlineFootnoteRef
	InlineEscape // "\*"：Open 为反斜杠，Text 为 "*"
	InlineEntity // "&amp;"：Text 为 "&"
	// Aurink 扩展
	InlineHighlight
	InlineSup
	InlineSub
	InlineEmoji    // Text 为表情字符
	InlineWikiLink // Dest 为目标，子节点为别名
)

// Inline 是一个内联节点，按文档顺序存放，子节点紧随其后直到 Next。
type Inline struct {
	Kind  InlineKind
	Range Range
	Next  int
	// Open 与 Close 是定界符源码区间，如 "**"；链接的 Open 为 "["，Close 为 "](dest "title")"。
	Open, Close Range
	// Text 是解码后的字面值（Text/Code/Math/HTML/Escape/Entity/Emoji/脚注标签）。
	Text        string
	Dest, Title string
	Auto        bool // 自动链接
	Display     bool // 块级公式（$$ 或 \[ \]）
}

// Heading 是大纲中的一个标题。
type Heading struct {
	Block BlockID
	Index int
	Level int
	Text  string // 纯文本
	Slug  string // GitHub 风格锚点，重名追加 -1、-2
	Range Range
}

// LinkDef 是引用式链接定义。
type LinkDef struct {
	Label, Dest, Title string
	Range              Range
}

// AST 是某一版本文本的解析结果，不可变，任意 goroutine 可读。
type AST interface {
	Version() Version
	Len() int
	// Block 返回第 i 个块（只读）。
	Block(i int) *Block
	// Index 返回 BlockID 当前的索引。
	Index(id BlockID) (int, bool)
	Children(i int) iter.Seq[int]
	// Inlines 返回叶子块的内联节点（惰性解析）；非叶子块返回 nil。
	Inlines(i int) []Inline
	// LeafAt 返回包含 offset 的叶子块；offset 落在块间空白时返回其前一个叶子块，
	// 之前没有叶子块时返回之后第一个，文档无叶子块时返回 -1。
	LeafAt(offset int) int
	// Path 返回从根到包含 offset 的叶子块的索引路径。
	Path(offset int) []int
	// Leaves 按文档顺序遍历全部叶子块索引（视图虚拟化的行序列）。
	Leaves() iter.Seq[int]
	LeafCount() int
	Outline() []Heading
	FrontMatter() (*Block, bool)
	Footnotes() map[string]int
	LinkDefs() map[string]LinkDef
}
