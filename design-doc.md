# LinDiag-Agent 重构设计方案

## 1. 重构目标

本次重构的主要目标是将 LinDiag-Agent 从单文件结构拆分为符合 Go 社区工程实践（Standard Go Project Layout）的目录结构，提高代码的可维护性、可扩展性和可读性。

## 2. 目录结构设计

### 2.1 重构后的目录结构

```
lindiag-agent/
├── cmd/
│   └── agent/
│       └── main.go          // 程序入口，解析命令行参数
├── internal/
│   ├── llm/
│   │   └── client.go        // 封装 DeepSeek 调用、消息历史管理
│   ├── platform/
│   │   ├── executor.go      // 执行系统命令、安全过滤
│   │   └── snapshot.go      // 采集系统快照 (uname, free, df等)
│   ├── report/
│   │   └── engine.go        // 报告生成核心逻辑
│   └── safety/
│       ├── rules.go         // 危险关键词定义与校验逻辑
│       └── analyzer.go      // 命令风险分析器
├── go.mod
├── whitelist.txt            // 安全命令白名单
└── rules.txt                // 外部规则文件
```

### 2.2 目录结构说明

- **cmd/agent/**：包含程序的入口点，负责解析命令行参数和启动应用。
- **internal/llm/**：封装 LLM（大语言模型）相关的功能，包括 API 调用、消息历史管理等。
- **internal/platform/**：抽象平台相关的功能，如执行系统命令和采集系统快照。
- **internal/report/**：负责报告生成的核心逻辑。
- **internal/safety/**：定义危险关键词和校验逻辑，确保系统安全。包含命令风险分析器，用于评估命令的风险级别。

## 3. 核心逻辑拆分理由

### 3.1 LLM 层隔离

- **原因**：原代码中 `callAI` 和 `ChatRequest` 结构体占据了大量空间，与业务逻辑混合在一起。
- **优势**：将 LLM 相关功能独立到 `internal/llm/client.go`，方便未来切换模型（比如从 DeepSeek 切换到其他模型）而无需改动业务逻辑。

### 3.2 平台能力抽象

- **原因**：`executeCommand` 和 `getSnapshot` 属于系统交互层，与业务逻辑耦合。
- **优势**：将这些功能移至 `internal/platform` 目录，实现系统交互层的抽象。如果以后要支持 Windows 运维，只需要增加一个 `executor_windows.go`。

### 3.3 安全逻辑解耦

- **原因**：`isSafeCommand` 与业务逻辑混合，不利于独立维护和扩展。
- **优势**：将安全逻辑移至 `internal/safety/rules.go`，使其成为一套独立的防火墙，甚至可以引入更复杂的正则表达式或语义分析。

### 3.4 报告生成模块化

- **原因**：报告生成相关逻辑与核心业务逻辑混合，导致代码结构混乱。
- **优势**：将报告生成逻辑移至 `internal/report/engine.go`，使报告生成逻辑与核心业务逻辑分离，便于独立维护和扩展。

## 4. 技术实现

### 4.1 模块职责划分

| 模块 | 主要职责 | 文件位置 |
|------|----------|----------|
| 入口模块 | 解析命令行参数，启动应用 | cmd/agent/main.go |
| LLM 模块 | 封装 DeepSeek API 调用，管理消息历史 | internal/llm/client.go |
| 平台模块 | 执行系统命令，采集系统快照 | internal/platform/executor.go, internal/platform/snapshot.go |
| 报告模块 | 生成各种格式的报告 | internal/report/engine.go |
| 安全模块 | 危险命令检测与校验，命令风险分析 | internal/safety/rules.go, internal/safety/analyzer.go |

### 4.2 关键函数设计

#### 4.2.1 LLM 模块

- `CallAI(messages []Message) string`：调用 DeepSeek API 获取 AI 响应
- `LoadDefaultChatHistory(reader *bufio.Reader, modeID int, systemPrompt string, snapshotCmds []string, rules string) []Message`：加载默认聊天历史
- `FixConsecutiveAssistantMessages(messages []Message) []Message`：修复连续的 assistant 消息
- `CleanInput(input string) string`：清理输入内容
- `TruncateOutput(output string, maxChars int) string`：截断输出内容

#### 4.2.2 平台模块

- `ExecuteCommand(cmd string) (string, error)`：执行系统命令并获取结果
- `GetSnapshot(cmds []string) string`：采集系统快照
- `GetHostname() string`：获取主机名
- `GetIPAddress() string`：获取 IP 地址

#### 4.2.3 安全模块

- `LoadWhitelist(filename string) error`：从文件加载安全命令白名单
- `NewCommandAnalyzer() *CommandAnalyzer`：创建一个新的命令分析器
- `AnalyzeCommand(cmd string) (RiskLevel, string)`：分析命令的风险级别
- `GetSafeCommands() []string`：获取安全命令列表

#### 4.2.4 报告模块

- `GenerateReport(historyFile string, format string)`：从历史记录生成报告
- `ExtractReportContent(chatHistory []llm.Message) map[string]string`：提取报告内容
- `GenerateMarkdownReport(filename string, content map[string]string)`：生成 Markdown 报告
- `GenerateHTMLReport(filename string, content map[string]string)`：生成 HTML 报告
- `GeneratePDFReport(filename string, content map[string]string)`：生成 PDF 报告

### 4.3 依赖管理

使用 `go.mod` 管理依赖，模块名称设置为 `github.com/LinDiag-Agent`，确保包导入路径的一致性。

## 5. 重构前后对比

### 5.1 重构前

- 单文件结构，所有代码混合在一起
- 难以维护和扩展
- 模块边界不清晰
- 测试困难

### 5.2 重构后

- 模块化结构，职责分明
- 易于维护和扩展
- 模块边界清晰
- 便于单元测试
- 符合 Go 社区最佳实践

## 6. 未来扩展计划

1. **模型切换**：通过修改 `internal/llm/client.go` 中的实现，可以轻松切换到其他 LLM 模型。

2. **跨平台支持**：通过在 `internal/platform` 目录中添加不同平台的实现，如 `executor_windows.go`，可以支持 Windows 运维。

3. **安全增强**：在 `internal/safety/rules.go` 中可以引入更复杂的安全检查机制，如正则表达式匹配、语义分析等。

4. **报告模板**：在 `internal/report` 目录中可以添加更多的报告模板，支持更多的报告格式和样式。

5. **配置管理**：可以添加 `internal/config` 包，用于管理应用配置，如 API 密钥、默认参数等。

## 7. 总结

本次重构成功将 LinDiag-Agent 从单文件结构拆分为符合 Go 社区工程实践的目录结构，提高了代码的可维护性、可扩展性和可读性。通过模块化设计，使得各个功能模块职责分明，便于独立维护和扩展。同时，这种结构也为未来的功能扩展和技术升级打下了良好的基础。

## 8. 优化记录

### 8.1 2026-03-10 优化内容

#### 8.1.1 安全模块优化

1. **增强危险命令检测**：改进了 `AnalyzeCommand` 函数，使其能够更准确地检测危险命令，包括：
   - 检测 `rm`、`mv`、`chmod`、`chown` 等危险操作
   - 检测 `find` 命令中的危险 `-exec` 参数
   - 检测命令组合中的危险操作（如 `cd /tmp && find . -exec rm {} \;`）

2. **白名单系统**：实现了可配置的安全命令白名单系统，通过 `whitelist.txt` 文件进行配置，提高了系统的安全性和灵活性。

3. **风险级别分类**：将命令风险分为多个级别（Safe、Low、Medium、High、Critical），根据风险级别采取不同的处理策略，提高了系统的安全性。

#### 8.1.2 平台模块优化

1. **后台命令处理**：改进了 `ExecuteCommand` 函数，使其能够正确处理后台运行的命令（以 `&` 结尾的命令），避免了命令卡住的问题。
   - 对于后台命令，使用 `Start()` 方法启动命令并立即返回
   - 对于前台命令，继续使用 `CombinedOutput()` 方法等待命令完成并获取输出

2. **命令执行优化**：提高了命令执行的可靠性和安全性，确保系统命令能够正确执行并返回结果。

### 8.1.3 功能改进

1. **安全提示**：为不同风险级别的命令提供了不同的安全提示，帮助用户了解命令的风险。

2. **用户交互**：改进了用户交互流程，对于高风险命令，要求用户确认并提供更多信息，提高了系统的安全性。

3. **报告生成**：优化了报告生成功能，支持生成 Markdown、HTML 和 PDF 格式的报告。

## 9. 构建与运行

### 9.1 构建

```bash
go build -o lindiag-agent cmd/agent/main.go
```

### 9.2 运行

```bash
# 正常启动
./lindiag-agent

# 加载历史记录文件继续对话
./lindiag-agent load <file>

# 从历史记录生成报告
./lindiag-agent report <file> <format>  # 格式: md, html, pdf
```
