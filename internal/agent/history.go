package agent

// history.go 承接从 internal/llm/history.go 迁出的聊天历史装配逻辑。
//
// 迁出原因：旧版 LoadDefaultChatHistory 在 llm 包内直接调用
// platform.GetSnapshot 并读取 stdin，造成两个问题：
//  1. llm 包反向依赖 platform（import 循环风险、违反分层）
//  2. I/O 逻辑钉死在 LLM 客户端层，职责越界
//
// 现在 history 装配是 agent 编排职责，与 Session、mode、env 同包，
// 可以自然依赖 platform / llm / diagnosis。
//
// 本次改造同时给每条消息打上 diagnosis.MessageKind 标签，替代过去
// report/engine.go 靠 strings.Contains("初始系统快照") 等魔法字符串
// 识别消息类型的脆弱方式。

import (
	"bufio"
	"strings"

	"github.com/LinDiag-Agent/internal/diagnosis"
	"github.com/LinDiag-Agent/internal/llm"
	"github.com/LinDiag-Agent/internal/output"
	"github.com/LinDiag-Agent/internal/platform"
)

// LoadDefaultChatHistory 加载默认聊天历史（系统提示 + 快照 + 用户需求）。
// reader 用于读取多行用户输入。
// systemPrompt 是当前模式的提示词；snapshotCmds 是模式与环境共同决定的快照命令；
// rules 是外置规则文件文本（未解析，仅拼接到 system 提示末尾）。
//
// 返回的消息序列：
//
//	[system: prompt+rules, user: 初始系统快照, user: 用户需求]
//
// 每条都带 diagnosis.MessageKind 标签，便于 trimHistory 保护前导块、
// 便于 report 引擎从历史中提取各部分数据（不再靠字符串前缀嗅探）。
func LoadDefaultChatHistory(reader *bufio.Reader, systemPrompt string, snapshotCmds []string, rules string) []diagnosis.Message {
	output.Promptln("\n正在采集系统快照（请稍等）...")
	history := []diagnosis.Message{
		diagnosis.NewMessage("system", systemPrompt+"\n"+rules, diagnosis.KindSystemPreamble),
		diagnosis.NewMessage("user", "初始系统快照:\n"+platform.GetSnapshot(snapshotCmds), diagnosis.KindSystemSnapshot),
	}

	output.Promptln("\n请输入现象描述/日志（输入 ok 结束，多行输入）:")
	var rawInput strings.Builder
	hasContent := false
	for {
		output.Prompt("> ")
		line, _ := reader.ReadString('\n')
		cleanLine := llm.CleanInput(line)
		trimmed := strings.TrimSpace(cleanLine)

		if strings.EqualFold(trimmed, "ok") {
			break
		}

		if trimmed != "" {
			hasContent = true
			rawInput.WriteString(cleanLine)
			rawInput.WriteString("\n")
		} else if hasContent {
			rawInput.WriteString("\n")
		}
	}
	history = append(history, diagnosis.NewMessage("user", "用户需求:\n"+rawInput.String(), diagnosis.KindUserRequirement))
	return history
}
