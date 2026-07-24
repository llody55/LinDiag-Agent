package rules

import (
	"regexp"
	"strings"

	"github.com/LinDiag-Agent/internal/diagnosis"
)

// rules_builtin.go 内置阈值规则集的平台无关工具函数。
// 规则实现按平台拆分到 rules_builtin_linux.go / rules_builtin_windows.go，
// 各自提供 newBuiltinRules() 返回对应平台的规则集。

// === 工具函数 ===

func parseFloatSafe(s string) float64 {
	n, _ := firstNumber(s)
	return n
}

// parseHumanSize 解析形如 "7.7Gi"/"234Mi"/"1.5G" 的容量值为 MB 数值。
// 规则引擎只需要做相对比较，精度到 MB 足够。
func parseHumanSize(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	re := regexp.MustCompile(`^(\d+(?:\.\d+)?)\s*([A-Za-z]+)$`)
	m := re.FindStringSubmatch(s)
	if m == nil {
		return 0
	}
	n, _ := firstNumber(m[1])
	unit := strings.ToLower(m[2])
	// 规范化到 MB
	// 支持常见单位：
	//   - free -h 输出的二进制前缀：gi/mi/ki
	//   - lsblk/df 风格：g/m/k、gb/mb/kb、gib/mib/kib
	switch unit {
	case "b":
		return n / 1024 / 1024
	case "k", "kb", "kib", "ki":
		return n / 1024
	case "m", "mb", "mib", "mi":
		return n
	case "g", "gb", "gib", "gi":
		return n * 1024
	case "t", "tb", "tib", "ti":
		return n * 1024 * 1024
	}
	return 0
}

func pctStr(ratio float64) string {
	return itoa(int(ratio*100+0.5)) + "%"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

func ftoa(f float64) string {
	// 简单 ftoa：整数部分 + 小数部分（最多两位）
	neg := f < 0
	if neg {
		f = -f
	}
	intPart := int64(f)
	frac := f - float64(intPart)
	s := itoa(int(intPart))
	if frac > 0 {
		// 取两位小数
		frac100 := int(frac*100 + 0.5)
		if frac100 > 0 {
			s += "." + itoa(frac100/10) + itoa(frac100%10)
		}
	}
	if neg {
		s = "-" + s
	}
	return s
}

// severityRank 本地用于比较严重度，数字越小越紧急。
// 直接复用 diagnosis.SeverityRank，避免两份 rank 表与严重度定义漂移。
//
// 这里包装一层仅是为了在规则实现里用更短的名字做比较。
func severityRank(v diagnosis.Severity) int {
	return diagnosis.SeverityRank(v)
}
