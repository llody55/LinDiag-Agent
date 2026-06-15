package safety

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/LinDiag-Agent/internal/llm"
)

// RiskLevel 风险级别
type RiskLevel int

const (
	RiskLevelSafe RiskLevel = iota
	RiskLevelLow
	RiskLevelMedium
	RiskLevelHigh
	RiskLevelCritical
)

// CommandAnalyzer 命令分析器
type CommandAnalyzer struct{}

// NewCommandAnalyzer 创建一个新的命令分析器
func NewCommandAnalyzer() *CommandAnalyzer {
	return &CommandAnalyzer{}
}

// AnalyzeCommand 分析命令的风险
func (a *CommandAnalyzer) AnalyzeCommand(cmd string) (RiskLevel, string) {
	// 1. 分析命令名称
	cmdParts := strings.Fields(cmd)
	if len(cmdParts) == 0 {
		return RiskLevelSafe, "空命令"
	}

	cmdName := cmdParts[0]

	// 2. 检查命令是否在白名单中
	// 特殊处理 sudo 命令
	if cmdName == "sudo" && len(cmdParts) > 1 {
		// 获取 sudo 后面的命令
		sudoCmd := cmdParts[1]
		// 检查 sudo 后面的命令是否在白名单中
		for _, safeCmd := range safeCommands {
			if sudoCmd == safeCmd {
				// 检查是否包含危险操作
				dangerousOps := []string{"-i", "-e", "-exec", "rm", "mv", "chmod", "chown", "reboot", "poweroff", "shutdown", "halt", "umount", "mount", "fsck", "dd", "sed -i", "apt-get", "usermod"}
				hasDangerousOp := false
				for _, op := range dangerousOps {
					if strings.Contains(cmd, " "+op+" ") || strings.Contains(cmd, " "+op+"-") || strings.HasPrefix(cmd, op+" ") {
						hasDangerousOp = true
						break
					}
				}
				if hasDangerousOp {
					return RiskLevelHigh, "sudo 命令包含危险操作"
				}
				// sudo 执行的是安全命令，视为低风险
				return RiskLevelLow, "sudo 执行安全命令"
			}
		}
	}

	// 检查普通命令是否在白名单中
	for _, safeCmd := range safeCommands {
		if cmdName == safeCmd {
			// 特殊处理 find 命令，检查是否包含危险的 -exec 参数
			if cmdName == "find" && strings.Contains(cmd, "-exec") {
				// 检查 -exec 后面是否包含危险命令
				if strings.Contains(cmd, "rm ") || strings.Contains(cmd, "rm-") || strings.Contains(cmd, "rm ") {
					return RiskLevelCritical, "find 命令包含危险的 rm 操作"
				}
				if strings.Contains(cmd, "mv ") || strings.Contains(cmd, "mv-") {
					return RiskLevelCritical, "find 命令包含危险的 mv 操作"
				}
				if strings.Contains(cmd, "chmod ") || strings.Contains(cmd, "chmod-") {
					return RiskLevelCritical, "find 命令包含危险的 chmod 操作"
				}
				if strings.Contains(cmd, "chown ") || strings.Contains(cmd, "chown-") {
					return RiskLevelCritical, "find 命令包含危险的 chown 操作"
				}
			}
			// 特殊处理包含 && 的命令，检查后面是否有危险命令
			if strings.Contains(cmd, "&&") {
				// 检查 && 后面是否包含危险命令
				dangerousCmds := []string{"rm", "mv", "chmod", "chown", "reboot", "poweroff", "shutdown", "halt", "umount", "mount", "fsck", "dd", "sed -i"}
				for _, dangerousCmd := range dangerousCmds {
					if strings.Contains(cmd, " "+dangerousCmd+" ") || strings.Contains(cmd, " "+dangerousCmd+"-") || strings.HasPrefix(cmd, dangerousCmd+" ") {
						return RiskLevelCritical, "命令包含危险操作的组合"
					}
				}
			}
			return RiskLevelSafe, "命令在安全白名单中"
		}
	}

	// 3. 检查命令是否匹配危险模式
	for _, rule := range riskyPatterns {
		if rule.pattern.MatchString(cmd) {
			return RiskLevelCritical, "命令匹配危险模式: " + rule.description
		}
	}

	// 4. 检查是否包含危险命令的组合
	if strings.Contains(cmd, " rm ") || strings.Contains(cmd, " rm-") || strings.HasPrefix(cmd, "rm ") || strings.HasSuffix(cmd, " rm") {
		return RiskLevelCritical, "命令包含危险的 rm 操作"
	}
	if strings.Contains(cmd, " mv ") || strings.Contains(cmd, " mv-") || strings.HasPrefix(cmd, "mv ") || strings.HasSuffix(cmd, " mv") {
		return RiskLevelCritical, "命令包含危险的 mv 操作"
	}
	if strings.Contains(cmd, " chmod ") || strings.Contains(cmd, " chmod-") || strings.HasPrefix(cmd, "chmod ") || strings.HasSuffix(cmd, " chmod") {
		return RiskLevelCritical, "命令包含危险的 chmod 操作"
	}
	if strings.Contains(cmd, " chown ") || strings.Contains(cmd, " chown-") || strings.HasPrefix(cmd, "chown ") || strings.HasSuffix(cmd, " chown") {
		return RiskLevelCritical, "命令包含危险的 chown 操作"
	}
	if strings.Contains(cmd, " reboot ") || strings.HasPrefix(cmd, "reboot ") || strings.HasSuffix(cmd, " reboot") || cmd == "reboot" {
		return RiskLevelCritical, "命令包含危险的 reboot 操作"
	}
	if strings.Contains(cmd, " poweroff ") || strings.HasPrefix(cmd, "poweroff ") || strings.HasSuffix(cmd, " poweroff") || cmd == "poweroff" {
		return RiskLevelCritical, "命令包含危险的 poweroff 操作"
	}
	if strings.Contains(cmd, " shutdown ") || strings.HasPrefix(cmd, "shutdown ") || strings.HasSuffix(cmd, " shutdown") || cmd == "shutdown" {
		return RiskLevelCritical, "命令包含危险的 shutdown 操作"
	}
	if strings.Contains(cmd, " halt ") || strings.HasPrefix(cmd, "halt ") || strings.HasSuffix(cmd, " halt") || cmd == "halt" {
		return RiskLevelCritical, "命令包含危险的 halt 操作"
	}
	if strings.Contains(cmd, " umount ") || strings.Contains(cmd, " umount-") || strings.HasPrefix(cmd, "umount ") || strings.HasSuffix(cmd, " umount") {
		return RiskLevelCritical, "命令包含危险的 umount 操作"
	}
	if strings.Contains(cmd, " mount ") || strings.Contains(cmd, " mount-") || strings.HasPrefix(cmd, "mount ") || strings.HasSuffix(cmd, " mount") {
		return RiskLevelCritical, "命令包含危险的 mount 操作"
	}
	if strings.Contains(cmd, " fsck ") || strings.Contains(cmd, " fsck-") || strings.HasPrefix(cmd, "fsck ") || strings.HasSuffix(cmd, " fsck") {
		return RiskLevelCritical, "命令包含危险的 fsck 操作"
	}
	if strings.Contains(cmd, " dd ") || strings.HasPrefix(cmd, "dd ") || strings.HasSuffix(cmd, " dd") {
		return RiskLevelCritical, "命令包含危险的 dd 操作"
	}
	if strings.Contains(cmd, "sed -i") {
		return RiskLevelCritical, "命令包含危险的 sed -i 操作"
	}
	if strings.Contains(cmd, " apt-get ") || strings.HasPrefix(cmd, "apt-get ") {
		return RiskLevelCritical, "命令包含危险的 apt-get 操作"
	}
	if strings.Contains(cmd, " usermod ") || strings.HasPrefix(cmd, "usermod ") {
		return RiskLevelCritical, "命令包含危险的 usermod 操作"
	}
	if strings.Contains(cmd, " passwd ") || strings.HasPrefix(cmd, "passwd ") {
		return RiskLevelCritical, "命令包含危险的 passwd 操作"
	}
	if strings.Contains(cmd, " >> /etc/") || strings.Contains(cmd, " > /etc/") {
		return RiskLevelCritical, "命令包含修改系统文件的操作"
	}
	// 检查 echo 命令写入系统文件
	if strings.Contains(cmd, "echo ") && (strings.Contains(cmd, " >> /etc/") || strings.Contains(cmd, " > /etc/")) {
		return RiskLevelCritical, "命令包含使用 echo 写入系统文件的操作"
	}
	// 检查 cat 命令写入系统文件
	if strings.Contains(cmd, "cat >> /etc/") || strings.Contains(cmd, "cat > /etc/") {
		return RiskLevelCritical, "命令包含使用 cat 写入系统文件的操作"
	}

	// 5. 分析命令的 && 和 || 操作
	if strings.Contains(cmd, "&&") || strings.Contains(cmd, "||") {
		// 检查是否包含危险命令
		dangerousCmds := []string{"rm", "mv", "chmod", "chown", "reboot", "poweroff", "shutdown", "halt", "umount", "mount", "fsck", "dd", "sed -i", "apt-get", "usermod"}
		hasDangerousCmd := false
		for _, dangerousCmd := range dangerousCmds {
			if strings.Contains(cmd, " "+dangerousCmd+" ") || strings.Contains(cmd, " "+dangerousCmd+"-") || strings.HasPrefix(cmd, dangerousCmd+" ") {
				hasDangerousCmd = true
				break
			}
		}
		if hasDangerousCmd {
			return RiskLevelHigh, "命令包含危险操作的组合"
		}

		// 检查是否只是读取操作
		safeReadCmds := []string{"cat", "grep", "head", "tail", "echo", "ls", "ps", "top", "free", "df", "uname", "hostname", "ip", "netstat", "ss", "who", "w", "last", "history", "uptime", "date", "cal", "du", "file", "which", "whereis", "type", "firewall-cmd", "ufw", "iptables"}
		cmdParts := strings.Fields(cmd)
		allSafe := true
		for _, part := range cmdParts {
			if part == "&&" || part == "||" {
				continue
			}
			if strings.HasPrefix(part, "-") {
				continue
			}
			safeCmd := false
			for _, safeReadCmd := range safeReadCmds {
				if part == safeReadCmd {
					safeCmd = true
					break
				}
			}
			if !safeCmd {
				allSafe = false
				break
			}
		}
		if allSafe {
			return RiskLevelSafe, "命令包含多个安全的读取操作"
		}

		return RiskLevelLow, "命令包含多个命令执行"
	}

	// 6. 分析命令参数
	for _, part := range cmdParts[1:] {
		if strings.Contains(part, "..") {
			return RiskLevelHigh, "命令包含路径遍历尝试"
		}
		// 只有当参数是路径时才检查相对路径
		if strings.Contains(part, "/") && !strings.HasPrefix(part, "/") && !strings.Contains(part, "-") {
			return RiskLevelMedium, "命令包含相对路径"
		}
	}

	// 7. 分析命令的重定向操作
	if strings.Contains(cmd, ">>") {
		// 追加写入操作，风险较高
		return RiskLevelMedium, "命令包含追加写入操作"
	} else if strings.Contains(cmd, ">") || strings.Contains(cmd, "<") {
		// 其他重定向操作，风险较低
		return RiskLevelLow, "命令包含重定向操作"
	}

	// 8. 分析命令的管道操作
	if strings.Contains(cmd, "|") {
		return RiskLevelLow, "命令包含管道操作"
	}

	// 默认风险级别
	return RiskLevelLow, "命令不在白名单中，存在潜在风险"
}

// AnalyzeCommandWithAI 使用AI分析命令的风险
func (a *CommandAnalyzer) AnalyzeCommandWithAI(cmd string) (RiskLevel, string, string) {
	// 首先使用规则分析
	riskLevel, reason := a.AnalyzeCommand(cmd)

	// 对所有命令使用AI进行分析，以捕获规则可能漏掉的危险
	// 构建安全分析提示
	messages := []llm.Message{
		{
			Role:    "system",
			Content: "你是一个命令安全分析专家，负责评估命令的危险程度。请分析以下命令的危险程度，并提供简要的风险评估。\n\n评估要求：\n1. 简要分析整个命令的功能和目的\n2. 识别主要的安全风险\n3. 评估对系统的危害程度\n4. 描述执行后的可能后果\n5. 提供简要的安全建议\n\n请以JSON格式输出评估结果，只包含两个字段：\n{\n  \"risk_level\": \"low|medium|high|critical\",\n  \"description\": \"一句话简要说明，包括命令功能、潜在风险、危害程度、可能后果和安全建议\"\n}\n\n风险等级定义：\n- low：低风险，安全读取操作，不会修改系统\n- medium：中风险，可能修改系统配置，但影响较小\n- high：高风险，可能对系统造成严重影响\n- critical：严重风险，可能导致系统崩溃或数据丢失\n\n请确保返回的是有效的JSON格式，不要包含任何额外的文本。",
		},
		{
			Role:    "user",
			Content: fmt.Sprintf("命令：%s", cmd),
		},
	}

	// 调用安全分析AI
	response := llm.CallSafetyAI(messages)

	// 解析AI返回的结果
	var aiResult struct {
		RiskLevel   string `json:"risk_level"`
		Description string `json:"description"`
	}

	err := json.Unmarshal([]byte(response), &aiResult)
	if err != nil {
		// 解析失败，尝试提取JSON部分
		startIdx := strings.Index(response, "{")
		endIdx := strings.LastIndex(response, "}")
		if startIdx != -1 && endIdx != -1 && endIdx > startIdx {
			jsonPart := response[startIdx : endIdx+1]
			err = json.Unmarshal([]byte(jsonPart), &aiResult)
			if err != nil {
				// 解析失败，返回原始风险评估
				return riskLevel, reason + "\n[AI分析] 无法获取详细风险评估", response
			}
		} else {
			// 解析失败，返回原始风险评估
			return riskLevel, reason + "\n[AI分析] 无法获取详细风险评估", response
		}
	}

	// 转换风险级别
	var aiRiskLevel RiskLevel
	switch aiResult.RiskLevel {
	case "low":
		aiRiskLevel = RiskLevelLow
	case "medium":
		aiRiskLevel = RiskLevelMedium
	case "high":
		aiRiskLevel = RiskLevelHigh
	case "critical":
		aiRiskLevel = RiskLevelCritical
	default:
		aiRiskLevel = riskLevel
	}

	// 构建详细的风险评估
	detailedReason := fmt.Sprintf("%s\n[AI分析] %s", reason, aiResult.Description)

	return aiRiskLevel, detailedReason, response
}
