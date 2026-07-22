package output

import (
	"unicode/utf8"
)

// displayWidth 返回字符串在终端中的显示宽度（列数）。
//
// 规则：
//   - ASCII 控制字符（0x00-0x1F）宽度为 0（不可见）
//   - ASCII 可见字符宽度为 1
//   - CJK 及全角符号（U+1100..U+115F, U+2E80..U+A4CF, U+AC00..U+D7A3,
//     U+F900..U+FAFF, U+FE30..U+FE4F, U+FF00..U+FF60, U+FFE0..U+FFE6）宽度为 2
//   - Emoji 宽度近似为 2（U+1F000 以上;实际宽度因终端而异,2 是多数终端的近似）
//   - 组合字符（U+0300..U+036F）宽度为 0
//   - 其余字符宽度为 1
//
// 注意：这是轻量级实现,不处理 ZWJ 序列和完整东亚宽度表,
// 但已覆盖本项目用到的中文、日文、全角符号和 Emoji 的绝大多数场景。
func displayWidth(s string) int {
	w := 0
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		w += runeWidth(r)
		i += size
	}
	return w
}

// runeWidth 返回单个 rune 的显示宽度。
func runeWidth(r rune) int {
	switch {
	// 控制字符与组合字符：不可见
	case r < 0x20, r == 0x7F:
		return 0
	case r >= 0x300 && r <= 0x36F: // 组合附加符号
		return 0
	case isWide(r):
		return 2
	default:
		return 1
	}
}

// isWide 判断 rune 是否属于宽字符（显示宽度为 2）。
func isWide(r rune) bool {
	switch {
	// Hangul Jamo
	case r >= 0x1100 && r <= 0x115F:
		return true
	// CJK 部首与笔画、康熙部首、CJK 符号和标点、平假名、片假名、注音、CJK 统一表意文字
	case r >= 0x2E80 && r <= 0xA4CF:
		return true
	// Hangul 音节
	case r >= 0xAC00 && r <= 0xD7A3:
		return true
	// CJK 兼容表意文字
	case r >= 0xF900 && r <= 0xFAFF:
		return true
	// CJK 兼容形式
	case r >= 0xFE30 && r <= 0xFE4F:
		return true
	// 全角 ASCII、全角标点
	case r >= 0xFF00 && r <= 0xFF60:
		return true
	case r >= 0xFFE0 && r <= 0xFFE6:
		return true
	// Miscellaneous Symbols / Dingbats（⚠️ ✅ ❌ 等符号,终端通常显示为 1-2 列,
	// 这里按 2 处理与多数现代终端一致;若终端显示偏窄则盒子略宽,影响很小）
	case r >= 0x2600 && r <= 0x27BF:
		return true
	// Emoji 及补充符号（近似处理，统一算宽字符）
	case r >= 0x1F000:
		return true
	}
	return false
}

// truncateByWidth 按显示宽度截断字符串，不会在 UTF-8 多字节字符中间断裂。
// 返回的字符串显示宽度不超过 maxW,超出时追加 tail（如 "..."）。
// tail 的宽度不计入 maxW 限制内。
func truncateByWidth(s string, maxW int, tail string) string {
	if displayWidth(s) <= maxW {
		return s
	}
	w := 0
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		rw := runeWidth(r)
		if w+rw > maxW {
			break
		}
		out = append(out, s[i:i+size]...)
		w += rw
		i += size
	}
	return string(out) + tail
}
