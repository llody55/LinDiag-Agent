// Package paths 统一管理 LinDiag-Agent 的所有应用文件路径。
//
// 设计原则（Phase 3 Task 9）：
//
//  1. 遵循 XDG Base Directory Specification：
//     - 配置文件走 $XDG_CONFIG_HOME/lindiag/（默认 ~/.config/lindiag/）
//     - 数据文件走 $XDG_DATA_HOME/lindiag/（默认 ~/.local/share/lindiag/）
//     这样支持无侵入的多用户/容器场景，避免历史/报告散落在 CWD。
//
//  2. 所有路径在此处集中计算，其他包不得再写死 ~/.config/lindiag
//     或硬编码 CWD 相对路径（user_prefs.json / whitelist.txt / rules.txt /
//     history_*.json / report_*.md 等）。新需求在这里加一个 getter，
//     消费方调用此处函数。
//
//  3. Ensure* 函数负责创建目录（含父级），调用方在写入前调用一次。
//     读取端不需要 Ensure，找不到文件就走旧回退逻辑。
//
//  4. 用户在命令行显式传入的路径（./lindiag-agent load <file> /
//     ./lindiag-agent report <file> <fmt>）保持原样透传，不做路径拼接，
//     以便用户能访问任意位置的历史文件（包括旧 CWD 文件）。
package paths

import (
	"fmt"
	"os"
	"path/filepath"
)

// ConfigDir 返回存放配置文件的根目录：$XDG_CONFIG_HOME/lindiag。
//
// 缺省回退到 $HOME/.config/lindiag。HOME 也取不到时回退到相对目录
// ".config/lindiag"（罕见；主要在裸容器/无 HOME 的测试场景出现）。
func ConfigDir() string {
	return filepath.Join(xdgConfigHome(), "lindiag")
}

// DataDir 返回存放数据文件（历史/报告）的根目录：$XDG_DATA_HOME/lindiag。
//
// 缺省回退到 $HOME/.local/share/lindiag。
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

// EnsureConfigDir 确保配置目录存在（含所有父级，权限 0755）。
// 供 SaveConfig / SaveUserPreferences 等写入前调用。
func EnsureConfigDir() error { return os.MkdirAll(ConfigDir(), 0755) }

// EnsureDataDir 确保数据目录存在（含所有父级，权限 0755）。
// 供 Session 落盘历史 / Report 写入文件前调用。
func EnsureDataDir() error { return os.MkdirAll(DataDir(), 0755) }

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
