package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/LinDiag-Agent/internal/llm"
	"github.com/LinDiag-Agent/internal/platform"
	"github.com/LinDiag-Agent/internal/report"
	"github.com/LinDiag-Agent/internal/safety"
)

const (
	Red    = "\033[31m"
	Green  = "\033[32m"
	Yellow = "\033[33m"
	Blue   = "\033[34m"
	Cyan   = "\033[36m"
	Bold   = "\033[1m"
	Reset  = "\033[0m"
)

// ==================== 模式定义 ====================
type Mode struct {
	ID           int
	Name         string
	SystemPrompt string
	SnapshotCmds []string
}

var Modes = []Mode{
	{
		ID:           1,
		Name:         "跨平台故障诊断专家 (专业版)",
		SystemPrompt: "## Role\n你是一个顶级多平台 SRE 诊断专家。支持 Windows (CMD/PowerShell)、Linux (Ubuntu, CentOS, Debian) 及国产系统 (Kylin, UOS)。\n\n\n## Workflow\n1. **环境侦测**：查看 Snapshot 中的 OS 类型。若是 Linux，通过 /etc/os-release 确认具体发行版；若是 Windows，确认是 CMD 还是 PowerShell。\n2. **多轮探测**：执行至少 3 次针对当前平台的 EXEC: 命令。禁止输出非当前平台的语法（例如在 Windows 上输出 ls）。\n3. **拒绝幻觉**：严禁输出带占位符的示例（如 <pod-name>, [ip]）。必须基于前序查询获取的实际 ID/名称。\n4. **决策链**：Thought -> EXEC -> Observation -> Thought -> FINAL。\n\n\n## Constraints\n- **禁止 Markdown 代码块**：EXEC: 必须独立成行，且不能被 ``` 包裹。\n- **动态路径**：Windows 路径需正确处理反斜杠，Kylin 等系统需注意权限限制。\n- **Final 结构**：必须包含 [根本原因]、[风险评估]、[平台专属修复步骤]、[验证命令]。\n\n\n## Output Example\nThought: 检测到系统为 Kylin V10。需要检查核心服务状态。\nEXEC: systemctl status sshd",
		SnapshotCmds: []string{
			"uname -a || ver",
			"cat /etc/os-release || systeminfo | findstr /B /C:\"OS Name\" /C:\"OS Version\"",
			"uptime || net stats srv",
			"free -h || wmic OS get FreePhysicalMemory",
		},
	},
	{
		ID:           2,
		Name:         "智能运维助手 (全平台通用)",
		SystemPrompt: "## Role\n你是一个跨平台运维实战助手。支持 Linux/Windows/Kylin/UOS。\n\n\n## Execution Rules\n1. **先查后说**：查询类请求（如\"谁占用了80端口\"、\"查IP归属地\"）必须先执行 EXEC:。\n   - Linux: EXEC:ss -antp | grep :80\n   - Windows: EXEC:netstat -ano | findstr :80\n2. **拒绝占位符**：绝对禁止输出带有尖括号 < > 或方括号 [ ] 的命令。如果不知道具体对象名，先执行 ls 或 get 获取。\n3. **交互逻辑**：如果是普通聊天，正常回复；涉及系统改动，必须使用 EXEC: 并详细说明风险。\n\n\n## Action Guidance\n- Windows 优先使用 PowerShell 指令（若环境支持）。\n- 国产系统注意适配 yum/apt/dnf 命令。\n- 任何情况下禁止生成猜测的数据。",
		SnapshotCmds: []string{"hostname", "whoami"},
	},
}

func main() {
	// 初始化 LLM 模块
	if err := llm.Init(); err != nil {
		fmt.Printf("❌ 初始化 LLM 模块失败: %v\n", err)
		return
	}

	// 创建平台实例
	p := platform.NewPlatform()

	// 加载白名单
	if err := safety.LoadWhitelist("whitelist.txt"); err != nil {
		fmt.Printf("⚠️ 加载白名单文件失败: %v，使用默认白名单\n", err)
	} else {
		fmt.Println("✅ 已加载白名单")
	}

	// 检查 API Key 配置
	if llm.GetConfig().LLM.APIKey == "" || llm.GetConfig().LLM.APIKey == "你的API_KEY" {
		fmt.Println("⚠️ 使用默认 API_KEY，可能存在使用限制")
	}

	// 处理命令行参数
	var historyFile string

	if len(os.Args) > 1 {
		if os.Args[1] == "load" && len(os.Args) > 2 {
			historyFile = os.Args[2]
			fmt.Printf("ℹ️ 尝试加载历史记录: %s\n", historyFile)
		} else if os.Args[1] == "report" && len(os.Args) > 3 {
			// 生成报告功能
			historyFile = os.Args[2]
			format := os.Args[3]
			if format != "md" && format != "html" && format != "pdf" {
				fmt.Println("❌ 不支持的报告格式，支持的格式: md, html, pdf")
				return
			}
			report.GenerateReport(historyFile, format)
			return
		} else {
			// 显示帮助信息
			fmt.Println("=== LinDiag-Agent v3.1 多场景通用运维专家 ===")
			fmt.Println("使用方法:")
			fmt.Println("  ./lindiag-agent               # 正常启动")
			fmt.Println("  ./lindiag-agent load <file>   # 加载历史记录文件继续对话")
			fmt.Println("  ./lindiag-agent report <file> <format>   # 从历史记录生成报告 (格式: md, html, pdf)")
			return
		}
	}

	reader := bufio.NewReader(os.Stdin)
	fmt.Println("=== LinDiag-Agent v3.1 多场景通用运维专家 ===")

	rules := ""
	if b, err := os.ReadFile("rules.txt"); err == nil {
		rules = string(b)
		fmt.Println("ℹ️ 已加载本地 rules.txt")
	}

	// 检测 Docker 和 Kubernetes 环境
	if out, _ := p.ExecuteCommand("docker info >/dev/null 2>&1 && echo docker"); strings.Contains(out, "docker") {
		fmt.Println("检测到 Docker 环境")
	} else if out, _ := p.ExecuteCommand("kubectl version --client >/dev/null 2>&1 && echo k8s"); strings.Contains(out, "k8s") {
		fmt.Println("检测到 Kubernetes 环境")
	}

	fmt.Println("\n请选择工作模式：")
	for i, m := range Modes {
		fmt.Printf("%d. %s\n", i+1, m.Name)
	}
	fmt.Print("\n请输入数字 (1-2): ")

	choiceStr, _ := reader.ReadString('\n')
	choiceStr = strings.TrimSpace(choiceStr)
	choice := 1
	fmt.Sscanf(choiceStr, "%d", &choice)
	if choice < 1 || choice > len(Modes) {
		choice = 1
	}

	selectedMode := Modes[choice-1]
	fmt.Printf("\n🚀 已进入【%s】\n", selectedMode.Name)

	var chatHistory []llm.Message

	// 尝试加载历史记录
	if historyFile != "" {
		if data, err := os.ReadFile(historyFile); err == nil {
			if err := json.Unmarshal(data, &chatHistory); err == nil {
				// 修复连续的assistant消息
				fixedHistory := llm.FixConsecutiveAssistantMessages(chatHistory)
				if len(fixedHistory) != len(chatHistory) {
					fmt.Println("ℹ️ 已修复历史记录中的连续assistant消息")
					chatHistory = fixedHistory
				}

				fmt.Println("✅ 成功加载历史记录")
				// 加载历史记录后，添加用户输入以继续对话
				fmt.Println("\n🔄 历史记录已加载，准备继续对话...")
				fmt.Println("请输入您的问题或命令 (输入 'exit' 退出):")
				line, _ := reader.ReadString('\n')
				input := llm.CleanInput(line)
				trimmedInput := strings.TrimSpace(input)
				if trimmedInput == "" {
					input = "continue"
				}
				chatHistory = append(chatHistory, llm.Message{Role: "user", Content: input})
			} else {
				fmt.Printf("❌ 历史记录文件格式错误: %v\n", err)
				// 加载失败，使用默认流程
				chatHistory = llm.LoadDefaultChatHistory(reader, selectedMode.ID, selectedMode.SystemPrompt, selectedMode.SnapshotCmds, rules, p)
			}
		} else {
			fmt.Printf("❌ 无法读取历史记录文件: %v\n", err)
			// 加载失败，使用默认流程
			chatHistory = llm.LoadDefaultChatHistory(reader, selectedMode.ID, selectedMode.SystemPrompt, selectedMode.SnapshotCmds, rules, p)
		}
	} else {
		// 没有提供历史记录文件，使用默认流程
		chatHistory = llm.LoadDefaultChatHistory(reader, selectedMode.ID, selectedMode.SystemPrompt, selectedMode.SnapshotCmds, rules, p)
	}

	// ==================== 诊断循环（已彻底简化） ====================
	if selectedMode.ID == 2 {
		// 智能模式（聊天模式）- 无限循环，直到用户输入exit
		for {
			const maxKeep = 8
			if len(chatHistory) > maxKeep+4 {
				preserved := make([]llm.Message, 4)
				copy(preserved, chatHistory[:4])
				chatHistory = append(preserved, chatHistory[len(chatHistory)-maxKeep:]...)
			}

			response := llm.CallAI(chatHistory)
			chatHistory = append(chatHistory, llm.Message{Role: "assistant", Content: response})

			// 处理EXEC命令的函数
			handleExecCommands := func() bool {
				hasExec := false
				if strings.Contains(response, "EXEC:") || strings.Contains(response, "```exec") {
					hasExec = true
					lines := strings.Split(response, "\n")
					cmds := []string{}
					inExecBlock := false
					for _, line := range lines {
						trimmed := strings.TrimSpace(line)
						// 检查是否是EXEC:格式
						if strings.HasPrefix(trimmed, "EXEC:") {
							cmd := strings.TrimSpace(strings.TrimPrefix(trimmed, "EXEC:"))
							// 移除可能的反引号
							cmd = strings.Trim(cmd, "`")
							if cmd != "" {
								cmds = append(cmds, cmd)
							}
						}
						// 检查是否是```exec格式
						if strings.HasPrefix(trimmed, "```exec") {
							// 处理```exec:command格式
							if strings.HasPrefix(trimmed, "```exec:") {
								cmd := strings.TrimSpace(strings.TrimPrefix(trimmed, "```exec:"))
								cmd = strings.TrimRight(cmd, "`")
								if cmd != "" {
									cmds = append(cmds, cmd)
								}
							} else {
								// 处理```exec代码块格式
								inExecBlock = true
							}
							continue
						}
						if inExecBlock && strings.HasPrefix(trimmed, "```") {
							inExecBlock = false
							continue
						}
						if inExecBlock && trimmed != "" {
							cmd := strings.TrimSpace(trimmed)
							// 移除可能的反引号
							cmd = strings.Trim(cmd, "`")
							if cmd != "" {
								cmds = append(cmds, cmd)
							}
						}
					}

					analyzer := safety.NewCommandAnalyzer()
					for _, cmd := range cmds {
						// 分析命令风险（使用AI增强分析）
						riskLevel, reason, _ := analyzer.AnalyzeCommandWithAI(cmd)

						// 根据风险级别处理
						switch riskLevel {
						case safety.RiskLevelSafe:
							// 安全命令，直接执行
							fmt.Printf("\n🔍 我需要执行这个查询来获取信息: %s\n", cmd)
							output, err := p.ExecuteCommand(cmd)
							if err != nil {
								output += "\n[Error]: " + err.Error()
								fmt.Printf("我获取到的结果:\n%s\n", output)
								// 将失败信息添加到聊天历史，要求AI尝试其他方案
								chatHistory = append(chatHistory, llm.Message{Role: "user", Content: fmt.Sprintf("命令执行失败 (%s):\n%s\n请尝试其他方案获取所需信息。", cmd, output)})
							} else {
								processed := llm.TruncateOutput(output, 2800)
								fmt.Printf("我获取到的结果:\n%s\n", processed)
								chatHistory = append(chatHistory, llm.Message{Role: "user", Content: "执行结果 (" + cmd + "):\n" + processed})
							}

						case safety.RiskLevelLow:
							// 低风险命令，显示提示后执行
							fmt.Printf("\nℹ️ 我注意到这是一个低风险命令: %s\n我将执行: %s\n", reason, cmd)
							fmt.Println("我正在执行...")
							output, err := p.ExecuteCommand(cmd)
							if err != nil {
								output += "\n[Error]: " + err.Error()
								fmt.Printf("我获取到的结果:\n%s\n", output)
								// 将失败信息添加到聊天历史，要求AI尝试其他方案
								chatHistory = append(chatHistory, llm.Message{Role: "user", Content: fmt.Sprintf("命令执行失败 (%s):\n%s\n请尝试其他方案获取所需信息。", cmd, output)})
							} else {
								processed := llm.TruncateOutput(output, 2800)
								fmt.Printf("我获取到的结果:\n%s\n", processed)
								chatHistory = append(chatHistory, llm.Message{Role: "user", Content: "执行结果 (" + cmd + "):\n" + processed})
							}

						case safety.RiskLevelMedium:
							// 中风险命令，需要用户确认
							fmt.Printf("\n⚠️ 我发现这是一个中风险命令: %s\n我需要执行: %s\n", reason, cmd)
							fmt.Print("我需要执行这个命令，您同意吗？(y/n): ")
							conf, _ := reader.ReadString('\n')
							if strings.TrimSpace(strings.ToLower(llm.CleanInput(conf))) != "y" {
								chatHistory = append(chatHistory, llm.Message{Role: "user", Content: "用户拒绝执行中风险命令: " + cmd})
								continue
							}
							output, err := p.ExecuteCommand(cmd)
							if err != nil {
								output += "\n[Error]: " + err.Error()
								fmt.Printf("我获取到的结果:\n%s\n", output)
								// 将失败信息添加到聊天历史，要求AI尝试其他方案
								chatHistory = append(chatHistory, llm.Message{Role: "user", Content: fmt.Sprintf("命令执行失败 (%s):\n%s\n请尝试其他方案获取所需信息。", cmd, output)})
							} else {
								processed := llm.TruncateOutput(output, 2800)
								fmt.Printf("我获取到的结果:\n%s\n", processed)
								chatHistory = append(chatHistory, llm.Message{Role: "user", Content: "执行结果 (" + cmd + "):\n" + processed})
							}

						case safety.RiskLevelHigh, safety.RiskLevelCritical:
							// 高风险命令，需要用户确认并提供更多信息
							fmt.Printf("\n🚨 我发现这是一个高风险命令: %s\n我需要执行: %s\n", reason, cmd)

							// 已经通过AI分析获取了详细信息，直接询问用户
							fmt.Print("我需要执行这个命令，您同意吗？(y/n): ")
							conf, _ := reader.ReadString('\n')
							if strings.TrimSpace(strings.ToLower(llm.CleanInput(conf))) != "y" {
								chatHistory = append(chatHistory, llm.Message{Role: "user", Content: "用户拒绝执行危险命令: " + cmd})
								continue
							}
							output, err := p.ExecuteCommand(cmd)
							if err != nil {
								output += "\n[Error]: " + err.Error()
								fmt.Printf("我获取到的结果:\n%s\n", output)
								// 将失败信息添加到聊天历史，要求AI尝试其他方案
								chatHistory = append(chatHistory, llm.Message{Role: "user", Content: fmt.Sprintf("命令执行失败 (%s):\n%s\n请尝试其他方案获取所需信息。", cmd, output)})
							} else {
								processed := llm.TruncateOutput(output, 2800)
								fmt.Printf("我获取到的结果:\n%s\n", processed)
								chatHistory = append(chatHistory, llm.Message{Role: "user", Content: "执行结果 (" + cmd + "):\n" + processed})
							}
						}
					}
				}
				return hasExec
			}

			// 智能模式的用户输入处理函数
			getUserInput := func() string {
				fmt.Print("\n等待用户输入（提示，默认继续交互，1.打印报告，2.退出 > ")
				line, _ := reader.ReadString('\n')
				input := llm.CleanInput(line)
				trimmedInput := strings.TrimSpace(input)

				if trimmedInput == "" {
					return "继续分析"
				}

				if trimmedInput == "1" || trimmedInput == "report" || trimmedInput == "报告" || strings.ToLower(trimmedInput) == "r" || strings.ToLower(trimmedInput) == "rep" {
					historyFilename := fmt.Sprintf("history_%s.json", time.Now().Format("20060102_150405"))
					historyData, _ := json.Marshal(chatHistory)
					_ = os.WriteFile(historyFilename, historyData, 0644)
					fmt.Printf("✅ 聊天历史已保存: %s\n", historyFilename)

					fmt.Print("\n报告格式 (1.MD 2.HTML 3.PDF) > ")
					reportChoiceStr, _ := reader.ReadString('\n')
					reportChoiceStr = strings.TrimSpace(reportChoiceStr)

					reportChoice := 1
					fmt.Sscanf(reportChoiceStr, "%d", &reportChoice)
					if reportChoice < 1 || reportChoice > 3 {
						reportChoice = 1
					}

					switch reportChoice {
					case 1:
						report.GenerateReport(historyFilename, "md")
					case 2:
						report.GenerateReport(historyFilename, "html")
					case 3:
						report.GenerateReport(historyFilename, "pdf")
					}

					fmt.Print("\n等待用户输入（提示，默认继续交互，1.打印报告，2.退出 > ")
					line, _ := reader.ReadString('\n')
					input := llm.CleanInput(line)
					return strings.TrimSpace(input)
				}

				if trimmedInput == "2" || strings.ToLower(trimmedInput) == "exit" || strings.ToLower(trimmedInput) == "quit" || strings.ToLower(trimmedInput) == "退出" || strings.ToLower(trimmedInput) == "q" {
					return "exit"
				}

				return trimmedInput
			}

			if strings.Contains(response, "EXEC:") || strings.Contains(response, "```exec") {
				// 如果包含EXEC命令或```exec代码块，只执行命令，不显示幻觉结果
				handleExecCommands()
				// 执行完命令后，继续下一次循环以获取AI的分析
				continue
			} else {
				// 显示AI的响应
				fmt.Println("AI 响应:")
				fmt.Println(response)

				// 获取用户输入
				input := getUserInput()
				if input == "exit" {
					break
				}

				// 将用户输入添加到聊天历史
				userInput := input
				// 如果是数字，添加更多上下文
				if len(input) == 1 && input >= "1" && input <= "9" {
					userInput = "选择选项 " + input
				}
				chatHistory = append(chatHistory, llm.Message{Role: "user", Content: userInput})
			}
		}
	} else {
		// 故障诊断模式的有限循环
		for i := 0; i < 10; i++ {
			const maxKeep = 8
			if len(chatHistory) > maxKeep+4 {
				preserved := make([]llm.Message, 4)
				copy(preserved, chatHistory[:4])
				chatHistory = append(preserved, chatHistory[len(chatHistory)-maxKeep:]...)
			}

			response := llm.CallAI(chatHistory)
			chatHistory = append(chatHistory, llm.Message{Role: "assistant", Content: response})

			// 处理EXEC命令
			if strings.Contains(response, "EXEC:") || strings.Contains(response, "```exec") {
				lines := strings.Split(response, "\n")
				cmds := []string{}
				inExecBlock := false
				for _, line := range lines {
					trimmed := strings.TrimSpace(line)
					// 检查是否是EXEC:格式
					if strings.HasPrefix(trimmed, "EXEC:") {
						cmd := strings.TrimSpace(strings.TrimPrefix(trimmed, "EXEC:"))
						// 移除可能的反引号
						cmd = strings.Trim(cmd, "`")
						if cmd != "" {
							cmds = append(cmds, cmd)
						}
					}
					// 检查是否是```exec格式
					if strings.HasPrefix(trimmed, "```exec") {
						// 处理```exec:command格式
						if strings.HasPrefix(trimmed, "```exec:") {
							cmd := strings.TrimSpace(strings.TrimPrefix(trimmed, "```exec:"))
							cmd = strings.TrimRight(cmd, "`")
							if cmd != "" {
								cmds = append(cmds, cmd)
							}
						} else {
							// 处理```exec代码块格式
							inExecBlock = true
						}
						continue
					}
					if inExecBlock && strings.HasPrefix(trimmed, "```") {
						inExecBlock = false
						continue
					}
					if inExecBlock && trimmed != "" {
						cmd := strings.TrimSpace(trimmed)
						// 移除可能的反引号
						cmd = strings.Trim(cmd, "`")
						if cmd != "" {
							cmds = append(cmds, cmd)
						}
					}
				}

				analyzer := safety.NewCommandAnalyzer()
				for _, cmd := range cmds {
					// 分析命令风险（使用AI增强分析）
					riskLevel, reason, _ := analyzer.AnalyzeCommandWithAI(cmd)

					// 根据风险级别处理
					switch riskLevel {
					case safety.RiskLevelSafe:
						// 安全命令，直接执行
						fmt.Printf("\n🔍 我需要执行这个查询来获取信息: %s\n", cmd)
						output, err := p.ExecuteCommand(cmd)
						if err != nil {
							output += "\n[Error]: " + err.Error()
							fmt.Printf("我获取到的结果:\n%s\n", output)
							// 将失败信息添加到聊天历史，要求AI尝试其他方案
							chatHistory = append(chatHistory, llm.Message{Role: "user", Content: fmt.Sprintf("命令执行失败 (%s):\n%s\n请尝试其他方案获取所需信息。", cmd, output)})
						} else {
							processed := llm.TruncateOutput(output, 2800)
							fmt.Printf("我获取到的结果:\n%s\n", processed)
							chatHistory = append(chatHistory, llm.Message{Role: "user", Content: "执行结果 (" + cmd + "):\n" + processed})
						}

					case safety.RiskLevelLow:
						// 低风险命令，显示提示后执行
						fmt.Printf("\nℹ️ 我注意到这是一个低风险命令: %s\n我将执行: %s\n", reason, cmd)
						fmt.Println("我正在执行...")
						output, err := p.ExecuteCommand(cmd)
						if err != nil {
							output += "\n[Error]: " + err.Error()
							fmt.Printf("我获取到的结果:\n%s\n", output)
							// 将失败信息添加到聊天历史，要求AI尝试其他方案
							chatHistory = append(chatHistory, llm.Message{Role: "user", Content: fmt.Sprintf("命令执行失败 (%s):\n%s\n请尝试其他方案获取所需信息。", cmd, output)})
						} else {
							processed := llm.TruncateOutput(output, 2800)
							fmt.Printf("我获取到的结果:\n%s\n", processed)
							chatHistory = append(chatHistory, llm.Message{Role: "user", Content: "执行结果 (" + cmd + "):\n" + processed})
						}

					case safety.RiskLevelMedium:
						// 中风险命令，需要用户确认
						fmt.Printf("\n⚠️ 我发现这是一个中风险命令: %s\n我需要执行: %s\n", reason, cmd)
						fmt.Print("我需要执行这个命令，您同意吗？(y/n): ")
						conf, _ := reader.ReadString('\n')
						if strings.TrimSpace(strings.ToLower(llm.CleanInput(conf))) != "y" {
							chatHistory = append(chatHistory, llm.Message{Role: "user", Content: "用户拒绝执行中风险命令: " + cmd})
							continue
						}
						output, err := p.ExecuteCommand(cmd)
						if err != nil {
							output += "\n[Error]: " + err.Error()
							fmt.Printf("我获取到的结果:\n%s\n", output)
							// 将失败信息添加到聊天历史，要求AI尝试其他方案
							chatHistory = append(chatHistory, llm.Message{Role: "user", Content: fmt.Sprintf("命令执行失败 (%s):\n%s\n请尝试其他方案获取所需信息。", cmd, output)})
						} else {
							processed := llm.TruncateOutput(output, 2800)
							fmt.Printf("我获取到的结果:\n%s\n", processed)
							chatHistory = append(chatHistory, llm.Message{Role: "user", Content: "执行结果 (" + cmd + "):\n" + processed})
						}

					case safety.RiskLevelHigh, safety.RiskLevelCritical:
						// 高风险命令，需要用户确认并提供更多信息
						fmt.Printf("\n🚨 我发现这是一个高风险命令: %s\n我需要执行: %s\n", reason, cmd)

						// 已经通过AI分析获取了详细信息，直接询问用户
						fmt.Print("我需要执行这个命令，您同意吗？(y/n): ")
						conf, _ := reader.ReadString('\n')
						if strings.TrimSpace(strings.ToLower(llm.CleanInput(conf))) != "y" {
							chatHistory = append(chatHistory, llm.Message{Role: "user", Content: "用户拒绝执行危险命令: " + cmd})
							continue
						}
						output, err := p.ExecuteCommand(cmd)
						if err != nil {
							output += "\n[Error]: " + err.Error()
							fmt.Printf("我获取到的结果:\n%s\n", output)
							// 将失败信息添加到聊天历史，要求AI尝试其他方案
							chatHistory = append(chatHistory, llm.Message{Role: "user", Content: fmt.Sprintf("命令执行失败 (%s):\n%s\n请尝试其他方案获取所需信息。", cmd, output)})
						} else {
							processed := llm.TruncateOutput(output, 2800)
							fmt.Printf("我获取到的结果:\n%s\n", processed)
							chatHistory = append(chatHistory, llm.Message{Role: "user", Content: "执行结果 (" + cmd + "):\n" + processed})
						}
					}
				}
			}

			// 检查是否包含FINAL:标记
			if strings.Contains(response, "FINAL:") {
				finalPart := strings.SplitN(response, "FINAL:", 2)[1]
				fmt.Println("\n" + Green + Bold + "🏆 诊断报告" + Reset)
				fmt.Println(strings.Repeat("─", 70))

				lines := strings.Split(strings.TrimSpace(finalPart), "\n")
				for _, line := range lines {
					line = strings.TrimSpace(line)
					if strings.HasPrefix(line, "根本原因") || strings.HasPrefix(line, "原因") {
						fmt.Println(Yellow + "● " + line + Reset)
					} else if strings.HasPrefix(line, "风险影响") || strings.HasPrefix(line, "风险") {
						fmt.Println(Red + "⚠ " + line + Reset)
					} else if strings.HasPrefix(line, "修复步骤") || strings.HasPrefix(line, "步骤") || strings.HasPrefix(line, "详细修复") {
						fmt.Println(Cyan + "→ " + line + Reset)
					} else if strings.HasPrefix(line, "预防措施") || strings.HasPrefix(line, "预防") {
						fmt.Println(Blue + "★ " + line + Reset)
					} else if strings.Contains(line, "umount") || strings.Contains(line, "fsck") || strings.Contains(line, "e2fsck") {
						fmt.Println(Blue + "   $ " + line + Reset)
					} else {
						fmt.Println("   " + line)
					}
				}
				fmt.Println(strings.Repeat("─", 70))

				// 添加诊断结果确认步骤
				fmt.Print("\n等待用户输入（提示，默认满意生成报告，1.进一步分析，2.修改需求，3.退出 > ")

				choiceStr, _ := reader.ReadString('\n')
				choiceStr = strings.TrimSpace(llm.CleanInput(choiceStr))

				if choiceStr == "" || choiceStr == "满意" {
					fmt.Println("\n📋 正在准备生成报告...")
					historyFilename := fmt.Sprintf("history_%s.json", time.Now().Format("20060102_150405"))
					historyData, _ := json.Marshal(chatHistory)
					_ = os.WriteFile(historyFilename, historyData, 0644)
					fmt.Printf("✅ 聊天历史已保存: %s\n", historyFilename)

					fmt.Print("\n报告格式 (1.MD 2.HTML 3.PDF) > ")
					reportChoiceStr, _ := reader.ReadString('\n')
					reportChoiceStr = strings.TrimSpace(reportChoiceStr)

					reportChoice := 1
					fmt.Sscanf(reportChoiceStr, "%d", &reportChoice)
					if reportChoice < 1 || reportChoice > 3 {
						reportChoice = 1
					}

					switch reportChoice {
					case 1:
						report.GenerateReport(historyFilename, "md")
					case 2:
						report.GenerateReport(historyFilename, "html")
					case 3:
						report.GenerateReport(historyFilename, "pdf")
					}
					break
				}

				if choiceStr == "3" || choiceStr == "exit" || choiceStr == "quit" || strings.Contains(choiceStr, "退出") {
					fmt.Println("ℹ️ 已退出")
					break
				}

				if choiceStr == "2" || strings.Contains(choiceStr, "修改") {
					fmt.Print("\n请输入修改需求 > ")
					line, _ := reader.ReadString('\n')
					input := llm.CleanInput(line)
					trimmedInput := strings.TrimSpace(input)
					if trimmedInput != "" {
						chatHistory = append(chatHistory, llm.Message{Role: "user", Content: "修改需求：" + trimmedInput})
					} else {
						chatHistory = append(chatHistory, llm.Message{Role: "user", Content: "修改需求：重新分析"})
					}
					continue
				}

				if choiceStr == "1" || strings.Contains(choiceStr, "不满意") || strings.Contains(choiceStr, "分析") {
					fmt.Print("\n请提供更多信息 > ")
					line, _ := reader.ReadString('\n')
					input := llm.CleanInput(line)
					trimmedInput := strings.TrimSpace(input)
					if trimmedInput != "" {
						chatHistory = append(chatHistory, llm.Message{Role: "user", Content: "需要进一步分析：" + trimmedInput})
					} else {
						chatHistory = append(chatHistory, llm.Message{Role: "user", Content: "需要进一步分析"})
					}
					continue
				}
			}
		}
	}

	// 保存聊天历史（如果在故障诊断模式中未保存）
	historySaved := false
	for _, msg := range chatHistory {
		if strings.Contains(msg.Content, "✅ 聊天历史已保存") {
			historySaved = true
			break
		}
	}

	if !historySaved {
		historyFilename := fmt.Sprintf("history_%s.json", time.Now().Format("20060102_150405"))
		historyData, _ := json.Marshal(chatHistory)
		_ = os.WriteFile(historyFilename, historyData, 0644)
		fmt.Printf("\n✅ 聊天历史已保存: %s\n", historyFilename)
	}
}
