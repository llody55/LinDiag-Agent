//go:build !windows

package platform

// NewPlatform 根据当前操作系统创建相应的平台实例
func NewPlatform() Platform {
	// 在非Windows环境下，直接返回Linux平台实例
	return NewLinuxPlatform()
}
