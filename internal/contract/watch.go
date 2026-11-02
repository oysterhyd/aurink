package contract

// WatchOp 是文件监听事件种类。
type WatchOp uint8

const (
	WatchCreate WatchOp = iota + 1
	WatchWrite          // 内容可能变了，包括被另一个文件原子替换
	WatchRemove
	WatchRename // 在被监听目录内改名；后端确认不了时报为 WatchRemove 加 WatchCreate
	// WatchOverflow 表示可能丢了事件，Path 为监听的根。收到后应重新读取所关心的一切。
	WatchOverflow
)

// WatchEvent 是一处变化。路径使用系统分隔符，以传给 Watch 的 path 开头（不解析符号
// 链接，macOS 的 /tmp 不会变成 /private/tmp）。
type WatchEvent struct {
	Op      WatchOp
	Path    string
	OldPath string // WatchRename 的原路径
	IsDir   bool   // WatchRemove 时为删除前的类型
}

// WatchOptions 配置一次监听。零值监听一个文件，或一个目录但不含子目录。
type WatchOptions struct {
	Recursive bool
	// Ignore 可为 nil。参数是相对监听根、以 / 分隔的路径；忽略的目录不会进入。
	// 在监听的 goroutine 上调用，必须快。
	Ignore func(rel string, isDir bool) bool
	// MaxEntries 是条目上限，0 为后端默认。超过时 Watch 返回 CodeTooLarge，
	// 或监听结束并以 CodeTooLarge 回调。
	MaxEntries int
}

// Watch 是一个进行中的监听。
type Watch interface {
	// Close 停止监听，返回后不再回调。可重复调用。
	Close()
}

// FileWatcher 启动文件监听（adapters/watch 以 mygo.WatchFiles 实现，D1）。
// Watch 阻塞到监听就绪，禁止在主线程调用。
//
// fn 在监听自己的 goroutine 上按批回调，同一个监听的回调不并发；使用者自行投递
// 到主线程。err 非 nil 时监听已结束，不再回调：目录消失为 CodeNotFound，超过
// MaxEntries 为 CodeTooLarge，其他为 CodeIO。
type FileWatcher interface {
	Watch(path string, opts WatchOptions, fn func(batch []WatchEvent, err error)) (Watch, error)
}

// SessionStateVersion 是 SessionState 的结构版本。
const SessionStateVersion = 1

// ViewState 是一个文档的视图位置（D10）。滚动以块为锚，字号和宽度变化后仍能回到原处。
type ViewState struct {
	Anchor, Head int     // 选区，字节偏移
	Block        int     // 视口顶部的块序号
	BlockOffset  float32 // 视口顶部在该块内的偏移，DIP
}

// TabState 是一个标签。
type TabState struct {
	Path string // 未命名文档不保存
	View ViewState
}

// SessionState 是重启后要恢复的界面状态（D9）。
type SessionState struct {
	Version  int
	Root     string   // 工作区根，"" 为未打开文件夹
	Expanded []string // 文件树中展开的目录，绝对路径
	Tabs     []TabState
	Active   int // Tabs 下标，没有标签时为 -1
}

// SessionStore 存取 SessionState（core/docsession 实现，存于 PathUserData/session.json）。
// 阻塞，禁止在主线程调用。
type SessionStore interface {
	// Load 在没有保存过时返回空状态（Version 为 SessionStateVersion，Active 为 -1，
	// 其余为零值）和 nil。文件损坏或版本更新时，把它改名备份，返回空状态和 CodeInvalid。
	Load() (SessionState, error)
	// Save 原子写入。
	Save(s SessionState) error
}

// Trasher 把文件或目录移到系统回收站（adapters 以 Shell.TrashItem 实现）。阻塞，禁止在
// 主线程调用。不存在时返回 CodeNotFound。
type Trasher interface {
	Trash(path string) error
}
