package llm

// history.go 承载聊天历史相关的纯工具函数与辅助 LLM 调用。
//
// 重要变更（与旧版差异）：
//   - LoadDefaultChatHistory 已被移除，迁至 internal/agent/history.go。
//     原因：旧实现直接调用 platform.GetSnapshot 并读 stdin，使 llm 包
//     反向依赖 platform、且把 I/O 逻辑钉死在 LLM 客户端层。
//     history 装配是 agent 编排职责，不属于 LLM 客户端。
//   - Message / ToJSON 已下沉到 internal/diagnosis（见 message.go、response.go），
//     llm 仅保留兼容别名。
//   - CallAISimple 改用独立降级状态，避免辅助调用污染主会话格式。
//
// 此文件不应再引入 platform / bufio 等非 LLM 客户端依赖。

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/LinDiag-Agent/internal/diagnosis"
)

// CleanInput 清理输入内容（移除 ANSI 转义和控制字符）。
// 保留为包级函数以避免调用方改动；新代码建议移到专门的 input util 包。
func CleanInput(input string) string {
	reAnsi := regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)
	input = reAnsi.ReplaceAllString(input, "")
	reControl := regexp.MustCompile(`[\x00-\x1F\x7F]`)
	return reControl.ReplaceAllString(input, "")
}

// TruncateOutput 截断输出内容，保留头尾。
// 用于避免大输出（如 journalctl、sar）撑爆 LLM 上下文。
// TODO: maxChars 与 50 行两套阈值并存，后续应统一为按 token 或按字节单一策略。
func TruncateOutput(output string, maxChars int) string {
	if len(output) <= maxChars {
		return output
	}
	lines := strings.Split(output, "\n")
	if len(lines) > 50 {
		keep := 20
		head := strings.Join(lines[:keep], "\n")
		tail := strings.Join(lines[len(lines)-keep:], "\n")
		return head + fmt.Sprintf("\n\n... [输出过长，已截断 %d 行] ...\n\n", len(lines)-keep*2) + tail
	}
	keep := 18
	if len(lines) < keep*2 {
		keep = len(lines) / 2
	}
	head := strings.Join(lines[:keep], "\n")
	tail := strings.Join(lines[len(lines)-keep:], "\n")
	return head + fmt.Sprintf("\n\n... [输出过长，已截断 %d 行] ...\n\n", len(lines)-keep*2) + tail
}

// FixConsecutiveAssistantMessages 修复连续的 assistant 消息（部分 API 要求交替）。
// 现在使用 diagnosis.KindAgentFiller 标记填充消息，便于 report 引擎识别并跳过。
func FixConsecutiveAssistantMessages(messages []diagnosis.Message) []diagnosis.Message {
	if len(messages) < 2 {
		return messages
	}

	fixed := []diagnosis.Message{messages[0]}
	for i := 1; i < len(messages); i++ {
		current := messages[i]
		previous := fixed[len(fixed)-1]

		if current.Role == "assistant" && previous.Role == "assistant" {
			fixed = append(fixed, diagnosis.NewMessage("user", "继续分析", diagnosis.KindAgentFiller))
		}
		fixed = append(fixed, current)
	}
	return fixed
}

// CallAISimple 以最简方式调用 AI（用于辅助场景，如命令解释），返回纯文本。
//
// 重要：此函数不会污染主会话的 degradedFormat。
// 原因：高危命令解释是单次辅助调用，若其 API 不支持 json_schema，
// 不应让后续整个主诊断会话永久降级到 none 模式。
//
// 实现方式：进入时加锁并快照主会话降级状态；调用结束后恢复并解锁。
// 注意：这意味着 LLM 调用串行化。当前会话是单线程模型，不引入并发开销。
// 若未来需要并发，应改为每个调用方持有独立降级状态变量。
func CallAISimple(ctx context.Context, systemPrompt, userContent string) string {
	saveAndRestore := withIsolatedDegradedState()
	defer saveAndRestore.restore()

	resp, err := CallAI(ctx, []diagnosis.Message{
		diagnosis.NewMessage("system", systemPrompt, diagnosis.KindSystemPreamble),
		diagnosis.NewMessage("user", userContent, diagnosis.KindUserFollowup),
	})
	if err != nil {
		return fmt.Sprintf("[错误]: %v", err)
	}
	return resp.Analysis
}
