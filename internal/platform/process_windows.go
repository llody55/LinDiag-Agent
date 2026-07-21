//go:build windows

package platform

import "os/exec"

// setProcessGroup Windows 下不需要进程组设置
func setProcessGroup(c *exec.Cmd) {
	// Windows 使用 Job Object 机制，这里简化处理
}

// killProcessGroup Windows 下杀掉进程
func killProcessGroup(c *exec.Cmd) {
	if c.Process != nil {
		c.Process.Kill()
	}
}
