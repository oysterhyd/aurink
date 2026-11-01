# CR-001 · 块间空行的归属

- 提出：FE-1（S1）· 兼容级别：冻结前修订（`contract-v1.0` 之后则为主版本）
- 状态：待 CEO 批复

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
