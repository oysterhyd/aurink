package settings

import (
	"math"
	"strings"

	"aurink/internal/contract"
)

// Defaults returns the default settings. Keymap contains user overrides only.
func Defaults() contract.Settings {
	return contract.Settings{
		Version: contract.SettingsVersion,
		Appearance: contract.Appearance{
			Theme: "system", AccentHue: 182, FontSize: 16,
			LineHeight: 1.75, ContentWidth: 720, ReduceMotion: "system",
		},
		Editor: contract.EditorSettings{
			Reveal: contract.RevealInline, SoftBreak: contract.SoftBreakNewline,
			AutoPair: true, BulletMarker: "-", StrongDelim: "**",
			EmphasisDelim: "*", TabSize: 4, SmoothCaret: true, Exts: contract.ExtDefault,
		},
		Files: contract.FileSettings{
			Autosave: true, AutosaveDelayMs: 1500, ImageDir: "./assets",
			CopyImages: true, RestoreSession: true, LargeFileBytes: 5 << 20,
		},
	}
}

func normalize(v *contract.Settings) {
	d := Defaults()
	v.Version = contract.SettingsVersion
	a, e, f := &v.Appearance, &v.Editor, &v.Files
	if strings.HasPrefix(a.Theme, "custom:") && len(strings.TrimSpace(strings.TrimPrefix(a.Theme, "custom:"))) > 0 && len(a.Theme) <= 128 {
		a.Theme = "custom:" + strings.TrimSpace(strings.TrimPrefix(a.Theme, "custom:"))
	} else {
		a.Theme = choice(a.Theme, d.Appearance.Theme, "system", "light", "dark")
	}
	a.AccentHue = min(max(a.AccentHue, 0), 359)
	a.FontSize = bounded(a.FontSize, 8, 72, d.Appearance.FontSize)
	a.LineHeight = bounded(a.LineHeight, 1, 3, d.Appearance.LineHeight)
	a.ContentWidth = bounded(a.ContentWidth, 320, 2000, d.Appearance.ContentWidth)
	a.ReduceMotion = choice(a.ReduceMotion, d.Appearance.ReduceMotion, "system", "on", "off")
	if e.Reveal > contract.RevealNone {
		e.Reveal = d.Editor.Reveal
	}
	if e.SoftBreak > contract.SoftBreakSpace {
		e.SoftBreak = d.Editor.SoftBreak
	}
	e.BulletMarker = choice(e.BulletMarker, d.Editor.BulletMarker, "-", "*", "+")
	e.StrongDelim = choice(e.StrongDelim, d.Editor.StrongDelim, "**", "__")
	e.EmphasisDelim = choice(e.EmphasisDelim, d.Editor.EmphasisDelim, "*", "_")
	e.TabSize = min(max(e.TabSize, 1), 8)
	e.Exts &= contract.ExtDefault | contract.ExtTildeSub | contract.ExtWikiLink
	f.AutosaveDelayMs = min(max(f.AutosaveDelayMs, 100), 60000)
	if f.ImageDir == "" {
		f.ImageDir = d.Files.ImageDir
	}
	if f.LargeFileBytes <= 0 {
		f.LargeFileBytes = d.Files.LargeFileBytes
	}
	v.Language = choice(v.Language, d.Language, "", "en", "zh-Hans")
}

func choice(value, fallback string, allowed ...string) string {
	for _, item := range allowed {
		if value == item {
			return value
		}
	}
	return fallback
}

func bounded(value, low, high, fallback float32) float32 {
	if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
		return fallback
	}
	return min(max(value, low), high)
}
