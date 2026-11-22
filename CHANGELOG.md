# Changelog

All notable changes to Aurink are documented in this file.

## 1.0.1 - 2026-11-22

Engine performance, table layout, slash menu, and UI refinement update.

### Highlights
- **Zero-Concatenation Chunked Incremental Parser**: `core/markdown` now validates changes and performs typing & local structural reparses directly over 16 KB `core/text` chunks without materializing full-document contiguous strings or running full-document byte scans on each keystroke.
- **Targeted Async Render & Image Cache Invalidation**: Completing an asynchronous MathJax formula, Mermaid diagram, or local image decode now invalidates only the affected document rows rather than dropping all cached row layouts.
- **Auto-Fit & Interactive Resizable Tables**: Live tables automatically size column widths proportionally to cell content and support interactive mouse dragging (and double-click reset) on internal column dividers.
- **Container Slash Menu & Atomic Command Replace**: `/` Slash Menu can now be triggered inside empty list items (`- /`), empty blockquotes (`> /`), and indented lines, features a visual block structure preview card, and executes via single-step `EditCommand.Replace` without transient model undo rollbacks.
- **Typography, Settings & Palette Refinements**: Refined heading typographic scale and CJK serif fallbacks, resolved bottom clipping of `Reduce Transparency` in Settings on initial open, and added semantic command icons, file tree icons, and inactive tab pills.

## 1.0.0 - 2026-11-18

First official Windows release of Aurink — a fast, lossless WYSIWYG Live Markdown desktop editor built in pure Go (`CGO_ENABLED=0`) on MyGo Native UI.

### Highlights
- **Lossless Live Markdown Engine**: Inline syntax markers reveal smoothly at the caret and fold back into clean editorial typography (`Newsreader` + `Noto Serif SC` + `JetBrains Mono`) without ever rewriting untouched bytes, line endings (`LF` / `CRLF` / mixed), or BOMs.
- **Extended Syntax & Rich Blocks**: GFM tables (with live toolbar & size grid), task lists, footnotes, YAML Front Matter header, `[TOC]`, `==highlight==`, `^sup^` / `~sub~`, `:emoji:`, `[[Wiki links]]`, offline MathJax formulas, Mermaid diagrams, and a strict safe HTML block subset (`<details><summary>`, aligned README badges, headings).
- **Focused Writing Modes**: Live WYSIWYG, Source code editor, Reading mode (`F7`), Focus mode, and Typewriter scrolling, plus Quick Open, Command Palette, `/` Slash Menu, floating Format Bar, Structure Rail, and Image Lightbox.
- **Workspace & Reliability**: Folder sidebar with live file watching, streaming full-text workspace search, atomic saves, autosave, crash recovery, 19-case external conflict handling, 5–64 MB large-file mode, and read-only GB18030 / GBK legacy file opening with explicit UTF-8 conversion.
- **Personalization & Updates**: Ink & Aurora light/dark themes, 4 accent hues, user custom JSON themes (`themes/*.json`), interactive shortcut recorder with conflict detection, English & Simplified Chinese UI, HTML/PDF export & print, and Ed25519-signed automatic updates via GitHub Releases.
