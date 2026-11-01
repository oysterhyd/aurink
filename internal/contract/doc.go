// Package contract 是 Aurink 各模块之间的契约：共享数据模型、服务接口、
// 事件与错误码。
//
// 规则：
//   - 只包含类型、接口、常量与无副作用的小型辅助函数；
//   - 只依赖标准库，不得 import 项目内其他包或任何 mygo 包；
//   - 偏移量一律为 UTF-8 字节偏移；UTF-16 只出现在 views 与 IME 边界；
//   - 修改规则见 docs/05-delivery-plan.md §4。
package contract

// ContractVersion 是契约版本。破坏性变更升主版本；新增类型/方法/字段升次版本。
const ContractVersion = "1.0.0"
