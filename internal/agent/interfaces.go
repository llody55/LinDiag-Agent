package agent

import (
	"context"

	"github.com/LinDiag-Agent/internal/diagnosis"
)

// interfaces.go 定义 Session 的三大可注入依赖接口：
//
//   - LLMClient        抽象大模型调用（主会话推进 + 高危命令解释）
//   - CommandExecutor  抽象命令执行与风险分级处理
//   - ReportGenerator  抽象报告生成
//
// 设计原则（与"最小抽象"对齐）：
//
//  1. 接口只包含真正有副作用的调用。纯工具函数（CleanInput /
//     TruncateOutput / FixConsecutiveAssistantMessages）保留为 llm
//     包级函数，不进接口——它们无外部依赖、不会破坏隔离性。
//  2. 接口都接受 context.Context 作为首参，统一取消/超时语义。
//  3. 接口方法签名与现有包级函数对齐，让旧实现可以直接 adapter
//     起来，无需重写底层逻辑。
//  4. 接口放在 agent 包而非各业务包：agent 是组装/消费方，
//     依据依赖倒置原则，接口由消费方定义，实现方适配。

// LLMClient 抽象对 LLM 的两类调用。
//
//   - CallAI       主会话推进：传入完整历史，返回结构化诊断响应
//   - CallAISimple 辅助说明：高危命令解释、其他提示性短调用
//
// 实现需要保证 CallAISimple 不污染主会话降级状态
// （见 llm.CallAISimple 的 withIsolatedDegradedState 注释）。
type LLMClient interface {
	// CallAI 调用 LLM 推进诊断。
	// 返回 AgentResponse 含分析文本、待执行命令、结构化问题、是否最终结论。
	CallAI(ctx context.Context, messages []diagnosis.Message) (diagnosis.AgentResponse, error)

	// CallAISimple 以最简方式调用 LLM，返回纯文本分析。
	// 用于命令解释等辅助场景；不污染主会话格式降级状态。
	CallAISimple(ctx context.Context, systemPrompt, userContent string) string
}

// CommandExecutor 抽象命令执行能力。
//
// 当前唯一的实现是 *CommandHandler，它封装了：
//   - 安全分析器对每条命令的风险分级
//   - safe/low/medium/high/critical 五个风险等级对应的执行策略
//     （safe/low 自动执行、medium 用户确认或自动确认、high+ 强制确认并附带 AI 解释）
//   - 命令输出的截断与 ResultBox 展示
//
// 抽象出此接口后，测试可注入 FakeCommandExecutor 解除 shell 依赖。
type CommandExecutor interface {
	// HandleCommands 处理 AI 返回的一批命令。
	// 返回每条命令的执行结果列表，以及是否实际执行了至少一条命令。
	HandleCommands(ctx context.Context, commands []diagnosis.CommandAction) ([]CommandResult, bool)
}

// ReportGenerator 抽象报告生成。
//
// 当前唯一的实现在 report 包，支持 md / html / pdf（pdf 暂时中转 HTML）。
// 抽象出接口后，测试可注入 FakeReportGenerator 检查触发流程；未来也可以
// 接入不同的报告格式实现而不修改 Session 调用点。
type ReportGenerator interface {
	// Generate 从历史记录文件生成指定格式的报告。
	// historyFile 是 JSON 历史文件路径；format ∈ {"md","html","pdf"}。
	// 返回生成的报告文件绝对路径与错误。
	Generate(historyFile, format string) (string, error)
}
