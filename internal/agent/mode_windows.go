//go:build windows

package agent

// serviceStatusCmd 返回获取指定服务状态的命令（Windows 实现）。
// 用 Get-Service 获取服务状态和启动类型。
func serviceStatusCmd(serviceName string) string {
	// PowerShell 语法，用单引号包裹服务名避免注入
	return "Get-Service -Name '" + serviceName + "' | Select-Object Name, Status, StartType | Format-List"
}

// baseSnapshotCmds 所有模式共享的基础快照命令集（Windows 实现）。
// 用 PowerShell cmdlet 和 CIM/WMI 查询替代 Linux 命令。
// 主题分块与 Linux 版对齐：基础资源 → 进程/负载 → 系统日志 → 磁盘详情 → 异常服务。
func baseSnapshotCmds(env *EnvInfo) []string {
	cmds := []string{
		// 系统运行时间（替代 uptime）
		"Get-CimInstance Win32_OperatingSystem | Select-Object LastBootUpTime, LocalDateTime | Format-List",
		// 内存使用（替代 free -h）
		"Get-CimInstance Win32_OperatingSystem | Select-Object FreePhysicalMemory, TotalVisibleMemorySize, FreeVirtualMemory, TotalVirtualMemorySize | Format-List",
		// 磁盘卷使用率（替代 df -h）
		"Get-Volume | Where-Object { $_.DriveLetter } | Select-Object DriveLetter, FileSystemLabel, FileSystem, SizeRemaining, Size | Format-Table -AutoSize",
		// 进程 TOP15：按工作集（内存）排序（替代 ps --sort）
		"Get-Process | Sort-Object WS -Descending | Select-Object -First 15 Id, ProcessName, CPU, @{Name='Mem(MB)';Expression={[math]::Round($_.WS/1MB,1)}} | Format-Table -AutoSize",
		// CPU 占用 TOP20 进程
		"Get-Process | Sort-Object CPU -Descending | Select-Object -First 20 Id, ProcessName, CPU | Format-Table -AutoSize",
		// 系统事件日志最近 30 条（替代 dmesg）
		"Get-EventLog -LogName System -Newest 30 | Select-Object TimeGenerated, EntryType, Source, Message | Format-List",
		// 无响应进程（Windows 无僵尸进程概念，检测不响应的进程替代）
		"Get-Process | Where-Object { $_.Responding -eq $false } | Select-Object Id, ProcessName | Format-Table -AutoSize",
		// 自动启动但已停止的服务（替代 systemctl --failed）
		"Get-Service | Where-Object { $_.Status -eq 'Stopped' -and $_.StartType -eq 'Automatic' } | Select-Object Name, DisplayName, Status | Format-Table -AutoSize",
	}
	if env == nil {
		return cmds
	}
	if env.DockerRunning {
		cmds = append(cmds,
			"docker ps -a --format 'table {{.Names}}\\t{{.Status}}\\t{{.Image}}' 2>$null | Select-Object -First 20",
			"docker stats --no-stream --format 'table {{.Name}}\\t{{.CPUPerc}}\\t{{.MemUsage}}' 2>$null | Select-Object -First 15",
		)
	}
	if env.K8sConnected {
		cmds = append(cmds,
			"kubectl get nodes -o wide 2>$null",
			"kubectl get pods -A --field-selector=status.phase!=Running 2>$null | Select-Object -First 30",
		)
	}
	for _, svc := range env.AbnormalServices {
		cmds = append(cmds, serviceStatusCmd(svc))
	}
	return cmds
}

// === 故障诊断模式（Windows）===

type faultDiagnosisMode struct{}

func (m *faultDiagnosisMode) ID() string   { return "fault_diagnosis" }
func (m *faultDiagnosisMode) Name() string { return "故障诊断模式（专业）" }
func (m *faultDiagnosisMode) Description() string {
	return "针对系统故障的深度诊断，至少3轮查询后给出根因和修复步骤"
}
func (m *faultDiagnosisMode) MaxRounds() int { return 10 }

func (m *faultDiagnosisMode) SystemPrompt() string {
	return `你是一个专业的 Windows 系统运维诊断专家。你的唯一职责是诊断系统故障，不处理闲聊。

## 核心原则
1. 必须基于系统快照和命令执行结果分析，绝对禁止编造、猜测或估算任何数据
2. 系统快照中已包含磁盘、内存、进程等基础信息，必须引用这些实际输出中的数字
3. 如果快照中没有你需要的信息，生成命令去获取，不要凭空补充
4. 至少执行 3 次有价值查询后才能给出最终结论

## 命令规范
1. 所有命令必须使用 PowerShell 语法
2. 使用 Get-CimInstance / Get-Process / Get-Volume / Get-EventLog / Get-Service 等 cmdlet
3. 禁止生成 Linux 命令（如 ps、top、df、free、systemctl、cat /proc 等）
4. 可以使用管道和 Select-Object / Where-Object / Sort-Object
5. 命令执行失败时，分析失败原因并尝试替代方案
6. 使用 Format-Table 或 Format-List 格式化输出以提高可读性

## 特别注意 — 数据准确性
- 磁盘使用率必须来自 Get-Volume 的实际输出，通过 SizeRemaining 和 Size 计算
- 内存使用率必须来自 Get-CimInstance Win32_OperatingSystem 的 FreePhysicalMemory 和 TotalVisibleMemorySize
- CPU 占用必须来自 Get-Process 的实际输出
- 任何百分比、数字都必须有对应的命令输出作为来源

## 最终结论要求
最终结论中给出：
- **根本原因**（必须引用实际命令输出作为证据）
- **风险影响**
- **详细修复步骤**（维护窗口、先备份、停止服务顺序）
- **预防措施` + outputFormatRule
}

func (m *faultDiagnosisMode) SnapshotCmds(env *EnvInfo) []string {
	cmds := baseSnapshotCmds(env)
	// 故障诊断模式追加更深的网络/系统状态
	cmds = append(cmds,
		// 已安装驱动列表（替代 lsmod）
		"Get-CimInstance Win32_PnPSignedDriver | Select-Object DeviceName, DriverVersion, DriverProvider | Format-Table -AutoSize | Out-String -Width 4096 | Select-Object -First 30",
		// TCP 连接状态分布（替代 ss -tan）
		"Get-NetTCPConnection | Group-Object State | Select-Object Name, Count | Sort-Object Count -Descending | Format-Table -AutoSize",
		// 路由表（替代 ip route show）
		"Get-NetRoute -DestinationPrefix 0.0.0.0/0 | Select-Object DestinationPrefix, NextHop, RouteMetric, InterfaceAlias | Format-Table -AutoSize",
		// 网卡统计（替代 ip -s link）
		"Get-NetAdapterStatistics | Select-Object Name, ReceivedBytes, ReceivedUnicastPackets, ReceivedDiscardedPackets, SentBytes, SentUnicastPackets, OutboundDiscardedPackets | Format-Table -AutoSize",
		// 磁盘性能计数器（替代 iostat）
		"Get-Counter -Counter '\\PhysicalDisk(*)\\% Disk Time','\\PhysicalDisk(*)\\Avg. Disk Queue Length' -ErrorAction SilentlyContinue | Select-Object -ExpandProperty CounterSamples | Select-Object Path, CookedValue | Format-Table -AutoSize",
	)
	return cmds
}

// === 智能模式（Windows）===

type smartMode struct{}

func (m *smartMode) ID() string   { return "smart" }
func (m *smartMode) Name() string { return "智能模式（通用）" }
func (m *smartMode) Description() string {
	return "通用运维诊断助手，适合日常问题排查和系统信息查询"
}
func (m *smartMode) MaxRounds() int { return 0 }

func (m *smartMode) SystemPrompt() string {
	return `你是一个 Windows 运维诊断助手。你的核心职责是帮助运维人员快速定位和分析系统问题。

## 工作原则
1. 用户查询系统信息时，必须通过命令获取真实数据，绝对禁止编造、猜测或估算任何数据
2. 系统快照中已包含磁盘、内存、进程等基础信息，应优先引用这些已有数据，避免重复执行相同命令
3. 基于执行结果提供简洁、结构化的分析
4. 只回答用户明确请求的信息，不要跑题

## 命令规范
1. 所有命令必须使用 PowerShell 语法
2. 使用 Get-CimInstance / Get-Process / Get-Volume / Get-EventLog / Get-Service 等 cmdlet
3. 禁止生成 Linux 命令（如 ps、top、df、free、systemctl、cat /proc 等）
4. 可以使用管道和 Select-Object / Where-Object / Sort-Object
5. 命令执行失败时，分析失败原因并尝试替代方案

## 数据准确性（关键）
- 磁盘使用率必须来自 Get-Volume 的实际输出，通过 SizeRemaining 和 Size 计算
- 内存使用率必须来自 Get-CimInstance Win32_OperatingSystem 的实际输出
- CPU 占用必须来自 Get-Process 的实际输出
- 任何百分比、数字都必须有对应的命令输出作为来源` + outputFormatRule
}

func (m *smartMode) SnapshotCmds(env *EnvInfo) []string {
	return baseSnapshotCmds(env)
}

func init() {
	RegisterMode(&faultDiagnosisMode{})
	RegisterMode(&smartMode{})
}
