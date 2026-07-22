//go:build windows

package output

import (
	"os"
	"syscall"
)

// isTerminal 判断文件是否为终端（Windows 实现）。
// 通过 GetConsoleMode 判断：若 fd 是控制台句柄，调用成功，即为终端。
// 对管道/文件返回失败，即非终端。
func isTerminal(f *os.File) bool {
	if f == nil {
		return false
	}
	var mode uint32
	err := syscall.GetConsoleMode(syscall.Handle(f.Fd()), &mode)
	return err == nil
}
