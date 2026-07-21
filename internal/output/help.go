package output

import "fmt"

// PrintHelp 打印主程序使用帮助。
// 收纳至此处统一 UI，cmd/agent/main.go 只负责调用。
func PrintHelp() {
	fmt.Println("=== LinDiag-Agent v4.0 智能运维诊断工具 ===")
	fmt.Println("使用方法:")
	fmt.Println("  ./lindiag-agent               # 正常启动")
	fmt.Println("  ./lindiag-agent load <file>   # 加载历史记录文件继续对话")
	fmt.Println("  ./lindiag-agent report <file> <format>   # 从历史记录生成报告 (格式: md, html, pdf)")
}

// PrintConfigHelp 打印配置文件帮助（路径 + 示例 + 环境变量）。
func PrintConfigHelp() {
	fmt.Println()
	fmt.Println(Yellow + "配置文件路径: $XDG_CONFIG_HOME/lindiag/config.json（缺省 ~/.config/lindiag/config.json）" + Reset)
	fmt.Println()
	fmt.Println("配置示例:")
	fmt.Println("```json")
	fmt.Println("{")
	fmt.Println("  \"llm\": {")
	fmt.Println("    \"api_url\": \"https://api.example.com/v1/chat/completions\",")
	fmt.Println("    \"api_key\": \"your-api-key-here\",")
	fmt.Println("    \"model_name\": \"your-model-name\",")
	fmt.Println("    \"response_format\": \"json_schema\"")
	fmt.Println("  },")
	fmt.Println("  \"command\": {")
	fmt.Println("    \"timeout_seconds\": 60")
	fmt.Println("  }")
	fmt.Println("}")
	fmt.Println("```")
	fmt.Println()
	fmt.Println("环境变量方式（优先级更高）:")
	fmt.Println("  LINDIAG_LLM_API_URL")
	fmt.Println("  LINDIAG_LLM_API_KEY")
	fmt.Println("  LINDIAG_LLM_MODEL_NAME")
	fmt.Println("  LINDIAG_LLM_RESPONSE_FORMAT  (json_schema | json_object | none)")
	fmt.Println()
	fmt.Println("response_format 说明:")
	fmt.Println("  json_schema  - Structured Outputs，最强约束（需 API 支持，默认）")
	fmt.Println("  json_object  - JSON Mode，保证合法 JSON")
	fmt.Println("  none         - 无格式约束，仅提示词引导")
}
