# CR-002 · 投影的缓存语义

- 提出：FE-1（S1）· 兼容级别：冻结前修订；只改注释和语义说明，不改签名
- 状态：**修改后批准**（CEO，2026-11-01）。最终变更见文末「CEO 批复」，以批复为准

## 动机

03 §6.2 要求排版按 `投影修订` 缓存，但现有说明有三处与之冲突：

1. **绝对偏移。** `Segment.Src` 是绝对偏移，文档前部每次击键都让后面所有块的偏移失效。04 §4.3.4 没有说 `Sig` 是否受偏移影响。如果受影响，一次键入会使全文缓存作废。S1 的做法是 `Sig` 只取决于显示内容，缓存的投影在使用时平移（`rebase`）[实测：键入每键排版 1.03 块]。
2. **`Display` 的零拷贝。** `projection.go` 注释要求 `SegText/SegMarker` 的 `Display` 是「源码子串（零拷贝）」。Go 的子串会让整个字符串无法回收，而 `Text` 每次编辑都是新 string（03 §4.1）。缓存的投影和 `TextLayout` 的 span 会钉住各自版本的整份源码：在 1 MB 文档中保留的 512 个排版，最坏情况可钉住约 512 MB [源码推理，未实测]。S1 对每个投影克隆一次块字节解决。
3. **`Affinity` 在隐藏边界的含义没有定义。** `ToDisplay` 对隐藏片段两侧返回同一索引，`ToSource` 必须靠 `Affinity` 区分。不写明，mock 和真实实现可能不一致。

## 变更

```diff
 // contract/projection.go
 type Segment struct {
     Src  Range
     Kind SegKind
-    // Display：SegText/SegMarker 为源码子串（零拷贝）；SegReplaced 为替代文本；
+    // Display：SegText/SegMarker 与对应源码字节相等，但不得引用整份源码的子串，
+    // 因为投影会被长期缓存（实现可对一个块克隆一次）；SegReplaced 为替代文本；
     // SegObject 为 "￼"；SegHidden 为空串。
     Display string
```

```diff
     Revealed   bool
-    // Sig 是投影签名：内容或显隐变化时改变，作为排版缓存键的一部分。
+    // Sig 是投影签名：显示内容或显隐变化时改变，作为排版缓存键的一部分。
+    // 它不包含绝对偏移：块只是整体平移时 Sig 不变，排版缓存继续有效。
     Sig uint64
```

```diff
 const (
+    // Upstream：在隐藏片段的边界上停在它之前。
     Upstream   Affinity = -1
+    // Downstream：在隐藏片段的边界上越过它，落到下一个可见片段的起点。
+    // 块尾 Trailer 不被越过（见 CR-001）。
     Downstream Affinity = 1
 )
```

文档 04 §4.3.4 改为：「同一 `(块显示内容, Reveal)` 产生相同 `Sig`；块平移不改变 `Sig`；任一影响显示的输入变化必须改变 `Sig`。」

## 影响

| 模块 | 改动 |
|---|---|
| BE-1 `core/projection` | 按新语义实现；不增加接口 |
| FE-1 `views/liveedit` | 投影按 `Sig` 复用排版，缓存的 `Src` 自行平移 |
| QA | contracttest 增加：块前插入文字后 `Sig` 不变；`Upstream`/`Downstream` 在隐藏边界的取值 |

签名不变，其他模块不受影响。

## 过渡

S1 已按此语义实现（`liveedit/project.go`）。

## CEO 批复（2026-11-01）· 修改后批准

### 结论

| 项 | 结论 |
|---|---|
| 1 `Sig` 不含绝对偏移 | **批准，并补充两点**：`Sig` 只是排版缓存键；投影能否平移复用，不能由 `Sig` 决定 |
| 2 `Display` 不得引用整份源码 | **批准，按原文**。512 MB 是推理值 [源码推理]，结论不依赖这个数字：子串会钉住整份源码是 Go 的确定行为，克隆一个块的成本很低 |
| 3 `Affinity` 的含义 | **批准，改为下面的精确定义**，并与 CR-001 的 `Lead`/`Trailer` 衔接 |

### 对第 1 项的补充

CEO 查看了 S1 的实现 [源码 `spikes/s1-liveedit/liveedit/editor.go` `laidFor`、`project.go`]。S1 的缓存是按块和显隐两种形态分槽，块未被编辑时直接 `rebase` 复用。`Sig` 在 S1 里算出来了，但没有用作缓存键。所以"键入每键排版 1.03 块"的实测证明的是"按块复用 + 平移"可行，不是"按 `Sig` 复用"可行。由此定下：

1. **`Sig` 只用于排版缓存键。** 它覆盖所有影响排版的输出：`Kind`、`Level`、`Lang`、`Revealed`、`Containers`（不含其中的 `Block` 与 `TaskSrc`），以及每个片段的 `Kind`、`Display`、`Style`、`Object` 的全部字段。不含 `Block`、`Index` 和任何绝对偏移。
2. **`Sig` 相同不代表两个投影可以互换。** 例如 `&amp;` 与 `&#x26;` 的显示和样式相同，源码长度却分别是 5 和 6；如果把一个块的投影按 `Sig` 套用到另一个块，`ToSource` 会把光标映射到错误字节，编辑会写坏源码。
3. **平移复用的条件（写入 04 §4.3.6）。** `Project` 的输出（平移后）只取决于：
   - 该叶子 `[Lead.Start, Trailer.End)` 的字节和容器链；
   - `Reveal` 中与本块相关的部分；
   - 全局表：`LinkDefs`、`Footnotes`、`Outline`（`[TOC]` 用）、`Exts`、`ObjectLangs`。

   缓存的投影只有在本块未与任何 `Change` 相交、且全局表未变时才能平移复用，否则重新投影。S1 REPORT 第 3 条已指出 S1 没处理全局表变化。

### 最终变更（替代原提案的 diff）

```diff
 // contract/projection.go
 type Segment struct {
     Src  Range
     Kind SegKind
-    // Display：SegText/SegMarker 为源码子串（零拷贝）；SegReplaced 为替代文本；
+    // Display：SegText/SegMarker 与对应源码字节相等，但不得引用整份源码的子串，
+    // 因为投影会被长期缓存（实现可对一个块克隆一次）；SegReplaced 为替代文本；
     // SegObject 为 "￼"；SegHidden 为空串。
     Display string

     Revealed   bool
-    // Sig 是投影签名：内容或显隐变化时改变，作为排版缓存键的一部分。
+    // Sig 是排版缓存键：Kind、Level、Lang、Revealed、Containers（不含 Block、TaskSrc）与各片段的
+    // Kind、Display、Style、Object 的摘要，不含 Block、Index 与绝对偏移，块整体平移时不变。
+    // Sig 相同不代表 Src 映射相同；投影能否平移复用见 04 §4.3.6。
     Sig uint64

-type Affinity int8
+// Affinity 决定零宽边界上的取向。显示索引 d 处夹着连续的零宽片段（SegHidden，含 Lead
+// 与 Trailer）时，d 对应源码区间 [h0, h1]；ToSource 与 Snap 的结果都限制在
+// [Range.Start, Range.End] 内。
+type Affinity int8

 const (
+    // Upstream 取 h0：停在零宽片段之前。
     Upstream   Affinity = -1
+    // Downstream 取 h1：越过零宽片段，落到下一个可见片段的起点；不越过 Trailer。
     Downstream Affinity = 1
 )
```

### 04 文档修改（CEO 已改，见 04 §4.3）

- §4.3.2 增加：`Display` 不得引用 `Text.String()` 的底层字节。
- §4.3.4 改为：`Sig` 是排版缓存键，覆盖上面列出的输出，不含 `Block`、`Index` 与绝对偏移；任一影响排版的输出变化必须改变 `Sig`。
- §4.3.6 新增：投影的输入与平移复用条件（上面第 3 点）。
- §4.3.7 新增：`Affinity` 的定义（同上面 diff 中的注释）；`ToDisplay` 对零宽区间内的任何偏移返回同一个 `d`。

### 落地与验证

1. **INT**：与 CR-001 一起修改 `internal/contract`，打标签 `contract-v1.0-rc2`。
2. **BE-1**：真正的 `Sig` 由 `core/projection` 计算（BE-1.3）。S1 的哈希没有包含 `Level`、`Lang`、`Revealed`、`Containers`、`Object`，也没有给 `Display` 加长度前缀，不要照搬。
3. **QA**：contracttest 增加以下测试：
   - 在块前插入文字后，`Sig` 不变，投影等于原投影平移；
   - 用 `unsafe.StringData` 检查 `Display` 不指向 `Text.String()` 的底层数组；
   - 在 `**粗**` 与 `&amp;` 的隐藏边界上检查 `Upstream`/`Downstream` 的取值；
   - 修改别处的链接定义后，使用该引用的块投影随之改变。
4. **FE-1**：FE-1.1 的投影缓存按第 3 点的条件复用；`rebase` 也要平移 `Containers[i].TaskSrc`（S1 只平移了片段）。
