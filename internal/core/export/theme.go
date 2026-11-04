package export

// These sRGB values mirror views/theme.For at the default hue. The test in
// tests/export checks every palette value against the view's tokens.
const lightCSS = `:root {
  --sheet: #ffffffff; --text: #15161cff; --muted: #5f616eff;
  --faint: #8b8d99ff; --border: #14162317; --accent: #007768ff;
  --mark: #ffefb0ff; --code-bg: #f7f7f9ff;
  --syntax-keyword: #5d3fa8ff; --syntax-func: #007768ff;
  --syntax-comment: #70727fff; --syntax-string: #946100ff;
}
`

const darkCSS = `:root {
  --sheet: #111218ff; --text: #ecedf2ff; --muted: #a2a4b3ff;
  --faint: #6c6e7eff; --border: #ffffff16; --accent: #57ddcaff;
  --mark: #ffc45442; --code-bg: #0c0d12ff;
  --syntax-keyword: #b9a7ffff; --syntax-func: #57ddcaff;
  --syntax-comment: #7f8192ff; --syntax-string: #e6c07bff;
}
`

const commonCSS = `html { background: var(--sheet); color: var(--text); }
body { max-width: 720px; margin: auto; padding: 72px 28px 120px;
  font: 16px/1.75 "Segoe UI Variable", "Microsoft YaHei", "SF Pro Text", "PingFang SC", Cantarell, "Noto Sans CJK SC", sans-serif; }
h1, h2 { font-family: Newsreader, "Noto Serif SC", Georgia, SimSun, "Microsoft YaHei", "PingFang SC", "Noto Sans CJK SC", serif; font-weight: 500; }
h1 { font-size: 2.6em; line-height: 1.14; letter-spacing: -.025em; }
h2 { font-size: 1.62em; line-height: 1.3; letter-spacing: -.012em; margin: 1.9em 0 .5em; }
h3 { font-size: 1.12em; font-weight: 650; margin: 1.5em 0 .4em; }
h4, h5 { font-size: 1em; font-weight: 600; }
h6 { font-size: .95em; font-weight: 600; }
a { color: var(--accent); } img { max-width: 100%; height: auto; }
blockquote { margin: 1em 0; padding: 0 18px; color: var(--muted);
  border-left: 4px solid color-mix(in srgb, var(--faint) 35%, transparent); }
code, pre { font: .85em/1.55 "JetBrains Mono", "Cascadia Code", "SF Mono", "DejaVu Sans Mono", monospace; font-variant-ligatures: none; }
code { background: var(--code-bg); border-radius: 4px; padding: .1em .25em; }
pre { overflow-x: auto; background: var(--code-bg); border-radius: 8px; padding: 16px; }
pre code { font-size: 1em; padding: 0; background: none; }
table { border-collapse: collapse; width: 100%; margin: 1.5em 0; }
th, td { border: 1px solid var(--border); padding: 8px 12px; }
th { background: var(--code-bg); } hr { border: 0; border-top: 1px solid var(--border); }
mark { background: var(--mark); color: inherit; }
sup, sub { line-height: 0; } .wikilink { color: var(--accent); }
.math { color: var(--text); } span.math { white-space: nowrap; }
div.math { text-align: center; margin: 1em 0; } .diagram { text-align: center; margin: 1em 0; }
.math svg, .math img, .diagram img { max-width: 100%; }
.diagram img { height: auto !important; }
.toc, .footnotes { color: var(--muted); border-top: 1px solid var(--border); margin-top: 2em; }
.toc ul { list-style: none; margin: 0; padding-left: 1.25em; } .toc > ul { padding-left: 0; }
.footnote-ref, .footnote-backref { font-size: .8em; } .footnote-backref { margin-left: .5em; }
.tok-keyword { color: var(--syntax-keyword); } .tok-function, .tok-func { color: var(--syntax-func); }
.tok-comment { color: var(--syntax-comment); } .tok-string { color: var(--syntax-string); }
@media print {
  :root { --sheet: #fff; --text: #15161c; --muted: #5f616e; --faint: #8b8d99;
    --border: #cccccc; --accent: #007768; --mark: #ffefb0; --code-bg: #f7f7f9;
    --syntax-keyword: #5d3fa8; --syntax-func: #007768; --syntax-comment: #70727f; --syntax-string: #946100; }
  html, body { background: #fff; color: #15161c; }
  body { max-width: none; padding: 0; }
  table { box-sizing: border-box; width: calc(100% - 2px); }
  pre { white-space: pre-wrap; overflow-wrap: anywhere; overflow: visible; }
  pre, img, figure, tr, div.math, p:has(> img) { break-inside: avoid; page-break-inside: avoid; }
  h1, h2, h3, h4, h5, h6 { break-after: avoid; page-break-after: avoid; }
}
`

// Dark renders use light ink, so invert their images only when printing a dark
// HTML document on the light print sheet. PDF exports request light renders.
const darkPrintCSS = `@media print {
  .math img, .math svg, .diagram img { filter: invert(1) hue-rotate(180deg); }
}
`
