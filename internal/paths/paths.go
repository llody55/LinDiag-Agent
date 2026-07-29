// Package paths 统一管理 LinDiag-Agent 的所有应用文件路径。
//
// 设计原则（Phase 3 Task 9）：
//
//  1. 遵循平台规范路径：
//     Linux/macOS: XDG Base Directory Specification
//       - 配置文件走 $XDG_CONFIG_HOME/lindiag/（默认 ~/.config/lindiag/）
//       - 数据文件走 $XDG_DATA_HOME/lindiag/（默认 ~/.local/share/lindiag/）
//     Windows: Known Folder 规范
//       - 配置文件走 %APPDATA%\lindiag\（如 C:\Users\<user>\AppData\Roaming\lindiag\）
//       - 数据文件走 %LOCALAPPDATA%\lindiag\（如 C:\Users\<user>\AppData\Local\lindiag\）
//
//  2. 所有路径在此处集中计算，其他包不得再写死路径硬编码。
//
//  3. Ensure* 函数负责创建目录（含父级），调用方在写入前调用一次。
//
//  4. 用户在命令行显式传入的路径保持原样透传，不做路径拼接。
package paths

import (
	"fmt"
	"os"
	"path/filepath"
)

// ConfigDir 返回存放配置文件的根目录。
// Linux/macOS: $XDG_CONFIG_HOME/lindiag
// Windows: %APPDATA%\lindiag
func ConfigDir() string {
	return filepath.Join(xdgConfigHome(), "lindiag")
}

// DataDir 返回存放数据文件（历史/报告）的根目录。
// Linux/macOS: $XDG_DATA_HOME/lindiag
// Windows: %LOCALAPPDATA%\lindiag
func DataDir() string {
	return filepath.Join(xdgDataHome(), "lindiag")
}

// ConfigFile 返回主配置文件 config.json 的完整路径。
func ConfigFile() string { return filepath.Join(ConfigDir(), "config.json") }

// UserPrefsFile 返回用户偏好文件 user_prefs.json 的完整路径。
func UserPrefsFile() string { return filepath.Join(ConfigDir(), "user_prefs.json") }

// WhitelistFile 返回安全命令白名单文件 whitelist.txt 的完整路径。
func WhitelistFile() string { return filepath.Join(ConfigDir(), "whitelist.txt") }

// RulesFile 返回外置规则文件 rules.txt 的完整路径。
func RulesFile() string { return filepath.Join(ConfigDir(), "rules.txt") }

// HistoryFile 返回带时间戳的历史文件完整路径（落在 DataDir 下）。
// timestamp 形如 "20060102_150405"。
func HistoryFile(timestamp string) string {
	return filepath.Join(DataDir(), fmt.Sprintf("history_%s.json", timestamp))
}

// ReportFile 返回报告文件完整路径（落在 DataDir 下）。
// timestamp 形如 "20060102_150405"；format ∈ {"md","html","pdf"}。
func ReportFile(timestamp, format string) string {
	return filepath.Join(DataDir(), fmt.Sprintf("report_%s.%s", timestamp, format))
}

// EnsureConfigDir 确保配置目录存在（含所有父级，权限 0700）。
// 供 SaveConfig / SaveUserPreferences 等写入前调用。
func EnsureConfigDir() error { return os.MkdirAll(ConfigDir(), 0700) }

// EnsureDataDir 确保数据目录存在（含所有父级，权限 0755）。
// 供 Session 落盘历史 / Report 写入文件前调用。
func EnsureDataDir() error { return os.MkdirAll(DataDir(), 0755) }
