//go:build linux || darwin

package paths

import (
	"os"
	"path/filepath"
)

// xdgConfigHome 解析 $XDG_CONFIG_HOME；未设置时回退到 $HOME/.config。
func xdgConfigHome() string {
	if v := os.Getenv("XDG_CONFIG_HOME"); v != "" {
		return v
	}
	return filepath.Join(homeDir(), ".config")
}

// xdgDataHome 解析 $XDG_DATA_HOME；未设置时回退到 $HOME/.local/share。
func xdgDataHome() string {
	if v := os.Getenv("XDG_DATA_HOME"); v != "" {
		return v
	}
	return filepath.Join(homeDir(), ".local", "share")
}

// homeDir 解析 $HOME；取不到返回空串，由调用方决定如何处理。
// 优先用 os.UserHomeDir() 而非 os.Getenv("HOME")：
// 前者在某些环境（如 systemd 服务）会自动补齐，且跨平台行为更稳定。
func homeDir() string {
	if h, err := os.UserHomeDir(); err == nil && h != "" {
		return h
	}
	return os.Getenv("HOME")
}
