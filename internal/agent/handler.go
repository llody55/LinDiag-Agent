package agent

import (
	"bufio"
	"context"
	"fmt"
	"strings"

	"github.com/LinDiag-Agent/internal/config"
	"github.com/LinDiag-Agent/internal/diagnosis"
	"github.com/LinDiag-Agent/internal/llm"
	"github.com/LinDiag-Agent/internal/output"
	"github.com/LinDiag-Agent/internal/platform"
	"github.com/LinDiag-Agent/internal/safety"
)

// CommandHandler 统一的命令执行处理器，消除原 main.go 中三处重复的风险分级逻辑。
// 根据安全分析器的风险评估结果，执行命令并返回执行结果消息（用于追加到聊天历史）。
//
// 依赖注入（Phase 3 Task 13）：
//   - llmClient 用于高危命令的 AI 解释；若未注入，回退到 llm 包级 CallAISimple。
//   - analyzer 用于命令风险分级，仍由 NewCommandHandler 内部创建
//     （safety 包无副作用依赖，不需要再抽接口）。
type CommandHandler struct {
	analyzer  *safety.CommandAnalyzer
	reader    *bufio.Reader
	cfg       *config.Config
	llmClient LLMClient // 可选；为 nil 时回退到 llm.CallAISimple
}

// NewCommandHandler 创建命令处理器
func NewCommandHandler(reader *bufio.Reader, cfg *config.Config) *CommandHandler {
	return &CommandHandler{
		analyzer: safety.NewCommandAnalyzer(),
		reader:   reader,
		cfg:      cfg,
	}
}

// WithLLMClient 为 CommandHandler 注入 LLMClient。
// 未调用此方法时，handler 走 llm.CallAISimple 包级函数，保持旧行为。
func (h *CommandHandler) WithLLMClient(c LLMClient) *CommandHandler {
	h.llmClient = c
	return h
}

// explainCommand 对高危命令做 AI 解释。
// 优先用注入的 llmClient；未注入则回退 llm.CallAISimple。
func (h *CommandHandler) explainCommand(ctx context.Context, cmd string) string {
	if h.llmClient != nil {
		return h.llmClient.CallAISimple(ctx,
			"用简洁的中文解释这个命令：它做什么？为什么执行？风险如何？",
			fmt.Sprintf("命令：%s", cmd))
	}
	return llm.CallAISimple(ctx,
		"用简洁的中文解释这个命令：它做什么？为什么执行？风险如何？",
		fmt.Sprintf("命令：%s", cmd))
}

// CommandResult 单条命令的执行结果
type CommandResult struct {
	Command string
	Output  string
	Success bool
}

// HandleCommands 处理 AgentResponse 中的所有命令。
// 返回执行结果列表和是否执行了命令。
func (h *CommandHandler) HandleCommands(ctx context.Context, commands []diagnosis.CommandAction) ([]CommandResult, bool) {
	if len(commands) == 0 {
		return nil, false
	}

	results := make([]CommandResult, 0, len(commands))
	for _, cmdAction := range commands {
		cmd := strings.TrimSpace(cmdAction.Command)
		if cmd == "" {
			continue
		}

		result := h.handleSingleCommand(ctx, cmd, cmdAction.Purpose)
		if result != nil {
			results = append(results, *result)
		}
	}
	return results, len(results) > 0
}

// handleSingleCommand 处理单条命令 — 唯一的风险分级处理逻辑
// purpose 是 AI 给出的命令目的说明（精准，因为它知道当前上下文）
// 安全风险由安全分析器独立判定（不交给 AI，因为 AI 可能为了执行而淡化风险）
func (h *CommandHandler) handleSingleCommand(ctx context.Context, cmd, purpose string) *CommandResult {
	cmdInfo := h.analyzer.AnalyzeCommand(cmd)
	riskStr := riskLevelToString(cmdInfo.RiskLevel)

	switch cmdInfo.RiskLevel {
	case safety.RiskLevelSafe:
		output.CommandBox(cmd, purpose, riskStr)
		return h.executeAndRecord(ctx, cmd, riskStr)

	case safety.RiskLevelLow:
		// 低风险：直接执行，显示 AI 的 purpose + 分析器的补充说明
		displayPurpose := purpose
		if cmdInfo.Reason != "" && cmdInfo.Reason != "命令在安全白名单中" {
			displayPurpose = purpose + "（" + cmdInfo.Reason + "）"
		}
		output.CommandBox(cmd, displayPurpose, riskStr)
		return h.executeAndRecord(ctx, cmd, riskStr)

	case safety.RiskLevelMedium:
		// 中风险：需要确认
		// 说明用 AI 的 purpose（为什么要在当前上下文执行这个命令）
		// 风险用安全分析器的判定（安全判断不交给 AI）
		if config.GetUserPreferences().AutoConfirmMediumRisk {
			output.InfoMessage("中风险命令（已自动确认）: " + cmd)
		} else {
			output.ConfirmBox(cmd, purpose, cmdInfo.Reason, riskStr)
			output.Promptln(output.Yellow + "  选项: 1. Yes  2. Yes, and don't ask me again  3. No" + output.Reset)
			output.Prompt("Enter your choice: ")
			choice, _ := h.reader.ReadString('\n')
			choice = strings.TrimSpace(strings.ToLower(llm.CleanInput(choice)))
			if choice == "2" {
				config.SetAutoConfirmMediumRisk(true)
				output.SuccessMessage("已设置中风险命令自动确认，后续将不再询问")
			} else if choice != "y" && choice != "yes" && choice != "1" {
				return &CommandResult{Command: cmd, Output: "用户拒绝执行中风险命令", Success: false}
			}
		}
		return h.executeAndRecord(ctx, cmd, riskStr)

	case safety.RiskLevelHigh, safety.RiskLevelCritical:
		// 高风险：必须确认 + AI 解释（走注入的 llmClient，未注入由 explainCommand 回退）
		output.ConfirmBox(cmd, purpose, cmdInfo.Reason, riskStr)
		explain := h.explainCommand(ctx, cmd)
		output.Promptln(output.Red + "│" + output.Reset + " AI分析: " + strings.TrimSpace(explain))
		output.Promptln(output.Red + "└─────────────────────────────────────────────────────────────" + output.Reset)
		output.Prompt("您确定要我执行吗？(y/n): ")
		conf, _ := h.reader.ReadString('\n')
		if strings.TrimSpace(strings.ToLower(llm.CleanInput(conf))) != "y" {
			return &CommandResult{Command: cmd, Output: "用户拒绝执行危险命令", Success: false}
		}
		return h.executeAndRecord(ctx, cmd, riskStr)
	}

	return nil
}

// executeAndRecord 执行命令并返回结果
func (h *CommandHandler) executeAndRecord(ctx context.Context, cmd, riskStr string) *CommandResult {
	outputStr, err := platform.ExecuteCommandWithProgress(cmd)
	if err != nil {
		outputStr += "\n[Error]: " + err.Error()
		output.ResultBox(false, outputStr)
		return &CommandResult{Command: cmd, Output: outputStr, Success: false}
	}

	processed := llm.TruncateOutput(outputStr, 2800)
	output.ResultBox(true, processed)
	return &CommandResult{Command: cmd, Output: processed, Success: true}
}

// BuildHistoryMessage 将命令结果转为追加到聊天历史的消息
func BuildHistoryMessage(results []CommandResult) string {
	if len(results) == 0 {
		return ""
	}
	var sb strings.Builder
	for _, r := range results {
		if r.Success {
			sb.WriteString(fmt.Sprintf("执行结果 (%s):\n%s\n", r.Command, r.Output))
		} else {
			sb.WriteString(fmt.Sprintf("命令执行失败 (%s):\n%s\n请尝试其他方案获取所需信息。\n", r.Command, r.Output))
		}
	}
	return sb.String()
}

// riskLevelToString 风险等级转字符串
func riskLevelToString(level safety.RiskLevel) string {
	switch level {
	case safety.RiskLevelSafe:
		return "safe"
	case safety.RiskLevelLow:
		return "low"
	case safety.RiskLevelMedium:
		return "medium"
	case safety.RiskLevelHigh:
		return "high"
	case safety.RiskLevelCritical:
		return "critical"
	default:
		return "unknown"
	}
}
