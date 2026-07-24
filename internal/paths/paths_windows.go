//go:build windows

package paths

import (
	"os"
	"path/filepath"
)

// xdgConfigHome 返回 Windows 配置根目录：优先 %APPDATA%，回退 %USERPROFILE%\AppData\Roaming。
// Windows 上 "XDG" 名称不适用，但保留函数名以统一 paths.go 的调用。
func xdgConfigHome() string {
	if v := os.Getenv("APPDATA"); v != "" {
		return v
	}
	// 回退：手动拼接 %USERPROFILE%\AppData\Roaming
	if h := homeDir(); h != "" {
		return filepath.Join(h, "AppData", "Roaming")
	}
	return "lindiag" // 最后兜底：相对路径
}

// xdgDataHome 返回 Windows 数据根目录：优先 %LOCALAPPDATA%，回退 %USERPROFILE%\AppData\Local。
// 配置（Roaming）和数据（Local）分离符合 Windows Known Folder 规范：
//   - Roaming: 跨机器漫游的配置（config / whitelist / rules）
//   - Local:   机器本地的数据（history / report）
func xdgDataHome() string {
	if v := os.Getenv("LOCALAPPDATA"); v != "" {
		return v
	}
	// 回退：手动拼接 %USERPROFILE%\AppData\Local
	if h := homeDir(); h != "" {
		return filepath.Join(h, "AppData", "Local")
	}
	return "lindiag" // 最后兜底：相对路径
}

// homeDir 返回用户主目录。
// Windows 下优先用 os.UserHomeDir()（会读 %USERPROFILE%），回退到环境变量。
func homeDir() string {
	if h, err := os.UserHomeDir(); err == nil && h != "" {
		return h
	}
	return os.Getenv("USERPROFILE")
}
