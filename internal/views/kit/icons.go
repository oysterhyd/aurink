package kit

import (
	"fmt"
	"sync"

	"github.com/egoist/mygo/ui"
)

func icon(path string) *ui.SVG {
	return ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.3" stroke-linecap="round" stroke-linejoin="round">` + path + `</svg>`))
}

// The icon set of the M1 interface, on a 16 grid.
var (
	IconSidebar   = icon(`<rect x="2" y="3" width="12" height="10" rx="2"/><path d="M6 3v10"/>`)
	IconPlus      = icon(`<path d="M8 3.5v9M3.5 8h9"/>`)
	IconClose     = icon(`<path d="M4 4l8 8M12 4l-8 8"/>`)
	IconSearch    = icon(`<circle cx="7" cy="7" r="4"/><path d="M10 10l3.5 3.5"/>`)
	IconChevron   = icon(`<path d="M4.5 6.5L8 10l3.5-3.5"/>`)
	IconContrast  = icon(`<circle cx="8" cy="8" r="5.5"/><path d="M8 2.5a5.5 5.5 0 010 11z" fill="currentColor" stroke="none"/>`)
	IconMinimize  = icon(`<path d="M3.5 8h9"/>`)
	IconMaximize  = icon(`<rect x="3.5" y="3.5" width="9" height="9" rx="1.5"/>`)
	IconCheck     = icon(`<path d="M3.5 8.5l3 3 6-7"/>`)
	IconFile      = icon(`<path d="M4 2.5h5l3 3v8H4z"/><path d="M9 2.5v3h3"/>`)
	IconInfo      = icon(`<circle cx="8" cy="8" r="5.5"/><path d="M8 7.5v3M8 5.2v.1"/>`)
	IconFolder    = icon(`<path d="M2.5 4.5a1 1 0 011-1h3l1.5 1.5h4.5a1 1 0 011 1v6a1 1 0 01-1 1h-9a1 1 0 01-1-1z"/>`)
	IconCommand   = icon(`<path d="M3.5 5l3 3-3 3M8 11.5h4.5"/>`)
	IconHeading   = icon(`<path d="M4 3.5v9M10 3.5v9M4 8h6M12.5 7.5l1-1v6"/>`)
	IconArrowUp   = icon(`<path d="M8 13V3.5M4 7.5l4-4 4 4"/>`)
	IconArrowDown = icon(`<path d="M8 3v9.5M4 8.5l4 4 4-4"/>`)
	IconLock      = icon(`<rect x="3.5" y="7" width="9" height="6.5" rx="1.5"/><path d="M5.5 7V5a2.5 2.5 0 015 0v2"/>`)
	IconTrash     = icon(`<path d="M3 4.5h10M6.5 4.5V3h3v1.5M4.5 4.5l.7 9h5.6l.7-9"/>`)
	IconSliders   = icon(`<path d="M2.5 5h6.5M12.5 5h1M2.5 11h1M7 11h6.5"/><circle cx="10.7" cy="5" r="1.7"/><circle cx="5.3" cy="11" r="1.7"/>`)
	IconLink      = icon(`<path d="M6.8 9.2a2.6 2.6 0 003.7 0l2-2a2.6 2.6 0 00-3.7-3.7l-.9.9"/><path d="M9.2 6.8a2.6 2.6 0 00-3.7 0l-2 2a2.6 2.6 0 003.7 3.7l.9-.9"/>`)
	IconWarn      = icon(`<path d="M8 2.8l5.8 10H2.2z"/><path d="M8 6.5v3M8 11.2v.1"/>`)
)

// Brand is the Aurink mark of the design (Main, the title bar): an arch in
// the theme's aurora gradient over a drop of ink in currentColor. The marks
// are made once for each set of colors.
func Brand(a1, a2, a3 ui.Color) *ui.SVG {
	key := [3]ui.Color{a1.SRGB(), a2.SRGB(), a3.SRGB()}
	brandMu.Lock()
	defer brandMu.Unlock()
	if s, ok := brands[key]; ok {
		return s
	}
	hex := func(c ui.Color) string { return fmt.Sprintf("#%02X%02X%02X", c.R, c.G, c.B) }
	s := ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none">` +
		`<defs><linearGradient id="mk" x1="0" y1="0" x2="1" y2="1"><stop offset="0" stop-color="` + hex(key[0]) +
		`"/><stop offset="0.55" stop-color="` + hex(key[1]) + `"/><stop offset="1" stop-color="` + hex(key[2]) + `"/></linearGradient></defs>` +
		`<path d="M3 20C4.5 11 7.8 4 12 4s7.5 7 9 16" stroke="url(#mk)" stroke-width="2.6" stroke-linecap="round"/>` +
		`<path d="M12 11.5c1.9 2.5 3 4 3 5.4a3 3 0 0 1-6 0c0-1.4 1.1-2.9 3-5.4z" fill="currentColor"/></svg>`))
	brands[key] = s
	return s
}

var (
	brandMu sync.Mutex
	brands  = map[[3]ui.Color]*ui.SVG{}
)
