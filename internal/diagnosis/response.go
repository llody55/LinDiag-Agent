package diagnosis

import "encoding/json"

// AgentResponse 是 LLM 结构化输出的标准格式。
//
// 此类型曾定义在 internal/llm/types.go，现下沉到 diagnosis 包作为跨层
// 数据契约：LLM 客户端产出它、agent/session 消费它、report/engine 解析它、
// 规则引擎未来也会产出此结构的子集。下沉后 llm 与 report 均依赖 diagnosis
// 这个叶子包，避免 report 强耦合 llm 客户端实现。
//
// 当 API 支持 Structured Outputs (json_schema) 时，模型返回值将 100% 符合
// 此结构；当仅支持 JSON Mode (json_object) 时通过提示词引导；当两者均不
// 支持时 (none) 作为兜底仍尝试解析。
type AgentResponse struct {
	// 分析说明：AI 对当前状态的判断和解释（始终输出，即使有命令待执行）
	Analysis string `json:"analysis" jsonschema:"description=对当前系统状态的分析说明"`
	// 待执行命令列表，为空表示本轮无需执行命令
	Commands []CommandAction `json:"commands" jsonschema:"description=需要执行的命令列表，无需执行命令时为空数组"`
	// 结构化诊断发现：每轮可输出零到多条 Issue。
	// 仅在识别到明确问题时输出；常规查询/采集轮次可为空数组。
	Issues []Issue `json:"issues" jsonschema:"description=本轮识别到的结构化问题列表，无问题则为空数组"`
	// 是否为最终诊断结论。true 时表示诊断完成，Analysis 中应包含完整结论
	IsFinal bool `json:"is_final" jsonschema:"description=是否为最终诊断结论"`
}

// CommandAction 描述一条待执行的命令
type CommandAction struct {
	// 要执行的 shell 命令
	Command string `json:"command" jsonschema:"required,description=要执行的shell命令"`
	// 命令目的说明，帮助用户理解为何执行
	Purpose string `json:"purpose" jsonschema:"required,description=命令目的"`
	// AI 自评的预期风险等级，仅作参考，实际风险由安全分析器独立判定
	ExpectedRisk string `json:"expected_risk" jsonschema:"enum=safe,enum=low,enum=medium,enum=high,enum=critical,description=AI自评风险等级"`
}

// ToJSON 将 AgentResponse 序列化为字符串，用于存入聊天历史。
// 失败时返回空串（历史落盘不应因序列化失败阻塞诊断流程）。
func (r AgentResponse) ToJSON() string {
	data, err := json.Marshal(r)
	if err != nil {
		return ""
	}
	return string(data)
}
