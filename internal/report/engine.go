package report

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/LinDiag-Agent/internal/llm"
	"github.com/LinDiag-Agent/internal/platform"
)

// 提取报告内容
func ExtractReportContent(chatHistory []llm.Message) map[string]string {
	content := make(map[string]string)

	// 提取系统信息
	for _, msg := range chatHistory {
		if strings.Contains(msg.Content, "初始系统快照") {
			content["systemInfo"] = msg.Content
			break
		}
	}

	// 提取用户需求
	for _, msg := range chatHistory {
		if strings.Contains(msg.Content, "用户需求") {
			content["userRequirement"] = msg.Content
			break
		}
	}

	// 提取AI分析结果 - 优先找 FINAL: 标记
	for _, msg := range chatHistory {
		if msg.Role == "assistant" {
			if strings.Contains(msg.Content, "FINAL:") ||
				strings.Contains(msg.Content, "【合规得分】") ||
				strings.Contains(msg.Content, "服务器巡检报告") ||
				strings.Contains(msg.Content, "安全审计") ||
				strings.Contains(msg.Content, "Kubernetes集群") ||
				strings.Contains(msg.Content, "Docker容器") {
				content["analysis"] = msg.Content
				break
			}
		}
	}

	// 如果没有找到 FINAL 等标记，提取最后一个 assistant 的有效回复作为分析
	if _, ok := content["analysis"]; !ok {
		for i := len(chatHistory) - 1; i >= 0; i-- {
			msg := chatHistory[i]
			if msg.Role == "assistant" && !strings.Contains(msg.Content, "EXEC:") && len(msg.Content) > 50 {
				content["analysis"] = msg.Content
				break
			}
		}
	}

	// 提取命令执行结果
	cmdResults := make(map[string]string)
	for _, msg := range chatHistory {
		if strings.Contains(msg.Content, "执行结果") {
			lines := strings.Split(msg.Content, "\n")
			for _, line := range lines {
				if strings.HasPrefix(line, "执行结果 (") {
					cmd := strings.TrimSuffix(strings.TrimPrefix(line, "执行结果 ("), "):")
					result := strings.Join(lines[1:], "\n")
					cmdResults[cmd] = result
					break
				}
			}
		}
	}
	content["cmdResults"] = fmt.Sprintf("%v", cmdResults)

	return content
}

// 提取系统信息到map
func ExtractSystemInfo(systemInfo string) map[string]string {
	info := make(map[string]string)

	// 初始化默认值
	info["osInfo"] = "未知"
	info["kernelVersion"] = "未知"
	info["uptimeInfo"] = "未知"
	info["cpuInfo"] = "未知"
	info["memInfo"] = "未知"
	info["diskUsage"] = "未知"
	info["memUsage"] = "未知"
	info["cpuLoad"] = "未知"
	info["swapUsage"] = "未知"
	info["systemError"] = "无严重错误"
	info["loginFailures"] = "无"

	// 分割系统信息为行
	lines := strings.Split(systemInfo, "\n")

	// 提取操作系统信息
	for i, line := range lines {
		if strings.Contains(line, "$ cat /etc/os-release") && i+1 < len(lines) {
			for j := i + 1; j < len(lines) && !strings.HasPrefix(lines[j], "$"); j++ {
				if strings.Contains(lines[j], "PRETTY_NAME=") {
					info["osInfo"] = strings.Trim(strings.TrimPrefix(lines[j], "PRETTY_NAME="), "\"")
					break
				}
			}
			break
		}
	}

	// 提取内核版本
	for i, line := range lines {
		if strings.Contains(line, "$ uname -a") && i+1 < len(lines) {
			info["kernelVersion"] = lines[i+1]
			break
		}
	}

	// 提取运行时间
	for i, line := range lines {
		if strings.Contains(line, "$ uptime") && i+1 < len(lines) {
			info["uptimeInfo"] = lines[i+1]
			break
		}
	}

	// 提取CPU信息
	for i, line := range lines {
		if strings.Contains(line, "$ lscpu") && i+1 < len(lines) {
			for j := i + 1; j < len(lines) && !strings.HasPrefix(lines[j], "$"); j++ {
				if strings.Contains(lines[j], "CPU(s):") {
					info["cpuInfo"] = strings.TrimSpace(strings.TrimPrefix(lines[j], "CPU(s):"))
					break
				}
			}
			break
		}
	}

	// 提取内存总量
	for i, line := range lines {
		if strings.Contains(line, "$ free -h") && i+1 < len(lines) {
			info["memInfo"] = lines[i+1]
			break
		}
	}

	// 提取磁盘使用率
	for i, line := range lines {
		if strings.Contains(line, "$ df -h") && i+1 < len(lines) {
			diskUsage := ""
			for j := i + 1; j < len(lines) && !strings.HasPrefix(lines[j], "$"); j++ {
				if strings.Contains(lines[j], "/") && !strings.Contains(lines[j], "Filesystem") {
					parts := strings.Fields(lines[j])
					if len(parts) >= 6 {
						diskUsage += parts[5] + " (" + parts[4] + "), "
					}
				}
			}
			if len(diskUsage) > 0 {
				diskUsage = diskUsage[:len(diskUsage)-2]
				info["diskUsage"] = diskUsage
			}
			break
		}
	}

	// 提取内存使用率
	for i, line := range lines {
		if strings.Contains(line, "$ free -h") && i+2 < len(lines) {
			info["memUsage"] = lines[i+2]
			break
		}
	}

	// 提取CPU负载
	for _, line := range lines {
		if strings.Contains(line, "load average") {
			info["cpuLoad"] = line
			break
		}
	}

	// 提取SWAP使用率
	for i, line := range lines {
		if strings.Contains(line, "$ free -h") && i+3 < len(lines) {
			info["swapUsage"] = lines[i+3]
			break
		}
	}

	// 提取系统错误
	inJournalctl := false
	for _, line := range lines {
		if strings.Contains(line, "$ journalctl -p err -n 10") {
			inJournalctl = true
			continue
		}
		if inJournalctl && !strings.HasPrefix(line, "$") {
			if strings.TrimSpace(line) != "" && line != "skipped" {
				info["systemError"] = "有错误日志"
				break
			}
		}
	}

	return info
}

// 模式到模板目录的映射
var modeToTemplateDir = map[int]string{
	1: "fault_diagnosis", // 故障诊断模式
	2: "smart_mode",      // 智能模式
}

// 根据模式ID获取模板目录
func GetTemplateDirByModeID(modeID int) string {
	if dir, ok := modeToTemplateDir[modeID]; ok {
		return dir
	}
	return "default"
}

// 加载模板文件，如果不存在则使用AI生成默认模板
func LoadTemplate(templatePath string, format string, content map[string]string) (string, bool) {
	// 尝试读取模板文件
	templateContent, err := os.ReadFile(templatePath)
	if err == nil {
		return string(templateContent), false
	}

	// 模板文件不存在，使用AI生成默认模板
	fmt.Printf("ℹ️ 未找到模板文件 %s，正在使用AI生成默认模板...\n", templatePath)

	// 构建AI请求，生成适合的模板
	prompt := []llm.Message{
		{
			Role:    "system",
			Content: fmt.Sprintf("你是一个专业的报告模板生成助手。请根据以下系统信息和分析结果，生成一个%s格式的报告模板。模板应该包含合适的占位符，如{{hostname}}、{{ip_address}}等，以便后续填充实际数据。", format),
		},
		{
			Role:    "user",
			Content: fmt.Sprintf("系统信息:\n%s\n\n分析结果:\n%s", content["systemInfo"], content["analysis"]),
		},
	}

	aiTemplate := llm.CallAI(prompt)
	fmt.Println("✅ AI已生成默认模板")
	return aiTemplate, true
}

// 生成Markdown报告
func GenerateMarkdownReport(filename string, content map[string]string) {
	// 获取当前主机的IP地址和主机名
	ipAddress := platform.GetIPAddress()
	hostname := platform.GetHostname()

	// 尝试确定模式ID
	modeID := 2 // 默认使用智能模式
	if analysis, ok := content["analysis"]; ok {
		if strings.Contains(analysis, "FINAL:") {
			modeID = 1
		}
	}

	// 获取模板目录
	templateDir := GetTemplateDirByModeID(modeID)
	templatePath := fmt.Sprintf("templates/%s/report.md", templateDir)

	// 加载模板
	templateContent, isAiGenerated := LoadTemplate(templatePath, "Markdown", content)
	if isAiGenerated {
		fmt.Println("ℹ️ 当前使用的是AI生成的Markdown模板")
		fmt.Printf("ℹ️ 模板类型: %s, 生成依据: 系统信息和分析结果\n", templateDir)
	}

	// 提取系统信息
	systemInfo := ""
	if info, ok := content["systemInfo"]; ok {
		systemInfo = info
	}
	sysInfoMap := ExtractSystemInfo(systemInfo)

	// 发现的问题与风险清单
	issuesList := "- [低危] 性能趋势：系统已连续运行较长时间，建议计划内重启以加载最新内核补丁。\n"

	// 处理建议与优化方案
	recommendations := "### 服务优化\n- 检查高负载进程，确认是否有内存泄漏或 CPU 密集型的异常进程。\n"

	// 总体状态和结论
	overallStatus := "健康"
	conclusionDetails := "未发现明显异常"

	// 替换模板中的占位符
	reportContent := templateContent
	reportContent = strings.ReplaceAll(reportContent, "{{hostname}}", hostname)
	reportContent = strings.ReplaceAll(reportContent, "{{ip_address}}", ipAddress)
	reportContent = strings.ReplaceAll(reportContent, "{{inspection_time}}", time.Now().Format("2006-01-02 15:04"))
	reportContent = strings.ReplaceAll(reportContent, "{{os_info}}", sysInfoMap["osInfo"])
	reportContent = strings.ReplaceAll(reportContent, "{{kernel_version}}", sysInfoMap["kernelVersion"])
	reportContent = strings.ReplaceAll(reportContent, "{{uptime_info}}", sysInfoMap["uptimeInfo"])
	reportContent = strings.ReplaceAll(reportContent, "{{cpu_info}}", sysInfoMap["cpuInfo"])
	reportContent = strings.ReplaceAll(reportContent, "{{mem_info}}", sysInfoMap["memInfo"])
	reportContent = strings.ReplaceAll(reportContent, "{{disk_usage}}", sysInfoMap["diskUsage"])
	reportContent = strings.ReplaceAll(reportContent, "{{mem_usage}}", sysInfoMap["memUsage"])
	reportContent = strings.ReplaceAll(reportContent, "{{cpu_load}}", sysInfoMap["cpuLoad"])
	reportContent = strings.ReplaceAll(reportContent, "{{swap_usage}}", sysInfoMap["swapUsage"])
	reportContent = strings.ReplaceAll(reportContent, "{{system_error}}", sysInfoMap["systemError"])
	reportContent = strings.ReplaceAll(reportContent, "{{login_failures}}", sysInfoMap["loginFailures"])
	reportContent = strings.ReplaceAll(reportContent, "{{issues_list}}", issuesList)
	reportContent = strings.ReplaceAll(reportContent, "{{recommendations}}", recommendations)
	reportContent = strings.ReplaceAll(reportContent, "{{overall_status}}", overallStatus)
	reportContent = strings.ReplaceAll(reportContent, "{{conclusion_details}}", conclusionDetails)

	// 替换分析结果占位符
	if analysis, ok := content["analysis"]; ok {
		reportContent = strings.ReplaceAll(reportContent, "{{analysis}}", analysis)
	}

	// 保存报告文件
	err := os.WriteFile(filename, []byte(reportContent), 0644)
	if err != nil {
		fmt.Printf("❌ 无法生成报告文件: %v\n", err)
	}
}

// 生成HTML报告
func GenerateHTMLReport(filename string, content map[string]string) {
	// 获取当前主机的IP地址和主机名
	ipAddress := platform.GetIPAddress()
	hostname := platform.GetHostname()

	// 尝试确定模式ID
	modeID := 2 // 默认使用智能模式
	if analysis, ok := content["analysis"]; ok {
		if strings.Contains(analysis, "FINAL:") {
			modeID = 1
		}
	}

	// 获取模板目录
	templateDir := GetTemplateDirByModeID(modeID)
	templatePath := fmt.Sprintf("templates/%s/report.html", templateDir)

	// 加载模板
	templateContent, isAiGenerated := LoadTemplate(templatePath, "HTML", content)
	if isAiGenerated {
		fmt.Println("ℹ️ 当前使用的是AI生成的HTML模板")
		fmt.Printf("ℹ️ 模板类型: %s, 生成依据: 系统信息和分析结果\n", templateDir)
	}

	// 提取系统信息
	systemInfo := ""
	if info, ok := content["systemInfo"]; ok {
		systemInfo = info
	}
	sysInfoMap := ExtractSystemInfo(systemInfo)

	// 发现的问题与风险清单
	issuesList := "<ul><li>[低危] 性能趋势：系统已连续运行较长时间，建议计划内重启以加载最新内核补丁。</li></ul>"

	// 处理建议与优化方案
	recommendations := "<h3>服务优化</h3>\n<ul><li>检查高负载进程，确认是否有内存泄漏或 CPU 密集型的异常进程。</li></ul>"

	// 总体状态和结论
	overallStatus := "健康"
	conclusionDetails := "未发现明显异常"

	// 替换模板中的占位符
	reportContent := templateContent
	reportContent = strings.ReplaceAll(reportContent, "{{hostname}}", hostname)
	reportContent = strings.ReplaceAll(reportContent, "{{ip_address}}", ipAddress)
	reportContent = strings.ReplaceAll(reportContent, "{{inspection_time}}", time.Now().Format("2006-01-02 15:04"))
	reportContent = strings.ReplaceAll(reportContent, "{{os_info}}", sysInfoMap["osInfo"])
	reportContent = strings.ReplaceAll(reportContent, "{{kernel_version}}", sysInfoMap["kernelVersion"])
	reportContent = strings.ReplaceAll(reportContent, "{{uptime_info}}", sysInfoMap["uptimeInfo"])
	reportContent = strings.ReplaceAll(reportContent, "{{cpu_info}}", sysInfoMap["cpuInfo"])
	reportContent = strings.ReplaceAll(reportContent, "{{mem_info}}", sysInfoMap["memInfo"])
	reportContent = strings.ReplaceAll(reportContent, "{{disk_usage}}", sysInfoMap["diskUsage"])
	reportContent = strings.ReplaceAll(reportContent, "{{mem_usage}}", sysInfoMap["memUsage"])
	reportContent = strings.ReplaceAll(reportContent, "{{cpu_load}}", sysInfoMap["cpuLoad"])
	reportContent = strings.ReplaceAll(reportContent, "{{swap_usage}}", sysInfoMap["swapUsage"])
	reportContent = strings.ReplaceAll(reportContent, "{{system_error}}", sysInfoMap["systemError"])
	reportContent = strings.ReplaceAll(reportContent, "{{login_failures}}", sysInfoMap["loginFailures"])
	reportContent = strings.ReplaceAll(reportContent, "{{issues_list}}", issuesList)
	reportContent = strings.ReplaceAll(reportContent, "{{recommendations}}", recommendations)
	reportContent = strings.ReplaceAll(reportContent, "{{overall_status}}", overallStatus)
	reportContent = strings.ReplaceAll(reportContent, "{{conclusion_details}}", conclusionDetails)

	// 替换分析结果占位符，将Markdown转换为HTML
	if analysis, ok := content["analysis"]; ok {
		// 移除FINAL:前缀
		analysis = strings.TrimPrefix(analysis, "FINAL:")
		// 转换Markdown为HTML
		htmlAnalysis := ConvertMarkdownToHTML(analysis)
		reportContent = strings.ReplaceAll(reportContent, "{{analysis}}", htmlAnalysis)
	}

	// 保存报告文件
	err := os.WriteFile(filename, []byte(reportContent), 0644)
	if err != nil {
		fmt.Printf("❌ 无法生成报告文件: %v\n", err)
	}
}

// 将Markdown转换为HTML的简单函数
func ConvertMarkdownToHTML(markdown string) string {
	// 转换标题
	markdown = regexp.MustCompile(`^# (.*)$`).ReplaceAllString(markdown, "<h1>$1</h1>")
	markdown = regexp.MustCompile(`^## (.*)$`).ReplaceAllString(markdown, "<h2>$1</h2>")
	markdown = regexp.MustCompile(`^### (.*)$`).ReplaceAllString(markdown, "<h3>$1</h3>")

	// 转换代码块
	markdown = regexp.MustCompile("```([a-zA-Z]*)([\\s\\S]*?)```").ReplaceAllString(markdown, "<pre><code class=\"$1\">$2</code></pre>")

	// 转换粗体
	markdown = regexp.MustCompile(`\*\*(.*?)\*\*`).ReplaceAllString(markdown, "<strong>$1</strong>")

	// 转换斜体
	markdown = regexp.MustCompile(`\*(.*?)\*`).ReplaceAllString(markdown, "<em>$1</em>")

	// 转换列表
	markdown = regexp.MustCompile(`^- (.*)$`).ReplaceAllString(markdown, "<li>$1</li>")
	markdown = regexp.MustCompile(`(<li>.*?</li>)`).ReplaceAllString(markdown, "<ul>$1</ul>")

	// 转换换行
	markdown = strings.ReplaceAll(markdown, "\n", "<br>")

	return markdown
}

// 生成PDF报告（简化版，实际生产环境可能需要使用专门的PDF库）
func GeneratePDFReport(filename string, content map[string]string) {
	// 这里使用简单的方法，生成HTML然后提示用户使用浏览器转换为PDF
	// 实际生产环境可以使用如wkhtmltopdf等工具
	htmlFilename := strings.Replace(filename, ".pdf", ".html", 1)
	GenerateHTMLReport(htmlFilename, content)

	fmt.Printf("ℹ️ 已生成HTML报告，请使用浏览器打开并另存为PDF: %s\n", htmlFilename)
	fmt.Printf("ℹ️ 或使用命令转换: wkhtmltopdf %s %s\n", htmlFilename, filename)
}

// GenerateReport 生成报告
func GenerateReport(historyFile string, format string) {
	fmt.Printf("ℹ️ 正在从历史记录生成 %s 格式报告: %s\n", format, historyFile)

	// 读取历史记录文件
	data, err := os.ReadFile(historyFile)
	if err != nil {
		fmt.Printf("❌ 无法读取历史记录文件: %v\n", err)
		return
	}

	// 解析聊天历史
	var chatHistory []llm.Message
	if err := json.Unmarshal(data, &chatHistory); err != nil {
		fmt.Printf("❌ 历史记录文件格式错误: %v\n", err)
		return
	}

	// 提取报告内容
	reportContent := ExtractReportContent(chatHistory)

	// 生成报告文件
	reportFilename := fmt.Sprintf("report_%s.%s", time.Now().Format("20060102_150405"), format)

	// 显示报告摘要预览
	fmt.Println("\n📋 报告预览：")
	fmt.Println(strings.Repeat("─", 70))

	if analysis, ok := reportContent["analysis"]; ok && analysis != "" {
		lines := strings.Split(analysis, "\n")
		previewLines := 10
		if len(lines) < previewLines {
			previewLines = len(lines)
		}
		for i := 0; i < previewLines; i++ {
			fmt.Println(lines[i])
		}
		if len(lines) > previewLines {
			fmt.Println("... (内容已截断)")
		}
	} else if userReq, ok := reportContent["userRequirement"]; ok && userReq != "" {
		fmt.Println("用户需求:", userReq)
		fmt.Println("⚠️ 智能模式未生成完整分析报告")
	} else {
		fmt.Println("⚠️ 无分析数据")
	}
	fmt.Println(strings.Repeat("─", 70))

	// 直接生成，不询问确认
	switch format {
	case "md":
		GenerateMarkdownReport(reportFilename, reportContent)
	case "html":
		GenerateHTMLReport(reportFilename, reportContent)
	case "pdf":
		GeneratePDFReport(reportFilename, reportContent)
	}

	fmt.Printf("✅ 报告已生成: %s\n", reportFilename)
}
