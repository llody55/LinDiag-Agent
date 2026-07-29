package output

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/LinDiag-Agent/internal/diagnosis"
)

// Writer 是 output 包的输出目标，默认 os.Stdout。
// 替换此变量可重定向所有通过 output 包输出的内容（用于测试/嵌入式场景）。
// 注意：当前大部分输出仍通过 fmt.Print* 直接写 stdout，
// 完整重定向需要将 fmt.Print* 调用替换为 fmt.Fprint(Writer, ...)，
// 此变量为渐进迁移提供入口。
var Writer = os.Stdout

// 颜色常量 — 全项目唯一颜色定义。
// 当 colorDisabled=true（非终端或 NO_COLOR）时，
// colorize/BoldText 等函数会忽略这些常量，返回原文本。
const (
	Red       = "\033[31m"
	Green     = "\033[32m"
	Yellow    = "\033[33m"
	Blue      = "\033[34m"
	Cyan      = "\033[36m"
	Bold      = "\033[1m"
	Reset     = "\033[0m"
	Underline = "\033[4m"
)

// 风险等级对应的颜色
func RiskColor(level string) string {
	if colorDisabled {
		return ""
	}
	switch strings.ToLower(level) {
	case "safe":
		return Green
	case "low":
		return Blue
	case "medium":
		return Yellow
	case "high", "critical":
		return Red
	default:
		return Cyan
	}
}

// Colorize 给文本上色。禁色模式下直接返回原文本。
func Colorize(text, color string) string {
	if colorDisabled || color == "" {
		return text
	}
	return color + text + Reset
}

// BoldText 加粗文本。禁色模式下直接返回原文本。
func BoldText(text string) string {
	if colorDisabled {
		return text
	}
	return Bold + text + Reset
}

// SuccessMessage 成功提示
func SuccessMessage(msg string) {
	fmt.Println(Green + icon("✅", "[OK] ") + " " + msg + Reset)
}

// ErrorMessage 错误提示
func ErrorMessage(msg string) {
	fmt.Println(Red + icon("❌", "[ERR] ") + " " + msg + Reset)
}

// WarningMessage 警告提示
func WarningMessage(msg string) {
	fmt.Println(Yellow + icon("⚠️", "[!] ") + " " + msg + Reset)
}

// InfoMessage 信息提示
func InfoMessage(msg string) {
	fmt.Println(Cyan + icon("ℹ️", "[i] ") + " " + msg + Reset)
}

// SectionTitle 章节标题
func SectionTitle(title string) {
	fmt.Println("\n" + Bold + Cyan + boxTLChar() + strings.Repeat(boxH, BoxWidth) + Reset)
	fmt.Println(Bold + Cyan + boxVChar() + Reset + " " + BoldText(title))
	fmt.Println(Bold + Cyan + boxBLChar() + strings.Repeat(boxH, BoxWidth) + Reset)
}

// CommandBox 显示待执行命令
func CommandBox(cmd, purpose, riskLevel string) {
	color := RiskColor(riskLevel)
	fmt.Println("\n" + color + boxTLChar() + strings.Repeat(boxH, BoxWidth) + Reset)
	fmt.Println(color + boxVChar() + Reset + " " + icon("🔍", "[?] ") + "我需要执行这个命令来获取信息")
	fmt.Println(color + boxMLChar() + strings.Repeat(boxH, BoxWidth) + Reset)
	fmt.Println(color + boxVChar() + Reset + " 命令: " + Yellow + cmd + Reset)
	if purpose != "" {
		fmt.Println(color + boxVChar() + Reset + " 说明: " + purpose)
	}
	fmt.Println(color + boxBLChar() + strings.Repeat(boxH, BoxWidth) + Reset)
}

// ResultBox 显示执行结果
func ResultBox(success bool, output string) {
	color := Green
	prefix := icon("✅", "[OK]")
	if !success {
		color = Red
		prefix = icon("❌", "[ERR]")
	}
	fmt.Println(color + boxTLChar() + strings.Repeat(boxH, BoxWidth) + Reset)
	fmt.Println(color + boxVChar() + Reset + " " + prefix + " 执行结果:")
	fmt.Println(color + boxBLChar() + strings.Repeat(boxH, BoxWidth) + Reset)
	fmt.Println(output)
}

// ConfirmBox 显示需要确认的命令
func ConfirmBox(cmd, explanation, reason, riskLevel string) {
	color := RiskColor(riskLevel)
	fmt.Println("\n" + color + boxTLChar() + strings.Repeat(boxH, BoxWidth) + Reset)
	fmt.Println(color + boxVChar() + Reset + " " + icon("⚠️", "[!] ") + "我需要执行这个命令，但需要您的确认")
	fmt.Println(color + boxMLChar() + strings.Repeat(boxH, BoxWidth) + Reset)
	fmt.Println(color + boxVChar() + Reset + " 命令: " + cmd)
	if explanation != "" {
		fmt.Println(color + boxVChar() + Reset + " 说明: " + explanation)
	}
	if reason != "" {
		fmt.Println(color + boxVChar() + Reset + " 风险: " + reason)
	}
	fmt.Println(color + boxBLChar() + strings.Repeat(boxH, BoxWidth) + Reset)
}

// AIAnalysisBox 显示 AI 分析内容（Markdown 渲染）
func AIAnalysisBox(content string) {
	fmt.Println()
	fmt.Println(Cyan + dBoxTLChar() + strings.Repeat(dboxH, BoxWidth) + Reset)
	fmt.Println(Cyan + dBoxVChar() + Reset + " " + icon("💡", "[*] ") + "AI 分析")
	fmt.Println(Cyan + dBoxBLChar() + strings.Repeat(dboxH, BoxWidth) + Reset)
	fmt.Println()
	fmt.Print(RenderMarkdown(content))
	fmt.Println()
}

// FinalReportBox 显示最终诊断报告（Markdown 渲染）
// 末尾附带菜单 + "您的输入 > " 光标提示，让用户立刻知道可以在此输入。
// 否则 runLoop 进入 readUserAction 时 afterFinal=true 不再单独打印 InputPromptBox，
// 用户会面对一个没有光标提示的空白行，只能在盲处敲输入。
func FinalReportBox(content string) {
	reportLine := icon("🏆", "[#] ") + "诊断报告"
	// 按显示宽度居中标题
	pad := BoxWidth - displayWidth(reportLine)
	if pad < 0 {
		pad = 0
	}
	left := pad / 2
	right := pad - left
	fmt.Println("\n" + Green + Bold + dBoxTLChar() + strings.Repeat(dboxH, BoxWidth) + Reset)
	fmt.Println(Green + Bold + dBoxVChar() + strings.Repeat(" ", left) + reportLine + strings.Repeat(" ", right) + dBoxVChar() + Reset)
	fmt.Println(Green + Bold + dBoxBLChar() + strings.Repeat(dboxH, BoxWidth) + Reset)
	fmt.Print(RenderMarkdown(content))
	fmt.Println("\n" + Cyan + boxTLChar() + strings.Repeat(boxH, BoxWidth) + Reset)
	fmt.Println(Cyan + boxVChar() + Reset + " " + icon("💡", "[*] ") + "您可以继续输入问题进行交互，或选择以下操作：")
	fmt.Println(Cyan + boxVChar() + Reset + "   输入 " + Yellow + "1" + Reset + " 或 " + Yellow + "report" + Reset + " — 生成诊断报告文件（MD/HTML/PDF）")
	fmt.Println(Cyan + boxVChar() + Reset + "   输入 " + Yellow + "2" + Reset + " 或 " + Yellow + "exit" + Reset + " — 退出")
	fmt.Println(Cyan + boxVChar() + Reset + "   直接输入任何问题 — 继续交互分析")
	fmt.Println(Cyan + boxBLChar() + strings.Repeat(boxH, BoxWidth) + Reset)
	fmt.Print("您的输入 > ")
}

// EnvDetectBox 显示环境检测结果
func EnvDetectBox(envs []string) {
	fmt.Println("\n" + Cyan + boxTLChar() + strings.Repeat(boxH, BoxWidth) + Reset)
	fmt.Println(Cyan + boxVChar() + Reset + " " + icon("🔍", "[?] ") + "环境检测结果")
	fmt.Println(Cyan + boxBLChar() + strings.Repeat(boxH, BoxWidth) + Reset)
	if len(envs) == 0 {
		fmt.Println("   " + Yellow + icon("⚠️", "[!]") + " 未检测到特殊环境" + Reset)
		return
	}
	for _, env := range envs {
		fmt.Println("   " + Green + icon("✓", "v") + Reset + " " + env)
	}
}

// IssuesBox 显示结构化诊断问题清单（按严重度已排序）
func IssuesBox(issues []diagnosis.Issue) {
	fmt.Println("\n" + Yellow + boxTLChar() + strings.Repeat(boxH, BoxWidth) + Reset)
	fmt.Println(Yellow + boxVChar() + Reset + " " + icon("📋", "[D] ") + "诊断问题清单（按严重度排序）")
	fmt.Println(Yellow + boxBLChar() + strings.Repeat(boxH, BoxWidth) + Reset)
	if len(issues) == 0 {
		fmt.Println("   " + Green + icon("✓", "v") + " 暂未识别到明确问题" + Reset)
		return
	}
	for i, is := range issues {
		color := severityColor(is.Severity)
		fmt.Printf("   %s[%s]%s %s· %s\n", color, is.Severity.Label(), Reset, is.Category.Label(), is.Title)
		fmt.Printf("     证据: %s\n", truncateLine(is.Evidence, 80))
		fmt.Printf("     建议: %s\n", truncateLine(is.Suggestion, 80))
		if i < len(issues)-1 {
			fmt.Println("   " + Cyan + boxH + Reset)
		}
	}
}

// severityColor 严重度对应的颜色
func severityColor(s diagnosis.Severity) string {
	if colorDisabled {
		return ""
	}
	switch s {
	case diagnosis.SeverityCritical, diagnosis.SeverityHigh:
		return Red
	case diagnosis.SeverityMedium:
		return Yellow
	case diagnosis.SeverityLow:
		return Blue
	case diagnosis.SeverityInfo:
		return Cyan
	default:
		return Cyan
	}
}

// truncateLine 截断单行文本用于 TUI 展示。
// 按显示宽度（而非字节数）截断，避免在中文字符中间断裂产生乱码。
func truncateLine(s string, maxLen int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.TrimSpace(s)
	return truncateByWidth(s, maxLen, "...")
}

// InputPromptBox 显示用户输入提示
func InputPromptBox() {
	fmt.Println("\n" + Blue + boxTLChar() + strings.Repeat(boxH, BoxWidth) + Reset)
	fmt.Println(Blue + boxVChar() + Reset + " " + icon("📝", "[>] ") + "请输入您的问题或选择操作：")
	fmt.Println(Blue + boxMLChar() + strings.Repeat(boxH, BoxWidth) + Reset)
	fmt.Println(Blue + boxVChar() + Reset + "  按 Enter 继续交互")
	fmt.Println(Blue + boxVChar() + Reset + "  1 / report / r    - 生成报告")
	fmt.Println(Blue + boxVChar() + Reset + "  2 / exit / q       - 退出")
	fmt.Println(Blue + boxBLChar() + strings.Repeat(boxH, BoxWidth) + Reset)
	fmt.Print("您的输入 > ")
}

// Table 简单表格渲染
type Table struct {
	Headers []string
	Rows    [][]string
}

// AddRow 添加行
func (t *Table) AddRow(row []string) {
	t.Rows = append(t.Rows, row)
}

// Render 渲染表格。
// 列宽与对齐按显示宽度计算，避免中英文混排时边框错位。
func (t *Table) Render() string {
	if len(t.Headers) == 0 {
		return ""
	}
	colWidths := make([]int, len(t.Headers))
	for i, h := range t.Headers {
		colWidths[i] = displayWidth(h)
	}
	for _, row := range t.Rows {
		for i, cell := range row {
			if i < len(colWidths) {
				if w := displayWidth(cell); w > colWidths[i] {
					colWidths[i] = w
				}
			}
		}
	}
	var sb strings.Builder
	border := "+"
	for _, w := range colWidths {
		border += strings.Repeat("-", w+2) + "+"
	}
	sb.WriteString(border + "\n")
	sb.WriteString("|")
	for i, h := range t.Headers {
		sb.WriteString(" " + padRightWidth(h, colWidths[i]) + " |")
	}
	sb.WriteString("\n" + border + "\n")
	for _, row := range t.Rows {
		sb.WriteString("|")
		for i, cell := range row {
			if i < len(colWidths) {
				sb.WriteString(" " + padRightWidth(cell, colWidths[i]) + " |")
			}
		}
		sb.WriteString("\n")
	}
	sb.WriteString(border + "\n")
	return sb.String()
}

// padRightWidth 在 s 右侧补空格至目标显示宽度。
// 用于表格列对齐：按 displayWidth 计算填充量，而非字节数。
func padRightWidth(s string, targetWidth int) string {
	w := displayWidth(s)
	if w >= targetWidth {
		return s
	}
	return s + strings.Repeat(" ", targetWidth-w)
}

// PrintTable 打印表格
func PrintTable(t *Table) {
	fmt.Print(t.Render())
}

// ============================================================
// 交互封装（Phase 3 Task 10）
//
// 以下封装统一了散落在 agent/session.go、agent/handler.go、
// platform/executor.go、llm/client.go 中的 fmt.Print* 与 \033[... 直写。
// 集中到这里后，未来要切换为 go-pretty 或其他渲染库只需改一处。
// ============================================================

// BoxWidth 所有 Box 的固定宽度（与现有 ─/═ 线保持一致）。
const BoxWidth = 61

// HLine 返回用指定字符与颜色拼出的横线（不含换行）。
// 例：HLine(Yellow, "─") → "─────...─"（共 BoxWidth 个字符）。
// 注意：在 ASCII 降级模式下，建议调用方传入 boxH（会自动替换为 "-")。
// 若调用方显式传入其他 Unicode 字符，需自行判断是否需要降级。
func HLine(color, ch string) string {
	if asciiMode {
		// 若传入的是 Unicode 制表符，回退为 ASCII "-"
		if isBoxChar(ch) {
			ch = asciiBoxH
		}
	}
	if colorDisabled {
		return strings.Repeat(ch, BoxWidth)
	}
	return color + strings.Repeat(ch, BoxWidth) + Reset
}

// isBoxChar 判断字符是否为 Unicode 制表符（需在 ASCII 模式下替换）。
func isBoxChar(ch string) bool {
	switch ch {
	case boxH, boxV, boxTL, boxBL, boxML, boxTR, boxBR,
		dboxH, dboxV, dboxTL, dboxBL:
		return true
	}
	return false
}

// Prompt 在行末打印提示符（不换行），用于读取用户输入前。
// 例：Prompt("选择 (1-2，默认重试) > ")
func Prompt(prompt string) {
	fmt.Print(prompt)
}

// Promptln 打印换行结尾的提示（用于不带后续输入的说明性提示）。
func Promptln(prompt string) {
	fmt.Println(prompt)
}

// OptionMenu 打印一个带标题的选项菜单，返回用户选择的序号（1-based）。
// options 至少 2 项；返回值范围 [1, len(options)]。
//
// 用法：
//
//	idx := output.OptionMenu("请选择报告格式", []string{"MD", "HTML", "PDF"})
//	// idx ∈ {1,2,3}
//
// 注意：本函数只负责"打印"菜单，不读输入；读取仍由调用方控制
// （保留与现有 bufio.Reader 一致的输入流语义）。
func OptionMenu(title string, options []string) int {
	fmt.Println("\n" + Yellow + boxTLChar() + strings.Repeat(boxH, BoxWidth) + Reset)
	fmt.Println(Yellow + boxVChar() + Reset + " " + title)
	fmt.Println(Yellow + boxMLChar() + strings.Repeat(boxH, BoxWidth) + Reset)
	for i, opt := range options {
		fmt.Printf("%s%s%s %d. %s\n", Yellow, boxVChar(), Reset, i+1, opt)
	}
	fmt.Println(Yellow + boxBLChar() + strings.Repeat(boxH, BoxWidth) + Reset)
	return len(options)
}

// Inplacef 在当前行原地输出（先清行再写），用于进度条/实时刷新。
// 调用方需要在最后一次刷新后调用 ClearLine 抹掉痕迹。
//
// 例：
//
//	output.Inplacef("⏳ 已执行 %ds...", elapsed)
//	// ...完成后：
//	output.ClearLine()
func Inplacef(format string, args ...any) {
	fmt.Printf("\r\033[K"+format, args...)
}

// ClearLine 清除当前行（ANSI \r\033[K），用于进度条刷完后恢复。
func ClearLine() {
	fmt.Print("\r\033[K")
}

// StatusTimef 打印带时间戳的状态行（用于 LLM 连接/重试等过程提示）。
// 例：StatusTimef(Cyan, "正在连接 AI (尝试 %d/3)...", 2)
func StatusTimef(color, format string, args ...any) {
	prefix := fmt.Sprintf("\n%s[%s]%s ", color, nowStamp(), Reset)
	fmt.Printf(prefix+format+"\n", args...)
}

// Statusln 打印一条纯状态行（带时间戳），无颜色强调。
func Statusln(format string, args ...any) {
	prefix := fmt.Sprintf("\n[%s] ", nowStamp())
	fmt.Printf(prefix+format+"\n", args...)
}

// WarningInplacef 打印一条可重试错误（黄色，带时间戳），
// 用于 LLM 调用失败但还能降级/重试时给用户一个可见提示。
func WarningInplacef(format string, args ...any) {
	prefix := fmt.Sprintf("\n%s[可重试错误]%s ", Yellow, Reset)
	fmt.Printf(prefix+format+"\n", args...)
}

// Degradef 打印一条降级提示（黄色，无时间戳），
// 用于 format 切换（json_schema → json_object → none）等可见但不必高亮的场景。
func Degradef(format string, args ...any) {
	prefix := fmt.Sprintf("\n%s%s%s ", Yellow, icon("⚠️", "[!]"), Reset)
	fmt.Printf(prefix+format+"\n", args...)
}

// nowStamp 返回 HH:MM:SS 时间戳。
// 抽到此处避免各调用方各自 fmt.Now().Format(...)，语义集中。
func nowStamp() string {
	return time.Now().Format("15:04:05")
}
