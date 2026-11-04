# PDF evidence — Windows

Generated with `go run ./tools/pdfcheck` on Windows, MyGo v0.7.3 and the
installed WebView2 runtime. Poppler 25.07.0 `pdfinfo` counted the pages;
`pdftoppm.exe -png -r 110` produced every page image; `pdftotext -layout -enc
UTF-8` produced the adjacent text files. The second export reused the render
service's warm cache, so its export time is not a cold-start measurement.

| Paper | Pages | Export | Print | PDF size | Render requests | Failures |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| A4 | 7 | 942 ms | 634 ms | 497,722 bytes | 6 | 0 |
| Letter | 8 | 2 ms | 558 ms | 499,878 bytes | 6 | 0 |

Both text layers contain the exact Chinese characters and punctuation of
“这是一段用于验证中文字体和文本提取的句子。” and “排版应当清晰，标点正确，字形不应显示为方框。”
after removing whitespace inserted by line wrapping. The formula rendered
from `\text{中文公式}` is a raster image and is therefore not a selectable text
layer; the surrounding Chinese prose is selectable.

Each PNG below was inspected at its full 110-dpi resolution. The Chinese
glyphs visible on every page are legible and are not tofu. Page boundaries
preserve table rows, code blocks, diagrams, and images; text and headings stay
within the printable area.

| Page | Visual check |
| --- | --- |
| A4 1 | Title, TOC, CJK and extensions are legible; inline and both block formulas are rendered, including Chinese math. |
| A4 2 | Table header and rows 01–19 are complete; no row is cut. |
| A4 3 | Repeated table header and rows 20–38 are complete. |
| A4 4 | Go, Python and JavaScript blocks are complete; long Go line wraps through `LONG_LINE_END`; first diagram is intact. |
| A4 5 | Second diagram and both PNG and JPEG are intact with their captions. |
| A4 6 | Section heading and paragraphs 01–07 are legible; no heading is stranded. |
| A4 7 | Paragraphs 08–12 are legible and stay within margins. |
| Letter 1 | Title, TOC, Chinese and extensions are legible; inline formulas align with text. |
| Letter 2 | Both block formulas are rendered; table header and rows 01–14 are complete. |
| Letter 3 | Repeated table header and rows 15–33 are complete. |
| Letter 4 | Table rows 34–38 and all three code blocks are complete; long Go line wraps through `LONG_LINE_END`. |
| Letter 5 | Both diagrams and the PNG are complete; the PNG caption stays with the image. |
| Letter 6 | JPEG and caption are complete; section heading and paragraphs 01–04 are legible. |
| Letter 7 | Paragraphs 05–11 are legible and unsplit. |
| Letter 8 | Paragraph 12 is legible and within the margins. |

The Letter page 2 border was additionally rasterized at 300 dpi before and
after the table was inset: `letter-page2-before-300dpi.png` shows only one
gray pixel at the clipped right edge, while `letter-page2-after-300dpi.png`
shows a four-pixel border fully inside the content area. The unnumbered TOC,
soft line breaks, and literal JavaScript `=>` are visible in the new pages.

In both paper sizes the first Mermaid diagram's short edge labels overlap
their arrows slightly; the nodes, labels, and arrows remain visible. This is
the diagram renderer's layout, not a page break or clipping. Mermaid's own
layout and default colors were left unchanged.
