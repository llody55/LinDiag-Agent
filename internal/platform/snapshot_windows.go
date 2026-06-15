package platform

import (
	"fmt"
	"strings"
)

// GetSnapshot 采集系统快照
func (p *WindowsPlatform) GetSnapshot(cmds []string) string {
	var ctx strings.Builder
	ctx.WriteString("=== 系统初始快照 ===\n")

	// 使用 gopsutil 获取系统信息
	systemInfo := GetSystemInfo()
	ctx.WriteString(fmt.Sprintf("主机名: %s\n", systemInfo.Hostname))
	ctx.WriteString(fmt.Sprintf("操作系统: %s %s\n", systemInfo.Platform, systemInfo.PlatformVersion))
	ctx.WriteString(fmt.Sprintf("CPU: %s\n", systemInfo.CPU))
	ctx.WriteString(fmt.Sprintf("内存: %s\n", systemInfo.Memory))
	ctx.WriteString(fmt.Sprintf("磁盘: %s\n", systemInfo.Disk))
	ctx.WriteString(fmt.Sprintf("网络: %s\n", systemInfo.Network))
	ctx.WriteString("\n")

	// 然后根据模式添加特定的基础命令
	for _, c := range cmds {
		// 转换Linux命令为Windows等效命令
		windowsCmd := p.convertLinuxToWindowsCmd(c)
		out, _ := p.ExecuteCommand(windowsCmd)
		ctx.WriteString(fmt.Sprintf("$ %s\n%s\n\n", c, out))
	}

	return ctx.String()
}
