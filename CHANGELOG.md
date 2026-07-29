# Changelog

本文件记录 LinDiag-Agent 各版本的变更。

格式参考 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，版本号遵循
[Semantic Versioning](https://semver.org/lang/zh-CN/)。

---

## [4.2.0] — 2026-07-29

v4.2.0 是安全与健壮性集中加固版本：覆盖信号处理、命令注入防护、并发安全、
LLM 调用健壮性、历史文件安全写入等多个维度，同时完成若干优化项。

### 安全

- **信号处理优雅退出**：main.go 引入 `signal.NotifyContext`，Ctrl+C / SIGTERM
  时取消 context、落盘历史、回收子进程，不再中断 LLM 调用或泄漏进程
- **配置文件权限 0600**：`config.json` / `user_prefs.json` 写入权限从 `0644` 改为
  `0600`，目录权限从 `0755` 改为 `0700`，防止同主机其他用户读取 API Key
- **HTML 报告 XSS 修复**：所有动态字段过 `escapeHTML`，消除 `Hostname` /
  `IPAddress` / `OSInfo` 等字段直接拼接到 HTML 的注入风险
- **splitCommandChain 引号感知**：改用基于 rune 的状态机扫描，正确处理引号
  上下文，堵住 `sh -c "rm -rf / | nc evil"` 类绕过
- **管道递归分析**：`analyzeCommandExecutor` 对管道末段 bash/sh 递归分析
  左侧输入内容，`echo "rm -rf /" | bash` 不再与 `echo "uptime" | bash`
  同判为 High
- **executor data race 修复**：超时分支用 channel 传递结果，消除闭包变量
  并发读写 race
- **readUserAction ctx 检查**：循环开头检查 `s.ctx.Err()`，SIGINT 后不再
  卡在 `reader.ReadString`
- **全局状态加锁**：safety `safeCommandsMu`、config `prefsMu`、llm `cfgMu`、
  platform `atomic.Int32` 保护包级可变全局变量，`go test -race` 不再报错

### 健壮性

- **sudo/xargs 误判修复**：sudo `-i/-s/--login/--shell` 显式判 Critical；
  xargs `-I/-L/-n/-P` 跳过下一个参数，避免 `{}` 干扰子命令检测
- **危险正则补全**：`chmod 777 /etc`、`chown root:root /var` 等路径后
  跟子路径的场景不再漏判
- **LLM 重试指数退避**：固定 3s 改为指数退避 + 抖动 (1s, 2s, 4s ± jitter)；
  降级判定收紧为按 HTTP 状态码 + 关键字精确匹配
- **WithLLMClient 同步到 handler**：`NewSession` 应用 opts 后将
  `s.llmClient` 同步到 `s.handler`，`explainCommand` 不再绕过 mock
- **baseSnapshotCmds 注入防护**：服务名用正则 `^[A-Za-z0-9_.@-]+$` 白名单校验
- **rules 规则修正**：僵尸进程严格按首字段 `=="Z"` 校验，conntrack max 缺失时
  跳过不再误报 100%
- **report 错误冒泡**：三个生成器返回 `error`，`GenerateReport` 返回
  `(string, error)`，写盘失败不再误报"已生成"
- **handleAIFailure 重试上限**：加重试次数上限 (5 次) + 指数退避，超过自动
  保存退出，不再无限重试烧 token
- **LoadHistory 路径校验**：传入路径必须位于 `paths.HistoryDir()` 下，防止
  路径遍历读取任意文件
- **saveHistory CreateTemp**：用 `os.CreateTemp` 在同目录生成唯一临时文件，
  避免多实例并发写冲突与崩溃后残留
- **UTF-8 截断修复**：`truncateLine` / `truncateForPreview` 改用 `[]rune`
  截断，不再落在中文字符中间

### 优化

- **trimHistory 滑动窗口+锚点**：关键节点作为锚点保留，滑动窗口覆盖锚点
  之间上下文，避免中间因果链丢失
- **design-doc.md 归档**：标注为历史稿 `design-history.md`，避免与代码不符
  误导维护者
- **whitelist.txt 整理**：根目录文件重命名为 `whitelist.example.txt`，消除
  路径与格式矛盾
- **rules 正则预编译**：`firstNumber` / `parseHumanSize` 等正则改为包级
  `var` 预编译，避免每次调用重新编译
- **output io.Writer 抽象**：提供 `var Writer io.Writer = os.Stdout`，支持
  测试重定向

---

## [4.1.0] — 2026-07-24

v4.1 是 Windows 平台支持版本：从"仅能编译、功能完全不可用"升级到"Windows 端功能
完整可用"，同时对不支持中文的 Linux 系统做了 ASCII 降级兼容。本次变更覆盖跨平台
分流、诊断命令集、规则引擎、安全分析器、进程管理、路径规范、报告引擎七个维度。

### 新增

#### 跨平台 Shell 分流

- **平台函数抽象**：`newShellCommand` / `newShellCommandContext` / `getIPAddress` /
  `isBackgroundCommand` / `isShellAvailable` / `snapshotPromptPrefix` 按 GOOS 分流
  - Unix：`sh -c` + `hostname -I` + `&` 后台识别
  - Windows：`powershell -NoProfile -NonInteractive -Command` + `Get-NetIPAddress`
- **快照前缀分流**：`snapshotPromptPrefix()` 返回平台提示符（Linux: `$ `，
  Windows: `> `），所有下游解析器（`extractCommandOutput` / `extractAfterCommand`）
  同时支持两种前缀
- **环境检测分流**：`probeDocker` / `probeKubernetes` / `probeHelmNoAbn` /
  `probeNaliNoAbn` / `prometheusDeployed` / `checkCmd` / `runOutput` 按平台实现
  - Unix：`which` + `sh -c`
  - Windows：`Get-Command` + `powershell -Command`

#### Windows 诊断命令集

- **基础快照命令**：`Get-CimInstance Win32_OperatingSystem`（OS 版本/运行时间/
  内存） / `Win32_ComputerSystem`（计算机信息） / `Win32_Processor`（CPU） /
  `$PSVersionTable`（PowerShell 版本）
- **故障诊断命令映射**：
  - `uptime` → `Get-CimInstance Win32_OperatingSystem | Select LastBootUpTime`
  - `free -h` → `Get-CimInstance Win32_OperatingSystem | Select FreePhysicalMemory, TotalVisibleMemorySize`
  - `df -h` → `Get-Volume | Where { $_.DriveLetter }`
  - `ps` → `Get-Process | Sort-Object WS -Descending | Select -First 15`
  - `dmesg` → `Get-EventLog -LogName System -Newest 30`
  - 僵尸进程 → `Get-Process | Where { $_.Responding -eq $false }`
  - `systemctl --failed` → `Get-Service | Where { Status=Stopped -and StartType=Automatic }`
- **深度诊断追加**：`Get-CimInstance Win32_PnPSignedDriver`（驱动） /
  `Get-NetTCPConnection | Group State`（连接） / `Get-NetRoute`（路由） /
  `Get-NetAdapterStatistics`（网卡） / `Get-Counter`（性能计数器）
- **服务状态查询**：`Get-Service -Name '<svc>' | Select Name, Status, StartType`

#### Windows LLM 提示词

- Windows 版 SystemPrompt 明确要求 PowerShell 命令，禁止生成 Linux 命令
  （ps/top/df/free/systemctl/cat /proc 等）
- 快照数据引用要求适配 Windows cmdlet 输出字段

#### Windows 规则引擎（5 条）

- `winMemPressureRule`：解析 `FreePhysicalMemory` / `TotalVisibleMemorySize` /
  `FreeVirtualMemory` / `TotalVirtualMemorySize`（内存+页面文件压力）
- `winDiskCapacityRule`：解析 `Get-Volume` 的 `SizeRemaining` / `Size`
- `winUnresponsiveProcessRule`：检测 `Responding=False` 进程（替代僵尸进程）
- `winStoppedServicesRule`：检测自动启动但已停止的服务（替代 systemctl --failed）
- `winCriticalEventRule`：统计 `Get-EventLog` 中 Error/Critical 事件（替代 dmesg OOM）

#### Windows 安全分析器扩展

- **白名单**：45 个只读 cmdlet（Get-* 系列）+ Windows 外部工具
  （ipconfig/systeminfo/tasklist/whoami 等）
- **危险命令**：20 个 Critical 级命令（Remove-Item/Stop-Computer/Clear-Disk/
  Format-Volume/Stop-Service/Stop-Process 等）
- **中等风险**：20 个变更类 cmdlet（Set-*/New-*/Start-*/Copy-Item 等）
- **危险模式正则**：7 个 Windows 专属模式（Remove-Item 递归删根盘符/format C:/
  diskpart/Stop-Computer/Clear-Disk 等）
- **Shell 递归分析**：`powershell -Command` / `pwsh -Command` / `cmd /c` 识别并
  递归分析子命令
- **重定向清理**：`hasWriteRedirect` 兼容 Windows `NUL` / `$null` 空目标

#### Windows 进程组管理

- `setProcessGroup`：`CREATE_NEW_PROCESS_GROUP` 创建新进程组
- `killProcessGroup`：`taskkill /T /F /PID` 递归终止进程树
  （等效 Linux `kill(-pgid, SIGKILL)`）

#### Windows 路径规范

- ConfigDir：`%APPDATA%\lindiag\`（Roaming，跨机器漫游配置）
- DataDir：`%LOCALAPPDATA%\lindiag\`（Local，机器本地数据）

#### 报告引擎平台适配

- `extractOSInfo`：自动检测平台，Linux 用 `PRETTY_NAME=`，Windows 用 `Caption` + `Version`
- `extractUptimeInfo`：Linux 用 `uptime`，Windows 用 `LastBootUpTime`
- `extractKernelVer`：Linux 用 `uname -a`，Windows 用 `Caption` + `BuildNumber`

#### ASCII 降级（不支持中文的 Linux 系统）

- **终端检测**：`DetectTerminalEnv` 检测 `LANG` / `LC_*` / `TERM` / `NO_COLOR`
- **图标降级**：`icon()` 在非 UTF-8 终端用 ASCII 字符替代 emoji
- **制表符降级**：`ascii_fallback.go` 将 Unicode 制表符映射为 `+-|` ASCII 字符
- **安全截断**：`displayWidth` / `runeWidth` / `truncateByWidth` 按显示宽度截断
- **报告截断**：`safeTruncate` 按 rune 边界截断

### 变更

- **规则引擎拆分**：`rules_builtin.go` 拆分为公共工具函数 + `rules_builtin_linux.go`
  （8 条 Linux 规则） + `rules_builtin_windows.go`（5 条 Windows 规则），
  `NewEngine()` 改为调用平台分流的 `newBuiltinRules()`
- **mode.go 拆分**：`mode_linux.go` / `mode_windows.go` 各自注册诊断模式，
  `mode.go` 仅保留平台无关的 `outputFormatRule` 常量
- **snapshot.go 拆分**：`snapshot_basics_unix.go` / `snapshot_basics_windows.go`
  各自提供 `basicSnapshotCmds()`
- **paths.go 拆分**：`paths_unix.go` / `paths_windows.go` 各自提供
  `xdgConfigHome()` / `xdgDataHome()` / `homeDir()`
- **executor.go**：所有 `exec.Command("sh","-c",cmd)` 替换为 `newShellCommand(cmd)`，
  `GetHostname` 改用 `os.Hostname()`，`GetIPAddress` 改用 `getIPAddress()`
- **engine.go `extractCommandOutput`**：同时支持 `$ ` 和 `> ` 前缀的快照格式

### 修复

- 修复 Windows 下 `sh -c` 不可用导致全部命令执行失败
- 修复 Windows 下进程组管理为空实现导致超时无法杀子进程
- 修复 Windows 下路径使用 `~/.config/lindiag/` 导致配置/历史找不到
- 修复 Windows 报告中 OSInfo/KernelVer/UptimeInfo 恒为"未知"
- 修复快照格式 `$ ` 前缀硬编码，Windows 适配分支成为死代码

### 已知限制

- Windows 未实现 `Get-Counter '\Processor(_Total)\% Processor Time'` 的 CPU 负载规则
  （PowerShell 性能计数器采集较慢，暂用进程 TOP 替代）
- Windows 安全分析器未覆盖 `reg.exe`（注册表命令行工具）的子命令分级
- 以下问题从 v4.0 继承，记录在 [ROADMAP.md](ROADMAP.md)：

---

## [4.0.0] — 2026-07-21

v4.0 是继 v3.x 单文件架构重构之后的一次大版本演进：从"可用的 AI 诊断脚本"
升级到"分层清晰、安全护栏完备、可持久化、可降级的运维诊断平台"。本次变更
覆盖架构分层、安全、LLM 调用、报告、配置、工程化六个维度。

### 新增

#### 架构分层

- **9 包分层架构**：在 v3.x 的 5 包(llm/platform/report/safety/config)基础上
  新增 4 个包，职责进一步正交化：
  - `internal/agent`：Session 主编排器 + Handler 命令分发 + Mode 模式注册 +
    Env 环境探测 + Context 上下文组装
  - `internal/diagnosis`：跨层数据契约(Message/Issue/Response)，替代魔法字符串
  - `internal/rules`：本地阈值规则引擎
  - `internal/paths`：XDG Base Directory 规范的统一路径管理
  - `internal/output`：输出格式化(Markdown 渲染、表格、提示框)
- **Functional Options 依赖注入**：`WithLLMClient` / `WithCommandExecutor` /
  `WithReportGenerator`，支持测试桩注入，兼容旧签名
- **插件化模式注册**：`DiagnosticMode` 接口 + `RegisterMode` + `init()` 自动注册，
  第三方模式扩展无需改核心代码

#### 数据契约

- **`diagnosis.MessageKind` 枚举**：7 个语义角色标记(system_preamble /
  system_snapshot / user_requirement / command_result / user_followup /
  agent_response / agent_filler)，替代过去靠 `strings.Contains("初始系统快照")`
  识别消息类型的脆弱方式；旧历史文件加载时自动回退兼容
- **`diagnosis.Issue` 结构化问题**：Title/Severity/Category/Evidence/Suggestion/
  Confidence，与本地规则引擎和 LLM 输出共享同一契约
- **`MergeByTitle` 去重**：本地规则 Issue 与 LLM Issue 按标题自动去重合并

#### LLM 调用

- **三级降级链**：`json_schema` → `json_object` → `none`，带会话级
  `degradedFormat` 记忆，避免重复试探不支持的格式
- **降级状态隔离**：`CallAISimple` 通过 `withIsolatedDegradedState` 不污染主会话
  降级状态
- **重试 + 超时机制**：`CallAI` 支持 context 取消，可重试错误识别
  (429/503/超时)，最多 3 次重试
- **`CallAISimple` 辅助调用**：用于命令解释等轻量场景，独立降级状态

#### 安全护栏

- **本地规则引擎**(8 条)：覆盖 loadavg/mem/disk容量/disk inode/zombie/oom/
  conntrack/failed_services，零依赖实现 parseHumanSize/itoa，与 LLM Issue 同契约
- **5 级风险分级**：Safe/Low/Medium/High/Critical，未知命令默认 Medium
  ("宁可多确认，不可漏确认")
- **23 类多义工具子命令分级表**：docker/kubectl/systemctl/ip/iptables/ffmpeg/
  find/systemctl 等，按复合子命令精确匹配(如 `docker network rm` ≠ `docker rm`)
- **递归分析**：对 `sh -c`/`bash`/`xargs`/`sudo` 等命令执行器递归分析参数
- **`baseSnapshotCmds` 主题分块**：基础资源 → 进程/负载 → 内核日志 → 磁盘详情 →
  僵尸进程，所有命令强制截断 + `2>/dev/null` 容错

#### 配置与路径

- **XDG Base Directory 合规**：配置走 `$XDG_CONFIG_HOME/lindiag/`，
  数据走 `$XDG_DATA_HOME/lindiag/`，避免历史/报告散落 CWD
- **配置优先级链**：环境变量 > 配置文件 > 默认值
- **用户偏好持久化**：`user_prefs.json` 记录"不再询问中风险"等偏好
- **外部规则文件**：`rules.txt` 支持用户自定义补充规则
- **白名单文件**：`whitelist.txt` 合并式加载(不替换内置白名单)

#### 报告

- **Markdown / HTML / PDF 三格式**：PDF 通过探测 wkhtmltopdf / chromium 转换
- **从历史提取报告数据**：`ExtractReportData` 从聊天历史提取系统信息、
  问题列表、最终结论
- **风险级别颜色区分**：Safe 青/Low 蓝/Medium 黄/High 红/Critical 高亮

#### 工程化

- **原子写历史**：`saveHistory` 用临时文件 + `os.Rename`，避免崩溃留下半截 JSON
- **历史增量保存**：关键节点调用 `saveHistorySilently`，崩溃不丢全过程
- **`trimHistory` 智能裁剪**：按 MessageKind 识别关键节点(用户诉求/快照/追问)
  保留 + 尾部窗口，避免上下文超长

### 变更

- **`llm.CallAI` 签名**：从 `CallAI(messages []Message)` 改为
  `CallAI(ctx context.Context, messages []diagnosis.Message)`，支持 context 取消
- **命令执行超时**：`platform.ExecuteCommandWithTimeout` 用 `exec.CommandContext`
  + 进程组管理(unix: setpgid/killpg，windows: 占位待实现)
- **`safety.analyzer.go` 合并 rules.go**：危险关键词与风险分级逻辑统一到单文件
- **API Key 不再内置**：v3.1 已移除，v4.0 保持，强制用户配置
- **`outputFormatRule` 全模式共用**：所有模式尾部追加统一的 Markdown 输出格式约束

### 修复

- 修复后台命令(`&` 结尾)卡住问题：改用 `Start()` 启动并立即返回
- 修复 AI 返回空内容死循环：增加最多 3 次重试 + 友好提示
- 修复连续 assistant 消息导致 API 报错：`FixConsecutiveAssistantMessages`
  自动插入占位 user 消息

### 移除

- **`templates/` 目录**：7 个模式子目录(compliance_check/docker/fault_diagnosis/
  inspection/kubernetes/security_audit/smart_mode)共 14 个 .md/.html 模板文件。
  历史遗留工件，实际报告生成走 [engine.go](internal/report/engine.go) 硬编码拼接，
  从未被代码引用。本版本彻底删除避免误导维护者。
- **根目录 `inspection_report.md` / `inspection_report.html`**：与
  `templates/inspection/` 内容重复的遗留文件，同步删除。

### 已知限制

下列问题已记录在 [ROADMAP.md](ROADMAP.md)，留待 4.1+ 版本逐步完善：

- main.go 无信号处理，Ctrl+C 无法优雅退出
- 包级全局可变状态(userPrefs/appConfig/degradedFormat)无并发锁
- `safety.splitCommandChain` 不处理引号上下文，存在注入绕过面
- `platform.ExecuteCommand` 用裸 `sh -c`，本身不做安全校验
- 全项目无结构化日志(仍用 `fmt.Println`)
- 测试覆盖约 0%(仅 4 个 `_test.go`)
- ~~Windows 下 `sh -c` 不可用 + Job Object 未实现，跨平台不完整~~（v4.1.0 已修复）
- `design-doc.md` 与当前代码存在多处不一致(历史稿)

---

## [3.1.1] — 2026-06-16

详见 [DEVELOPMENT_LOG.md](DEVELOPMENT_LOG.md)。要点：

- 命令执行卡片式展示 + 风险级别颜色区分
- 报告输出 kubectl-ai 风格表格
- 高风险命令恢复 AI 解释
- 安全分析器重构：合并 rules.go，新增 CommandInfo 结构体
- 移除内置 API Key，强制用户配置
- 中风险命令"不再询问"选项
- 环境自动检测(Docker/K8s/Prometheus/Helm)

---

## [3.0.0] — 2026-03-10

详见 [design-doc.md](design-doc.md) §8 优化记录。要点：

- 从单文件结构拆分为 5 包(cmd/llm/platform/report/safety)分层架构
- 5 级风险分级(Safe/Low/Medium/High/Critical)
- 白名单系统(whitelist.txt)
- 后台命令处理(`&` 结尾用 Start())
- Markdown/HTML/PDF 三格式报告
