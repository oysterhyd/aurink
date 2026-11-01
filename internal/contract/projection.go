package contract

// RevealMode 决定哪些语法标记以源码形式显示。
type RevealMode uint8

const (
	RevealInline RevealMode = iota // 光标所在内联节点（及其祖先）显示标记：默认
	RevealBlock                    // 光标所在块全部标记显示
	RevealAlways                   // 全部显示
	RevealNone                     // 阅读模式：全部隐藏
)

// SoftBreakMode 决定段落内单个换行的显示。
type SoftBreakMode uint8

const (
	SoftBreakNewline SoftBreakMode = iota // 显示为换行（默认）
	SoftBreakSpace                        // 显示为空格（CommonMark 语义）
)

// Reveal 是投影一个块时的显隐上下文。
type Reveal struct {
	Mode RevealMode
	// Caret 是光标源码偏移；< 0 表示光标不在本块。
	Caret int
	// Selection 非空时，选区覆盖的节点全部显示标记。
	Selection Range
	SoftBreak SoftBreakMode
	// ObjectLangs 是整块渲染为对象的代码语言，如 "mermaid"。
	ObjectLangs []string
	Exts        Extensions
}

// SegKind 是显示片段的种类。
type SegKind uint8

const (
	SegText     SegKind = iota // 原样显示的源码
	SegHidden                  // 不显示的源码（未激活标记、容器前缀）
	SegMarker                  // 显示为淡色标记的源码（已显隐）
	SegReplaced                // 源码显示为 Display 替代文本（实体、转义、软换行、emoji）
	SegObject                  // 源码显示为一个内联对象 U+FFFC（图片、公式、复选框、脚注上标）
)

// Style 是片段的内联样式位。
type Style uint32

const (
	StyleStrong Style = 1 << iota
	StyleEmphasis
	StyleStrike
	StyleCode
	StyleLink
	StyleHighlight
	StyleSup
	StyleSub
	StyleMath
	StyleHTML
	StyleFootnoteRef
	StyleWikiLink
	StyleMarker
	StyleAutolink
	StyleUnderline // <u>
	StyleKbd       // <kbd>
)

// ObjectKind 是内联/块对象的种类。
type ObjectKind uint8

const (
	ObjNone ObjectKind = iota
	ObjImage
	ObjMath
	ObjCheckbox
	ObjFootnoteRef
	ObjDiagram // 整块渲染的代码块（mermaid 等）
	ObjTOC
	ObjFrontMatter
)

// ObjectRef 描述对象的内容，视图据此加载或请求渲染。
type ObjectRef struct {
	Kind    ObjectKind
	Source  string // 图片地址 / TeX / 图表源 / 脚注标签
	Alt     string
	Title   string
	Lang    string // ObjDiagram 的语言
	Display bool   // 块级对象（独占一行）
	Checked bool   // ObjCheckbox
}

// Segment 是投影的一个显示片段，覆盖源码 Src。一个块的片段按源码顺序覆盖
// 块的完整 Range（含前缀与标记），首尾相接、无缝无叠。
type Segment struct {
	Src  Range
	Kind SegKind
	// Display：SegText/SegMarker 为源码子串（零拷贝）；SegReplaced 为替代文本；
	// SegObject 为 "￼"；SegHidden 为空串。
	Display string
	Style   Style
	Link    string // StyleLink/StyleWikiLink 的目标
	Object  *ObjectRef
}

// ContainerDecor 是叶子块外层容器的装饰（引用竖条、列表符号、脚注标签）。
type ContainerDecor struct {
	Kind    BlockKind // BlockQuote、BlockItem、BlockFootnoteDef
	Block   BlockID
	Depth   int
	Ordered bool
	Number  int // 本项的显示编号
	Marker  byte
	Task    TaskState
	TaskSrc Range // 任务标记源码，点击切换
	// First 表示本叶子块是该容器的第一个叶子（列表符号只画在第一个叶子上）。
	First bool
	Label string
}

// BlockProjection 是一个叶子块在给定显隐上下文下的显示投影。
type BlockProjection struct {
	Block      BlockID
	Index      int
	Kind       BlockKind
	Level      int
	Lang       string
	Segments   []Segment
	Containers []ContainerDecor
	Revealed   bool
	// Sig 是投影签名：内容或显隐变化时改变，作为排版缓存键的一部分。
	Sig uint64
}

// Affinity 决定映射落在边界时偏向哪一侧。
type Affinity int8

const (
	Upstream   Affinity = -1
	Downstream Affinity = 1
)

// Projector 生成投影并在源码偏移与显示索引之间映射（core/projection 实现，纯函数）。
type Projector interface {
	Project(ast AST, text Text, leaf int, r Reveal) BlockProjection
	// DisplayText 返回片段 Display 的串接。
	DisplayText(p *BlockProjection) string
	// ToDisplay 把源码偏移映射为显示文本中的 UTF-16 索引。
	ToDisplay(p *BlockProjection, src int, aff Affinity) int
	// ToSource 把显示文本 UTF-16 索引映射回源码偏移。
	ToSource(p *BlockProjection, disp int, aff Affinity) int
	// Snap 把源码偏移吸附到合法光标位置（不在隐藏片段、对象或字素内部）。
	Snap(p *BlockProjection, src int, aff Affinity) int
}
