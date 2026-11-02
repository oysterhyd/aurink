package contract

import (
	"context"
	"time"
)

// FileStamp 用于检测外部修改。
type FileStamp struct {
	ModTime time.Time
	Size    int64
}

// FileContent 是读入的文件。
type FileContent struct {
	Path  string
	Text  string // 已解码、已剥离 BOM 的 UTF-8，换行保持原样
	Meta  DocMeta
	Stamp FileStamp
}

// ReadOptions 控制读取。
type ReadOptions struct {
	MaxSize     int64 // 0 表示默认 64 MB；超出返回 CodeTooLarge
	AllowBinary bool  // false 时二进制返回 CodeBinary
}

// FileStore 读写文件（core/fsstore 实现）。全部方法阻塞，禁止在主线程调用。
type FileStore interface {
	Read(path string, opts ReadOptions) (FileContent, error)
	// Write 原子写入；expect 非 nil 且磁盘戳不符时返回 CodeConflict。
	Write(path string, text string, meta DocMeta, expect *FileStamp) (FileStamp, error)
	Stat(path string) (FileStamp, error)
}

// DocState 是会话状态，变化时经 Session.OnState 通知。
type DocState struct {
	ID        DocID
	Path      string // 未命名为 ""
	Name      string // 显示名
	Edited    bool
	Saving    bool
	Conflict  bool
	Deleted   bool
	ReadOnly  bool
	LargeFile bool
	SavedAt   time.Time
	Err       *Error // 最近一次保存/读取错误
}

// CloseDecision 是关闭请求的结果。
type CloseDecision uint8

const (
	CloseDone        CloseDecision = iota // 已关闭
	CloseNeedsPrompt                      // 有未保存修改且无法自动保存，需要询问用户
)

// Session 是一个打开的文件（core/docsession 实现）。方法在主线程调用、立即返回；
// done 回调在主线程触发。
type Session interface {
	ID() DocID
	Model() DocumentModel
	State() DocState
	Save(done func(error))
	SaveAs(path string, done func(error))
	Revert(done func(error))
	// KeepMine 冲突时以当前内容覆盖磁盘。
	KeepMine(done func(error))
	// LoadTheirs 冲突时载入磁盘版本（作为可撤销的一步）。
	LoadTheirs(done func(error))
	// Close 关闭会话；force 为 true 时丢弃未保存修改（恢复副本同时删除）。
	Close(force bool) CloseDecision
	OnState(fn func(DocState)) (off func())
}

// SessionsEventKind 是会话集合事件种类。
type SessionsEventKind uint8

const (
	SessionOpened SessionsEventKind = iota + 1
	SessionClosed
	SessionRestored
)

// SessionsEvent 在主线程上通知。
type SessionsEvent struct {
	Kind SessionsEventKind
	ID   DocID
}

// Sessions 管理全部打开的会话，同一路径只有一个会话。方法在主线程调用。
type Sessions interface {
	Open(path string, done func(Session, error))
	New() Session
	Get(id DocID) (Session, bool)
	FindByPath(path string) (Session, bool)
	All() []Session
	// Restore 启动时恢复崩溃副本与上次会话。
	Restore(done func([]Session, error))
	Subscribe(fn func(SessionsEvent)) (off func())
}

// Entry 是工作区中的一个文件或目录。
type Entry struct {
	Path    string
	Name    string
	IsDir   bool
	Size    int64
	ModTime time.Time
}

// WorkspaceEventKind 是工作区事件种类。
type WorkspaceEventKind uint8

const (
	WsRootChanged WorkspaceEventKind = iota + 1
	WsListed                         // 目录列表已加载/刷新
	WsCreated
	WsRemoved
	WsRenamed
	WsModified
	WsDegraded // 条目超过监听上限，降级为仅监听已展开目录
)

// WorkspaceEvent 在主线程上通知。
type WorkspaceEvent struct {
	Kind    WorkspaceEventKind
	Path    string
	OldPath string
	IsDir   bool
}

// Workspace 是打开的文件夹（core/workspace 实现）。方法在主线程调用、立即返回。
type Workspace interface {
	Root() string
	SetRoot(path string, done func(error))
	// List 返回缓存的目录列表；loaded=false 时开始异步加载并在完成后发出 WsListed。
	List(dir string) (entries []Entry, loaded bool)
	Refresh(dir string)
	Create(dir, name string, isDir bool, done func(path string, err error))
	Rename(path, newName string, done func(newPath string, err error))
	Move(paths []string, dstDir string, done func(err error))
	Trash(paths []string, done func(err error))
	// SetShowAll 切换显示全部文件或仅 Markdown 与图片（.md .markdown .png .jpg .jpeg .gif
	// .svg .webp，不分大小写）。目录总是显示。切换后已加载的目录各发一次 WsListed。
	SetShowAll(all bool)
	// Ignored 报告忽略规则（.git、node_modules、隐藏文件）是否排除 rel；rel 相对根、以 /
	// 分隔。文件树和 WorkspaceSearch 共用这份规则（D15）。不受 SetShowAll 影响。
	// 任意 goroutine 可调用。
	Ignored(rel string, isDir bool) bool
	Subscribe(fn func(WorkspaceEvent)) (off func())
}

// Assets 把图片导入文档的资源目录，返回可写入 Markdown 的路径。阻塞。
//
// 目录来自 Settings.Files.ImageDir，其中 ${filename} 是文档名去掉扩展名。返回的路径相对
// 文档所在目录，以 / 分隔，空格写成 %20；目录在另一个盘符等无法相对时返回绝对路径。
// 重名时在扩展名前加 -1、-2……，不覆盖已有文件。CopyImages 为 false 时 ImportFiles 不
// 复制，直接返回原文件的路径。docPath 为 "" 时返回 CodeInvalid。
type Assets interface {
	ImportFiles(docPath string, files []string) ([]string, error)
	// ImportImage 写入 PNG；nameHint 为 "" 时用 "image"，扩展名总是 .png。
	ImportImage(docPath string, png []byte, nameHint string) (string, error)
}

// RecentMax 是 Recent 每个列表的上限。
const RecentMax = 20

// Recent 是最近打开的文件与文件夹（主线程，写盘在后台）。列表最近的在前，同一路径只
// 出现一次（Windows 上不分大小写），超过 RecentMax 时丢弃最旧的。
type Recent interface {
	Files() []string
	Folders() []string
	Add(path string, isDir bool)
	Clear()
}

// Query 是查找条件。
type Query struct {
	Text          string
	CaseSensitive bool // false 时按 Unicode 简单大小写折叠比较
	// WholeWord：匹配两侧紧邻的字符都不是字母、数字或下划线（按 Unicode 判断）。中文等
	// 没有空格分词的文字按字母处理。
	WholeWord bool
	Regex     bool // RE2 语法；Text 为空时没有匹配
}

// Match 是一处匹配。匹配按位置排序，互不重叠；空匹配（如正则 `a*`）不报告。
type Match struct {
	Range Range // 在文本中，字节偏移
	Line  int   // Range.Start 所在行，0 起
	// Preview 是所在行，不含换行符。超过 PreviewMax 字节时截取匹配附近，截断处不切开
	// UTF-8 字符，并以 "…" 标出。
	Preview string
	Hit     Range // 匹配在 Preview 中的区间；跨行的匹配截到行尾
}

// PreviewMax 是 Match.Preview 的最大字节数（不含 "…"）。
const PreviewMax = 200

// Finder 在一个文本快照中查找（纯函数，可在任意 goroutine）。
type Finder interface {
	// Find 返回至多 limit 个匹配（0 不限）；正则错误返回 CodeInvalid。
	Find(t Text, q Query, limit int) ([]Match, error)
}

// FileMatches 是一个文件中的匹配。
type FileMatches struct {
	Path    string
	Matches []Match
}

// SearchOptions 是目录搜索的范围。
type SearchOptions struct {
	// Ignore 可为 nil；通常是 Workspace.Ignored。参数同 Workspace.Ignored。
	Ignore func(rel string, isDir bool) bool
	// MaxFileSize 之上的文件跳过；0 为 5 MB。
	MaxFileSize int64
	// PerFile 是每个文件最多的匹配数；0 不限。
	PerFile int
}

// WorkspaceSearch 在目录树中搜索文本文件，跳过二进制、超大和非 UTF-8/UTF-16 的文件。
// 阻塞；onBatch 在调用者 goroutine 上按批回调，每个文件至多出现一次，没有匹配的文件
// 不出现。ctx 取消时尽快返回 CodeCanceled，之后不再回调。
type WorkspaceSearch interface {
	Search(ctx context.Context, root string, q Query, opts SearchOptions, onBatch func([]FileMatches)) error
}

// FuzzyItem 是模糊匹配的候选项。
type FuzzyItem struct {
	Text   string
	Detail string
}

// FuzzyHit 是一个匹配结果；Positions 为 Text 中命中的 rune 下标。
type FuzzyHit struct {
	Index     int
	Score     int
	Positions []int
}

// Fuzzy 是模糊匹配（纯函数）。query 的每个字符（忽略空白）须按顺序出现在 Text 中，
// 不分大小写，否则不算命中。结果按 Score 降序，同分按 Index 升序，至多 limit 个
// （0 不限）。连续命中、词首（分隔符或大小写边界之后）和文件名部分（最后一个 / 或 // 之后）的命中得分更高。query 为空时按原顺序返回，Score 为 0，Positions 为空。
type Fuzzy interface {
	Match(query string, items []FuzzyItem, limit int) []FuzzyHit
}
