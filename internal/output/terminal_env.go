package output

import (
	"os"
	"strings"
)

// TerminalEnv 描述终端环境对中文 / Unicode 的支持情况。
//
// 设计目标：在不改变默认中文体验的前提下，为不支持中文的环境
// （裸 Linux 虚拟控制台、无 UTF-8 locale、重定向到文件等）
// 提供合理的降级，避免输出全是问号或空白。
type TerminalEnv struct {
	// IsTerminal stdout 是否为终端（非管道/重定向）。
	IsTerminal bool
	// Lang 相关环境变量 LANG/LC_ALL/LC_CTYPE 的值（用于判断 UTF-8）。
	Lang string
	// IsUTF8 locale 是否声明为 UTF-8（含 C.UTF-8）。
	IsUTF8 bool
	// IsLinuxConsole 是否为 Linux 内核原生虚拟控制台（TERM=linux）。
	// 此环境内核字体不含 CJK，无法显示中文，需 ASCII 降级。
	IsLinuxConsole bool
	// NeedASCII 是否应启用 ASCII 降级模式。
	// 触发条件：非终端；或 locale 非 UTF-8；或 Linux 原生控制台。
	NeedASCII bool
	// NeedNoColor 是否应禁用 ANSI 颜色。
	// 触发条件：非终端；或 NO_COLOR 环境变量已设置。
	NeedNoColor bool
}

// asciiMode 与 colorDisabled 是包级开关，由 SetTerminalEnv 设置。
// formatter.go 中的图标 / 制表符 / 颜色函数会读取这两个开关决定输出形态。
var (
	asciiMode     = false
	colorDisabled = false
)

// DetectTerminalEnv 检测当前终端环境，返回 TerminalEnv。
// 不修改任何环境变量，仅读取。调用方据此调用 SetTerminalEnv 应用降级。
func DetectTerminalEnv() TerminalEnv {
	env := TerminalEnv{
		IsTerminal: isTerminal(os.Stdout),
	}

	// 读取 locale 相关变量（LC_ALL 优先于 LC_CTYPE 优先于 LANG）
	env.Lang = firstNonEmpty(
		os.Getenv("LC_ALL"),
		os.Getenv("LC_CTYPE"),
		os.Getenv("LANG"),
	)
	env.IsUTF8 = isUTF8Locale(env.Lang)

	// TERM=linux 表示内核原生虚拟控制台（Ctrl+Alt+F1~F6），
	// 其字体不含 CJK，中文无法显示。
	env.IsLinuxConsole = os.Getenv("TERM") == "linux"

	// NO_COLOR 是社区约定（https://no-color.org/），
	// 设置该环境变量即请求程序禁用颜色。
	noColor := os.Getenv("NO_COLOR") != ""

	// 降级判定
	env.NeedASCII = !env.IsTerminal || !env.IsUTF8 || env.IsLinuxConsole
	env.NeedNoColor = !env.IsTerminal || noColor

	return env
}

// SetTerminalEnv 根据 TerminalEnv 应用包级降级开关。
// 应在 main 启动时调用一次，在任何 output.* 输出之前。
func SetTerminalEnv(env TerminalEnv) {
	asciiMode = env.NeedASCII
	colorDisabled = env.NeedNoColor
}

// firstNonEmpty 返回第一个非空字符串。
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// isUTF8Locale 判断 locale 字符串是否声明为 UTF-8。
// 匹配 "UTF-8" / "utf8" / "UTF8"（大小写不敏感，忽略连字符）。
func isUTF8Locale(loc string) bool {
	if loc == "" || loc == "C" || loc == "POSIX" {
		return false
	}
	normalized := strings.ToLower(strings.ReplaceAll(loc, "-", ""))
	return strings.Contains(normalized, "utf8")
}

// isTerminal 判断 stdout 是否为终端。
// 实现随平台不同：Unix 系见 terminal_unix.go，Windows 见 terminal_windows.go。
