package kit

import "github.com/egoist/mygo/ui"

func icon(path string) *ui.SVG {
	return ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.3" stroke-linecap="round" stroke-linejoin="round">` + path + `</svg>`))
}

// The icon set of the M1 interface, on a 16 grid.
var (
	IconSidebar  = icon(`<rect x="2" y="3" width="12" height="10" rx="2"/><path d="M6 3v10"/>`)
	IconPlus     = icon(`<path d="M8 3.5v9M3.5 8h9"/>`)
	IconClose    = icon(`<path d="M4 4l8 8M12 4l-8 8"/>`)
	IconSearch   = icon(`<circle cx="7" cy="7" r="4"/><path d="M10 10l3.5 3.5"/>`)
	IconChevron  = icon(`<path d="M4.5 6.5L8 10l3.5-3.5"/>`)
	IconContrast = icon(`<circle cx="8" cy="8" r="5.5"/><path d="M8 2.5a5.5 5.5 0 010 11z" fill="currentColor" stroke="none"/>`)
	IconMinimize = icon(`<path d="M3.5 8h9"/>`)
	IconMaximize = icon(`<rect x="3.5" y="3.5" width="9" height="9" rx="1.5"/>`)
	IconCheck    = icon(`<path d="M3.5 8.5l3 3 6-7"/>`)
	IconFile     = icon(`<path d="M4 2.5h5l3 3v8H4z"/><path d="M9 2.5v3h3"/>`)
	IconInfo     = icon(`<circle cx="8" cy="8" r="5.5"/><path d="M8 7.5v3M8 5.2v.1"/>`)
	IconFolder   = icon(`<path d="M2.5 4.5a1 1 0 011-1h3l1.5 1.5h4.5a1 1 0 011 1v6a1 1 0 01-1 1h-9a1 1 0 01-1-1z"/>`)
	IconCommand  = icon(`<path d="M3.5 5l3 3-3 3M8 11.5h4.5"/>`)
	IconHeading  = icon(`<path d="M4 3.5v9M10 3.5v9M4 8h6M12.5 7.5l1-1v6"/>`)
	IconWarn     = icon(`<path d="M8 2.8l5.8 10H2.2z"/><path d="M8 6.5v3M8 11.2v.1"/>`)
)

// Brand is the Aurink mark: an arch in the three aurora colors.
var Brand = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20" fill="none" stroke-width="2.2" stroke-linecap="round"><path d="M3 17C3 8 6 3 10 3" stroke="#2BA59B"/><path d="M10 3c4 0 7 5 7 14" stroke="#D6498F"/><path d="M6.8 17c0-5 1.3-8 3.2-8s3.2 3 3.2 8" stroke="#5B5BD6"/></svg>`))
