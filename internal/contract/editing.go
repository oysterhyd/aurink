package contract

// IntentKind 是键入意图的种类。
type IntentKind uint8

const (
	IntentInsertText IntentKind = iota + 1
	IntentNewline               // Enter
	IntentLineBreak             // Shift+Enter：硬换行
	IntentDeleteBackward
	IntentDeleteForward
	IntentIndent  // Tab
	IntentOutdent // Shift+Tab
	IntentPaste
)

// Granularity 是删除的粒度。
type Granularity uint8

const (
	GranChar Granularity = iota // 字素
	GranWord
	GranLine
)

// PastePayload 是剪贴板内容的各种表示（视图读取后交给编辑逻辑）。
type PastePayload struct {
	Plain    string
	HTML     string
	RTF      string
	Markdown string
	// AsPlain 表示"粘贴为纯文本"。
	AsPlain bool
}

// Intent 是一次键入意图。
type Intent struct {
	Kind  IntentKind
	Text  string
	Gran  Granularity
	Paste *PastePayload
}

// EditPrefs 是影响编辑逻辑的偏好。
type EditPrefs struct {
	BulletMarker  byte   // '-'
	StrongDelim   string // "**"
	EmphasisDelim string // "*"
	FenceDelim    string // "```"
	AutoPair      bool
	TabSize       int
	Exts          Extensions
}

// EditContext 是编辑逻辑的全部输入（纯函数，无副作用）。
type EditContext struct {
	Text      Text
	AST       AST
	Selection Selection
	Meta      DocMeta
	Prefs     EditPrefs
}

// EditCommand 是一条格式/结构命令及其参数。
type EditCommand struct {
	ID    CommandID
	Level int    // 标题级别、缩进层数
	Text  string // 代码语言、链接文字
	URL   string
	Rows  int
	Cols  int
	Align Align
	Paths []string // 已导入资源的相对路径
}

// TriState 是选区内某格式的状态。
type TriState uint8

const (
	Off TriState = iota
	On
	Mixed
)

// ListKind 是列表种类。
type ListKind uint8

const (
	ListNone ListKind = iota
	ListBullet
	ListOrdered
	ListTask
)

// FormatState 描述选区的格式，用于菜单勾选与格式条高亮。
type FormatState struct {
	Strong, Emphasis, Strike, Code, Highlight, Link TriState
	LinkDest                                        string
	Block                                           BlockKind
	Level                                           int
	List                                            ListKind
	InTable, InCode                                 bool
}

// Editing 把意图与命令转换为事务（core/editing 实现，纯函数）。
type Editing interface {
	// HandleIntent 在选区处执行意图，包含输入规则；ok=false 表示不处理。
	HandleIntent(ctx EditContext, in Intent) (tx Transaction, ok bool)
	// Run 执行命令；ok=false 表示当前上下文不适用。
	Run(ctx EditContext, cmd EditCommand) (tx Transaction, ok bool, err error)
	Formatting(ctx EditContext) FormatState
}

// PasteConverter 把富文本剪贴板内容转换为 Markdown（adapters/richpaste 实现）。
type PasteConverter interface {
	HTMLToMarkdown(html string) (string, error)
	RTFToMarkdown(rtf string) (string, error)
}
