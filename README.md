# LinDiag-Agent

LinDiag-Agent 是一款基于 AI 的跨平台系统诊断与运维助手，支持 Linux、Windows、Kylin、UOS 等多种操作系统，为运维工程师提供智能化的故障诊断和系统分析能力。

## ✨ 功能特性

- **智能故障诊断**：基于大语言模型的专业故障诊断能力
- **跨平台支持**：完美支持 Linux、Windows、Kylin、UOS 等操作系统
- **安全防护**：内置命令风险分析引擎，支持白名单机制
- **多模式工作**：故障诊断专家模式和智能运维助手模式
- **报告生成**：支持 Markdown、HTML、PDF 多种格式报告输出
- **历史记录**：支持对话历史的保存与恢复

## 🏗️ 技术架构

```
┌─────────────────────────────────────────────────────────────────┐
│                    LinDiag-Agent 架构                          │
├─────────────────────────────────────────────────────────────────┤
│  cmd/agent/main.go          # 主入口 - CLI交互与流程控制        │
├─────────────────────────────────────────────────────────────────┤
│  internal/config/           # 配置管理模块                      │
│    └── config.go            # 环境变量/配置文件解析             │
├─────────────────────────────────────────────────────────────────┤
│  internal/llm/              # LLM 客户端模块                    │
│    └── client.go            # AI API 调用、消息管理             │
├─────────────────────────────────────────────────────────────────┤
│  internal/platform/         # 平台抽象层                       │
│    ├── platform.go          # 平台接口定义                     │
│    ├── executor.go          # Linux 命令执行器                 │
│    ├── executor_windows.go  # Windows 命令执行器               │
│    ├── snapshot.go          # 系统快照采集                     │
│    ├── factory.go           # 平台工厂（Linux）                │
│    └── factory_windows.go   # 平台工厂（Windows）              │
├─────────────────────────────────────────────────────────────────┤
│  internal/safety/           # 安全分析模块                     │
│    ├── analyzer.go          # 命令风险分析器                   │
│    └── rules.go             # 危险命令规则与白名单             │
├─────────────────────────────────────────────────────────────────┤
│  internal/report/           # 报告生成模块                     │
│    └── engine.go            # Markdown/HTML/PDF 报告生成       │
└─────────────────────────────────────────────────────────────────┘
```

## 🚀 快速开始

### 环境要求

- Go 1.24+
- 支持 Linux / Windows / Kylin / UOS

### 安装方式

```bash
# 克隆项目
git clone https://github.com/llody55/LinDiag-Agent.git
cd LinDiag-Agent

# 编译项目
go build -o lindiag-agent cmd/agent/main.go

# 运行
./lindiag-agent
```

### 命令行参数

```bash
# 正常启动
./lindiag-agent

# 加载历史记录继续对话
./lindiag-agent load history.json

# 从历史记录生成报告
./lindiag-agent report history.json md
./lindiag-agent report history.json html
./lindiag-agent report history.json pdf
```

## ⚙️ 配置说明

### 配置方式

支持通过环境变量或配置文件进行配置，优先级：**环境变量 > 配置文件 > 默认值**

### 环境变量配置

```bash
# LLM 主模型配置
export LINDIAG_LLM_API_URL="https://dashscope.aliyuncs.com/compatible-mode/v1/chat/completions"
export LINDIAG_LLM_API_KEY="your-api-key"
export LINDIAG_LLM_MODEL_NAME="qwen-flash-character-2026-02-26"

# 安全分析模型配置
export LINDIAG_SAFETY_LLM_API_URL="https://api.siliconflow.cn/v1/chat/completions"
export LINDIAG_SAFETY_LLM_API_KEY="your-api-key"
export LINDIAG_SAFETY_LLM_MODEL_NAME="Qwen/Qwen3-8B"

# 超时配置（单位：秒）
export LINDIAG_TIMEOUT_COMMAND=30
export LINDIAG_TIMEOUT_HTTP=180
```

### 配置文件

配置文件路径：
- Linux/macOS: `~/.config/lindiag/config.json`
- Windows: `%APPDATA%\lindiag\config.json`

```json
{
  "llm": {
    "api_url": "https://dashscope.aliyuncs.com/compatible-mode/v1/chat/completions",
    "api_key": "YOUR_API_KEY_HERE",
    "model_name": "qwen-flash-character-2026-02-26"
  },
  "safety_llm": {
    "api_url": "https://api.siliconflow.cn/v1/chat/completions",
    "api_key": "YOUR_API_KEY_HERE",
    "model_name": "Qwen/Qwen3-8B"
  },
  "timeout": {
    "command_timeout": 30,
    "http_timeout": 180
  }
}
```

## 🎯 使用指南

### 工作模式选择

启动后会提示选择工作模式：

1. **跨平台故障诊断专家 (专业版)**：适用于深度故障排查，支持多轮探测和最终诊断报告生成
2. **智能运维助手 (全平台通用)**：适用于日常运维查询，支持聊天式交互

### 命令执行格式

AI 响应支持两种命令执行格式：

```bash
# EXEC: 格式
EXEC: ls -l /var/log

# 代码块格式
```exec
df -h
```

### 风险等级说明

| 风险等级 | 说明 | 处理方式 |
|---------|------|---------|
| Safe | 安全命令 | 直接执行 |
| Low | 低风险命令 | 提示后执行 |
| Medium | 中风险命令 | 用户确认后执行 |
| High | 高风险命令 | 用户确认后执行 |
| Critical | 严重风险命令 | 用户确认后执行 |

## 📊 报告生成

支持三种报告格式：

```bash
# 生成 Markdown 报告
./lindiag-agent report history.json md

# 生成 HTML 报告
./lindiag-agent report history.json html

# 生成 PDF 报告（需要 wkhtmltopdf）
./lindiag-agent report history.json pdf
```

## 🛡️ 安全特性

### 命令白名单

默认白名单包含安全读取命令：`ls`, `pwd`, `echo`, `cat`, `grep`, `find`, `ps`, `top`, `free`, `df` 等。

可通过 `whitelist.txt` 文件自定义白名单：

```txt
# whitelist.txt - 安全命令白名单
ls
pwd
echo
cat
grep
```

### 危险命令检测

系统会自动检测以下危险操作：
- 文件系统操作：`rm`, `mv`, `chmod`, `chown`
- 系统操作：`reboot`, `shutdown`, `poweroff`, `halt`
- 存储操作：`umount`, `mount`, `fsck`, `dd`
- 配置修改：`sed -i`, `apt-get`, 文件写入 `/etc/`

## 🔧 开发指南

### 项目结构

```
LinDiag-Agent/
├── cmd/
│   └── agent/
│       └── main.go          # 主入口
├── internal/
│   ├── config/              # 配置管理
│   ├── llm/                 # LLM 客户端
│   ├── platform/            # 平台抽象
│   ├── safety/              # 安全分析
│   └── report/              # 报告生成
├── go.mod
├── go.sum
└── README.md
```

### 模块职责

| 模块 | 职责 | 核心功能 |
|-----|------|---------|
| `config` | 配置管理 | 环境变量解析、配置文件读写 |
| `llm` | AI 交互 | API 调用、消息管理、输入清理 |
| `platform` | 平台适配 | 命令执行、系统快照、跨平台支持 |
| `safety` | 安全分析 | 命令风险评估、AI 增强分析 |
| `report` | 报告生成 | 多格式报告输出、模板支持 |

### 跨平台开发

平台抽象层通过 Go 的 build tags 实现跨平台支持：

```go
//go:build !windows
package platform

func NewPlatform() Platform {
    return NewLinuxPlatform()
}
```

```go
//go:build windows
package platform

func NewPlatform() Platform {
    return NewWindowsPlatform()
}
```

## 📝 许可证

MIT License

## 🤝 贡献指南

欢迎提交 Issue 和 Pull Request！

### 贡献流程

1. Fork 本仓库
2. 创建特性分支：`git checkout -b feature/xxx`
3. 提交变更：`git commit -m "feat: xxx"`
4. 推送到分支：`git push origin feature/xxx`
5. 创建 Pull Request

### 代码规范

- 遵循 Go 官方代码风格
- 使用 `go fmt` 格式化代码
- 编写单元测试
- 保持代码简洁清晰

## 📧 联系方式

如有问题或建议，请通过以下方式联系：

- 提交 Issue

---
**注意**: 使用前请确保已正确配置 API Key，请充分在测试环境进行测试，生产环境请慎重运行，并遵守相关服务提供商的使用条款。