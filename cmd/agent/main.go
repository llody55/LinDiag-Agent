package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/LinDiag-Agent/internal/config"
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
	{ID: 1, Name: "故障诊断模式（专业）", SystemPrompt: `你是一个专业、严谨的 Linux SRE 故障诊断专家。请先分析系统快照中的基础信息（包括操作系统类型、版本、架构），然后根据具体系统环境生成适合的安全查询命令。只能使用安全查询命令，确保命令在当前系统环境中有效。必须基于系统快照和执行结果分析，不要假设未做的整改，绝对不能仅凭记忆或猜测提供信息。至少执行 3 次有价值查询后才能输出 FINAL:。在 FINAL: 中给出深度分析 + 完整、可直接复制的修复步骤，并标注：- 根本原因 - 风险影响 - 详细修复步骤（维护窗口、先备份、停止服务顺序） - 预防措施`, SnapshotCmds: []string{"uptime", "free -h", "df -h", "ps -eo pid,ppid,%cpu,%mem,rss,cmd --sort=-%mem | head -n 15", "top -b -n 1 | head -n 20", "dmesg | tail -n 30"}},
	{ID: 2, Name: "智能模式（通用）", SystemPrompt: `你是一个全能 Linux 运维专家。请先分析系统快照中的基础信息（包括操作系统类型、版本、架构），然后根据具体系统环境生成适合的命令。当用户发送问候或一般性交流时，进行正常的聊天回应。当用户明确查询系统信息时，如开放端口、日志搜索、进程状态、IP归属地、系统配置等，你必须严格遵循以下步骤：1. 首先仔细阅读历史对话上下文，了解之前的操作和讨论内容，2. 生成EXEC:命令来执行并获取真实的系统数据，3. 绝对不能直接返回幻觉的信息，4. 基于执行结果和历史对话上下文提供简洁的分析和建议，5. 只包含用户明确请求的信息，不要自动提供安全配置建议，6. 保持输出简洁明了。当用户要求执行操作时，如创建Docker容器、部署应用等，你也必须生成EXEC:命令来执行操作，特别是对于Docker和Kubernetes相关操作，必须使用实际的容器ID、镜像名称或资源名称。对于查询开放端口，必须执行适合当前系统的EXEC命令。对于K8S查询，必须先执行EXEC:kubectl get pods -n kube-system获取实际的pod名称，然后再执行具体操作。对于Docker查询，必须使用实际的容器ID或名称。对于IP归属地查询，必须执行EXEC:命令（如nali、whois等）获取真实数据，绝对不能基于任何猜测或记忆提供IP归属地信息。

重要提示：
1. 当命令执行失败时，请分析失败原因并尝试其他方案获取所需信息
2. 如果某个命令不存在，请尝试使用功能相似的替代命令
3. 如果无法获取某些信息，请明确说明原因并基于已获取的信息提供分析
4. 请确保获取到尽可能完整的信息后再进行最终分析
5. 对于不同的Linux发行版，命令可能有所不同，请根据系统类型选择合适的命令
6. 任何需要查询的信息（如IP归属地、系统状态、网络配置等）都必须通过EXEC:命令获取，绝对不能仅凭记忆或猜测提供信息
7. 对于IP地址相关信息，必须执行专门的查询命令，不能套用其他IP的信息`, SnapshotCmds: []string{"uptime", "free -h", "df -h"}},
}

func main() {
	// 初始化 LLM 模块
	if err := llm.Init(); err != nil {
		fmt.Printf("❌ 初始化 LLM 模块失败: %v\n", err)
		return
	}

	// 设置命令执行超时时间
	timeout := llm.GetConfig().Command.TimeoutSeconds
	if timeout <= 0 {
		timeout = 30
	}
	platform.SetDefaultTimeout(timeout)
	fmt.Printf("ℹ️ 命令执行超时时间: %d秒\n", timeout)

	// 加载用户偏好设置
	config.LoadUserPreferences()

	// 加载白名单
	if err := safety.LoadWhitelist("whitelist.txt"); err != nil {
		fmt.Printf("⚠️ 加载白名单文件失败: %v，使用默认白名单\n", err)
	} else {
		fmt.Println("✅ 已加载白名单")
	}

	// 检查 API Key 配置
	// 检查必要的配置
	if llm.GetConfig().LLM.APIURL == "" || llm.GetConfig().LLM.APIKey == "" || llm.GetConfig().LLM.ModelName == "" {
		fmt.Println(Red + "❌ 未配置 LLM 参数，请先创建配置文件" + Reset)
		fmt.Println()
		fmt.Println(Yellow + "配置文件路径: ~/.config/lindiag/config.json" + Reset)
		fmt.Println()
		fmt.Println("配置示例:")
		fmt.Println("```json")
		fmt.Println("{")
		fmt.Println("  \"llm\": {")
		fmt.Println("    \"api_url\": \"https://api.example.com/v1/chat/completions\",")
		fmt.Println("    \"api_key\": \"your-api-key-here\",")
		fmt.Println("    \"model_name\": \"your-model-name\"")
		fmt.Println("  },")
		fmt.Println("  \"command\": {")
		fmt.Println("    \"timeout_seconds\": 60")
		fmt.Println("  }")
		fmt.Println("}")
		fmt.Println("```")
		fmt.Println()
		fmt.Println("环境变量方式（优先级更高）:")
		fmt.Println("  LINDIAG_LLM_API_URL")
		fmt.Println("  LINDIAG_LLM_API_KEY")
		fmt.Println("  LINDIAG_LLM_MODEL_NAME")
		return
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

	// 检测运行环境
	envInfo := detectEnvironment()
	
	fmt.Println("\n" + Cyan + "┌─────────────────────────────────────────────────────────────" + Reset)
	fmt.Println(Cyan + "│" + Reset + " 🔍 环境检测结果")
	fmt.Println(Cyan + "└─────────────────────────────────────────────────────────────" + Reset)
	for _, env := range envInfo {
		fmt.Println("   " + Green + "✓" + Reset + " " + env)
	}
	if len(envInfo) == 0 {
		fmt.Println("   " + Yellow + "⚠️ 未检测到特殊环境" + Reset)
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
				chatHistory = llm.LoadDefaultChatHistory(reader, selectedMode.ID, selectedMode.SystemPrompt, selectedMode.SnapshotCmds, rules)
			}
		} else {
			fmt.Printf("❌ 无法读取历史记录文件: %v\n", err)
			// 加载失败，使用默认流程
			chatHistory = llm.LoadDefaultChatHistory(reader, selectedMode.ID, selectedMode.SystemPrompt, selectedMode.SnapshotCmds, rules)
		}
	} else {
		// 没有提供历史记录文件，使用默认流程
		chatHistory = llm.LoadDefaultChatHistory(reader, selectedMode.ID, selectedMode.SystemPrompt, selectedMode.SnapshotCmds, rules)
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
						cmdInfo := analyzer.AnalyzeCommand(cmd)

						switch cmdInfo.RiskLevel {
						case safety.RiskLevelSafe:
							fmt.Println("\n" + Cyan + "┌─────────────────────────────────────────────────────────────" + Reset)
							fmt.Println(Cyan + "│" + Reset + " 🔍 我需要执行这个命令来获取信息")
							fmt.Println(Cyan + "├─────────────────────────────────────────────────────────────" + Reset)
							fmt.Println(Cyan + "│" + Reset + " 命令: " + Yellow + cmd + Reset)
							fmt.Println(Cyan + "│" + Reset + " 说明: " + cmdInfo.Explanation)
							fmt.Println(Cyan + "└─────────────────────────────────────────────────────────────" + Reset)
							output, err := platform.ExecuteCommandWithProgress(cmd)
							if err != nil {
								output += "\n[Error]: " + err.Error()
								fmt.Println(Cyan + "┌─────────────────────────────────────────────────────────────" + Reset)
								fmt.Println(Cyan + "│" + Reset + " ❌ 执行结果 (失败):")
								fmt.Println(Cyan + "└─────────────────────────────────────────────────────────────" + Reset)
								fmt.Println(Red + output + Reset)
								chatHistory = append(chatHistory, llm.Message{Role: "user", Content: fmt.Sprintf("命令执行失败 (%s):\n%s\n请尝试其他方案获取所需信息。", cmd, output)})
							} else {
								processed := llm.TruncateOutput(output, 2800)
								fmt.Println(Cyan + "┌─────────────────────────────────────────────────────────────" + Reset)
								fmt.Println(Cyan + "│" + Reset + " ✅ 执行结果:")
								fmt.Println(Cyan + "└─────────────────────────────────────────────────────────────" + Reset)
								fmt.Println(processed)
								chatHistory = append(chatHistory, llm.Message{Role: "user", Content: "执行结果 (" + cmd + "):\n" + processed})
							}

						case safety.RiskLevelLow:
							fmt.Println("\n" + Blue + "┌─────────────────────────────────────────────────────────────" + Reset)
							fmt.Println(Blue + "│" + Reset + " ℹ️ 我需要执行这个命令")
							fmt.Println(Blue + "├─────────────────────────────────────────────────────────────" + Reset)
							fmt.Println(Blue + "│" + Reset + " 命令: " + Yellow + cmd + Reset)
							fmt.Println(Blue + "│" + Reset + " 说明: " + cmdInfo.Explanation)
							fmt.Println(Blue + "│" + Reset + " 提示: " + cmdInfo.Reason)
							fmt.Println(Blue + "└─────────────────────────────────────────────────────────────" + Reset)
							output, err := platform.ExecuteCommandWithProgress(cmd)
							if err != nil {
								output += "\n[Error]: " + err.Error()
								fmt.Println(Blue + "┌─────────────────────────────────────────────────────────────" + Reset)
								fmt.Println(Blue + "│" + Reset + " ❌ 执行结果 (失败):")
								fmt.Println(Blue + "└─────────────────────────────────────────────────────────────" + Reset)
								fmt.Println(Red + output + Reset)
								chatHistory = append(chatHistory, llm.Message{Role: "user", Content: fmt.Sprintf("命令执行失败 (%s):\n%s\n请尝试其他方案获取所需信息。", cmd, output)})
							} else {
								processed := llm.TruncateOutput(output, 2800)
								fmt.Println(Blue + "┌─────────────────────────────────────────────────────────────" + Reset)
								fmt.Println(Blue + "│" + Reset + " ✅ 执行结果:")
								fmt.Println(Blue + "└─────────────────────────────────────────────────────────────" + Reset)
								fmt.Println(processed)
								chatHistory = append(chatHistory, llm.Message{Role: "user", Content: "执行结果 (" + cmd + "):\n" + processed})
							}

						case safety.RiskLevelMedium:
							if config.GetUserPreferences().AutoConfirmMediumRisk {
								fmt.Println("\n" + Yellow + "┌─────────────────────────────────────────────────────────────" + Reset)
								fmt.Println(Yellow + "│" + Reset + " ⚠️ 中风险命令（已自动确认）")
								fmt.Println(Yellow + "├─────────────────────────────────────────────────────────────" + Reset)
								fmt.Println(Yellow + "│" + Reset + " 命令: " + cmd)
								fmt.Println(Yellow + "└─────────────────────────────────────────────────────────────" + Reset)
							} else {
								fmt.Println("\n" + Yellow + "┌─────────────────────────────────────────────────────────────" + Reset)
								fmt.Println(Yellow + "│" + Reset + " ⚠️ 我需要执行这个命令，但需要您的确认")
								fmt.Println(Yellow + "├─────────────────────────────────────────────────────────────" + Reset)
								fmt.Println(Yellow + "│" + Reset + " 命令: " + cmd)
								fmt.Println(Yellow + "│" + Reset + " 说明: " + cmdInfo.Explanation)
								fmt.Println(Yellow + "│" + Reset + " 风险: " + cmdInfo.Reason)
								fmt.Println(Yellow + "├─────────────────────────────────────────────────────────────" + Reset)
								fmt.Println(Yellow + "│" + Reset + " 选项: 1. Yes  2. Yes, and don't ask me again  3. No")
								fmt.Println(Yellow + "└─────────────────────────────────────────────────────────────" + Reset)
								fmt.Print("Enter your choice: ")
								conf, _ := reader.ReadString('\n')
								choice := strings.TrimSpace(strings.ToLower(llm.CleanInput(conf)))
								if choice == "2" {
									config.SetAutoConfirmMediumRisk(true)
									fmt.Println(Green + "✅ 已设置中风险命令自动确认，后续将不再询问" + Reset)
								} else if choice != "y" && choice != "yes" && choice != "1" {
									chatHistory = append(chatHistory, llm.Message{Role: "user", Content: "用户拒绝执行中风险命令: " + cmd})
									continue
								}
							}
							output, err := platform.ExecuteCommandWithProgress(cmd)
							if err != nil {
								output += "\n[Error]: " + err.Error()
								fmt.Println(Yellow + "┌─────────────────────────────────────────────────────────────" + Reset)
								fmt.Println(Yellow + "│" + Reset + " ❌ 执行结果 (失败):")
								fmt.Println(Yellow + "└─────────────────────────────────────────────────────────────" + Reset)
								fmt.Println(Red + output + Reset)
								chatHistory = append(chatHistory, llm.Message{Role: "user", Content: fmt.Sprintf("命令执行失败 (%s):\n%s\n请尝试其他方案获取所需信息。", cmd, output)})
							} else {
								processed := llm.TruncateOutput(output, 2800)
								fmt.Println(Yellow + "┌─────────────────────────────────────────────────────────────" + Reset)
								fmt.Println(Yellow + "│" + Reset + " ✅ 执行结果:")
								fmt.Println(Yellow + "└─────────────────────────────────────────────────────────────" + Reset)
								fmt.Println(processed)
								chatHistory = append(chatHistory, llm.Message{Role: "user", Content: "执行结果 (" + cmd + "):\n" + processed})
							}

						case safety.RiskLevelHigh, safety.RiskLevelCritical:
							fmt.Println("\n" + Red + "┌─────────────────────────────────────────────────────────────" + Reset)
							fmt.Println(Red + "│" + Reset + " 🚨 我发现这是一个高风险命令")
							fmt.Println(Red + "├─────────────────────────────────────────────────────────────" + Reset)
							fmt.Println(Red + "│" + Reset + " 命令: " + cmd)
							fmt.Println(Red + "│" + Reset + " 说明: " + cmdInfo.Explanation)
							fmt.Println(Red + "│" + Reset + " 风险: " + cmdInfo.Reason)
							fmt.Println(Red + "├─────────────────────────────────────────────────────────────" + Reset)
							explainPrompt := []llm.Message{
								{Role: "system", Content: "用简洁的中文解释这个命令：它做什么？为什么执行？风险如何？"},
								{Role: "user", Content: fmt.Sprintf("命令：%s", cmd)},
							}
							explain := llm.CallAI(explainPrompt)
							fmt.Println(Red + "│" + Reset + " AI分析: " + strings.TrimSpace(explain))
							fmt.Println(Red + "└─────────────────────────────────────────────────────────────" + Reset)
							fmt.Print("您确定要我执行吗？(y/n): ")
							conf, _ := reader.ReadString('\n')
							if strings.TrimSpace(strings.ToLower(llm.CleanInput(conf))) != "y" {
								chatHistory = append(chatHistory, llm.Message{Role: "user", Content: "用户拒绝执行危险命令: " + cmd})
								continue
							}
							output, err := platform.ExecuteCommandWithProgress(cmd)
							if err != nil {
								output += "\n[Error]: " + err.Error()
								fmt.Println(Red + "┌─────────────────────────────────────────────────────────────" + Reset)
								fmt.Println(Red + "│" + Reset + " ❌ 执行结果 (失败):")
								fmt.Println(Red + "└─────────────────────────────────────────────────────────────" + Reset)
								fmt.Println(Red + output + Reset)
								chatHistory = append(chatHistory, llm.Message{Role: "user", Content: fmt.Sprintf("命令执行失败 (%s):\n%s\n请尝试其他方案获取所需信息。", cmd, output)})
							} else {
								processed := llm.TruncateOutput(output, 2800)
								fmt.Println(Red + "┌─────────────────────────────────────────────────────────────" + Reset)
								fmt.Println(Red + "│" + Reset + " ✅ 执行结果:")
								fmt.Println(Red + "└─────────────────────────────────────────────────────────────" + Reset)
								fmt.Println(processed)
								chatHistory = append(chatHistory, llm.Message{Role: "user", Content: "执行结果 (" + cmd + "):\n" + processed})
							}
						}
					}
				}
				return hasExec
			}

			// 智能模式的用户输入处理函数
			getUserInput := func() string {
				fmt.Println("\n" + Blue + "┌─────────────────────────────────────────────────────────────" + Reset)
				fmt.Println(Blue + "│" + Reset + " 📝 请输入您的问题或选择操作：")
				fmt.Println(Blue + "├─────────────────────────────────────────────────────────────" + Reset)
				fmt.Println(Blue + "│" + Reset + "  按 Enter 继续交互")
				fmt.Println(Blue + "│" + Reset + "  1 / report / r    - 生成报告")
				fmt.Println(Blue + "│" + Reset + "  2 / exit / q       - 退出")
				fmt.Println(Blue + "└─────────────────────────────────────────────────────────────" + Reset)
				fmt.Print("您的输入 > ")
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
					fmt.Printf(Green + "✅ 聊天历史已保存: %s\n" + Reset, historyFilename)

					fmt.Println("\n" + Yellow + "┌─────────────────────────────────────────────────────────────" + Reset)
					fmt.Println(Yellow + "│" + Reset + " 📊 请选择报告格式：")
					fmt.Println(Yellow + "├─────────────────────────────────────────────────────────────" + Reset)
					fmt.Println(Yellow + "│" + Reset + "  1. MD   - Markdown 格式")
					fmt.Println(Yellow + "│" + Reset + "  2. HTML - 网页格式")
					fmt.Println(Yellow + "│" + Reset + "  3. PDF  - PDF 格式")
					fmt.Println(Yellow + "└─────────────────────────────────────────────────────────────" + Reset)
					fmt.Print("选择格式 (1-3) > ")
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
						fmt.Println(Green + "✅ Markdown 报告已生成" + Reset)
					case 2:
						report.GenerateReport(historyFilename, "html")
						fmt.Println(Green + "✅ HTML 报告已生成" + Reset)
					case 3:
						report.GenerateReport(historyFilename, "pdf")
						fmt.Println(Green + "✅ PDF 报告已生成" + Reset)
					}

					fmt.Println("\n" + Blue + "┌─────────────────────────────────────────────────────────────" + Reset)
					fmt.Println(Blue + "│" + Reset + " 📝 请输入您的问题或选择操作：")
					fmt.Println(Blue + "├─────────────────────────────────────────────────────────────" + Reset)
					fmt.Println(Blue + "│" + Reset + "  按 Enter 继续交互")
					fmt.Println(Blue + "│" + Reset + "  1 / report / r    - 生成报告")
					fmt.Println(Blue + "│" + Reset + "  2 / exit / q       - 退出")
					fmt.Println(Blue + "└─────────────────────────────────────────────────────────────" + Reset)
					fmt.Print("您的输入 > ")
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
				if strings.TrimSpace(response) == "" {
					fmt.Println("\n" + Yellow + "⚠️ AI 暂时无法提供分析，请尝试重新描述问题或提供更多信息" + Reset)
				} else {
					fmt.Println()
					fmt.Println(Cyan + "╔═══════════════════════════════════════════════════════════════" + Reset)
					fmt.Println(Cyan + "║" + Reset + " 💡 AI 分析")
					fmt.Println(Cyan + "╚═══════════════════════════════════════════════════════════════" + Reset)
					fmt.Println()
					fmt.Println(response)
					fmt.Println()
				}

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
					cmdInfo := analyzer.AnalyzeCommand(cmd)

					switch cmdInfo.RiskLevel {
					case safety.RiskLevelSafe:
						fmt.Println("\n" + Cyan + "┌─────────────────────────────────────────────────────────────" + Reset)
						fmt.Println(Cyan + "│" + Reset + " 🔍 我需要执行这个命令来获取信息")
						fmt.Println(Cyan + "├─────────────────────────────────────────────────────────────" + Reset)
						fmt.Println(Cyan + "│" + Reset + " 命令: " + Yellow + cmd + Reset)
						fmt.Println(Cyan + "│" + Reset + " 说明: " + cmdInfo.Explanation)
						fmt.Println(Cyan + "└─────────────────────────────────────────────────────────────" + Reset)
						output, err := platform.ExecuteCommandWithProgress(cmd)
						if err != nil {
							output += "\n[Error]: " + err.Error()
							fmt.Println(Cyan + "┌─────────────────────────────────────────────────────────────" + Reset)
							fmt.Println(Cyan + "│" + Reset + " ❌ 执行结果 (失败):")
							fmt.Println(Cyan + "└─────────────────────────────────────────────────────────────" + Reset)
							fmt.Println(Red + output + Reset)
							chatHistory = append(chatHistory, llm.Message{Role: "user", Content: fmt.Sprintf("命令执行失败 (%s):\n%s\n请尝试其他方案获取所需信息。", cmd, output)})
						} else {
							processed := llm.TruncateOutput(output, 2800)
							fmt.Println(Cyan + "┌─────────────────────────────────────────────────────────────" + Reset)
							fmt.Println(Cyan + "│" + Reset + " ✅ 执行结果:")
							fmt.Println(Cyan + "└─────────────────────────────────────────────────────────────" + Reset)
							fmt.Println(processed)
							chatHistory = append(chatHistory, llm.Message{Role: "user", Content: "执行结果 (" + cmd + "):\n" + processed})
						}

					case safety.RiskLevelLow:
						fmt.Println("\n" + Blue + "┌─────────────────────────────────────────────────────────────" + Reset)
						fmt.Println(Blue + "│" + Reset + " ℹ️ 我需要执行这个命令")
						fmt.Println(Blue + "├─────────────────────────────────────────────────────────────" + Reset)
						fmt.Println(Blue + "│" + Reset + " 命令: " + Yellow + cmd + Reset)
						fmt.Println(Blue + "│" + Reset + " 说明: " + cmdInfo.Explanation)
						fmt.Println(Blue + "│" + Reset + " 提示: " + cmdInfo.Reason)
						fmt.Println(Blue + "└─────────────────────────────────────────────────────────────" + Reset)
						output, err := platform.ExecuteCommandWithProgress(cmd)
						if err != nil {
							output += "\n[Error]: " + err.Error()
							fmt.Println(Blue + "┌─────────────────────────────────────────────────────────────" + Reset)
							fmt.Println(Blue + "│" + Reset + " ❌ 执行结果 (失败):")
							fmt.Println(Blue + "└─────────────────────────────────────────────────────────────" + Reset)
							fmt.Println(Red + output + Reset)
							chatHistory = append(chatHistory, llm.Message{Role: "user", Content: fmt.Sprintf("命令执行失败 (%s):\n%s\n请尝试其他方案获取所需信息。", cmd, output)})
						} else {
							processed := llm.TruncateOutput(output, 2800)
							fmt.Println(Blue + "┌─────────────────────────────────────────────────────────────" + Reset)
							fmt.Println(Blue + "│" + Reset + " ✅ 执行结果:")
							fmt.Println(Blue + "└─────────────────────────────────────────────────────────────" + Reset)
							fmt.Println(processed)
							chatHistory = append(chatHistory, llm.Message{Role: "user", Content: "执行结果 (" + cmd + "):\n" + processed})
						}

					case safety.RiskLevelMedium:
						fmt.Println("\n" + Yellow + "┌─────────────────────────────────────────────────────────────" + Reset)
						fmt.Println(Yellow + "│" + Reset + " ⚠️ 我需要执行这个命令，但需要您的确认")
						fmt.Println(Yellow + "├─────────────────────────────────────────────────────────────" + Reset)
						fmt.Println(Yellow + "│" + Reset + " 命令: " + cmd)
						fmt.Println(Yellow + "│" + Reset + " 说明: " + cmdInfo.Explanation)
						fmt.Println(Yellow + "│" + Reset + " 风险: " + cmdInfo.Reason)
						fmt.Println(Yellow + "└─────────────────────────────────────────────────────────────" + Reset)
						fmt.Print("您同意我执行吗？(y/n): ")
						conf, _ := reader.ReadString('\n')
						if strings.TrimSpace(strings.ToLower(llm.CleanInput(conf))) != "y" {
							chatHistory = append(chatHistory, llm.Message{Role: "user", Content: "用户拒绝执行中风险命令: " + cmd})
							continue
						}
						output, err := platform.ExecuteCommandWithProgress(cmd)
						if err != nil {
							output += "\n[Error]: " + err.Error()
							fmt.Println(Yellow + "┌─────────────────────────────────────────────────────────────" + Reset)
							fmt.Println(Yellow + "│" + Reset + " ❌ 执行结果 (失败):")
							fmt.Println(Yellow + "└─────────────────────────────────────────────────────────────" + Reset)
							fmt.Println(Red + output + Reset)
							chatHistory = append(chatHistory, llm.Message{Role: "user", Content: fmt.Sprintf("命令执行失败 (%s):\n%s\n请尝试其他方案获取所需信息。", cmd, output)})
						} else {
							processed := llm.TruncateOutput(output, 2800)
							fmt.Println(Yellow + "┌─────────────────────────────────────────────────────────────" + Reset)
							fmt.Println(Yellow + "│" + Reset + " ✅ 执行结果:")
							fmt.Println(Yellow + "└─────────────────────────────────────────────────────────────" + Reset)
							fmt.Println(processed)
							chatHistory = append(chatHistory, llm.Message{Role: "user", Content: "执行结果 (" + cmd + "):\n" + processed})
						}

					case safety.RiskLevelHigh, safety.RiskLevelCritical:
						fmt.Println("\n" + Red + "┌─────────────────────────────────────────────────────────────" + Reset)
						fmt.Println(Red + "│" + Reset + " 🚨 我发现这是一个高风险命令")
						fmt.Println(Red + "├─────────────────────────────────────────────────────────────" + Reset)
						fmt.Println(Red + "│" + Reset + " 命令: " + cmd)
						fmt.Println(Red + "│" + Reset + " 说明: " + cmdInfo.Explanation)
						fmt.Println(Red + "│" + Reset + " 风险: " + cmdInfo.Reason)
						fmt.Println(Red + "├─────────────────────────────────────────────────────────────" + Reset)
						explainPrompt := []llm.Message{
							{Role: "system", Content: "用简洁的中文解释这个命令：它做什么？为什么执行？风险如何？"},
							{Role: "user", Content: fmt.Sprintf("命令：%s", cmd)},
						}
						explain := llm.CallAI(explainPrompt)
						fmt.Println(Red + "│" + Reset + " AI分析: " + strings.TrimSpace(explain))
						fmt.Println(Red + "└─────────────────────────────────────────────────────────────" + Reset)
						fmt.Print("您确定要我执行吗？(y/n): ")
						conf, _ := reader.ReadString('\n')
						if strings.TrimSpace(strings.ToLower(llm.CleanInput(conf))) != "y" {
							chatHistory = append(chatHistory, llm.Message{Role: "user", Content: "用户拒绝执行危险命令: " + cmd})
							continue
						}
						output, err := platform.ExecuteCommandWithProgress(cmd)
						if err != nil {
							output += "\n[Error]: " + err.Error()
							fmt.Println(Red + "┌─────────────────────────────────────────────────────────────" + Reset)
							fmt.Println(Red + "│" + Reset + " ❌ 执行结果 (失败):")
							fmt.Println(Red + "└─────────────────────────────────────────────────────────────" + Reset)
							fmt.Println(Red + output + Reset)
							chatHistory = append(chatHistory, llm.Message{Role: "user", Content: fmt.Sprintf("命令执行失败 (%s):\n%s\n请尝试其他方案获取所需信息。", cmd, output)})
						} else {
							processed := llm.TruncateOutput(output, 2800)
							fmt.Println(Red + "┌─────────────────────────────────────────────────────────────" + Reset)
							fmt.Println(Red + "│" + Reset + " ✅ 执行结果:")
							fmt.Println(Red + "└─────────────────────────────────────────────────────────────" + Reset)
							fmt.Println(processed)
							chatHistory = append(chatHistory, llm.Message{Role: "user", Content: "执行结果 (" + cmd + "):\n" + processed})
						}
					}
				}
			}

			// 检查是否包含FINAL:标记
			if strings.Contains(response, "FINAL:") {
				finalPart := strings.SplitN(response, "FINAL:", 2)[1]
				fmt.Println("\n" + Green + Bold + "╔══════════════════════════════════════════════════════════════════════════╗" + Reset)
				fmt.Println(Green + Bold + "║                    🏆 诊断报告 🏆                                      ║" + Reset)
				fmt.Println(Green + Bold + "╚══════════════════════════════════════════════════════════════════════════╝" + Reset)

				lines := strings.Split(strings.TrimSpace(finalPart), "\n")
				inCodeBlock := false
				for _, line := range lines {
					line = strings.TrimSpace(line)

					if strings.HasPrefix(line, "```") {
						inCodeBlock = !inCodeBlock
						continue
					}

					if inCodeBlock {
						fmt.Println(Yellow + "   $ " + line + Reset)
						continue
					}

					if strings.HasPrefix(line, "根本原因") || strings.HasPrefix(line, "原因") {
						fmt.Println("\n" + Yellow + Bold + "╔══════════════════════════════════════════════════════════════════════════╗" + Reset)
						fmt.Println(Yellow + Bold + "║                      🎯 根本原因                                        ║" + Reset)
						fmt.Println(Yellow + Bold + "╚══════════════════════════════════════════════════════════════════════════╝" + Reset)
						fmt.Println(Yellow + "   " + line + Reset)
					} else if strings.HasPrefix(line, "风险影响") || strings.HasPrefix(line, "风险") {
						fmt.Println("\n" + Red + Bold + "╔══════════════════════════════════════════════════════════════════════════╗" + Reset)
						fmt.Println(Red + Bold + "║                      ⚠️ 风险影响                                        ║" + Reset)
						fmt.Println(Red + Bold + "╚══════════════════════════════════════════════════════════════════════════╝" + Reset)
						fmt.Println(Red + "   " + line + Reset)
					} else if strings.HasPrefix(line, "修复步骤") || strings.HasPrefix(line, "步骤") || strings.HasPrefix(line, "详细修复") {
						fmt.Println("\n" + Cyan + Bold + "╔══════════════════════════════════════════════════════════════════════════╗" + Reset)
						fmt.Println(Cyan + Bold + "║                      🛠️ 修复步骤                                        ║" + Reset)
						fmt.Println(Cyan + Bold + "╚══════════════════════════════════════════════════════════════════════════╝" + Reset)
						fmt.Println(Cyan + "   " + line + Reset)
					} else if strings.HasPrefix(line, "预防措施") || strings.HasPrefix(line, "预防") {
						fmt.Println("\n" + Blue + Bold + "╔══════════════════════════════════════════════════════════════════════════╗" + Reset)
						fmt.Println(Blue + Bold + "║                      🛡️ 预防措施                                        ║" + Reset)
						fmt.Println(Blue + Bold + "╚══════════════════════════════════════════════════════════════════════════╝" + Reset)
						fmt.Println(Blue + "   " + line + Reset)
					} else if strings.Contains(line, ":") && !strings.Contains(line, " ") {
						// 可能是表格标题
						fmt.Println("\n" + Bold + "📊 " + line + Reset)
					} else {
						fmt.Println("   " + line)
					}
				}
				fmt.Println("\n" + Green + Bold + "╔══════════════════════════════════════════════════════════════════════════╗" + Reset)
				fmt.Println(Green + Bold + "║                         报告结束                                        ║" + Reset)
				fmt.Println(Green + Bold + "╚══════════════════════════════════════════════════════════════════════════╝" + Reset)

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

func detectEnvironment() []string {
	var envList []string
	
	if out, _ := exec.Command("sh", "-c", "docker info >/dev/null 2>&1 && echo docker").CombinedOutput(); strings.Contains(string(out), "docker") {
		envList = append(envList, "Docker 环境")
	}
	
	if out, _ := exec.Command("sh", "-c", "kubectl version --client >/dev/null 2>&1 && echo k8s").CombinedOutput(); strings.Contains(string(out), "k8s") {
		envList = append(envList, "Kubernetes 客户端")
		
		if out, _ := exec.Command("sh", "-c", "kubectl cluster-info >/dev/null 2>&1 && echo connected").CombinedOutput(); strings.Contains(string(out), "connected") {
			envList = append(envList, "K8s 集群已连接")
		}
		
		if out, _ := exec.Command("sh", "-c", "kubectl get ns 2>/dev/null | grep -i prometheus | head -1 | awk '{print $1}'").CombinedOutput(); strings.TrimSpace(string(out)) != "" {
			envList = append(envList, "Prometheus 已部署")
		}
	}
	
	if out, _ := exec.Command("sh", "-c", "systemctl is-active docker >/dev/null 2>&1 && echo active").CombinedOutput(); strings.Contains(string(out), "active") {
		envList = append(envList, "Docker 服务运行中")
	}
	
	if out, _ := exec.Command("sh", "-c", "which helm >/dev/null 2>&1 && echo helm").CombinedOutput(); strings.Contains(string(out), "helm") {
		envList = append(envList, "Helm 客户端")
	}
	
	if out, _ := exec.Command("sh", "-c", "which nali >/dev/null 2>&1 && echo nali").CombinedOutput(); strings.Contains(string(out), "nali") {
		envList = append(envList, "nali IP归属地查询工具")
	}
	
	return envList
}
