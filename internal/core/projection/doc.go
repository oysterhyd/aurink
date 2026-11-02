// Package projection projects M1 Markdown leaves into immutable display segments.
//
// New implements contract.Projector without I/O, UI dependencies, or caches.
// Block and inline rule tables define syntax behavior; container prefixes stay
// hidden and are described by ContainerDecor. Supported inline HTML and defined
// footnotes have display projections; HTML blocks and Front Matter retain muted
// source text. Display strings and object metadata own their bytes.
//
// Mapping uses UTF-16 display positions and Unicode 17.0 extended grapheme
// boundaries. The first and last segments always encode Lead and Trailer, even
// when empty, so the mapper can recover the editable range from the projection.
//
// Sig describes layout only. A consumer may translate a retained projection only
// if its source and relevant global tables are unchanged (04 section 4.3.6);
// equal signatures alone never authorize reusing source mappings.
package projection
