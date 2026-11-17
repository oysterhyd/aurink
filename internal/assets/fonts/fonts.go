// Package fonts embeds and registers the bundled editorial and monospace fonts
// (09 D14, INT-4.1).
package fonts

import (
	"embed"

	"github.com/egoist/mygo/ui"
)

//go:embed *.ttf
var fs embed.FS

// The subset of Noto Serif SC calls itself something else inside, so every family
// is named here (ADR-004).
var families = map[string]string{
	"Newsreader-opsz-wght.ttf":         "Newsreader",
	"Newsreader-Italic-opsz-wght.ttf":  "Newsreader",
	"Noto_Serif_SC-gb2312-500-700.ttf": "Noto Serif SC",
	"JetBrains_Mono-wght.ttf":          "JetBrains Mono",
}

// RegisterFonts registers the bundled fonts with the text system.
func RegisterFonts() error {
	for file, family := range families {
		data, err := fs.ReadFile(file)
		if err != nil {
			return err
		}
		if err := ui.RegisterFont(data, family); err != nil {
			return err
		}
	}
	return nil
}
