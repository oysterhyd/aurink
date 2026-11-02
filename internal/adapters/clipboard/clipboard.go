package clipboard

import (
	"aurink/internal/contract"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"
)

// RTF is how MyGo names the system's rich text: RichEdit's "Rich Text Format"
// on Windows.
const RTF transfer.Format = "text/rtf"

// Clipboard is the system clipboard (contract.Clipboard).
type Clipboard struct{}

// New returns the system clipboard.
func New() *Clipboard { return &Clipboard{} }

// Read reads the formats Aurink pastes; a format that cannot be read is empty.
func (Clipboard) Read() contract.ClipData {
	d, err := mygo.Clipboard.Read(transfer.Markdown, transfer.HTML, RTF, transfer.Text, transfer.FileList, transfer.PNG)
	if err != nil {
		return contract.ClipData{}
	}
	return FromData(d)
}

// Write puts every format d has on the clipboard as one item.
func (Clipboard) Write(d contract.ClipData) error { return mygo.Clipboard.Write(ToData(d)) }

// FromData reads a transfer into ClipData.
func FromData(d transfer.Data) contract.ClipData {
	read := func(f transfer.Format) string {
		b, err := d.Read(f)
		if err != nil {
			return ""
		}
		return string(b)
	}
	out := contract.ClipData{Text: read(transfer.Text), Markdown: read(transfer.Markdown), HTML: read(transfer.HTML), RTF: read(RTF)}
	if files, err := d.Files(); err == nil {
		out.Files = files
	}
	if png, err := d.Read(transfer.PNG); err == nil {
		out.PNG = png
	}
	return out
}

// ToData makes one transfer item of the formats d has, text first.
func ToData(d contract.ClipData) transfer.Data {
	var reps []transfer.Representation
	add := func(f transfer.Format, s string) {
		if s != "" {
			reps = append(reps, transfer.Bytes(f, []byte(s)))
		}
	}
	add(transfer.Text, d.Text)
	add(transfer.Markdown, d.Markdown)
	add(transfer.HTML, d.HTML)
	add(RTF, d.RTF)
	if len(d.PNG) > 0 {
		reps = append(reps, transfer.Bytes(transfer.PNG, d.PNG))
	}
	if len(reps) == 0 && len(d.Files) == 0 {
		return transfer.Data{}
	}
	if len(d.Files) > 0 {
		files, err := transfer.FileData(d.Files...)
		if err == nil && len(reps) == 0 {
			return files
		}
	}
	return transfer.New(transfer.NewItem(reps...))
}
