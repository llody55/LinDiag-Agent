package diagnosis

// MessageKind 标记消息在诊断流程中的语义角色。
//
// 引入此字段是为了替代过去靠 strings.Contains(content, "初始系统快照") 等
// 魔法字符串识别消息类型的脆弱方式：写入端（agent/handler、llm/history）与
// 读取端（report/engine）必须维持一致的文案，任何一处措辞变化都会让报告
// 解析失败、数据缺失。MessageKind 让契约从"文本特征"升级为"显式枚举"，
// 内容字符串不再承担类型识别职责。
//
// 该字段在旧历史文件中不存在；加载旧文件时 Kind 为空字符串，读取端会
// 回退到按内容特征识别（见 report.Engine.ExtractReportData 的兼容分支）。
type MessageKind string

const (
	// KindSystemPreamble 系统提示+规则前导块（role=system）
	KindSystemPreamble MessageKind = "system_preamble"
	// KindSystemSnapshot 初始系统快照（role=user）
	KindSystemSnapshot MessageKind = "system_snapshot"
	// KindUserRequirement 用户原始问题描述（role=user）
	// 该消息必须在历史 trim 时被保留，否则报告丢失用户原始诉求。
	KindUserRequirement MessageKind = "user_requirement"
	// KindCommandResult 命令执行结果回传给 AI（role=user）
	KindCommandResult MessageKind = "command_result"
	// KindUserFollowup 用户后续追问/补充（role=user）
	KindUserFollowup MessageKind = "user_followup"
	// KindAgentResponse assistant 的结构化响应 JSON（role=assistant）
	KindAgentResponse MessageKind = "agent_response"
	// KindAgentFiller 占位 user 消息（用于修复连续 assistant 消息）
	KindAgentFiller MessageKind = "agent_filler"
)

// Message 聊天消息 — 贯穿 agent/llm/report 的稳定持久化契约。
//
// 字段 Kind 是语义角色标记（见 MessageKind），用于替代魔法字符串识别；
// 在旧历史文件中可能缺失，读取端应做兼容处理。
type Message struct {
	Role    string      `json:"role"`
	Content string      `json:"content"`
	Kind    MessageKind `json:"kind,omitempty"`
}

// NewMessage 构造带 kind 的消息，避免调用方遗漏设置 Kind 字段。
func NewMessage(role string, content string, kind MessageKind) Message {
	return Message{Role: role, Content: content, Kind: kind}
}
