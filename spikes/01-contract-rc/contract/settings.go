package contract

// SettingsVersion 是设置文件的结构版本，用于迁移。
const SettingsVersion = 1

// Appearance 是外观设置。
type Appearance struct {
	Theme        string  // "system" | "light" | "dark"
	AccentHue    int     // 强调色 OKLCH 色相 0–359，默认 182
	ContentFont  string  // "" 为默认
	CodeFont     string  // "" 为默认
	FontSize     float32 // 正文字号，默认 16
	LineHeight   float32 // 默认 1.75
	ContentWidth float32 // 默认 720
	ReduceMotion string  // "system" | "on" | "off"
	// ReduceTransparency 为 true 时走低功耗路径：不使用玻璃、Vibrancy 与极光。
	ReduceTransparency bool
}

// EditorSettings 是编辑器设置。
type EditorSettings struct {
	Reveal        RevealMode
	SoftBreak     SoftBreakMode
	AutoPair      bool
	BulletMarker  string // "-" | "*" | "+"
	StrongDelim   string // "**" | "__"
	EmphasisDelim string // "*" | "_"
	TabSize       int
	SmoothCaret   bool
	Exts          Extensions
}

// FileSettings 是文件与工作区设置。
type FileSettings struct {
	Autosave        bool
	AutosaveDelayMs int
	// ImageDir 是图片导入目录模板："./assets"、"./${filename}.assets" 或绝对路径。
	ImageDir       string
	CopyImages     bool
	ShowAllFiles   bool
	RemoteImages   bool // 是否加载网络图片（默认否）
	RestoreSession bool
	LargeFileBytes int64 // 超过即进入大文件模式，默认 1 MB
}

// Settings 是全部用户设置。
type Settings struct {
	Version    int
	Appearance Appearance
	Editor     EditorSettings
	Files      FileSettings
	Language   string // "" 跟随系统，或 "en"、"zh-Hans"
	Keymap     []KeyBinding
}

// SettingsStore 存取设置（core/settings 实现）。方法在主线程调用；写盘在后台进行。
type SettingsStore interface {
	Get() Settings
	// Update 修改设置并持久化；fn 在主线程同步执行。
	Update(fn func(*Settings)) error
	Subscribe(fn func(Settings)) (off func())
}
