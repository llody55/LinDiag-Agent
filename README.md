# LinDiag-Agent

> 多场景通用运维专家 - 基于 AI 的智能系统诊断工具

LinDiag-Agent 是一款基于大语言模型的智能运维诊断工具，支持故障诊断和智能问答两种模式，帮助运维人员快速定位和解决系统问题。

## ✨ 功能特性

### 🎯 故障诊断模式（专业）

- 自动化深度分析系统问题
- 强制遵循安全检查流程
- 生成标准化诊断报告
- 输出格式包括：根本原因、风险影响、修复步骤、预防措施

### 💬 智能模式（通用）

- 自然语言交互，支持多轮对话
- 动态执行系统命令获取真实数据
- 支持 Docker、Kubernetes、Linux、Windows 等多种环境
- 智能分析并提供简洁的建议

## 🚀 快速开始

### 安装

#### Linux

```bash
# 克隆仓库
git clone https://github.com/llody55/LinDiag-Agent.git
cd LinDiag-Agent

# 编译（同时构建 Linux amd64/arm64 与 Windows 版本）
./build.sh

# 运行
./output/lindiag-agent_amd64_linux      # x86_64
./output/lindiag-agent_arm64_linux      # arm64
```

#### Windows

> 自 v4.2.0 起，LinDiag-Agent 提供 Windows 平台原生支持。
> Windows 端通过 PowerShell 执行诊断命令（`powershell -NoProfile -NonInteractive -Command`），
> 内置 5 条 Windows 规则、Windows 安全分析器与路径规范。

**方式一：使用预编译二进制**

```powershell
# 从 Release 页面下载 lindiag-agent.exe，放入任意目录后直接运行
.\lindiag-agent.exe
```

**方式二：从源码编译**

```powershell
# 需要先安装 Go 1.21+ 与 PowerShell 5.1+
git clone https://github.com/llody55/LinDiag-Agent.git
cd LinDiag-Agent

# 本地直接构建（适用于当前平台）
go build -o lindiag-agent.exe cmd\agent\main.go

# 或在 Linux 上交叉编译 Windows 版本
# CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o output/lindiag-agent.exe cmd/agent/main.go
```

> 说明：`build.sh` 已包含 Windows 交叉编译目标，在 Linux 上执行 `./build.sh` 即可同时产出
> `output/lindiag-agent.exe`。

### 配置

配置文件路径因平台而异：

| 平台       | 配置目录                                   | 数据目录（历史/报告）                          |
| ---------- | ------------------------------------------ | ---------------------------------------------- |
| Linux      | `~/.config/lindiag/`（`$XDG_CONFIG_HOME`） | `~/.local/share/lindiag/`（`$XDG_DATA_HOME`） |
| Windows    | `%APPDATA%\lindiag\`                       | `%LOCALAPPDATA%\lindiag\`                      |

#### Linux 配置

创建配置文件 `~/.config/lindiag/config.json`：

```json
{
  "llm": {
    "api_url": "https://api.example.com/v1/chat/completions",
    "api_key": "your-api-key-here",
    "model_name": "your-model-name"
  },
  "command": {
    "timeout_seconds": 60
  }
}
```

#### Windows 配置

创建配置文件 `%APPDATA%\lindiag\config.json`（通常展开为
`C:\Users\<用户名>\AppData\Roaming\lindiag\config.json`）：

```json
{
  "llm": {
    "api_url": "https://api.example.com/v1/chat/completions",
    "api_key": "your-api-key-here",
    "model_name": "your-model-name"
  },
  "command": {
    "timeout_seconds": 60
  }
}
```

**环境变量配置（优先级更高）：**

Linux（bash/zsh）：

```bash
export LINDIAG_LLM_API_URL="https://api.example.com/v1/chat/completions"
export LINDIAG_LLM_API_KEY="your-api-key-here"
export LINDIAG_LLM_MODEL_NAME="your-model-name"
```

Windows（PowerShell）：

```powershell
$env:LINDIAG_LLM_API_URL = "https://api.example.com/v1/chat/completions"
$env:LINDIAG_LLM_API_KEY = "your-api-key-here"
$env:LINDIAG_LLM_MODEL_NAME = "your-model-name"
```

## 📖 使用说明

### 启动工具

Linux：

```bash
./lindiag-agent
```

Windows（PowerShell）：

```powershell
.\lindiag-agent.exe
```

> 建议在 PowerShell 中运行（而非 `cmd.exe`），以获得最佳终端渲染效果。
> 若终端不支持 UTF-8，工具会自动降级为 ASCII 字符显示（表格边框为 `+-|`，图标用文本替代）。

### 选择工作模式

```
请选择工作模式：
1. 故障诊断模式（专业）
2. 智能模式（通用）

请输入数字 (1-2): 
```

### 输入问题

```
请输入现象描述/日志（输入 ok 结束，多行输入）:
> 帮我分析一下根目录的磁盘占用怎么这么高
> ok
```

### 命令确认

对于中风险命令，系统会提示确认（Linux 示例）：

```
┌─────────────────────────────────────────────────────────────
│ ⚠️ 我需要执行这个命令，但需要您的确认
├─────────────────────────────────────────────────────────────
│ 命令: du -sh /* 2>/dev/null | sort -rh | head -20
│ 说明: 查看目录大小
│ 风险: 命令包含相对路径
├─────────────────────────────────────────────────────────────
│ 选项: 1. Yes  2. Yes, and don't ask me again  3. No
└─────────────────────────────────────────────────────────────
Enter your choice: 
```

Windows 示例（PowerShell 诊断命令同样会经过安全分析器校验）：

```
┌─────────────────────────────────────────────────────────────
│ ⚠️ 我需要执行这个命令，但需要您的确认
├─────────────────────────────────────────────────────────────
│ 命令: Get-Volume | Where-Object { $_.DriveLetter } | Format-Table -AutoSize
│ 说明: 查看磁盘卷使用情况
│ 风险: 信息查询命令
├─────────────────────────────────────────────────────────────
│ 选项: 1. Yes  2. Yes, and don't ask me again  3. No
└─────────────────────────────────────────────────────────────
Enter your choice: 
```

### 平台命令差异

两个平台执行相同语义的诊断任务，但底层命令不同：

| 诊断目标     | Linux 命令                              | Windows 命令（PowerShell）                                              |
| ------------ | --------------------------------------- | ---------------------------------------------------------------------- |
| 系统运行时间 | `uptime`                                | `Get-CimInstance Win32_OperatingSystem \| Select LastBootUpTime`        |
| 内存使用     | `free -h`                               | `Get-CimInstance Win32_OperatingSystem \| Select FreePhysicalMemory`   |
| 磁盘使用     | `df -h`                                 | `Get-Volume \| Where { $_.DriveLetter }`                               |
| 进程列表     | `ps -eo pid,user,%cpu,%mem,cmd --sort`  | `Get-Process \| Sort WS -Desc \| Select -First 15`                     |
| 内核日志     | `dmesg -T`                              | `Get-EventLog -LogName System -Newest 30`                              |
| 失败服务     | `systemctl --failed`                    | `Get-Service \| Where { Status=Stopped -and StartType=Automatic }`    |
| 僵尸进程     | `ps -eo stat,pid,cmd \| grep '^Z'`      | `Get-Process \| Where { $_.Responding -eq $false }`                    |

## ⚙️ 配置参数

| 参数                        | 类型     | 默认值 | 说明          |
| ------------------------- | ------ | --- | ----------- |
| `llm.api_url`             | string | 无   | LLM API 地址  |
| `llm.api_key`             | string | 无   | API Key     |
| `llm.model_name`          | string | 无   | 模型名称        |
| `command.timeout_seconds` | int    | 30  | 命令执行超时时间（秒） |

## 🛡️ 安全特性

- **命令白名单**：只允许执行安全的命令
- **风险评估**：自动评估命令风险等级（安全/低/中/高/严重）
- **用户确认**：中高风险命令需要用户确认后才能执行
- **操作记录**：记录所有执行的命令和结果

## 📊 支持的环境检测

- ✅ Docker 环境
- ✅ Kubernetes 客户端
- ✅ K8s 集群连接状态
- ✅ Prometheus 部署检测
- ✅ Helm 客户端
- ✅ nali IP归属地查询工具

## 💻 平台支持

| 平台       | 架构        | 二进制名称                      | Shell     | 状态              |
| ---------- | ----------- | ------------------------------- | --------- | ----------------- |
| Linux      | amd64       | `lindiag-agent_amd64_linux`     | `sh -c`   | v4.0 起完整可用   |
| Linux      | arm64       | `lindiag-agent_arm64_linux`     | `sh -c`   | v4.0 起完整可用   |
| Windows    | amd64       | `lindiag-agent.exe`             | PowerShell| v4.2.0 起完整可用 |

> **Windows 依赖**：需 Windows 7+ 与 PowerShell 5.1+（Windows 10/11 默认自带）。
> Windows 端通过 `Get-CimInstance` / `Get-Process` / `Get-Volume` 等 cmdlet 实现
> 与 Linux 等价的诊断能力，规则引擎与安全分析器均已适配 PowerShell 命令。

<br />

## 📁 项目结构

```
LinDiag-Agent/
├── cmd/
│   └── agent/
│       └── main.go              # 主程序入口
├── internal/
│   ├── agent/                   # 主编排器：Session / Handler / Mode / Env / Context
│   │   ├── mode.go              # 模式接口与通用常量
│   │   ├── mode_linux.go        # Linux 诊断模式（故障诊断 + 智能模式）
│   │   ├── mode_windows.go      # Windows 诊断模式（PowerShell 命令集）
│   │   ├── env.go               # 环境探测接口
│   │   ├── env_unix.go          # Linux 环境探测（Docker/K8s/Helm）
│   │   └── env_windows.go       # Windows 环境探测
│   ├── config/                  # 配置管理与用户偏好持久化
│   ├── diagnosis/               # 跨层数据契约（Message / Issue / Response）
│   ├── llm/                     # LLM 客户端（三级降级链 + 重试）
│   ├── output/                  # 输出格式化（Markdown / 表格 / ASCII 降级）
│   │   ├── terminal_env.go      # 终端环境检测
│   │   ├── terminal_unix.go     # Unix 终端模式
│   │   ├── terminal_windows.go  # Windows 终端模式
│   │   └── ascii_fallback.go    # 非 UTF-8 终端 ASCII 降级
│   ├── paths/                   # XDG / Windows Known Folder 路径管理
│   │   ├── paths_unix.go        # Linux：~/.config, ~/.local/share
│   │   └── paths_windows.go     # Windows：%APPDATA%, %LOCALAPPDATA%
│   ├── platform/                # 平台工具（Shell / 执行器 / 快照）
│   │   ├── shell_unix.go        # sh -c 实现
│   │   ├── shell_windows.go     # powershell 实现
│   │   ├── snapshot_basics_unix.go
│   │   ├── snapshot_basics_windows.go
│   │   ├── process_unix.go      # setpgid / killpg
│   │   └── process_windows.go   # CREATE_NEW_PROCESS_GROUP / taskkill
│   ├── report/                  # 报告生成（Markdown / HTML / PDF）
│   ├── rules/                   # 本地阈值规则引擎
│   │   ├── rules_builtin.go     # 公共工具函数
│   │   ├── rules_builtin_linux.go    # 8 条 Linux 规则
│   │   └── rules_builtin_windows.go  # 5 条 Windows 规则
│   └── safety/                  # 安全护栏（风险分级 / 白名单）
│       ├── analyzer.go          # 通用分析逻辑
│       └── safety_windows.go    # Windows cmdlet 分级
├── build.sh                     # 跨平台构建脚本
├── CHANGELOG.md                 # 版本变更记录
├── ROADMAP.md                   # 待办路线图
└── README.md                    # 项目文档
```

## 🤝 贡献

欢迎提交 Issue 和 Pull Request！

## 📄 许可证

MIT License

## 📧 联系方式

如有问题或建议，请通过以下方式联系：

- GitHub Issues: <https://github.com/llody55/LinDiag-Agent/issues>

