package safety

import (
	"bufio"
	"os"
	"regexp"
	"strings"
)

type RiskLevel int

const (
	RiskLevelSafe RiskLevel = iota
	RiskLevelLow
	RiskLevelMedium
	RiskLevelHigh
	RiskLevelCritical
)

type CommandInfo struct {
	RiskLevel    RiskLevel
	Reason       string
	Explanation  string
}

type CommandAnalyzer struct{}

func NewCommandAnalyzer() *CommandAnalyzer {
	return &CommandAnalyzer{}
}

var safeCommands []string = []string{
	"uptime", "free", "df", "ps", "top", "dmesg", "ls", "cat", "echo",
	"date", "who", "w", "uname", "hostname", "id", "groups", "netstat",
	"ss", "ifconfig", "ip", "ping", "traceroute", "curl", "wget",
	"head", "tail", "grep", "find", "less", "more", "wc", "du", "which",
	"whereis", "file", "stat", "last", "history", "netstat", "dig", "nslookup",
	"systemctl", "journalctl", "kubectl", "docker", "git", "env", "printenv",
}

var riskyPatterns = []struct {
	pattern     *regexp.Regexp
	description string
}{
	{regexp.MustCompile(`^\s*rm\s+-[rf]+\s+/`), "递归删除根目录"},
	{regexp.MustCompile(`^\s*rm\s+-[rf]+\s*\.{2}`), "删除上级目录"},
	{regexp.MustCompile(`^\s*mkfs\s+`), "格式化磁盘"},
	{regexp.MustCompile(`^\s*dd\s+if=\S+\s+of=/dev/`), "向设备写入数据"},
	{regexp.MustCompile(`^\s*chmod\s+777\s+/`), "全局可写"},
	{regexp.MustCompile(`^\s*chown\s+\S+:\S+\s+/`), "更改根目录所有权"},
}

var commandDescriptions = map[string]string{
	"uptime":      "查看系统运行时间和负载",
	"free":        "查看内存使用情况",
	"df":          "查看磁盘使用情况",
	"ps":          "查看进程状态",
	"top":         "实时查看系统资源使用",
	"dmesg":       "查看内核日志",
	"ls":          "列出目录内容",
	"cat":         "查看文件内容",
	"echo":        "输出文本",
	"date":        "显示日期时间",
	"who":         "查看当前登录用户",
	"w":           "查看系统负载和用户",
	"uname":       "查看系统信息",
	"hostname":    "查看主机名",
	"id":          "查看用户ID信息",
	"groups":      "查看用户组",
	"netstat":     "查看网络连接状态",
	"ss":          "查看网络套接字",
	"ifconfig":    "查看网络接口配置",
	"ip":          "网络配置工具",
	"ping":        "测试网络连通性",
	"traceroute":  "追踪路由",
	"curl":        "网络请求工具",
	"wget":        "下载文件",
	"head":        "查看文件头部",
	"tail":        "查看文件尾部",
	"grep":        "文本搜索",
	"find":        "查找文件",
	"less":        "分页查看文件",
	"more":        "分页查看文件",
	"wc":          "统计文件行数/字数",
	"du":          "查看目录大小",
	"which":       "查找命令位置",
	"whereis":     "查找命令位置",
	"file":        "识别文件类型",
	"stat":        "查看文件状态",
	"last":        "查看登录历史",
	"history":     "查看命令历史",
	"dig":         "DNS查询",
	"nslookup":    "DNS查询",
	"systemctl":   "系统服务管理",
	"journalctl":  "查看系统日志",
	"kubectl":     "Kubernetes命令行工具",
	"docker":      "Docker命令行工具",
	"git":         "版本控制工具",
	"env":         "查看环境变量",
	"printenv":    "查看环境变量",
	"rm":          "删除文件或目录",
	"mv":          "移动或重命名文件",
	"chmod":       "修改文件权限",
	"chown":       "修改文件所有者",
	"reboot":      "重启系统",
	"poweroff":    "关闭系统",
	"shutdown":    "关闭系统",
	"halt":        "停止系统",
	"umount":      "卸载挂载点",
	"mount":       "挂载文件系统",
	"fsck":        "文件系统检查",
	"dd":          "数据复制工具",
	"sed":         "文本替换",
}

func LoadWhitelist(filename string) error {
	file, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	var whitelist []string

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" && !strings.HasPrefix(line, "#") {
			whitelist = append(whitelist, line)
		}
	}

	if len(whitelist) > 0 {
		safeCommands = whitelist
	}

	return scanner.Err()
}

func GetSafeCommands() []string {
	return safeCommands
}

func (a *CommandAnalyzer) AnalyzeCommand(cmd string) *CommandInfo {
	cmdParts := strings.Fields(cmd)
	if len(cmdParts) == 0 {
		return &CommandInfo{RiskLevel: RiskLevelSafe, Reason: "空命令", Explanation: "这是一个空命令"}
	}

	cmdName := cmdParts[0]

	for _, safeCmd := range safeCommands {
		if cmdName == safeCmd {
			if cmdName == "find" && strings.Contains(cmd, "-exec") {
				if strings.Contains(cmd, "rm ") || strings.Contains(cmd, "rm-") {
					return &CommandInfo{RiskLevel: RiskLevelCritical, Reason: "find 命令包含危险的 rm 操作", Explanation: "该命令使用find配合rm删除文件，可能会误删重要文件"}
				}
				if strings.Contains(cmd, "mv ") || strings.Contains(cmd, "mv-") {
					return &CommandInfo{RiskLevel: RiskLevelCritical, Reason: "find 命令包含危险的 mv 操作", Explanation: "该命令使用find配合mv移动文件，可能会破坏文件结构"}
				}
				if strings.Contains(cmd, "chmod ") || strings.Contains(cmd, "chmod-") {
					return &CommandInfo{RiskLevel: RiskLevelCritical, Reason: "find 命令包含危险的 chmod 操作", Explanation: "该命令使用find配合chmod修改权限，可能导致安全问题"}
				}
				if strings.Contains(cmd, "chown ") || strings.Contains(cmd, "chown-") {
					return &CommandInfo{RiskLevel: RiskLevelCritical, Reason: "find 命令包含危险的 chown 操作", Explanation: "该命令使用find配合chown修改所有者，可能导致权限混乱"}
				}
			}
			if strings.Contains(cmd, "&&") {
				if (strings.Contains(cmd, " rm ") || strings.Contains(cmd, " rm-") || strings.Contains(cmd, "rm ")) ||
					(strings.Contains(cmd, " mv ") || strings.Contains(cmd, " mv-") || strings.Contains(cmd, "mv ")) ||
					(strings.Contains(cmd, " chmod ") || strings.Contains(cmd, " chmod-") || strings.Contains(cmd, "chmod ")) ||
					(strings.Contains(cmd, " chown ") || strings.Contains(cmd, " chown-") || strings.Contains(cmd, "chown ")) ||
					(strings.Contains(cmd, " reboot ") || strings.Contains(cmd, "reboot ")) ||
					(strings.Contains(cmd, " poweroff ") || strings.Contains(cmd, "poweroff ")) ||
					(strings.Contains(cmd, " shutdown ") || strings.Contains(cmd, "shutdown ")) ||
					(strings.Contains(cmd, " halt ") || strings.Contains(cmd, "halt ")) ||
					(strings.Contains(cmd, " umount ") || strings.Contains(cmd, " umount-") || strings.Contains(cmd, "umount ")) ||
					(strings.Contains(cmd, " mount ") || strings.Contains(cmd, " mount-") || strings.Contains(cmd, "mount ")) ||
					(strings.Contains(cmd, " fsck ") || strings.Contains(cmd, " fsck-") || strings.Contains(cmd, "fsck ")) ||
					(strings.Contains(cmd, " dd ") || strings.Contains(cmd, "dd ")) ||
					strings.Contains(cmd, "sed -i") {
					return &CommandInfo{RiskLevel: RiskLevelCritical, Reason: "命令包含危险操作的组合", Explanation: "该命令组合包含多个操作，其中包含危险命令，可能造成系统损坏"}
				}
			}
			desc := commandDescriptions[cmdName]
			if desc == "" {
				desc = "查询系统信息"
			}
			return &CommandInfo{RiskLevel: RiskLevelSafe, Reason: "命令在安全白名单中", Explanation: desc}
		}
	}

	for _, rule := range riskyPatterns {
		if rule.pattern.MatchString(cmd) {
			return &CommandInfo{RiskLevel: RiskLevelCritical, Reason: "命令匹配危险模式: " + rule.description, Explanation: "该命令执行后可能导致系统严重损坏，请谨慎操作"}
		}
	}

	if strings.Contains(cmd, " rm ") || strings.Contains(cmd, " rm-") || strings.HasPrefix(cmd, "rm ") || strings.HasSuffix(cmd, " rm") {
		return &CommandInfo{RiskLevel: RiskLevelCritical, Reason: "命令包含危险的 rm 操作", Explanation: "rm命令用于删除文件或目录，错误使用可能导致数据丢失"}
	}
	if strings.Contains(cmd, " mv ") || strings.Contains(cmd, " mv-") || strings.HasPrefix(cmd, "mv ") || strings.HasSuffix(cmd, " mv") {
		return &CommandInfo{RiskLevel: RiskLevelCritical, Reason: "命令包含危险的 mv 操作", Explanation: "mv命令用于移动或重命名文件，可能导致文件丢失或覆盖"}
	}
	if strings.Contains(cmd, " chmod ") || strings.Contains(cmd, " chmod-") || strings.HasPrefix(cmd, "chmod ") || strings.HasSuffix(cmd, " chmod") {
		return &CommandInfo{RiskLevel: RiskLevelCritical, Reason: "命令包含危险的 chmod 操作", Explanation: "chmod命令用于修改文件权限，错误设置可能导致安全漏洞"}
	}
	if strings.Contains(cmd, " chown ") || strings.Contains(cmd, " chown-") || strings.HasPrefix(cmd, "chown ") || strings.HasSuffix(cmd, " chown") {
		return &CommandInfo{RiskLevel: RiskLevelCritical, Reason: "命令包含危险的 chown 操作", Explanation: "chown命令用于修改文件所有者，错误设置可能导致权限混乱"}
	}
	if strings.Contains(cmd, " reboot ") || strings.HasPrefix(cmd, "reboot ") || strings.HasSuffix(cmd, " reboot") || cmd == "reboot" {
		return &CommandInfo{RiskLevel: RiskLevelCritical, Reason: "命令包含危险的 reboot 操作", Explanation: "reboot命令会重启系统，当前工作可能丢失"}
	}
	if strings.Contains(cmd, " poweroff ") || strings.HasPrefix(cmd, "poweroff ") || strings.HasSuffix(cmd, " poweroff") || cmd == "poweroff" {
		return &CommandInfo{RiskLevel: RiskLevelCritical, Reason: "命令包含危险的 poweroff 操作", Explanation: "poweroff命令会关闭系统电源，所有未保存的工作将丢失"}
	}
	if strings.Contains(cmd, " shutdown ") || strings.HasPrefix(cmd, "shutdown ") || strings.HasSuffix(cmd, " shutdown") || cmd == "shutdown" {
		return &CommandInfo{RiskLevel: RiskLevelCritical, Reason: "命令包含危险的 shutdown 操作", Explanation: "shutdown命令会关闭系统，所有未保存的工作将丢失"}
	}
	if strings.Contains(cmd, " halt ") || strings.HasPrefix(cmd, "halt ") || strings.HasSuffix(cmd, " halt") || cmd == "halt" {
		return &CommandInfo{RiskLevel: RiskLevelCritical, Reason: "命令包含危险的 halt 操作", Explanation: "halt命令会停止系统，所有未保存的工作将丢失"}
	}
	if strings.Contains(cmd, " umount ") || strings.Contains(cmd, " umount-") || strings.HasPrefix(cmd, "umount ") || strings.HasSuffix(cmd, " umount") {
		return &CommandInfo{RiskLevel: RiskLevelCritical, Reason: "命令包含危险的 umount 操作", Explanation: "umount命令会卸载文件系统，可能导致数据丢失"}
	}
	if strings.Contains(cmd, " mount ") || strings.Contains(cmd, " mount-") || strings.HasPrefix(cmd, "mount ") || strings.HasSuffix(cmd, " mount") {
		return &CommandInfo{RiskLevel: RiskLevelCritical, Reason: "命令包含危险的 mount 操作", Explanation: "mount命令用于挂载文件系统，错误挂载可能导致数据损坏"}
	}
	if strings.Contains(cmd, " fsck ") || strings.Contains(cmd, " fsck-") || strings.HasPrefix(cmd, "fsck ") || strings.HasSuffix(cmd, " fsck") {
		return &CommandInfo{RiskLevel: RiskLevelCritical, Reason: "命令包含危险的 fsck 操作", Explanation: "fsck命令用于检查和修复文件系统，可能导致数据丢失"}
	}
	if strings.Contains(cmd, " dd ") || strings.HasPrefix(cmd, "dd ") || strings.HasSuffix(cmd, " dd") {
		return &CommandInfo{RiskLevel: RiskLevelCritical, Reason: "命令包含危险的 dd 操作", Explanation: "dd命令用于复制数据，错误使用可能覆盖重要数据"}
	}
	if strings.Contains(cmd, "sed -i") {
		return &CommandInfo{RiskLevel: RiskLevelCritical, Reason: "命令包含危险的 sed -i 操作", Explanation: "sed -i命令会直接修改文件内容，可能导致文件损坏"}
	}

	for _, part := range cmdParts[1:] {
		if strings.Contains(part, "..") {
			return &CommandInfo{RiskLevel: RiskLevelHigh, Reason: "命令包含路径遍历尝试", Explanation: "命令中包含路径遍历字符'..'，可能访问非预期的目录"}
		}
		if strings.Contains(part, "/") && !strings.HasPrefix(part, "/") {
			return &CommandInfo{RiskLevel: RiskLevelMedium, Reason: "命令包含相对路径", Explanation: "命令使用相对路径，可能执行非预期位置的文件"}
		}
	}

	if strings.Contains(cmd, ">") || strings.Contains(cmd, ">>") || strings.Contains(cmd, "<") {
		return &CommandInfo{RiskLevel: RiskLevelMedium, Reason: "命令包含重定向操作", Explanation: "命令包含重定向操作，可能覆盖或创建文件"}
	}

	if strings.Contains(cmd, "|") {
		return &CommandInfo{RiskLevel: RiskLevelLow, Reason: "命令包含管道操作", Explanation: "命令使用管道连接多个命令，用于数据处理"}
	}

	if strings.Contains(cmd, "&&") {
		return &CommandInfo{RiskLevel: RiskLevelMedium, Reason: "命令包含多个命令执行", Explanation: "命令包含多个操作，依次执行"}
	}

	desc := commandDescriptions[cmdName]
	if desc == "" {
		desc = "执行系统命令"
	}
	return &CommandInfo{RiskLevel: RiskLevelLow, Reason: "命令不在白名单中，存在潜在风险", Explanation: desc}
}