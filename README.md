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
- 支持 Docker、Kubernetes、Linux 等多种环境
- 智能分析并提供简洁的建议

## 🚀 快速开始

### 安装

```bash
# 克隆仓库
git clone https://github.com/llody55/LinDiag-Agent.git
cd LinDiag-Agent

# 编译
./build.sh

# 运行
./output/lindiag-agent_amd64_linux
```

### 配置

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

**环境变量配置（优先级更高）：**

```bash
export LINDIAG_LLM_API_URL="https://api.example.com/v1/chat/completions"
export LINDIAG_LLM_API_KEY="your-api-key-here"
export LINDIAG_LLM_MODEL_NAME="your-model-name"
```

## 📖 使用说明

### 启动工具

```bash
./lindiag-agent
```

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

对于中风险命令，系统会提示确认：

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

<br />

## 📁 项目结构

```
LinDiag-Agent/
├── cmd/
│   └── agent/
│       └── main.go          # 主程序入口
├── internal/
│   ├── config/              # 配置管理
│   ├── llm/                 # LLM 客户端
│   ├── output/              # 输出格式化
│   ├── platform/            # 平台工具
│   ├── report/              # 报告生成
│   └── safety/              # 安全检查
├── build.sh                 # 构建脚本
└── README.md                # 项目文档
```

## 🤝 贡献

欢迎提交 Issue 和 Pull Request！

## 📄 许可证

MIT License

## 📧 联系方式

如有问题或建议，请通过以下方式联系：

- GitHub Issues: <https://github.com/llody55/LinDiag-Agent/issues>

