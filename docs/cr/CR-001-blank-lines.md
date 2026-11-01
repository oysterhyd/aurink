# CR-001 · 块间空行的归属

- 提出：FE-1（S1）· 兼容级别：冻结前修订（`contract-v1.0` 之后则为主版本）
- 状态：**修改后批准**（CEO，2026-11-01）。最终变更见文末「CEO 批复」，以批复为准

## 动机

`AST.Leaves()` 是编辑器虚拟化的行序列（04 §4.2.3），`Project` 的片段覆盖叶子块的 `Range`（04 §4.3.1）。解析器的块不包含块间空行，由此产生两个问题 [实测 S1]：

1. **光标无处可放。** 在段落末尾回车得到 `a\n\n|`，或连按两次回车，光标都站在空行上，而空行不属于任何叶子。`LeafAt` 只会返回前一个叶子，于是光标回跳，用户无法在新段落里键入。
2. **块间字节无人负责。** 退格合并两块时要删除的区间、选区矩形和 `SegHidden` 都需要知道块尾的终止符和分隔空行，而它们落在所有 `Range` 之外。

S1 在 `liveedit/document.go` 用私有分块解决，规则经随机编辑测试验证：叶子后的第一个空行是它的隐藏分隔符；其后每个空行是一个独立的空块；文档以分隔空行结尾时，末尾再加一个空块。

这条规则涉及 `core/editing`（Enter、Backspace 的上下文），不应由 `liveedit` 私有。

## 变更

```diff
 // contract/ast.go
 const (
     ...
     BlockFrontMatter
     BlockTOC
+    // BlockBlank 是一个没有内容的叶子：分隔空行之外的每个空行，以及以分隔空行
+    // 结尾的文档末尾。光标可以停在这里。
+    BlockBlank
 )

 type Block struct {
     ...
     // Open 表示块在文档末尾未闭合（如缺少闭合围栏）。
     Open bool
+    // Trailer 是叶子块之后、下一个叶子块之前的源码：行终止符和至多一个分隔空行。
+    // 所有叶子的 Range ∪ Trailer 无缝覆盖整个文档。
+    Trailer Range
 }
```

文档 04 §4.2 增加不变式：

> 8. 叶子块的 `[Range.Start, Trailer.End)` 首尾相接，覆盖 `[0, len(Text))`。`LeafAt(offset)` 对任何偏移都返回拥有它的叶子。

04 §4.3.1 改为：片段覆盖 `[Range.Start, Trailer.End)`，`Trailer` 部分为 `SegHidden`。

## 影响

| 模块 | 改动 |
|---|---|
| BE-1 `core/markdown` | 计算 `BlockBlank` 和 `Trailer`，可取自 `spikes/s1-liveedit/liveedit/document.go` 的 `partition`。BlockID 规则同样适用于空块 |
| BE-1 `core/projection` | 片段覆盖 `Trailer`；`Snap` 不落在 `Trailer` 内 |
| BE-1 `core/editing` | Enter 插入两个换行；Backspace 在块首时删除上一块的 `Trailer` |
| FE-2 大纲、BE-2 导出 | 跳过 `BlockBlank`（导出不受影响，因为 `Text` 未变） |
| QA | mock 与 contracttest 增加不变式 8；`mock.AST` 需要同样计算 |

## 过渡

批复前，S1 和 FE-1.1 使用 `liveedit` 私有的分块。

## CEO 批复（2026-11-01）· 修改后批准

### 结论

方向批准：源码里每个字节都要有一个叶子负责，光标要能停在空行上，这条规则归 `core/markdown`，不归 `liveedit`。但原提案的不变式只在顶层段落和标题上验证过（S1 只切分 `Children(0)`，容器整块按源码显示，见 S1 REPORT「尚未覆盖」），**在容器、表格和链接定义上不成立**。CEO 用 MyGo 解析器实测 [实测，2026-11-01]：

| 源码 | 解析器给出的叶子 | 原提案下无人负责的字节 |
|---|---|---|
| `- a\n- b\n` | 段落 `a` [2,3)、段落 `b` [6,7) | 第二项的 `- ` [4,6) |
| `> a\n>\n> b\n` | 段落 `a` [2,3)、段落 `b` [8,9) | `>` 行不是纯空白，不算分隔行；`> ` [6,8) |
| `- a\n- \n` | 只有段落 `a`；第二项 [4,5) 没有子块 | 空列表项上光标无处可放（列表里按回车的常见情况） |
| `[x]: /u\n\npara\n` | 只有段落 `para` | 链接定义 [0,7) 不属于任何块 |
| `\| a \| b \|\n\|---\|---\|\n…` | 单元格 `a` [2,3)、`b` [6,7) | 单元格之间的 ` \| `、行尾 ` \|`、分隔行 |

因此增加 `Lead`、把 `Trailer` 推广到"所在行的剩余部分"、为空容器和链接定义各补一种叶子。

### 最终变更（替代原提案的 diff）

```diff
 // contract/ast.go
     BlockFrontMatter
     BlockTOC
+    // BlockBlank 是没有内容的叶子，让光标能停在空行上（产生位置见 04 §4.2.9）。
+    BlockBlank
+    // BlockLinkDef 是链接引用定义所在的连续行。解析器不为它们产生块；按源码淡色显示。
+    BlockLinkDef
 )

 type Block struct {
     ...
     // Open 表示块在文档末尾未闭合（如缺少闭合围栏）。
     Open bool
+    // Lead 只用于叶子块：上一个叶子的 Trailer 之后、本叶子 Range 之前的源码，即首行上的
+    // 容器语法与缩进（"> "、"- [ ] "、表格的 "| "、表格分隔行）。Lead.End == Range.Start。
+    Lead Range
+    // Trailer 只用于叶子块：本叶子是所在行最后一个叶子时，为该行 Range.End 之后的剩余字节
+    // （如表格行尾的 " |"）、行终止符，以及至多一个紧随的分隔行（去掉容器前缀后只剩空白的行）；
+    // 否则为空区间（如表格中非末列的单元格）。Trailer.Start == Range.End。
+    Trailer Range
 }

 // AST 接口
-    // LeafAt 返回包含 offset 的叶子块；offset 落在块间空白时返回其前一个叶子块，
-    // ...
+    // LeafAt 返回 [Lead.Start, Trailer.End) 包含 offset 的叶子块；offset 等于源码长度时
+    // 返回最后一个叶子。
     LeafAt(offset int) int
```

新常量追加在 `BlockTOC` 之后，既有常量的值不变。

### 04 文档修改（CEO 已改，见 04 §4.2、§4.3）

- §4.2.3 叶子列表加入 `BlockBlank`、`BlockLinkDef`。
- §4.2.8 覆盖不变式：按 `Leaves()` 顺序，第一个叶子的 `Lead.Start == 0`；每个叶子 `Lead.End == Range.Start`、`Range.End == Trailer.Start`；下一个叶子的 `Lead.Start` 等于本叶子的 `Trailer.End`；最后一个叶子的 `Trailer.End` 等于源码长度。
- §4.2.9 `BlockBlank` 只在以下位置产生：
  1. 叶子的 `Trailer` 已含一个分隔行后，其后每多一个分隔行产生一个（`Lead` 为该行容器前缀，`Range` 为剩余空白，`Trailer` 为行终止符）；
  2. 没有叶子后代的容器（空列表项、空引用、空脚注定义）产生一个子块，`Range` 是容器语法之后的空区间；
  3. 最后一个叶子的 `Trailer` 以分隔行结尾时，在源码末尾追加一个三个区间都为空的；
  4. 空文档恰有一个，三个区间都是 `[0,0)`。
  其 `Parent` 是包含该行的最内层容器（没有则为根）；BlockID 规则（§4.2.6）同样适用。
- §4.3.1 片段覆盖 `[Lead.Start, Trailer.End)`；`Lead`、`Trailer` 一律为 `SegHidden`，容器语法的视觉由 `ContainerDecor` 表达。

### 不纳入契约的部分

原提案"Enter 插入两个换行、Backspace 删除上一块的 Trailer"是编辑规则，归 `core/editing`（03 §4.5），以黄金测试为准：段落中 Enter 插入两个主导换行符；列表、引用中按容器规则（续项、退出容器）处理，不是简单删除 `Trailer`。

### 落地与验证

1. **INT**：按上面的 diff 修改 `internal/contract`，与 CR-002 一起合并，打标签 `contract-v1.0-rc2`。`spikes/01-contract-rc` 保持原样。
2. **BE-1（S3，M0 前必须完成）**：在 S3 中按本批复计算 `Lead`、`Trailer`、`BlockBlank`、`BlockLinkDef`，在全部语料（含列表、引用、表格、链接定义、空列表项、CRLF、Front Matter）和随机编辑序列上验证 §4.2.8、§4.2.9，结果写进 S3 REPORT。不成立的情况写成对本批复的修订提案，M0 冻结前处理。
3. **QA**：`mock.AST` 按同样规则计算；contracttest 增加 §4.2.8、§4.2.9 和上表 5 个例子。
4. **FE-1**：FE-1.1 起删除 `liveedit` 私有的分块，改用 AST 的 `Lead`、`Trailer`。
