// Package markdown adapts the pinned MyGo parser to immutable contract.AST
// snapshots with stable block identities and source coverage.
//
// Core/document creates a Parser with New(source, version, extensions), retains
// Snapshot(), and calls Update(nextText, changes) after core/text.Apply. Apply
// is a convenience when the caller has edits instead of an updated Text.
// Update.Dirty lists new or changed IDs, including users of changed definitions;
// an ID absent from the new Snapshot.Index has been removed.
//
// A Parser has one writer. Snapshots and their lazy caches support concurrent
// readers, including while the writer advances. Returned blocks and slices are
// read-only. Content-line records are shared across versions in relative
// coordinates; absolute blocks are materialized on demand.
//
// Structural updates parse complete top-level subtrees with a verified left
// context and two unchanged suffix anchors. The window includes all block
// kinds and original line endings. Growth stops at 24 KiB or 64 nonblank
// top-level subtrees. Coverage-only blank rows count toward the byte limit,
// and cannot certify a boundary. Missing anchors, oversized windows, Front
// Matter changes and ambiguous caret labels cause counted full parses.
// parser.Document.Edit is not used, avoiding its indented-code, table and
// reference-cache defects.
//
// In the pinned parser, block definition recognition consumes syntax regardless
// of existing definitions; reference lookup only affects inlines. Window block
// parsing therefore needs no definition injection. Inlines retain the snapshot's
// whole-document definition context. Definition changes compare dependent inline
// meanings after the window is spliced, including empty footnote labels.
package markdown
