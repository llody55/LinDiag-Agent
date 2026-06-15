package safety

import (
	"bufio"
	"os"
	"regexp"
	"strings"
)

var (
	// 危险命令规则，使用正则表达式
	riskyPatterns = []struct {
		pattern     *regexp.Regexp
		description string
	}{
		{regexp.MustCompile(`^\s*rm\s+(-r|--recursive)?\s*`), "删除命令，可能导致数据丢失"},
		{regexp.MustCompile(`^\s*mv\s+`), "移动命令，可能覆盖文件"},
		{regexp.MustCompile(`^\s*cp\s+`), "复制命令，可能覆盖文件"},
		{regexp.MustCompile(`^\s*chmod\s+`), "权限修改命令，可能导致安全问题"},
		{regexp.MustCompile(`^\s*chown\s+`), "所有权修改命令，可能导致安全问题"},
		{regexp.MustCompile(`^\s*kill\s+`), "进程终止命令，可能导致服务中断"},
		{regexp.MustCompile(`^\s*reboot\s*`), "系统重启命令"},
		{regexp.MustCompile(`^\s*poweroff\s*`), "系统关机命令"},
		{regexp.MustCompile(`^\s*halt\s*`), "系统停止命令"},
		{regexp.MustCompile(`^\s*shutdown\s*`), "系统关机命令"},
		{regexp.MustCompile(`^\s*umount\s+`), "卸载文件系统命令"},
		{regexp.MustCompile(`^\s*mount\s+`), "挂载文件系统命令"},
		{regexp.MustCompile(`^\s*fsck\s+`), "文件系统检查命令，可能导致数据丢失"},
		{regexp.MustCompile(`^\s*dd\s+`), "数据复制命令，可能导致数据覆盖"},
		{regexp.MustCompile(`^\s*sed\s+-i\s+`), "直接修改文件的sed命令"},
		{regexp.MustCompile(`^\s*systemctl\s+(start|stop|restart|enable|disable|reload)\s+`), "系统服务管理命令"},
		{regexp.MustCompile(`^\s*docker\s+(run|build|push|pull|rm|rmi|stop|start|restart|exec|volume\s+rm|network\s+rm)\s+`), "Docker管理命令"},
		{regexp.MustCompile(`^\s*kubectl\s+(create|apply|delete|scale|patch|edit|rollout|taint|cordon|drain|uncordon)\s+`), "Kubernetes管理命令"},
	}
	
	// 安全命令白名单
	safeCommands []string
)

// LoadWhitelist 从文件加载白名单
func LoadWhitelist(filename string) error {
	file, err := os.Open(filename)
	if err != nil {
		// 如果文件不存在，使用默认白名单
		safeCommands = getDefaultWhitelist()
		return nil
	}
	defer file.Close()
	
	scanner := bufio.NewScanner(file)
	var whitelist []string
	
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// 跳过注释和空行
		if line != "" && !strings.HasPrefix(line, "#") {
			whitelist = append(whitelist, line)
		}
	}
	
	if len(whitelist) > 0 {
		safeCommands = whitelist
	} else {
		// 如果文件为空，使用默认白名单
		safeCommands = getDefaultWhitelist()
	}
	
	return scanner.Err()
}

// getDefaultWhitelist 获取默认白名单
func getDefaultWhitelist() []string {
	return []string{
		"ls", "pwd", "echo", "cat", "grep", "find", "ps", "top", "free", "df",
		"uname", "hostname", "ip", "netstat", "ss", "who", "w", "last", "history",
		"uptime", "date", "cal", "du", "file", "which", "whereis", "type",
	}
}

// GetSafeCommands 获取安全命令列表
func GetSafeCommands() []string {
	return safeCommands
}
