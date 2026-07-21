package agent

import (
	"fmt"
	"strings"
)

// context.go 负责把 EnvInfo 渲染为一段 Markdown 上下文，
// 拼接到模式 systemPrompt 之前，让 AI 首轮就掌握环境全貌：
//
//   - 哪些核心服务已安装但运行异常（应优先诊断）
//   - 哪些平台（Docker / K8s / Helm）可用（可用的话 AI 可选择相关命令）
//
// 拼接位置在模式规则前：env 是"事实"，模式规则是"如何分析事实"，
// 事实在前让 LLM 更容易把规则套用 到具体环境。
//
// 保持 systemPrompt（模式定义）的纯净：模式不知道 env 细节，
// env 上下文不依赖模式规则；两者正交解耦。

// buildEnvContext 把 EnvInfo 渲染为一段 Markdown 上下文。
// env 为 nil 时返回空串（调用方应保证非 nil，但这里稳健处理）。
func buildEnvContext(env *EnvInfo) string {
	if env == nil {
		return ""
	}

	var sb strings.Builder
	hasHeading := false

	// 1. 异常服务：AI 优先诊断方向（最重要，放最前）
	if len(env.AbnormalServices) > 0 {
		sb.WriteString("## 环境上下文\n\n")
		hasHeading = true
		sb.WriteString(fmt.Sprintf("**检测到以下服务异常，请优先诊断其失败原因：**\n"))
		for _, svc := range env.AbnormalServices {
			sb.WriteString(fmt.Sprintf("- %s（已安装但状态异常，快照中已含 `systemctl status %s` 输出）\n", svc, svc))
		}
		sb.WriteString("\n在分析中应首先评估这些异常服务是否与用户问题相关；若相关，给出针对该服务的诊断命令与修复步骤。\n\n")
	}

	// 2. 可用平台：告诉 AI 哪些工具可以用，避免它生成不存在的命令
	var platforms []string
	if env.DockerRunning {
		platforms = append(platforms, "Docker（可执行 docker 系列命令）")
	}
	if env.K8sConnected {
		platforms = append(platforms, "Kubernetes（可执行 kubectl 系列命令）")
	}
	if env.HasHelm {
		platforms = append(platforms, "Helm CLI")
	}
	if env.HasNali {
		platforms = append(platforms, "nali（IP 归属地查询）")
	}
	if len(platforms) > 0 {
		if !hasHeading {
			sb.WriteString("## 环境上下文\n\n")
			hasHeading = true
		}
		sb.WriteString("**可用平台与工具：**\n")
		for _, p := range platforms {
			sb.WriteString(fmt.Sprintf("- %s\n", p))
		}
		sb.WriteString("\n生成命令时优先使用上述已确认可用的工具；对未列出的工具，先确认其存在再使用。\n\n")
	}

	return sb.String()
}

// composeSystemPrompt 把 env 上下文拼接到模式 systemPrompt 之前。
// 调用方：Session.InitHistory。
func composeSystemPrompt(modePrompt string, env *EnvInfo) string {
	ctxText := buildEnvContext(env)
	if ctxText == "" {
		return modePrompt
	}
	return ctxText + modePrompt
}
