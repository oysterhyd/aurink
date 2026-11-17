package contract

// Range 是源码中的半开字节区间 [Start, End)。
type Range struct{ Start, End int }

// Len 返回区间长度。
func (r Range) Len() int { return r.End - r.Start }

// Empty 报告区间是否为空。
func (r Range) Empty() bool { return r.Start >= r.End }

// Contains 报告 off 是否在区间内。
func (r Range) Contains(off int) bool { return off >= r.Start && off < r.End }

// Overlaps 报告两个区间是否相交。
func (r Range) Overlaps(o Range) bool { return r.Start < o.End && o.Start < r.End }

// Version 单调递增：每个已提交事务 +1；0 表示无效。
type Version uint64

// TextEdit 把"事务应用前"文本中的 Range 替换为 Text。
type TextEdit struct {
	Range Range
	Text  string
}

// Change 描述一次替换在旧文本与新文本中的位置，语义与 ui/editor.Change 相同：
// 旧文本 [OldStart,OldEnd) 变为新文本 [NewStart,NewEnd)。一个事务的 Changes
// 有序且不重叠。
type Change struct{ OldStart, OldEnd, NewStart, NewEnd int }

// Text 是不可变的文档快照，任何 goroutine 均可读取。
type Text interface {
	Version() Version
	Len() int
	String() string
	Slice(r Range) string
	LineCount() int
	// LineStart 返回第 line 行（0 起）首字节偏移；line == LineCount() 时返回 Len()。
	LineStart(line int) int
	// LineOf 返回 offset 所在行号（0 起）。
	LineOf(offset int) int
}

// EOLStyle 是文档的换行风格。文本始终保留原始字节，不做归一化。
type EOLStyle uint8

const (
	EOLLF EOLStyle = iota
	EOLCRLF
	EOLMixed
)

// DocMeta 是文件层面的元信息。Text 本身不含 BOM。
type DocMeta struct {
	EOL EOLStyle
	// Dominant 是新插入换行使用的序列："\n" 或 "\r\n"。
	Dominant string
	BOM      bool
	// Encoding："utf-8"、"utf-16le"、"utf-16be"、"gb18030"（只读打开，转换后以 utf-8 保存）。
	Encoding string
	// Large 表示大文件模式（字节数 ≥ Settings.Files.LargeFileBytes）：跳过 Markdown 解析，
	// AST() 返回满足不变式的单叶子纯文本快照，编辑在源码模式进行（09 D1–D3）。
	Large bool
}
