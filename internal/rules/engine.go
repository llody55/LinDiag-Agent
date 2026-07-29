// Package rules 实现本地诊断规则引擎。
//
// 设计目标：
//
// 在快照采集后立即用规则做一次"本地体检"，把命中阈值的已知问题直接产出为
// diagnosis.Issue。这些 Issue 与 LLM 产出的 Issue 走同一契约（MergeByTitle
// 去重），让 LLM 首轮就能看到本地已确认的问题，从而：
//
//   - 加快根因定位：LLM 不必再为了确认 "磁盘是否满" 单独发命令
//   - 提供安全网：LLM 偶尔会漏看快照中明显的问题，规则引擎兜底
//   - 支持离线/降级：即使 LLM 不可用，本地规则仍能给出基本诊断
//
// 规则编写原则：
//
//  1. 规则只读快照文本，不执行命令（不产生副作用，可幂等重复执行）
//  2. 规则阈值要保守、明确（避免高频误报产生噪声 Issue）
//  3. 规则产出的 Issue 必须 Evidence 字段引用实际命令输出片段，
//     与 LLM Issue 拥有相同的证据标准，不可凭空断言
//  4. 规则只覆盖"有明确数字阈值"的问题；模糊诊断仍交给 LLM
package rules

import (
	"strconv"
	"strings"

	"github.com/LinDiag-Agent/internal/diagnosis"
)

// Rule 一条本地诊断规则。
// Match 在快照文本上运行，返回命中的 Issue（可为空）。
//
// snapshot 是完整快照文本；helpers.go 提供 extractCommandOutput 辅助
// 按命令切分输出，规则实现应使用它而非自己手写解析。
type Rule interface {
	// ID 规则唯一标识（调试与去重日志用）
	ID() string
	// Match 在快照文本上运行规则，返回命中产生的 Issue（可能为多条）
	Match(snapshot string) []diagnosis.Issue
}

// Engine 规则引擎：聚合多条规则，对一次快照批量执行。
type Engine struct {
	rules []Rule
}

// NewEngine 创建引擎并装载内置规则集。
// 规则集通过 newBuiltinRules() 按平台分流加载（见 rules_builtin_linux.go /
// rules_builtin_windows.go），便于审阅与增减。
func NewEngine() *Engine {
	return &Engine{
		rules: newBuiltinRules(),
	}
}

// Run 对快照执行所有规则，合并去重后返回 Issue 列表（按严重度排序）。
// 同一标题的 Issue 保留严重度更高的一条（与 LLM Issue 走同一合并语义）。
func (e *Engine) Run(snapshot string) []diagnosis.Issue {
	var out []diagnosis.Issue
	for _, r := range e.rules {
		for _, is := range r.Match(snapshot) {
			out = diagnosis.MergeByTitle(out, []diagnosis.Issue{is})
		}
	}
	return out
}

// === 辅助函数 ===

// extractCommandOutput 从快照中提取指定命令的输出段。
// 快照格式：以 "$ cmd\n"（Linux）或 "> cmd\n"（Windows）开头的段，
// 下一段以 "$ " 或 ">" 开头或 EOF 结束。
// cmd 可以是命令的前缀（如 "df -h" 匹配 "$ df -h"）。
// 找不到返回空串。
func extractCommandOutput(snapshot, cmdPrefix string) string {
	lines := strings.Split(snapshot, "\n")
	var b strings.Builder
	capturing := false
	// 尝试两种前缀
	prefixes := []string{"$ " + cmdPrefix, "> " + cmdPrefix}
	for _, ln := range lines {
		// 检查是否是命令提示符开头（Linux: $, Windows: >）
		if strings.HasPrefix(ln, "$ ") || strings.HasPrefix(ln, "> ") {
			if capturing {
				break // 下一条命令开始，结束当前段
			}
			for _, prefix := range prefixes {
				if strings.HasPrefix(ln, prefix) {
					capturing = true
					break // 开始捕获输出
				}
			}
		} else if capturing {
			b.WriteString(ln)
			b.WriteString("\n")
		}
	}
	return strings.TrimSpace(b.String())
}

// parsePercent 从形如 "75%" 的字符串中解析数值部分。
func parsePercent(s string) (int, bool) {
	s = strings.TrimSpace(s)
	s = strings.TrimSuffix(s, "%")
	if s == "" {
		return 0, false
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, false
	}
	return n, true
}

// firstNumber 返回字符串中第一段可解析的整数（用于 uptime load 等）。
func firstNumber(s string) (float64, bool) {
	loc := firstNumberRe.FindStringIndex(s)
	if loc == nil {
		return 0, false
	}
	n, err := strconv.ParseFloat(s[loc[0]:loc[1]], 64)
	if err != nil {
		return 0, false
	}
	return n, true
}
