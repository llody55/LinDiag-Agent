package llm

// 此文件曾定义 AgentResponse / CommandAction 两个跨层契约类型。
// 现已下沉到 internal/diagnosis 包（见 diagnosis/response.go），
// llm 仅保留客户端职责：调用 LLM API、降级、重试。
//
// 旧导入路径：llm.AgentResponse / llm.CommandAction
// 新导入路径：diagnosis.AgentResponse / diagnosis.CommandAction
//
// 为兼容历史并减少调用方改动，这里 re-export 同名别名；新代码建议直接
// 使用 diagnosis 包的类型，以便后续彻底移除本文件。
//
// 注意：不要在此文件添加新逻辑。新功能请直接使用 diagnosis 包。

import "github.com/LinDiag-Agent/internal/diagnosis"

// AgentResponse 兼容别名（别名指向 diagnosis.AgentResponse）。
//
// Deprecated: 使用 diagnosis.AgentResponse。本别名仅为减少调用方改动而保留，
// 后续版本将移除。
type AgentResponse = diagnosis.AgentResponse

// CommandAction 兼容别名（别名指向 diagnosis.CommandAction）。
//
// Deprecated: 使用 diagnosis.CommandAction。
type CommandAction = diagnosis.CommandAction
