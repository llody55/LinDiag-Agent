//go:build linux || darwin

package platform

import (
	"context"
	"os/exec"
	"strings"
)

// newShellCommand 创建一个执行指定命令的 shell 进程（Unix 实现）。
// Unix 下统一用 sh -c，保持与原有行为一致。
func newShellCommand(cmd string) *exec.Cmd {
	return exec.Command("sh", "-c", cmd)
}

// newShellCommandContext 同 newShellCommand，但支持 context 超时控制。
func newShellCommandContext(ctx context.Context, cmd string) *exec.Cmd {
	return exec.CommandContext(ctx, "sh", "-c", cmd)
}

// getIPAddress 获取本机首选 IP 地址（Unix 实现）。
// hostname -I 在 Linux 上列出所有 IPv4 地址，取第一个。
func getIPAddress() string {
	cmd := "hostname -I | awk '{print $1}'"
	out, err := exec.Command("sh", "-c", cmd).CombinedOutput()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}

// isBackgroundCommand 判断命令是否以后台方式运行（Unix 语义：以 & 结尾）。
func isBackgroundCommand(cmd string) bool {
	return strings.HasSuffix(strings.TrimSpace(cmd), "&")
}

// isShellAvailable 检查当前平台的 shell 是否可用。
// Unix 下 sh 几乎必然存在，直接返回 true。
func isShellAvailable() bool {
	return true
}

// snapshotPromptPrefix 返回快照格式的命令提示符前缀。
// Unix 使用 "$ " 前缀（传统 shell 提示符风格）。
func snapshotPromptPrefix() string {
	return "$ "
}
