package output

import (
	"regexp"
	"strings"
)

// RenderMarkdown 将 Markdown 文本渲染为带颜色的终端输出。
// 支持：标题（###/##/#）、粗体、行内代码、列表项、代码块。
// 这是一个轻量级渲染器，不依赖外部库，适合终端场景。
func RenderMarkdown(content string) string {
	lines := strings.Split(content, "\n")
	var result strings.Builder

	inCodeBlock := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// 代码块开始/结束
		if strings.HasPrefix(trimmed, "```") {
			sep := boxH
			if asciiMode {
				sep = asciiBoxH
			}
			if inCodeBlock {
				inCodeBlock = false
				result.WriteString(Yellow + strings.Repeat(sep, 60) + Reset + "\n")
			} else {
				inCodeBlock = true
				result.WriteString(Yellow + strings.Repeat(sep, 60) + Reset + "\n")
			}
			continue
		}

		if inCodeBlock {
			// 代码块内容：黄色等宽风格
			result.WriteString(Yellow + "  " + line + Reset + "\n")
			continue
		}

		// 空行
		if trimmed == "" {
			result.WriteString("\n")
			continue
		}

		// 标题渲染
		if strings.HasPrefix(trimmed, "### ") {
			title := strings.TrimPrefix(trimmed, "### ")
			mark := icon("▶", ">")
			result.WriteString("\n" + Bold + Cyan + mark + " " + title + Reset + "\n")
			continue
		}
		if strings.HasPrefix(trimmed, "## ") {
			title := strings.TrimPrefix(trimmed, "## ")
			mark := icon("■", "#")
			result.WriteString("\n" + Bold + Blue + mark + " " + title + Reset + "\n")
			continue
		}
		if strings.HasPrefix(trimmed, "# ") {
			title := strings.TrimPrefix(trimmed, "# ")
			mark := icon("●", "*")
			result.WriteString("\n" + Bold + Green + mark + " " + title + Reset + "\n")
			continue
		}

		// 列表项（- 或 * 或 数字.）
		if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") {
			item := strings.TrimPrefix(strings.TrimPrefix(trimmed, "- "), "* ")
			rendered := renderInline(item)
			bullet := icon("•", "-")
			result.WriteString("  " + Green + bullet + Reset + " " + rendered + "\n")
			continue
		}
		if matched, _ := regexp.MatchString(`^\d+\.\s`, trimmed); matched {
			re := regexp.MustCompile(`^(\d+)\.\s(.*)`)
			m := re.FindStringSubmatch(trimmed)
			if len(m) == 3 {
				rendered := renderInline(m[2])
				result.WriteString("  " + Cyan + m[1] + "." + Reset + " " + rendered + "\n")
				continue
			}
		}

		// 普通段落
		rendered := renderInline(line)
		result.WriteString(rendered + "\n")
	}

	return result.String()
}

// renderInline 渲染行内 markdown：粗体、行内代码
func renderInline(text string) string {
	// 粗体 **text** → Bold
	reBold := regexp.MustCompile(`\*\*(.+?)\*\*`)
	text = reBold.ReplaceAllStringFunc(text, func(m string) string {
		inner := strings.TrimSuffix(strings.TrimPrefix(m, "**"), "**")
		return Bold + inner + Reset
	})

	// 行内代码 `text` → Yellow
	reCode := regexp.MustCompile("`([^`]+)`")
	text = reCode.ReplaceAllStringFunc(text, func(m string) string {
		inner := strings.TrimSuffix(strings.TrimPrefix(m, "`"), "`")
		return Yellow + inner + Reset
	})

	return text
}
