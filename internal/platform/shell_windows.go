//go:build windows

package platform

import (
	"context"
	"os/exec"
	"strings"
)

// newShellCommand 创建一个执行指定命令的 shell 进程（Windows 实现）。
// Windows 下用 PowerShell，因为诊断 cmdlet 生态（Get-Process/Get-Volume/
// Get-EventLog 等）远比 cmd 丰富。
//
// -NoProfile：跳过用户 profile 加载，避免 profile 中的错误阻塞执行
// -NonInteractive：禁止弹出交互式提示（如 Read-Host），防止卡死
func newShellCommand(cmd string) *exec.Cmd {
	return exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", cmd)
}

// newShellCommandContext 同 newShellCommand，但支持 context 超时控制。
func newShellCommandContext(ctx context.Context, cmd string) *exec.Cmd {
	return exec.CommandContext(ctx, "powershell", "-NoProfile", "-NonInteractive", "-Command", cmd)
}

// getIPAddress 获取本机首选 IP 地址（Windows 实现）。
// 用 Get-NetIPAddress 获取第一个 IPv4 地址（排除回环）。
func getIPAddress() string {
	cmd := "Get-NetIPAddress -AddressFamily IPv4 | Where-Object { $_.InterfaceAlias -notlike '*Loopback*' -and $_.IPAddress -notlike '169.254.*' } | Select-Object -First 1 -ExpandProperty IPAddress"
	out, err := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", cmd).CombinedOutput()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}

// isBackgroundCommand 判断命令是否以后台方式运行。
// Windows PowerShell 的后台语法（Start-Job）与 Unix 的 & 不同，
// 当前不支持后台命令，统一返回 false。
func isBackgroundCommand(cmd string) bool {
	return false
}

// isShellAvailable 检查 PowerShell 是否可用。
// Windows 默认自带 PowerShell，但为稳健起见做一次探测。
func isShellAvailable() bool {
	_, err := exec.LookPath("powershell")
	return err == nil
}

// snapshotPromptPrefix 返回快照格式的命令提示符前缀。
// Windows 使用 "> " 前缀（PowerShell 提示符风格）。
func snapshotPromptPrefix() string {
	return "> "
}
