//go:build windows

package platform

import (
	"runtime"
)

// NewPlatform 根据当前操作系统创建相应的平台实例
func NewPlatform() Platform {
	// 在运行时检测操作系统
	if runtime.GOOS == "windows" {
		// 返回Windows平台实例
		return NewWindowsPlatform()
	}
	// 默认使用Linux平台实现
	return NewLinuxPlatform()
}
