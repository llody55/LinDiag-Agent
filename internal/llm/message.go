package llm

// 此文件曾定义 Message 与 ToJSON。
// Message 已下沉到 internal/diagnosis 包（见 diagnosis/message.go），
// llm 不再承担跨层持久化契约职责。
//
// 旧导入路径：llm.Message
// 新导入路径：diagnosis.Message
//
// 这里 re-export 同名别名以减少调用方改动；新代码建议直接使用 diagnosis 包。

import "github.com/LinDiag-Agent/internal/diagnosis"

// Message 兼容别名（别名指向 diagnosis.Message）。
//
// Deprecated: 使用 diagnosis.Message。本别名仅为减少调用方改动而保留，
// 后续版本将移除。
type Message = diagnosis.Message

// NewMessage 兼容包装：构造带 kind 的消息。
//
// Deprecated: 直接使用 diagnosis.NewMessage。
func NewMessage(role string, content string, kind diagnosis.MessageKind) diagnosis.Message {
	return diagnosis.NewMessage(role, content, kind)
}
