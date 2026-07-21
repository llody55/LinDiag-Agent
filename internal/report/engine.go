package report

import (
	"encoding/json"
	"fmt"
	"html"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/LinDiag-Agent/internal/diagnosis"
	"github.com/LinDiag-Agent/internal/output"
	"github.com/LinDiag-Agent/internal/paths"
	"github.com/LinDiag-Agent/internal/platform"
)

// DiagnosticRecord 一条诊断记录：命令 + 执行结果
type DiagnosticRecord struct {
	Command string
	Purpose string
	Output  string
	Success bool
}

// ReportData 从聊天历史中提取的完整报告数据
type ReportData struct {
	Hostname     string
	IPAddress    string
	OSInfo       string
	KernelVer    string
	UptimeInfo   string
	GenerateTime string
	// UserProblem 用户描述的问题现象
	UserProblem string
	// SystemSnapshot 系统快照
	SystemSnapshot string
	// DiagnosticRecords 诊断过程中执行的命令记录
	DiagnosticRecords []DiagnosticRecord
	// IntermediateAnalysis 中间分析（非最终的 AI 分析）
	IntermediateAnalysis []string
	// Issues 结构化诊断发现，按严重度排序。来自每轮 AI 输出与规则引擎。
	Issues []diagnosis.Issue
	// FinalConclusion 最终诊断结论
	FinalConclusion string
}

// ExtractReportData 从聊天历史中提取完整的报告数据。
//
// 消息类型识别优先使用 diagnosis.MessageKind 字段（显式契约）；
// 对旧历史文件（Kind 为空）回退到按内容特征识别，保证向后兼容。
func ExtractReportData(history []diagnosis.Message) ReportData {
	data := ReportData{
		Hostname:     platform.GetHostname(),
		IPAddress:    platform.GetIPAddress(),
		GenerateTime: time.Now().Format("2006-01-02 15:04:05"),
	}

	for _, msg := range history {
		switch msg.Role {
		case "system":
			// 系统提示中可能包含模式信息，跳过
			continue
		case "user":
			content := msg.Content
			// 优先按 Kind 识别；Kind 为空时回退到内容特征识别（兼容旧历史）
			kind := msg.Kind
			if kind == "" {
				kind = sniffUserKind(content)
			}
			switch kind {
			case diagnosis.KindSystemSnapshot:
				data.SystemSnapshot = extractSnapshotContent(content)
				data.OSInfo = extractField(content, "PRETTY_NAME=")
				data.KernelVer = extractAfterCommand(content, "uname -a")
				data.UptimeInfo = extractAfterCommand(content, "uptime")
			case diagnosis.KindCommandResult:
				rec := parseCommandResult(content)
				if rec.Command != "" {
					data.DiagnosticRecords = append(data.DiagnosticRecords, rec)
				}
			case diagnosis.KindUserRequirement:
				data.UserProblem = extractUserProblem(content)
			default:
				// 其他用户输入（追问等）
				if strings.TrimSpace(content) != "" && !strings.Contains(content, "修改需求") {
					if data.UserProblem == "" {
						data.UserProblem = strings.TrimSpace(content)
					}
				}
			}
		case "assistant":
			analysis, isFinal := extractAnalysisFromResponse(msg.Content)
			if analysis == "" {
				continue
			}
			// 收集该轮 LLM 输出的结构化 issues
			if issues := extractIssuesFromResponse(msg.Content); len(issues) > 0 {
				data.Issues = diagnosis.MergeByTitle(data.Issues, issues)
			}
			if isFinal {
				data.FinalConclusion = analysis
			} else {
				data.IntermediateAnalysis = append(data.IntermediateAnalysis, analysis)
			}
		}
	}

	return data
}

// sniffUserKind 按 content 文本特征推断 user 消息的 Kind。
// 仅用于加载旧历史文件（无 Kind 字段）时的兜底识别。
func sniffUserKind(content string) diagnosis.MessageKind {
	switch {
	case strings.Contains(content, "初始系统快照"):
		return diagnosis.KindSystemSnapshot
	case strings.Contains(content, "用户需求"):
		return diagnosis.KindUserRequirement
	case strings.Contains(content, "执行结果") || strings.Contains(content, "命令执行失败"):
		return diagnosis.KindCommandResult
	default:
		return diagnosis.KindUserFollowup
	}
}

// extractSnapshotContent 从消息中提取快照内容
func extractSnapshotContent(content string) string {
	// 去掉 "初始系统快照" 前缀说明
	lines := strings.Split(content, "\n")
	var result []string
	skipPrefix := true
	for _, line := range lines {
		if skipPrefix && (strings.Contains(line, "初始系统快照") || strings.Contains(line, "用户需求")) {
			continue
		}
		skipPrefix = false
		result = append(result, line)
	}
	return strings.TrimSpace(strings.Join(result, "\n"))
}

// extractUserProblem 提取用户需求
func extractUserProblem(content string) string {
	// 格式可能是 "用户需求：xxx" 或 "用户需求: xxx"
	re := regexp.MustCompile(`用户需求[：:]\s*`)
	loc := re.FindStringIndex(content)
	if loc != nil {
		return strings.TrimSpace(content[loc[1]:])
	}
	return strings.TrimSpace(content)
}

// extractField 从文本中提取指定字段
func extractField(content, prefix string) string {
	idx := strings.Index(content, prefix)
	if idx == -1 {
		return "未知"
	}
	start := idx + len(prefix)
	if start >= len(content) {
		return ""
	}
	// 处理引号包裹的值：PRETTY_NAME="Ubuntu 22.04"
	if content[start] == '"' {
		start++ // 跳过开头的引号
		end := strings.Index(content[start:], "\"")
		if end == -1 {
			return content[start:]
		}
		return content[start : start+end]
	}
	// 无引号值，按换行符截断
	end := strings.Index(content[start:], "\n")
	if end == -1 {
		return content[start:]
	}
	return strings.TrimSpace(content[start : start+end])
}

// extractAfterCommand 提取命令后的第一行输出
func extractAfterCommand(content, cmd string) string {
	idx := strings.Index(content, cmd)
	if idx == -1 {
		return "未知"
	}
	after := content[idx+len(cmd):]
	lines := strings.Split(after, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && !strings.HasPrefix(trimmed, "$") {
			return trimmed
		}
	}
	return "未知"
}

// parseCommandResult 解析命令执行结果消息
func parseCommandResult(content string) DiagnosticRecord {
	rec := DiagnosticRecord{}
	lines := strings.Split(content, "\n")

	// 格式1: "执行结果 (command):\noutput"
	// 格式2: "命令执行失败 (command):\noutput"
	for i, line := range lines {
		if strings.HasPrefix(line, "执行结果 (") || strings.HasPrefix(line, "命令执行失败 (") {
			prefix := "执行结果 ("
			isFail := false
			if strings.HasPrefix(line, "命令执行失败 (") {
				prefix = "命令执行失败 ("
				isFail = true
			}
			cmdStr := strings.TrimSuffix(strings.TrimPrefix(line, prefix), "):")
			rec.Command = cmdStr
			rec.Success = !isFail
			// 剩余行是输出
			if i+1 < len(lines) {
				rec.Output = strings.Join(lines[i+1:], "\n")
			}
			break
		}
	}
	return rec
}

// extractAnalysisFromResponse 从 assistant 消息中提取分析
func extractAnalysisFromResponse(content string) (analysis string, isFinal bool) {
	var resp diagnosis.AgentResponse
	if err := json.Unmarshal([]byte(content), &resp); err == nil {
		if resp.Analysis != "" {
			return resp.Analysis, resp.IsFinal
		}
	}
	if strings.Contains(content, "FINAL:") {
		return strings.TrimSpace(strings.SplitN(content, "FINAL:", 2)[1]), true
	}
	if !strings.HasPrefix(strings.TrimSpace(content), "{") {
		return content, false
	}
	return "", false
}

// extractIssuesFromResponse 从 assistant JSON 消息中提取结构化 issues
func extractIssuesFromResponse(content string) []diagnosis.Issue {
	var resp diagnosis.AgentResponse
	if err := json.Unmarshal([]byte(content), &resp); err != nil {
		return nil
	}
	return resp.Issues
}

// GenerateMarkdownReport 生成结构化 Markdown 报告
func GenerateMarkdownReport(filename string, data ReportData) {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("# 运维诊断报告\n\n"))
	sb.WriteString(fmt.Sprintf("| 项目 | 信息 |\n|------|------|\n"))
	sb.WriteString(fmt.Sprintf("| 主机名 | %s |\n", data.Hostname))
	sb.WriteString(fmt.Sprintf("| IP 地址 | %s |\n", data.IPAddress))
	sb.WriteString(fmt.Sprintf("| 操作系统 | %s |\n", data.OSInfo))
	sb.WriteString(fmt.Sprintf("| 内核版本 | %s |\n", data.KernelVer))
	sb.WriteString(fmt.Sprintf("| 运行时长 | %s |\n", data.UptimeInfo))
	sb.WriteString(fmt.Sprintf("| 生成时间 | %s |\n\n", data.GenerateTime))

	// 1. 问题描述
	sb.WriteString("## 一、问题现象\n\n")
	if data.UserProblem != "" {
		sb.WriteString(data.UserProblem + "\n\n")
	} else {
		sb.WriteString("（未记录问题描述）\n\n")
	}

	// 2. 问题清单（结构化 Issue，按严重度排序）
	if len(data.Issues) > 0 {
		sb.WriteString("## 二、问题清单\n\n")
		sb.WriteString("| # | 严重度 | 分类 | 问题 | 证据 | 建议 |\n")
		sb.WriteString("|---|--------|------|------|------|------|\n")
		for i, is := range data.Issues {
			sb.WriteString(fmt.Sprintf("| %d | %s | %s | %s | %s | %s |\n",
				i+1, is.Severity.Label(), is.Category.Label(),
				escapeMarkdownTable(is.Title),
				escapeMarkdownTable(is.Evidence),
				escapeMarkdownTable(is.Suggestion)))
		}
		sb.WriteString("\n")
	}

	// 3. 诊断过程
	if len(data.DiagnosticRecords) > 0 {
		sb.WriteString("## 三、诊断过程\n\n")
		for i, rec := range data.DiagnosticRecords {
			status := "✅ 成功"
			if !rec.Success {
				status = "❌ 失败"
			}
			sb.WriteString(fmt.Sprintf("### 步骤 %d：%s [%s]\n\n", i+1, rec.Command, status))
			if rec.Output != "" {
				sb.WriteString("```\n")
				// 截断过长的输出
				output := rec.Output
				if len(output) > 2000 {
					output = output[:2000] + "\n... (输出已截断)"
				}
				sb.WriteString(output)
				sb.WriteString("\n```\n\n")
			}
		}
	}

	// 4. 中间分析
	if len(data.IntermediateAnalysis) > 0 {
		sb.WriteString("## 四、中间分析\n\n")
		for _, analysis := range data.IntermediateAnalysis {
			sb.WriteString(analysis + "\n\n")
		}
	}

	// 5. 最终结论
	sb.WriteString("## 五、诊断结论\n\n")
	if data.FinalConclusion != "" {
		sb.WriteString(data.FinalConclusion + "\n\n")
	} else if len(data.IntermediateAnalysis) > 0 {
		sb.WriteString(data.IntermediateAnalysis[len(data.IntermediateAnalysis)-1] + "\n\n")
	} else {
		sb.WriteString("（未生成最终结论）\n\n")
	}

	// 6. 系统快照
	if data.SystemSnapshot != "" {
		sb.WriteString("## 六、系统快照\n\n")
		sb.WriteString("```\n")
		snap := data.SystemSnapshot
		if len(snap) > 3000 {
			snap = snap[:3000] + "\n... (快照已截断)"
		}
		sb.WriteString(snap)
		sb.WriteString("\n```\n\n")
	}

	sb.WriteString("---\n")
	sb.WriteString(fmt.Sprintf("*本报告由 LinDiag-Agent 于 %s 自动生成*\n", data.GenerateTime))

	if err := os.WriteFile(filename, []byte(sb.String()), 0644); err != nil {
		output.ErrorMessage("无法生成报告文件: " + err.Error())
		return
	}
}

// GenerateHTMLReport 生成结构化 HTML 报告
func GenerateHTMLReport(filename string, data ReportData) {
	var sb strings.Builder

	sb.WriteString(`<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>运维诊断报告</title>
<style>
body { font-family: -apple-system, "Microsoft YaHei", sans-serif; max-width: 960px; margin: 40px auto; padding: 0 20px; color: #333; line-height: 1.8; }
h1 { color: #1a73e8; border-bottom: 3px solid #1a73e8; padding-bottom: 10px; }
h2 { color: #1557b0; margin-top: 30px; border-left: 4px solid #1a73e8; padding-left: 12px; }
h3 { color: #333; margin-top: 20px; }
table { border-collapse: collapse; width: 100%; margin: 15px 0; }
th, td { border: 1px solid #ddd; padding: 8px 12px; text-align: left; }
th { background: #f5f5f5; }
pre { background: #f6f8fa; border: 1px solid #e1e4e8; border-radius: 6px; padding: 16px; overflow-x: auto; font-size: 13px; }
code { background: #f6f8fa; padding: 2px 6px; border-radius: 3px; font-size: 13px; }
.success { color: #2e7d32; } .fail { color: #c62828; }
.footer { margin-top: 40px; padding-top: 15px; border-top: 1px solid #ddd; color: #999; font-size: 12px; }
</style>
</head>
<body>
`)

	sb.WriteString("<h1>运维诊断报告</h1>\n")
	sb.WriteString("<table>\n")
	sb.WriteString(fmt.Sprintf("<tr><th>主机名</th><td>%s</td><th>IP 地址</th><td>%s</td></tr>\n", data.Hostname, data.IPAddress))
	sb.WriteString(fmt.Sprintf("<tr><th>操作系统</th><td>%s</td><th>内核版本</th><td>%s</td></tr>\n", data.OSInfo, data.KernelVer))
	sb.WriteString(fmt.Sprintf("<tr><th>运行时长</th><td>%s</td><th>生成时间</th><td>%s</td></tr>\n", data.UptimeInfo, data.GenerateTime))
	sb.WriteString("</table>\n\n")

	// 1. 问题描述
	sb.WriteString("<h2>一、问题现象</h2>\n")
	if data.UserProblem != "" {
		sb.WriteString("<p>" + escapeHTML(data.UserProblem) + "</p>\n\n")
	} else {
		sb.WriteString("<p>（未记录问题描述）</p>\n\n")
	}

	// 2. 问题清单（结构化 Issue）
	if len(data.Issues) > 0 {
		sb.WriteString("<h2>二、问题清单</h2>\n")
		sb.WriteString("<table>\n<thead><tr><th>#</th><th>严重度</th><th>分类</th><th>问题</th><th>证据</th><th>建议</th></tr></thead><tbody>\n")
		for i, is := range data.Issues {
			sb.WriteString(fmt.Sprintf("<tr><td>%d</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td></tr>\n",
				i+1, escapeHTML(is.Severity.Label()), escapeHTML(is.Category.Label()),
				escapeHTML(is.Title), escapeHTML(is.Evidence), escapeHTML(is.Suggestion)))
		}
		sb.WriteString("</tbody></table>\n\n")
	}

	// 3. 诊断过程
	if len(data.DiagnosticRecords) > 0 {
		sb.WriteString("<h2>三、诊断过程</h2>\n")
		for i, rec := range data.DiagnosticRecords {
			cls := "success"
			status := "✅ 成功"
			if !rec.Success {
				cls = "fail"
				status = "❌ 失败"
			}
			sb.WriteString(fmt.Sprintf("<h3>步骤 %d：%s <span class=\"%s\">[%s]</span></h3>\n", i+1, escapeHTML(rec.Command), cls, status))
			if rec.Output != "" {
				output := rec.Output
				if len(output) > 2000 {
					output = output[:2000] + "\n... (输出已截断)"
				}
				sb.WriteString("<pre>" + escapeHTML(output) + "</pre>\n\n")
			}
		}
	}

	// 4. 中间分析
	if len(data.IntermediateAnalysis) > 0 {
		sb.WriteString("<h2>四、中间分析</h2>\n")
		for _, analysis := range data.IntermediateAnalysis {
			sb.WriteString("<div>" + convertMarkdownToHTML(analysis) + "</div>\n\n")
		}
	}

	// 5. 最终结论
	sb.WriteString("<h2>五、诊断结论</h2>\n")
	if data.FinalConclusion != "" {
		sb.WriteString("<div>" + convertMarkdownToHTML(data.FinalConclusion) + "</div>\n\n")
	} else if len(data.IntermediateAnalysis) > 0 {
		sb.WriteString("<div>" + convertMarkdownToHTML(data.IntermediateAnalysis[len(data.IntermediateAnalysis)-1]) + "</div>\n\n")
	} else {
		sb.WriteString("<p>（未生成最终结论）</p>\n\n")
	}

	// 6. 系统快照
	if data.SystemSnapshot != "" {
		sb.WriteString("<h2>六、系统快照</h2>\n")
		snap := data.SystemSnapshot
		if len(snap) > 3000 {
			snap = snap[:3000] + "\n... (快照已截断)"
		}
		sb.WriteString("<pre>" + escapeHTML(snap) + "</pre>\n\n")
	}

	sb.WriteString(fmt.Sprintf("<div class=\"footer\">本报告由 LinDiag-Agent 于 %s 自动生成</div>\n", data.GenerateTime))
	sb.WriteString("</body></html>\n")

	if err := os.WriteFile(filename, []byte(sb.String()), 0644); err != nil {
		output.ErrorMessage("无法生成报告文件: " + err.Error())
		return
	}
}

// escapeHTML 转义 HTML 特殊字符（复用标准库，避免手写实体拼写错误）
func escapeHTML(s string) string {
	return html.EscapeString(s)
}

// escapeMarkdownTable 转义 Markdown 表格中的分隔符与换行，避免破坏表格结构
func escapeMarkdownTable(s string) string {
	s = strings.ReplaceAll(s, "|", "\\|")
	s = strings.ReplaceAll(s, "\n", " ")
	return s
}

// convertMarkdownToHTML 简单 Markdown 转 HTML
func convertMarkdownToHTML(markdown string) string {
	// 转义 HTML
	html := escapeHTML(markdown)
	// 标题
	html = regexp.MustCompile(`### (.+)`).ReplaceAllString(html, "<h3>$1</h3>")
	html = regexp.MustCompile(`## (.+)`).ReplaceAllString(html, "<h2>$1</h2>")
	html = regexp.MustCompile(`# (.+)`).ReplaceAllString(html, "<h1>$1</h1>")
	// 粗体
	html = regexp.MustCompile(`\*\*(.+?)\*\*`).ReplaceAllString(html, "<strong>$1</strong>")
	// 行内代码
	html = regexp.MustCompile("`([^`]+)`").ReplaceAllString(html, "<code>$1</code>")
	// 列表
	html = regexp.MustCompile(`(?m)^- (.+)$`).ReplaceAllString(html, "<li>$1</li>")
	html = regexp.MustCompile(`(<li>.+?</li>)`).ReplaceAllString(html, "<ul>$1</ul>")
	// 换行
	html = strings.ReplaceAll(html, "\n", "<br>")
	// 清理多余的 br 在块级元素前后
	html = strings.ReplaceAll(html, "<br></h", "</h")
	html = strings.ReplaceAll(html, "><br><", "><")
	return html
}

// GeneratePDFReport 见 pdf.go（实现移至此处以便集中管理转换器探测逻辑）。

// GenerateReport 从历史记录生成报告
func GenerateReport(historyFile string, format string) {
	output.InfoMessage(fmt.Sprintf("正在从历史记录生成 %s 格式报告: %s", format, historyFile))

	data, err := os.ReadFile(historyFile)
	if err != nil {
		output.ErrorMessage("无法读取历史记录文件: " + err.Error())
		return
	}

	var chatHistory []diagnosis.Message
	if err := json.Unmarshal(data, &chatHistory); err != nil {
		output.ErrorMessage("历史记录文件格式错误: " + err.Error())
		return
	}

	reportData := ExtractReportData(chatHistory)
	if err := paths.EnsureDataDir(); err != nil {
		output.ErrorMessage("无法创建数据目录: " + err.Error())
		return
	}
	reportFilename := paths.ReportFile(time.Now().Format("20060102_150405"), format)

	// 预览
	printPreview(reportData)

	switch format {
	case "md":
		GenerateMarkdownReport(reportFilename, reportData)
	case "html":
		GenerateHTMLReport(reportFilename, reportData)
	case "pdf":
		GeneratePDFReport(reportFilename, reportData)
	}

	output.SuccessMessage("报告已生成: " + reportFilename)
}

// printPreview 打印报告预览
func printPreview(data ReportData) {
	output.Promptln("\n📋 报告预览：")
	output.Promptln(strings.Repeat("─", 70))
	output.Statusln("主机: %s (%s) | 时间: %s", data.Hostname, data.IPAddress, data.GenerateTime)
	output.Statusln("问题: %s", truncateForPreview(data.UserProblem, 80))
	output.Statusln("诊断步骤: %d | 中间分析: %d | 结构化问题: %d",
		len(data.DiagnosticRecords), len(data.IntermediateAnalysis), len(data.Issues))
	if data.FinalConclusion != "" {
		output.Statusln("最终结论: %s", truncateForPreview(data.FinalConclusion, 100))
	} else {
		output.Promptln("最终结论: （无）")
	}
	output.Promptln(strings.Repeat("─", 70))
}

// truncateForPreview 截断预览文本
func truncateForPreview(s string, maxLen int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.TrimSpace(s)
	if len(s) > maxLen {
		return s[:maxLen] + "..."
	}
	if s == "" {
		return "（无）"
	}
	return s
}
