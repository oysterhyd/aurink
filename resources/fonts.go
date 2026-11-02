// Package resources embeds what ships inside the binary.
package resources

import (
	"embed"

	"github.com/egoist/mygo/ui"
)

//go:embed fonts/*.ttf
var fonts embed.FS

// The subset of Noto Serif SC calls itself something else inside, so every family
// is named here (ADR-004).
var families = map[string]string{
	"fonts/Newsreader-opsz-wght.ttf":         "Newsreader",
	"fonts/Newsreader-Italic-opsz-wght.ttf":  "Newsreader",
	"fonts/Noto_Serif_SC-gb2312-500-700.ttf": "Noto Serif SC",
	"fonts/JetBrains_Mono-wght.ttf":          "JetBrains Mono",
}

// RegisterFonts registers the bundled fonts with the text system.
func RegisterFonts() error {
	for file, family := range families {
		data, err := fonts.ReadFile(file)
		if err != nil {
			return err
		}
		if err := ui.RegisterFont(data, family); err != nil {
			return err
		}
	}
	return nil
}
