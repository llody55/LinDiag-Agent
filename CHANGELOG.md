# Changelog

本文件记录 LinDiag-Agent 各版本的变更。

格式参考 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，版本号遵循
[Semantic Versioning](https://semver.org/lang/zh-CN/)。

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
- Windows 下 `sh -c` 不可用 + Job Object 未实现，跨平台不完整
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
