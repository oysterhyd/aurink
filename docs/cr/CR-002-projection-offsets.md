# CR-002 · 投影的缓存语义

- 提出：FE-1（S1）· 兼容级别：冻结前修订；只改注释和语义说明，不改签名
- 状态：待 CEO 批复

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
