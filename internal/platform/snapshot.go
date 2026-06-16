package platform

import (
	"fmt"
	"os/exec"
	"strings"
)

// GetSnapshot 采集系统快照
func GetSnapshot(cmds []string) string {
	var ctx strings.Builder
	ctx.WriteString("=== 系统初始快照 ===\n")

	// 先收集最基础的系统信息
	basicCmds := []string{
		"uname -a",
		"cat /etc/os-release 2>/dev/null || cat /etc/redhat-release 2>/dev/null || cat /etc/debian_version 2>/dev/null || echo 'OS info not found'",
		"lsb_release -a 2>/dev/null || echo 'lsb_release not available'",
		"arch",
	}

	for _, c := range basicCmds {
		cmd := "timeout 10 " + c
		out, _ := exec.Command("sh", "-c", cmd).CombinedOutput()
		ctx.WriteString(fmt.Sprintf("$ %s\n%s\n\n", c, string(out)))
	}

	// 然后根据模式添加特定的基础命令
	for _, c := range cmds {
		cmd := "timeout 10 " + c
		out, _ := exec.Command("sh", "-c", cmd).CombinedOutput()
		ctx.WriteString(fmt.Sprintf("$ %s\n%s\n\n", c, string(out)))
	}

	return ctx.String()
}
