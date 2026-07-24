package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/LinDiag-Agent/internal/diagnosis"
	"github.com/LinDiag-Agent/internal/llm"
	"github.com/LinDiag-Agent/internal/output"
	"github.com/LinDiag-Agent/internal/paths"
	"github.com/LinDiag-Agent/internal/report"
	"github.com/LinDiag-Agent/internal/rules"
)

// Session 管理一次完整的诊断/交互会话。
//
// 会话由单一显式状态机循环驱动（非递归）：保证退出语义明确、调用栈不随
// 追问深度增长。历史在关键节点（每批命令执行后、最终结论后、每次用户输入后）
// 增量保存到稳定文件名，避免长排查中途崩溃丢失全过程。
//
// 依赖注入（Phase 3 Task 13）：
//   - llmClient、cmdExecutor、reportGen 由外部注入，便于测试与扩展。
//   - ruleText 仍保留字符串形式（纯数据，不需注入层）。
//   - ruleEngine 保持具体类型 *rules.Engine：本地规则引擎没有副作用的可替换
//     实现，注入接口反而过度抽象；若未来规则来源多样化再抽。
type Session struct {
	mode    DiagnosticMode
	reader  *bufio.Reader
	handler *CommandHandler // 保留旧字段以兼容 handler 的内部 builder 用法

	// 三大可注入依赖（Phase 3 Task 13）
	llmClient   LLMClient
	cmdExecutor CommandExecutor
	reportGen   ReportGenerator

	// history 聊天历史。元素类型为 diagnosis.Message（带语义 Kind 标签），
	// 替代过去使用 llm.Message 仅靠字符串前缀识别消息类型的方式。
	history     []diagnosis.Message
	ruleText    string // 外置规则文件文本（未解析，仅拼到 system prompt）
	ruleEngine  *rules.Engine
	ctx         context.Context
	envInfo     *EnvInfo
	historyFile string
	issues      []diagnosis.Issue
	afterFinal  bool
	// justGeneratedReport 标记刚执行完 handleReportRequest，
	// 让 readUserAction 在下一轮读取前重新打印提示框。
	// 避免"报告生成完→无提示→用户敲回车→空输入被当作'继续分析'→误触发 CallAI"。
	justGeneratedReport bool
	// lastSaveErr 记录增量保存过程中遇到的最后一个错误。
	// Run() 结尾会连同最终保存结果一起报告，避免中间失败被静默忽略。
	lastSaveErr error
}

// SessionOption NewSession 的可选依赖注入参数。
//
// 采用 Functional Options 模式：调用方可以零个或多个 Option 注入依赖，
// 未注入的部分回退到包内默认实现（llmClient 默认走 llm 包级函数，
// cmdExecutor 默认使用 NewCommandHandler，reportGen 默认走 report 包）。
//
// 这样既支持 main.go 注入生产实现，也支持测试注入 mock，又能让
// 旧的 NewSession 调用不破坏编译。
type SessionOption func(*Session)

// WithLLMClient 注入 LLMClient 实现。测试时传入 FakeLLMClient。
func WithLLMClient(c LLMClient) SessionOption {
	return func(s *Session) { s.llmClient = c }
}

// WithCommandExecutor 注入 CommandExecutor 实现。测试时传入 FakeCommandExecutor。
func WithCommandExecutor(e CommandExecutor) SessionOption {
	return func(s *Session) { s.cmdExecutor = e }
}

// WithReportGenerator 注入 ReportGenerator 实现。测试时传入 FakeReportGenerator。
func WithReportGenerator(g ReportGenerator) SessionOption {
	return func(s *Session) { s.reportGen = g }
}

// defaultLLMClient 默认 LLM 客户端实现：直接调用 llm 包级函数。
// 不需要单独的类型，用一个轻 adapter 即可。
type llmPackageAdapter struct{}

func (llmPackageAdapter) CallAI(ctx context.Context, messages []diagnosis.Message) (diagnosis.AgentResponse, error) {
	return llm.CallAI(ctx, messages)
}

func (llmPackageAdapter) CallAISimple(ctx context.Context, systemPrompt, userContent string) string {
	return llm.CallAISimple(ctx, systemPrompt, userContent)
}

// defaultReportGenerator 默认报告生成实现：调用 report 包级 GenerateReport
// 并把其内部输出的文件路径通过 stdout 解析回返。
//
// 注意：当前 report.GenerateReport 是无返回值的包级函数（直接打印结果），
// 这里 adapter 暂时返回空字符串与 nil；后续 Task 8 会把 report 包
// 改造为返回 (path, error)，那时此 adapter 直接转发即可。
type reportPackageAdapter struct{}

func (reportPackageAdapter) Generate(historyFile, format string) (string, error) {
	report.GenerateReport(historyFile, format)
	return "", nil
}

// NewSession 创建会话
//
// 兼容说明：
//   - 旧签名 NewSession(mode, reader, ruleText, ctx) 保持不变；
//   - 三大依赖默认回退到 llm 包 / NewCommandHandler / report 包，
//     与旧实现行为完全对齐；
//   - 调用方可用 WithLLMClient / WithCommandExecutor / WithReportGenerator
//     注入实现，主要用于测试与未来扩展。
//
// 路径说明（Phase 3 Task 9）：历史文件落在 $XDG_DATA_HOME/lindiag/ 下，
// 这里在构造 Session 时一次性 EnsureDataDir，后续每轮增量保存只 WriteFile
// 即可，避免在热循环里反复 MkdirAll 打断节奏。调用方传入的 historyFile
// （LoadHistory 模式）保持原样使用，不做路径拼接。
func NewSession(mode DiagnosticMode, reader *bufio.Reader, ruleText string, ctx context.Context, opts ...SessionOption) *Session {
	cfg := llm.GetConfig()
	handler := NewCommandHandler(reader, cfg)
	s := &Session{
		mode:        mode,
		handler:     handler,
		reader:      reader,
		llmClient:   llmPackageAdapter{},    // 默认走包级函数
		cmdExecutor: handler,                // 默认 handler 实现 CommandExecutor
		reportGen:   reportPackageAdapter{}, // 默认走 report 包
		ruleText:    ruleText,
		ruleEngine:  rules.NewEngine(),
		ctx:         ctx,
		envInfo:     DetectEnvInfo(),
		historyFile: newHistoryFilename(),
	}
	for _, opt := range opts {
		opt(s)
	}
	// 数据目录一次性就绪；失败不阻塞 Session 构造，
	// 真正写盘时 saveHistory 会再返回具体错误。
	_ = paths.EnsureDataDir()
	return s
}

// newHistoryFilename 生成带时间戳的历史文件完整路径。
// 落点由 paths 包统一管理（$XDG_DATA_HOME/lindiag/history_*.json），
// 不再写到 CWD，避免不同启动目录导致历史散落、难以检索。
func newHistoryFilename() string {
	return paths.HistoryFile(time.Now().Format("20060102_150405"))
}

// LoadHistory 加载已有历史记录，并复用该文件作为后续增量保存的目标。
// 兼容旧版历史文件（无 Kind 字段）：json.Unmarshal 会把缺失字段置零，
// 读取端 report/engine.go 会回退到字符串特征识别。
func (s *Session) LoadHistory(historyFile string) error {
	data, err := os.ReadFile(historyFile)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, &s.history); err != nil {
		return err
	}
	// 给旧历史消息补 Kind（已加载的旧文件无 kind 字段，这里按内容嗅探补齐，
	// 后续 trim/extract 即可走 Kind 路径）
	s.backfillKinds()
	fixed := llm.FixConsecutiveAssistantMessages(s.history)
	if len(fixed) != len(s.history) {
		s.history = fixed
		output.InfoMessage("已修复历史记录中的连续assistant消息")
	}
	s.historyFile = historyFile
	return nil
}

// backfillKinds 为旧版历史消息（无 Kind 字段）按内容特征补齐 Kind。
// 这是兼容旧历史文件的过渡逻辑；新写入的消息都会在创建时带上 Kind。
func (s *Session) backfillKinds() {
	for i, m := range s.history {
		if m.Kind != "" {
			continue
		}
		switch m.Role {
		case "system":
			s.history[i].Kind = diagnosis.KindSystemPreamble
		case "user":
			switch {
			case strings.Contains(m.Content, "初始系统快照"):
				s.history[i].Kind = diagnosis.KindSystemSnapshot
			case strings.Contains(m.Content, "用户需求"):
				s.history[i].Kind = diagnosis.KindUserRequirement
			case strings.Contains(m.Content, "执行结果") || strings.Contains(m.Content, "命令执行失败"):
				s.history[i].Kind = diagnosis.KindCommandResult
			default:
				s.history[i].Kind = diagnosis.KindUserFollowup
			}
		case "assistant":
			s.history[i].Kind = diagnosis.KindAgentResponse
		}
	}
}

// InitHistory 初始化默认聊天历史（系统提示 + 快照 + 用户需求）。
// envInfo 通过 composeSystemPrompt 注入到 systemPrompt 前面，让 AI
// 首轮就能看到异常服务与可用平台，从而定向诊断。
//
// 同时在此处触发本地规则引擎：对刚采集的快照文本跑一遍阈值规则，
// 把命中的本地 Issue 注入 s.issues，让 LLM 首轮就能在已有 Issue
// (通过 IssuesBox 展示后) 基础上做验证或深入分析，加速根因定位。
func (s *Session) InitHistory() {
	systemPrompt := composeSystemPrompt(s.mode.SystemPrompt(), s.envInfo)
	s.history = LoadDefaultChatHistory(s.reader, systemPrompt, s.mode.SnapshotCmds(s.envInfo), s.ruleText)
	s.runLocalRulesOnSnapshot()
}

// runLocalRulesOnSnapshot 在最新快照消息上跑本地规则引擎，
// 把命中的 Issue 并入 s.issues 并通过 IssuesBox 展示给用户。
//
// 本地 Issue 与 LLM Issue 走同一 MergeByTitle 合并语义，
// 不会互相覆盖；标题相同者保留严重度更高的一条。
func (s *Session) runLocalRulesOnSnapshot() {
	if s.ruleEngine == nil {
		return
	}
	// 找快照消息（最末一条 KindSystemSnapshot）
	snapshotText := ""
	for i := len(s.history) - 1; i >= 0; i-- {
		if s.history[i].Kind == diagnosis.KindSystemSnapshot {
			snapshotText = s.history[i].Content
			break
		}
	}
	if snapshotText == "" {
		return
	}
	localIssues := s.ruleEngine.Run(snapshotText)
	if len(localIssues) == 0 {
		return
	}
	s.issues = diagnosis.MergeByTitle(s.issues, localIssues)
	output.IssuesBox(localIssues)
	output.InfoMessage(fmt.Sprintf("本地规则引擎命中 %d 条已知问题，已注入诊断上下文", len(localIssues)))
}

// Issues 返回会话累积的结构化诊断发现（已按严重度排序、去重）
func (s *Session) Issues() []diagnosis.Issue {
	return s.issues
}

// ContinueWithInput 加载历史后追加用户输入继续对话
func (s *Session) ContinueWithInput() {
	output.Promptln("\n🔄 历史记录已加载，准备继续对话...")
	output.Promptln("请输入您的问题或命令 (输入 'exit' 退出):")
	line, _ := s.reader.ReadString('\n')
	input := llm.CleanInput(line)
	if strings.TrimSpace(input) == "" {
		input = "continue"
	}
	s.history = append(s.history, diagnosis.NewMessage("user", input, diagnosis.KindUserFollowup))
}

// Run 启动会话主循环，结束后落盘最终历史
func (s *Session) Run() {
	s.runLoop()
	if err := s.saveHistory(); err != nil {
		output.ErrorMessage("保存聊天历史失败: " + err.Error())
	} else {
		output.SuccessMessage("聊天历史已保存: " + s.historyFile)
	}
	// 报告增量保存过程中累积的错误（过去被静默忽略）
	if s.lastSaveErr != nil {
		output.WarningMessage("历史增量保存过程中出现过错误: " + s.lastSaveErr.Error())
	}
}

// userAction 用户输入的动作类型
type userAction int

const (
	actionContinue userAction = iota
	actionExit
)

// runLoop 会话主循环 —— 单一显式状态机，非递归。
//
// 诊断模式（MaxRounds>0）：仅在最终结论或达到轮次上限时与用户交互，其余轮次
// 持续调用 AI 推进诊断。
// 智能模式（MaxRounds==0）：每次 AI 响应后都与用户交互。
//
// 退出只在本循环的 return 处发生，语义明确。
func (s *Session) runLoop() {
	roundsSinceFinal := 0
	for {
		s.trimHistory()

		maxRounds := s.mode.MaxRounds()
		if maxRounds > 0 && roundsSinceFinal >= maxRounds {
			output.WarningMessage(fmt.Sprintf("已达到最大诊断轮次 %d，可继续追问或生成报告", maxRounds))
			input, action := s.readUserAction()
			s.saveHistorySilently()
			if action == actionExit {
				return
			}
			if input != "" {
				s.history = append(s.history, diagnosis.NewMessage("user", input, diagnosis.KindUserFollowup))
				roundsSinceFinal = 0
			}
			continue
		}

		resp, err := s.llmClient.CallAI(s.ctx, s.history)
		if err != nil {
			if !s.handleAIFailure(err) {
				return
			}
			continue
		}
		roundsSinceFinal++
		s.history = append(s.history, diagnosis.NewMessage("assistant", resp.ToJSON(), diagnosis.KindAgentResponse))
		if len(resp.Issues) > 0 {
			s.mergeIssues(resp.Issues)
			output.IssuesBox(s.issues)
		}

		if len(resp.Commands) > 0 {
			results, _ := s.cmdExecutor.HandleCommands(s.ctx, resp.Commands)
			if msg := BuildHistoryMessage(results); msg != "" {
				s.history = append(s.history, diagnosis.NewMessage("user", msg, diagnosis.KindCommandResult))
			}
			s.saveHistorySilently()
			continue
		}

		// 诊断模式：只有最终结论才打断 AI 推进
		if maxRounds > 0 {
			if resp.IsFinal {
				if strings.TrimSpace(resp.Analysis) != "" {
					output.FinalReportBox(resp.Analysis)
				}
				roundsSinceFinal = 0
				s.afterFinal = true
				s.saveHistorySilently()
				input, action := s.readUserAction()
				s.afterFinal = false
				s.saveHistorySilently()
				if action == actionExit {
					return
				}
				if input != "" {
					s.history = append(s.history, diagnosis.NewMessage("user", input, diagnosis.KindUserFollowup))
				}
			} else if strings.TrimSpace(resp.Analysis) != "" {
				output.AIAnalysisBox(resp.Analysis)
			}
			continue
		}

		// 智能模式：每次响应后都与用户交互
		if resp.IsFinal {
			if strings.TrimSpace(resp.Analysis) != "" {
				output.FinalReportBox(resp.Analysis)
			}
			s.afterFinal = true
		} else if strings.TrimSpace(resp.Analysis) != "" {
			output.AIAnalysisBox(resp.Analysis)
		} else {
			output.WarningMessage("AI 暂时无法提供分析，请尝试重新描述问题或提供更多信息")
		}
		s.saveHistorySilently()
		input, action := s.readUserAction()
		s.afterFinal = false
		s.saveHistorySilently()
		if action == actionExit {
			return
		}
		if input != "" {
			s.history = append(s.history, diagnosis.NewMessage("user", input, diagnosis.KindUserFollowup))
		}
	}
}

// handleAIFailure 处理 AI 调用失败，让用户选择重试或退出，
// 而非直接结束整个会话（历史仍由 Run() 末尾保存，不会丢失）。
// 返回 true 表示重试（继续主循环），false 表示退出。
func (s *Session) handleAIFailure(err error) bool {
	output.ErrorMessage("AI 调用失败: " + err.Error())
	output.Promptln(output.Yellow + "请选择: 1.重试  2.保存并退出" + output.Reset)
	output.Prompt("选择 (1-2，默认重试) > ")
	line, _ := s.reader.ReadString('\n')
	choice := strings.TrimSpace(strings.ToLower(llm.CleanInput(line)))
	if choice == "2" || choice == "exit" || choice == "q" {
		return false
	}
	return true
}

// readUserAction 读取用户输入并处理 report/exit 等特殊指令。
// 非递归：生成报告后通过 for 循环回到读取，而非递归调用自身。
func (s *Session) readUserAction() (string, userAction) {
	firstIter := true
	for {
		// 每次阻塞读取前都打印提示：
		//   - 首次进入：按 afterFinal 决定是否显示完整 InputPromptBox
		//   - 报告生成后回到循环：必须重新显示，否则用户看不到程序在等待输入
		//     （曾出现"报告已生成→用户敲回车想退出→被空输入默认为'继续分析'→误触发 CallAI"）
		if firstIter || s.justGeneratedReport {
			if !s.afterFinal || s.justGeneratedReport {
				output.InputPromptBox()
			}
			s.justGeneratedReport = false
			firstIter = false
		}
		line, _ := s.reader.ReadString('\n')
		input := llm.CleanInput(line)
		trimmed := strings.TrimSpace(input)

		if trimmed == "" {
			return "继续分析", actionContinue
		}

		// 报告生成
		if trimmed == "1" || trimmed == "report" || trimmed == "报告" ||
			strings.ToLower(trimmed) == "r" || strings.ToLower(trimmed) == "rep" {
			s.handleReportRequest()
			continue
		}

		// 退出
		if trimmed == "2" || strings.ToLower(trimmed) == "exit" ||
			strings.ToLower(trimmed) == "quit" || strings.ToLower(trimmed) == "退出" ||
			strings.ToLower(trimmed) == "q" {
			return "", actionExit
		}

		return trimmed, actionContinue
	}
}

// mergeIssues 将本轮 AI 返回的 issues 并入会话累积列表。
// 直接使用 diagnosis.MergeByTitle，消除与 report/engine 的重复实现。
func (s *Session) mergeIssues(newIssues []diagnosis.Issue) {
	if len(newIssues) == 0 {
		return
	}
	s.issues = diagnosis.MergeByTitle(s.issues, newIssues)
}

// trimHistory 裁剪历史，保留前导块与关键节点消息。
//
// 改造说明（与旧版差异）：
//  1. 前导块识别从 strings.Contains 改为按 Kind == KindSystemSnapshot 判定，
//     更准确，避免误把其他含"初始系统快照"文本的消息当作前导块。
//  2. 关键节点消息（KindUserRequirement、含 IsFinal 的 assistant 响应、
//     含 Issues 的 assistant 响应）在裁剪时保留，避免用户原始问题描述
//     与中间根因分析被尾部窗口裁掉。
//  3. 尾部窗口 maxKeep 从 8 扩到 12，保留更多上下文。
func (s *Session) trimHistory() {
	const maxKeep = 12
	if len(s.history) <= maxKeep {
		return
	}

	// 识别前导块：system 消息 + 紧随其后的 KindSystemSnapshot user 消息
	preamble := 0
	for preamble < len(s.history) {
		m := s.history[preamble]
		if m.Role == "system" || m.Kind == diagnosis.KindSystemSnapshot {
			preamble++
			continue
		}
		break
	}

	// 识别关键节点消息：用户原始问题、命令执行结果、含分析的 assistant 响应。
	// 这些消息在多轮交互中承载诊断证据与分析，不能被尾部窗口裁掉，
	// 否则报告只包含最后一轮交互。
	keyIndices := make([]int, 0, 16)
	for i := preamble; i < len(s.history); i++ {
		m := s.history[i]
		switch m.Kind {
		case diagnosis.KindUserRequirement, diagnosis.KindCommandResult:
			keyIndices = append(keyIndices, i)
			continue
		}
		// 所有带分析内容的 assistant 响应都保留（含 IsFinal 与含 Issues 的）；
		// 不再只保留 IsFinal/issues 的，否则中间轮次的分析会丢失。
		if m.Role == "assistant" && m.Kind == diagnosis.KindAgentResponse {
			var resp diagnosis.AgentResponse
			if json.Unmarshal([]byte(m.Content), &resp) == nil {
				if strings.TrimSpace(resp.Analysis) != "" || len(resp.Issues) > 0 {
					keyIndices = append(keyIndices, i)
				}
			}
		}
	}

	// 待保留的尾部窗口
	tailStart := len(s.history) - maxKeep
	// 关键节点若落在窗口外，则额外保留
	preserve := make([]diagnosis.Message, 0, preamble+maxKeep+len(keyIndices))
	preserve = append(preserve, s.history[:preamble]...)
	for _, idx := range keyIndices {
		if idx < tailStart {
			preserve = append(preserve, s.history[idx])
		}
	}
	preserve = append(preserve, s.history[tailStart:]...)
	s.history = preserve
}

// handleReportRequest 处理报告生成请求，复用会话历史文件，不再另写副本
func (s *Session) handleReportRequest() {
	s.saveHistorySilently() // 确保磁盘上是最新历史

	output.OptionMenu("📊 请选择报告格式：", []string{
		"MD   - Markdown 格式",
		"HTML - 网页格式",
		"PDF  - PDF 格式",
	})
	output.Prompt("选择格式 (1-3) > ")

	choiceStr, _ := s.reader.ReadString('\n')
	choiceStr = strings.TrimSpace(choiceStr)
	choice := 1
	fmt.Sscanf(choiceStr, "%d", &choice)
	if choice < 1 || choice > 3 {
		choice = 1
	}

	format := "md"
	switch choice {
	case 2:
		format = "html"
	case 3:
		format = "pdf"
	}
	if _, err := s.reportGen.Generate(s.historyFile, format); err != nil {
		output.ErrorMessage("生成报告失败: " + err.Error())
	}
	// 标记刚生成完报告，让 readUserAction 在下一次读取前重新打印提示框
	s.justGeneratedReport = true
}

// saveHistory 原子地写入历史到 s.historyFile（写临时文件 + rename），
// 避免崩溃时留下半个文件。返回错误由调用方决定如何处理。
func (s *Session) saveHistory() error {
	data, err := json.Marshal(s.history)
	if err != nil {
		return err
	}
	tmp := s.historyFile + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, s.historyFile)
}

// saveHistorySilently 增量保存历史，记录错误到 lastSaveErr 供 Run() 末尾报告。
// 避免中间每轮弹错打断诊断流程；但错误不会被完全吞掉。
func (s *Session) saveHistorySilently() {
	if err := s.saveHistory(); err != nil {
		s.lastSaveErr = err
	}
}
