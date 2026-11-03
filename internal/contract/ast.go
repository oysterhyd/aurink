package contract

import "iter"

// BlockID 是块的稳定标识；语法重解析后仍存活的块继承 ID，用作视图 Key。
// 源码未与 Change 相交且种类、映射后的完整区间相同的块，必须继承 ID。
// 对其余候选，种类和映射后的起点相同、旧起点未被删除，且旧 ID 尚未占用时，按文档
// 顺序继承 ID；必须先为完整区间相同的存活块保留身份。
// 合并、拆分或被容器吞并而消失的块不保留独立身份；新 ID 永不复用。
// ID 不表示内容修订；内联依赖改变时须另行失效投影与排版缓存。
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
	// BlockBlank 是没有内容的叶子，让光标能停在空行上（产生位置见 04 §4.2.9）。
	BlockBlank
	// BlockLinkDef 是链接引用定义所在的连续行。解析器不为它们产生块；按源码淡色显示。
	BlockLinkDef
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

	// ExtDefault 是默认启用的扩展：全部（用户 2026-11-02，08 §2.2 U1）。
	ExtDefault = ExtFrontMatter | ExtHighlight | ExtSupSub | ExtTildeSub | ExtEmoji | ExtWikiLink | ExtTOC | ExtDollarMath
)

// BlockLine 描述解析器的一条内容行，不包含行终止符。
type BlockLine struct {
	// Source 是去掉该行前缀与块标记后的原始字节区间。
	Source Range
	// Padding 是部分消耗 Tab 剩下的空格数，范围 0..3；不改写源码。
	Padding uint8
}

// Block 是一个块。块按文档顺序存放，子块紧随父块，直到 Next。只读，不得修改。
type Block struct {
	ID     BlockID
	Kind   BlockKind
	Parent int // 父块索引，根为 -1
	Next   int // 本块及其子孙之后的索引
	// Range 是块的完整源码（含标记）。
	Range Range
	// Content 是内容源码的包围区间（标题文字、代码正文、数学 TeX、Front Matter YAML）；
	// 多行时可含续行前缀和原始换行。精确的逐行内容通过 Lines 取得。
	Content Range
	// Markers 是块自身的语法标记，按源码顺序：如 "## "、闭合 "##"、围栏行、"$$" 行、"---" 行。
	Markers []Range
	// Prefixes 是 Range 内各内容行被解析器剥离的行首字节（容器前缀、结构缩进、内容前空白），
	// WYSIWYG 中隐藏。首行的容器语法归 Lead。
	Prefixes []Range
	// Lines 按源码顺序列出内容行；无内容行的块为 nil。第 k 行的解析器内容 =
	// Padding 个空格 + 源码[Source]。转义和实体由内联阶段解码。
	Lines []BlockLine

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
	// Lead 只用于叶子块：上一个叶子的 Trailer 之后、本叶子 Range 之前的源码，即首行上的
	// 容器语法与缩进（"> "、"- [ ] "、表格的 "| "、表格分隔行）。Lead.End == Range.Start。
	Lead Range
	// Trailer 只用于叶子块：本叶子是所在行最后一个叶子时，为该行 Range.End 之后的剩余字节
	// （如表格行尾的 " |"）、行终止符，以及至多一个紧随的分隔行（去掉容器前缀后只剩空白的行）；
	// 否则为空区间（如表格中非末列的单元格）。Trailer.Start == Range.End。
	Trailer Range
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
	// LeafAt 返回 [Lead.Start, Trailer.End) 包含 offset 的叶子块；offset 等于源码长度时
	// 返回最后一个叶子。
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
