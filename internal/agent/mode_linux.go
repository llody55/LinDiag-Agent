//go:build linux

package agent

import "fmt"

// serviceStatusCmd 返回获取指定服务状态的命令（Linux 实现）。
// 用 systemctl status 获取服务失败原因。
func serviceStatusCmd(serviceName string) string {
	return fmt.Sprintf("systemctl status %s --no-pager 2>/dev/null | head -n 15", serviceName)
}

// baseSnapshotCmds 所有模式共享的基础快照命令集（Linux 实现）。
// 主题分块：基础资源 → 进程/负载 → 内核日志 → 磁盘详情 → 僵尸进程。
func baseSnapshotCmds(env *EnvInfo) []string {
	cmds := []string{
		// 基础资源：CPU负载/内存/磁盘
		"uptime", "free -h", "df -h",
		// 进程 TOP15：按内存排序
		"ps -eo pid,ppid,%cpu,%mem,rss,cmd --sort=-%mem | head -n 15",
		// top 单次快照
		"top -b -n 1 | head -n 20",
		// 内核环形缓冲区
		"dmesg 2>/dev/null | tail -n 30",
		// 文件系统 inode 使用率
		"df -i 2>/dev/null | head -n 15",
		// 僵尸进程
		"ps -eo stat,pid,cmd 2>/dev/null | awk '$1 ~ /Z/ {print}' | head -n 10",
		// 失败的 systemd 服务
		"systemctl --failed --no-legend 2>/dev/null | head -n 15",
	}
	if env == nil {
		return cmds
	}
	if env.DockerRunning {
		cmds = append(cmds,
			"docker ps -a --format 'table {{.Names}}\\t{{.Status}}\\t{{.Image}}' 2>/dev/null | head -n 20",
			"docker stats --no-stream --format 'table {{.Name}}\\t{{.CPUPerc}}\\t{{.MemUsage}}' 2>/dev/null | head -n 15",
		)
	}
	if env.K8sConnected {
		cmds = append(cmds,
			"kubectl get nodes -o wide 2>/dev/null",
			"kubectl get pods -A --field-selector=status.phase!=Running 2>/dev/null | head -n 30",
		)
	}
	for _, svc := range env.AbnormalServices {
		cmds = append(cmds, serviceStatusCmd(svc))
	}
	return cmds
}

// === 故障诊断模式（Linux）===

type faultDiagnosisMode struct{}

func (m *faultDiagnosisMode) ID() string   { return "fault_diagnosis" }
func (m *faultDiagnosisMode) Name() string { return "故障诊断模式（专业）" }
func (m *faultDiagnosisMode) Description() string {
	return "针对系统故障的深度诊断，至少3轮查询后给出根因和修复步骤"
}
func (m *faultDiagnosisMode) MaxRounds() int { return 10 }

func (m *faultDiagnosisMode) SystemPrompt() string {
	return `你是一个专业、严谨的 Linux SRE 故障诊断专家。你的唯一职责是诊断系统故障，不处理闲聊。

## 核心原则
1. 必须基于系统快照和命令执行结果分析，绝对禁止编造、猜测或估算任何数据
2. 系统快照中已包含 df -h、free -h、uptime 等基础信息，必须引用这些实际输出中的数字
3. 如果快照中没有你需要的信息，生成命令去获取，不要凭空补充
4. 至少执行 3 次有价值查询后才能给出最终结论

## 特别注意 — 数据准确性
- 磁盘使用率必须来自 ` + "`df -h`" + ` 的实际输出 Use% 列，不能自己计算或估算
- ` + "`du`" + ` 统计的是文件实际大小，` + "`df`" + ` 统计的是文件系统已用空间，两者含义不同，不能互相推断
- 不能把各目录 ` + "`du`" + ` 的大小加总后推断磁盘使用率，必须以 ` + "`df`" + ` 的 Use% 列为准
- 内存使用率必须来自 ` + "`free -h`" + ` 的实际输出
- CPU 负载必须来自 ` + "`uptime`" + ` 的实际输出
- swap 文件、保留块等因素会导致 du 和 df 数值不一致，以 df 为准

## 最终结论要求
最终结论中给出：
- **根本原因**（必须引用实际命令输出作为证据）
- **风险影响**
- **详细修复步骤**（维护窗口、先备份、停止服务顺序）
- **预防措施` + outputFormatRule
}

func (m *faultDiagnosisMode) SnapshotCmds(env *EnvInfo) []string {
	cmds := baseSnapshotCmds(env)
	cmds = append(cmds,
		// 内核模块与参数
		"lsmod 2>/dev/null | head -n 30",
		"sysctl -a 2>/dev/null | grep -E '^(net\\.ipv4\\.tcp_|net\\.core\\.somaxconn|vm\\.overcommit_memory|kernel\\.pid_max|fs\\.file-max)' 2>/dev/null | head -n 20",
		// TCP 连接分布
		"ss -tan 2>/dev/null | awk 'NR>1{print $1}' | sort | uniq -c | sort -rn",
		// 路由表
		"ip route show 2>/dev/null | head -n 20",
		// 连接跟踪表规模
		"cat /proc/sys/net/netfilter/nf_conntrack_count 2>/dev/null; cat /proc/sys/net/netfilter/nf_conntrack_max 2>/dev/null",
		// 网卡错误计数
		"ip -s link 2>/dev/null | head -n 40",
	)
	return cmds
}

// === 智能模式（Linux）===

type smartMode struct{}

func (m *smartMode) ID() string   { return "smart" }
func (m *smartMode) Name() string { return "智能模式（通用）" }
func (m *smartMode) Description() string {
	return "通用运维诊断助手，适合日常问题排查和系统信息查询"
}
func (m *smartMode) MaxRounds() int { return 0 }

func (m *smartMode) SystemPrompt() string {
	return `你是一个 Linux 运维诊断助手。你的核心职责是帮助运维人员快速定位和分析系统问题。

## 工作原则
1. 用户查询系统信息时，必须通过命令获取真实数据，绝对禁止编造、猜测或估算任何数据
2. 系统快照中已包含 df -h、free -h、uptime 等基础信息，应优先引用这些已有数据，避免重复执行相同命令
3. 基于执行结果提供简洁、结构化的分析
4. 只回答用户明确请求的信息，不要跑题

## 数据准确性（关键）
- 磁盘使用率必须来自 ` + "`df -h`" + ` 的 Use% 列，不能自己计算或估算
- ` + "`du`" + ` 统计的是文件实际大小，` + "`df`" + ` 统计的是文件系统已用空间，两者含义不同
- 不能把各目录 ` + "`du`" + ` 的大小加总后推断磁盘使用率，必须以 ` + "`df`" + ` 的 Use% 列为准
- 内存使用率必须来自 ` + "`free -h`" + ` 的实际输出
- CPU 负载必须来自 ` + "`uptime`" + ` 或 ` + "`top`" + ` 的实际输出
- 任何百分比、数字都必须有对应的命令输出作为来源

## 命令生成规则
1. 命令中可以使用 2>/dev/null 来抑制错误输出，这是安全的
2. 命令执行失败时，分析失败原因并尝试替代方案
3. 如果某个命令不存在，尝试功能相似的替代命令
4. 对于不同的 Linux 发行版，命令可能不同，根据系统类型选择合适的命令
5. 对于 IP 地址相关信息，必须执行专门的查询命令，不能套用其他 IP 的信息` + outputFormatRule
}

func (m *smartMode) SnapshotCmds(env *EnvInfo) []string {
	return baseSnapshotCmds(env)
}

func init() {
	RegisterMode(&faultDiagnosisMode{})
	RegisterMode(&smartMode{})
}
