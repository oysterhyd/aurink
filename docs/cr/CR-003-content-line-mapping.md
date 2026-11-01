# CR-003 · AST 内容行与部分 Tab 的映射

- 提出：BE-1（S3），2026-11-01
- 状态：待 CEO 批复；本提案没有修改任何契约源码。
- 兼容级别：冻结前修订；新增字段可作次版本变更，但 `Content`/`Prefixes` 注释的澄清须与冻结一起确认。

## 动机与证据

[实测] `spikes/s3-ast/evidence/tests.txt`：CommonMark 652 例、GFM **全部** 670 例及 17 个自造样例，共 1,934 个叶子块、2,383 个内容行。仅删除前缀和标记并按行比较，通过 1,925/1,934 块；9 块失败涉及 10 个部分消耗 Tab 的内容行。段落/标题子集为 1,531/1,531。

例如 CommonMark #6 的 `>\t\tfoo`：引用和缩进代码仅消耗第二个 Tab 的一部分，解析器返回 `"  foo"`；源码删除一个字节前缀得到 `"foo"`，不能凭删除产生两个空格。[源码] v0.7.3 `block.go:addLine` 使用私有 `seg.pad` 表示这些空格；公开 `Line(i,k)` 返回合成后的行和原始字节起点。无需复制容器解析器或访问私有字段。

[实测] 多行引用的 `Content` 也只能是内容的**包围区间**：`> a\n> b` 的内容区间为 `a\n> b`，不是一个连续的去前缀子串。CR-001 的块间空白归属不能单独解决行内 Tab 映射。

## Go diff（提案）

```diff
 // contract/ast.go
+// BlockLine 描述解析器的一条内容行，不包含行终止符。
+// Source 为去掉该行前缀与块标记后的原始字节区间。
+// Padding 为部分消耗 Tab 剩下的空格数，范围 0..3；不改写源码。
+type BlockLine struct {
+    Source Range
+    Padding uint8
+}

 type Block struct {
     ...
-    // Content 是去掉标记后的内容（标题文字、代码正文、数学 TeX、Front Matter YAML）。
+    // Content 是内容源码的包围区间；多行时可含续行前缀和原始换行。
+    // 精确的逐行内容通过 Lines 取得。
     Content Range
     ...
-    // Prefixes 是叶子块内每一续行的容器前缀（"> "、列表缩进），WYSIWYG 中隐藏。
+    // Prefixes 是 Range 内各内容行被解析器剥离的行首字节：容器前缀、
+    // 结构缩进和内容前空白。首行祖先标记仍由祖先拥有，具体覆盖边界见 CR-001。
     Prefixes []Range
     ...
+    // Lines 按源码顺序列出内容行；无内容行的块为 nil。
+    // 第 k 行的解析器内容 = Padding 个空格 + Text[Source]。
+    // 表格的反斜杠管道、转义和实体由内联阶段解码。
+    Lines []BlockLine
 }
```

建议将 04 §4.2.4 的逐行断言明确为 `Repeat(" ", line.Padding) + Text.Slice(line.Source) == parser.Line(i,k)`。连接为 `parser.Lines(i)` 时，每条内容行追加 LF，包括没有尾换行的源码行；源文件保持原始 CRLF/LF/CR 字节。这里的 LF 仅用于比较解析内容，不进入 Text。

不建议把 Tab 归一化写回 Text，也不建议伪造 `Range` 的字节数。投影需要隐藏整个被消费的 Tab，并对剩余空格产生 `SegReplaced`，由 PosMapper 按 affinity 对齐原始 Tab 边界。

## 影响与过渡

- BE-1 `core/markdown`：用公开 `Line`/`LineCount` 建立映射；`Content` 继续保留便于切片的包围区间。`core/projection` 在代码等需要显示剩余缩进时使用 `Padding`。
- FE-1：通过统一投影获得剩余空格；若直接读取 `Block.Content`，需改用逐行内容或投影。CR-001 的 `Trailer`/空块设计须与这里的内容行独立组合，不能把祖先标记重复交给多个叶子。
- QA：mock 填充 Lines；contracttest 加部分 Tab、CRLF、无尾换行、Setext、嵌套列表/引用和空表格单元格的例子；分别断言原始字节保真和解析内容等价。
- BE-2 导出：`Content` 不是可直接输出的纯文本，优先使用 Inlines 或 Lines。

[实测] S3 使用私有 `contentLine{Source, Padding}` 过渡，1,934/1,934 块映射通过。[待验证] 新字段在正式 Projector/PosMapper 中的 Tab 光标映射以及与 CR-001 的统一覆盖规则，留待批复后的 BE-1.2/1.3；S3 不实现产品任务。
