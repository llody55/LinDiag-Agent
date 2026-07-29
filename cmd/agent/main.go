package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/LinDiag-Agent/internal/agent"
	"github.com/LinDiag-Agent/internal/config"
	"github.com/LinDiag-Agent/internal/llm"
	"github.com/LinDiag-Agent/internal/output"
	"github.com/LinDiag-Agent/internal/paths"
	"github.com/LinDiag-Agent/internal/platform"
	"github.com/LinDiag-Agent/internal/report"
	"github.com/LinDiag-Agent/internal/safety"
)

// 以下变量由构建流水线通过 -ldflags -X 注入；本地 go build 时保持默认值。
var (
	version   = "dev"
	commit    = "unknown"
	buildDate = "unknown"
)

func main() {
	// 终端环境检测：在任何 output.* 调用之前执行，
	// 判断是否需要 ASCII 降级 / 禁用颜色。
	// 触发条件：非终端（重定向）；locale 非 UTF-8；Linux 原生控制台（TERM=linux）。
	termEnv := output.DetectTerminalEnv()
	output.SetTerminalEnv(termEnv)

	// 在非 UTF-8 或 Linux 原生控制台环境下给出一次性提示，
	// 帮助用户理解为何图标/边框变成 ASCII 形态。
	// 提示本身使用纯 ASCII，确保在任何环境下可读。
	if termEnv.NeedASCII && termEnv.IsTerminal {
		fmt.Println("[LinDiag] Detected non-UTF-8 or Linux console environment.")
		fmt.Println("[LinDiag] Switched to ASCII fallback mode for compatibility.")
		if !termEnv.IsUTF8 && termEnv.Lang != "" {
			fmt.Printf("[LinDiag] Your locale is %q; consider using a UTF-8 locale (e.g. C.UTF-8).\n", termEnv.Lang)
		} else if termEnv.Lang == "" {
			fmt.Println("[LinDiag] No locale (LANG/LC_ALL) is set; consider setting LANG=C.UTF-8.")
		}
		if termEnv.IsLinuxConsole {
			fmt.Println("[LinDiag] Linux native console (TERM=linux) has no CJK font; ASCII mode enabled.")
		}
		fmt.Println()
	}

	output.InfoMessage(fmt.Sprintf("LinDiag-Agent %s (commit: %s, built: %s)", version, commit, buildDate))

	// 初始化 LLM 模块
	if err := llm.Init(); err != nil {
		output.ErrorMessage("初始化 LLM 模块失败: " + err.Error())
		return
	}

	// 设置命令执行超时
	timeout := llm.GetConfig().Command.TimeoutSeconds
	if timeout <= 0 {
		timeout = 30
	}
	platform.SetDefaultTimeout(timeout)
	output.InfoMessage(fmt.Sprintf("命令执行超时时间: %d秒", timeout))

	// 加载用户偏好
	config.LoadUserPreferences()

	// 加载白名单（路径由 paths 包统一管理：$XDG_CONFIG_HOME/lindiag/whitelist.txt）
	if err := safety.LoadWhitelist(paths.WhitelistFile()); err != nil {
		output.WarningMessage("加载白名单文件失败: " + err.Error() + "，使用默认白名单")
	} else {
		output.SuccessMessage("已加载白名单")
	}

	// 检查 LLM 配置
	cfg := llm.GetConfig()
	if cfg.LLM.APIURL == "" || cfg.LLM.APIKey == "" || cfg.LLM.ModelName == "" {
		output.ErrorMessage("未配置 LLM 参数，请先创建配置文件")
		output.PrintConfigHelp()
		return
	}

	// 信号处理：Ctrl+C 优雅退出
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// 处理命令行参数
	if len(os.Args) > 1 {
		if os.Args[1] == "load" && len(os.Args) > 2 {
			// 加载历史记录模式
			runWithHistory(ctx, os.Args[2])
			return
		}
		if os.Args[1] == "report" && len(os.Args) > 3 {
			// 报告生成模式
			format := os.Args[3]
			if format != "md" && format != "html" && format != "pdf" {
				output.ErrorMessage("不支持的报告格式，支持的格式: md, html, pdf")
				return
			}
			report.GenerateReport(os.Args[2], format)
			return
		}
		// 显示帮助
		output.PrintHelp()
		return
	}

	// 正常启动交互模式
	runInteractive(ctx)
}

func runInteractive(ctx context.Context) {
	reader := bufio.NewReader(os.Stdin)
	output.SectionTitle("LinDiag-Agent v4.2 智能运维诊断工具")

	// 加载本地规则（路径由 paths 包统一管理：$XDG_CONFIG_HOME/lindiag/rules.txt）
	rules := ""
	if b, err := os.ReadFile(paths.RulesFile()); err == nil {
		rules = string(b)
		output.InfoMessage("已加载本地 rules.txt")
	}

	// 环境检测
	envs := agent.DetectEnvironment()
	output.EnvDetectBox(envs)

	// 选择模式
	modes := agent.ListModes()
	options := make([]string, len(modes))
	for i, m := range modes {
		options[i] = fmt.Sprintf("%s — %s", m.Name(), m.Description())
	}
	output.OptionMenu("请选择工作模式：", options)
	output.Prompt(fmt.Sprintf("\n请输入数字 (1-%d): ", len(modes)))

	choiceStr, _ := reader.ReadString('\n')
	choiceStr = strings.TrimSpace(choiceStr)
	choice := 1
	fmt.Sscanf(choiceStr, "%d", &choice)
	if choice < 1 || choice > len(modes) {
		choice = 1
	}

	mode := agent.GetModeByMenuIndex(choice)
	if mode == nil {
		output.ErrorMessage("未找到可用的诊断模式")
		return
	}
	output.Statusln("🚀 已进入【%s】", mode.Name())

	// 创建并运行会话
	session := agent.NewSession(mode, reader, rules, ctx)
	session.InitHistory()
	session.Run()
}

func runWithHistory(ctx context.Context, historyFile string) {
	reader := bufio.NewReader(os.Stdin)
	output.SectionTitle("LinDiag-Agent v4.2 智能运维诊断工具")

	output.InfoMessage("尝试加载历史记录: " + historyFile)

	// 默认使用智能模式加载历史
	mode := agent.GetMode("smart")
	if mode == nil {
		output.ErrorMessage("未找到可用的诊断模式")
		return
	}
	session := agent.NewSession(mode, reader, "", ctx)

	if err := session.LoadHistory(historyFile); err != nil {
		output.ErrorMessage("加载历史记录失败: " + err.Error())
		output.InfoMessage("将使用默认流程启动")
		session.InitHistory()
	} else {
		output.SuccessMessage("成功加载历史记录")
		session.ContinueWithInput()
	}
	session.Run()
}
