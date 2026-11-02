# Paste sample provenance

All outputs were checked against their input fragments. HTML is encoded as UTF-8.

| Samples | Source and capture |
| --- | --- |
| `word-01`–`word-05` | Microsoft Word for Windows, COM version 16.0, captured 2026-11-01 from the `HTML Format` clipboard fragment. `capture-word.ps1` builds five small documents in Word, copies the content, and saves Word's actual clipboard HTML. The samples cover inline formatting, heading/paragraph, a Word list, a table, and a link with accented text. |
| `google-docs-01` | [Leaflet's captured Google Docs full-document clipboard fragment](https://tangled.org/leaflet.pub/leaflet/blob/d2d2a1b7ced9df287e447f526829e923533e164a/src/utils/paste/__fixtures__/captured/google-docs.html), retrieved 2026-11-01. |
| `google-docs-02` | [Leaflet's captured Google Docs mid-paragraph clipboard fragment](https://tangled.org/leaflet.pub/leaflet/blob/d2d2a1b7ced9df287e447f526829e923533e164a/src/utils/paste/__fixtures__/captured/google-docs-mid-paragraph.html), retrieved 2026-11-01. |
| `web-01` | Paragraph selected from [RFC 9110](https://www.rfc-editor.org/rfc/rfc9110.html), retrieved 2026-11-01. |
| `web-02` | Paragraph selected from [PNG specification](https://www.w3.org/TR/png/), retrieved 2026-11-01. |
| `web-03` | Paragraph selected from [MDN's `em` reference](https://developer.mozilla.org/en-US/docs/Web/HTML/Reference/Elements/em), retrieved 2026-11-01. |
| `web-04` | Paragraph selected from [CSS Color Module Level 4](https://www.w3.org/TR/css-color-4/), retrieved 2026-11-01. |
| `web-05` | Paragraph selected from [Python: About](https://www.python.org/about/), retrieved 2026-11-01. |

The web samples are actual element fragments extracted from fetched pages; no
browser clipboard normalization was applied. The two Google Docs inputs are
independent captured selections, but both come from the same source document.
Three additional verified Google Docs clipboard samples were not available.

The two Leaflet fixtures are distributed under the MIT License:

Copyright (c) 2025 Learning Futures Inc.

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
