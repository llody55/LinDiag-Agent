// Package diagnosis 定义结构化诊断模型 Issue 与相关工具。
//
// Issue 是贯穿 session→report 的核心数据载体：LLM 输出、规则引擎匹配、
// 报告渲染均以此结构为契约，避免各处用自由文本导致信息丢失。
package diagnosis

import "sort"

// Severity 问题严重度分级。
type Severity string

const (
	SeverityCritical Severity = "critical" // 系统不可用 / 数据丢失风险
	SeverityHigh     Severity = "high"     // 核心功能受损，需立即处理
	SeverityMedium   Severity = "medium"   // 性能或容量告警，建议尽快处理
	SeverityLow      Severity = "low"      // 轻微/潜在隐患，可排期
	SeverityInfo     Severity = "info"     // 提示性信息，无需处理
)

// severityOrder 用于排序与去重比较，数字越小越紧急
var severityOrder = map[Severity]int{
	SeverityCritical: 0,
	SeverityHigh:     1,
	SeverityMedium:   2,
	SeverityLow:      3,
	SeverityInfo:     4,
}

// SeverityRank 返回严重度的排序权重，数字越小越紧急。
// 未知严重度返回最大值，排在最后。
func SeverityRank(v Severity) int {
	r, ok := severityOrder[v]
	if !ok {
		return 1<<31 - 1
	}
	return r
}

// Category 问题分类，便于报告分组与后续规则引擎匹配。
type Category string

const (
	CategoryCPU        Category = "cpu"
	CategoryMemory     Category = "memory"
	CategoryDisk       Category = "disk"
	CategoryNetwork    Category = "network"
	CategoryProcess    Category = "process"
	CategoryKernel     Category = "kernel"
	CategoryService    Category = "service"
	CategoryContainer  Category = "container"
	CategoryKubernetes Category = "kubernetes"
	CategorySecurity   Category = "security"
	CategoryOther      Category = "other"
)

// Issue 一条结构化诊断发现。
// 字段命名与 jsonschema 标签配合，LLM 通过 Structured Outputs 直接产出。
type Issue struct {
	// Severity 严重度
	Severity Severity `json:"severity" jsonschema:"enum=critical,enum=high,enum=medium,enum=low,enum=info,description=问题严重度"`
	// Category 分类
	Category Category `json:"category" jsonschema:"enum=cpu,enum=memory,enum=disk,enum=network,enum=process,enum=kernel,enum=service,enum=container,enum=kubernetes,enum=security,enum=other,description=问题分类"`
	// Title 一句话标题
	Title string `json:"title" jsonschema:"required,description=问题的一句话标题"`
	// Evidence 证据：来自哪个命令的输出中的具体数值或行
	Evidence string `json:"evidence" jsonschema:"required,description=证据，引用实际命令输出"`
	// Suggestion 修复建议
	Suggestion string `json:"suggestion" jsonschema:"required,description=修复建议与步骤"`
}

// SortBySeverity 原地按严重度升序排序（critical 在前）
func SortBySeverity(issues []Issue) {
	sort.SliceStable(issues, func(i, j int) bool {
		return SeverityRank(issues[i].Severity) < SeverityRank(issues[j].Severity)
	})
}

// MergeByTitle 按 Title 去重合并两组 issues，保留严重度更高的一条，
// 结果按严重度排序。提供单一实现，避免 session / report 各自重复。
//
// 用法：
//
//	merged := diagnosis.MergeByTitle(existing, incoming)
func MergeByTitle(existing, incoming []Issue) []Issue {
	out := append([]Issue(nil), existing...)
	for _, ni := range incoming {
		merged := false
		for i, ex := range out {
			if ex.Title == ni.Title {
				if SeverityRank(ni.Severity) < SeverityRank(ex.Severity) {
					out[i] = ni
				}
				merged = true
				break
			}
		}
		if !merged {
			out = append(out, ni)
		}
	}
	SortBySeverity(out)
	return out
}

// SeverityLabel 中文严重度标签，用于 TUI 与报告展示
func (s Severity) Label() string {
	switch s {
	case SeverityCritical:
		return "严重"
	case SeverityHigh:
		return "高危"
	case SeverityMedium:
		return "中危"
	case SeverityLow:
		return "低危"
	case SeverityInfo:
		return "提示"
	default:
		return "未知"
	}
}

// CategoryLabel 中文分类标签
func (c Category) Label() string {
	switch c {
	case CategoryCPU:
		return "CPU"
	case CategoryMemory:
		return "内存"
	case CategoryDisk:
		return "磁盘"
	case CategoryNetwork:
		return "网络"
	case CategoryProcess:
		return "进程"
	case CategoryKernel:
		return "内核"
	case CategoryService:
		return "服务"
	case CategoryContainer:
		return "容器"
	case CategoryKubernetes:
		return "K8s"
	case CategorySecurity:
		return "安全"
	default:
		return "其他"
	}
}
