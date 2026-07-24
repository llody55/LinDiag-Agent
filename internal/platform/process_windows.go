//go:build windows

package platform

import (
	"fmt"
	"os/exec"
	"syscall"
)

// setProcessGroup Windows 下使用 CREATE_NEW_PROCESS_GROUP 创建新进程组，
// 使得子进程及其后代可以通过 taskkill /T 统一终止。
func setProcessGroup(c *exec.Cmd) {
	c.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
	}
}

// killProcessGroup 杀掉整个进程组（包括子进程树）。
// 使用 taskkill /T /F /PID 递归终止进程及其所有子进程，
// 等效于 Linux 的 kill(-pgid, SIGKILL)。
func killProcessGroup(c *exec.Cmd) {
	if c.Process == nil {
		return
	}
	pid := c.Process.Pid
	// taskkill /T: 终止指定进程及其子进程（进程树）
	// taskkill /F: 强制终止
	// taskkill /PID <pid>: 指定进程 ID
	killCmd := exec.Command("taskkill", "/T", "/F", "/PID", fmt.Sprintf("%d", pid))
	_ = killCmd.Run() // 忽略错误：进程可能已退出
}
