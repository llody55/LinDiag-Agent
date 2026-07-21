//go:build linux || darwin

package platform

import (
	"os/exec"
	"syscall"
)

// setProcessGroup 设置进程组，超时时可以杀掉整个进程组（包括子进程）
func setProcessGroup(c *exec.Cmd) {
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// killProcessGroup 杀掉整个进程组
func killProcessGroup(c *exec.Cmd) {
	if c.Process != nil {
		syscall.Kill(-c.Process.Pid, syscall.SIGKILL)
	}
}
