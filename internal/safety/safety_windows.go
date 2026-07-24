//go:build windows

package safety

// safety_windows.go Windows 平台安全分析器扩展。
// 在 init() 中向 safeCommands / dangerousCommands / commandDescriptions 追加
// Windows 专属条目，并重建 safeCommandSet / mediumRiskCommandsMap。
//
// analyzer.go 中的平台无关逻辑（analyzeSingleCommand / riskyPatterns /
// hasWriteRedirect / analyzeCommandExecutor 的 Windows 分支）会自动识别这些条目。
//
// PowerShell cmdlet 何时放白名单：
//   - Get-* 系列只读 cmdlet → Safe（Get-Process / Get-EventLog / Get-Volume 等）
//   - Set-* / Remove-* / Stop-* / Start-* / Restart-* → 变更类， Medium/High/Critical
//   - 不整名放白名单的多义工具（如 Get-Service 的 Stop-Service 同族）走 riskyPatterns
//     正则或 commandExecutors 递归分析。

func init() {
	// 1. 追加 Windows 安全命令到白名单并重建查找集
	for _, c := range winSafeCommands {
		if !safeCommandSet[c] {
			safeCommands = append(safeCommands, c)
			safeCommandSet[c] = true
		}
	}

	// 2. 追加 Windows 危险命令
	for k, v := range winDangerousCommands {
		if _, exists := dangerousCommands[k]; !exists {
			dangerousCommands[k] = v
		}
	}

	// 3. 追加 Windows 中等风险命令映射
	for _, c := range winMediumRiskCommands {
		if !mediumRiskCommandsMap[c] {
			mediumRiskCommandsMap[c] = true
		}
	}

	// 4. 追加 Windows 命令描述
	for k, v := range winCommandDescriptions {
		if _, exists := commandDescriptions[k]; !exists {
			commandDescriptions[k] = v
		}
	}
}

// winSafeCommands Windows 只读安全命令白名单（PowerShell cmdlet + 外部工具）。
// 设计原则：只收录 Get-* 读取系列和纯查询类。
var winSafeCommands = []string{
	// PowerShell 只读 cmdlet（Get-* 系列）
	"Get-Process", "Get-Service", "Get-Volume", "Get-EventLog",
	"Get-CimInstance", "Get-NetTCPConnection", "Get-NetRoute",
	"Get-NetAdapter", "Get-NetAdapterStatistics", "Get-NetIPAddress",
	"Get-Counter", "Get-ComputerInfo", "Get-Command", "Get-Member",
	"Get-Date", "Get-ChildItem", "Get-Content", "Get-Item",
	"Get-ItemProperty", "Get-Variable", "Get-History", "Get-Location",
	"Get-WmiObject", "Get-PSDrive", "Get-PSProvider", "Get-PfxCertificate",
	"Get-HotFix", "Get-ScheduledTask", "Get-LocalUser", "Get-LocalGroup",
	"Get-NetFirewallRule", "Get-Disk", "Get-Partition",
	// PowerShell 别名（常见只读别名）
	"dir", "type", "gc", "gci", "gp", "gl", "gh", "gv",
	// 外部只读工具
	"ipconfig", "systeminfo", "tasklist", "whoami", "hostname",
	"ping", "tracert", "pathping", "arp", "nbtstat", "netstat",
	"route", "print",
}

// winDangerousCommands Windows 高危命令，默认 Critical。
// 这些命令一旦执行错误可能：删除数据 / 关机 / 改账号 / 改磁盘 / 改系统配置。
var winDangerousCommands = map[string]string{
	// === 文件/系统破坏 ===
	"Remove-Item":     "删除文件或目录，错误使用可能导致数据丢失",
	"Remove-ItemProperty": "删除文件属性，可能导致配置丢失",
	"Clear-Content":   "清除文件内容，数据将丢失",
	"Clear-Item":      "清除项内容，数据将丢失",
	// === 系统关机/重启 ===
	"Stop-Computer":   "关闭系统，所有未保存的工作将丢失",
	"Restart-Computer": "重启系统，当前工作可能丢失",
	// === 磁盘/分区管理（改磁盘结构） ===
	"Clear-Disk":      "清除磁盘数据，永久删除所有分区和卷",
	"Initialize-Disk": "初始化磁盘，会清除分区表",
	"Format-Volume":   "格式化卷，会清除所有数据",
	"Resize-Partition": "调整分区大小，操作不当可能导致数据丢失",
	"Remove-Partition": "删除分区，数据将丢失",
	// === 服务/进程变更（影响运行状态） ===
	"Stop-Service":    "停止服务，可能影响系统功能",
	"Stop-Process":    "停止进程，可能导致数据丢失",
	// === 用户/账号管理 ===
	"Remove-LocalUser":  "删除本地用户账号",
	"Remove-LocalGroup": "删除本地用户组",
	"New-LocalUser":     "创建本地用户账号",
	// === 网络配置变更 ===
	"Disable-NetAdapter": "禁用网卡，可能导致网络中断",
	"Enable-NetAdapter":  "启用网卡，改变网络状态",
	"Remove-NetRoute":    "删除路由，可能导致网络中断",
	// === 注册表（系统核心配置） ===
	"Set-ItemProperty":    "修改注册表或文件属性，错误设置可能导致系统故障",
}

// winMediumRiskCommands Windows 有副作用但通常可控的命令。
// Set-* / New-* / Start-* / Restart-* 等变更类但非破坏性的 cmdlet。
var winMediumRiskCommands = []string{
	"Set-Location", "Set-Variable", "Set-Content", "Set-Item",
	"Set-ItemProperty",
	"New-Item", "New-ItemProperty", "New-Variable",
	"Copy-Item", "Move-Item", "Rename-Item",
	"Start-Service", "Restart-Service", "Set-Service",
	"Start-Process",
	"Add-Content", "Add-Member",
	"Out-File", "Tee-Object",
	"Export-Csv", "Export-Clixml", "ConvertTo-Html",
	"Write-Output", "Write-Host",
}

// winCommandDescriptions Windows 命令描述。
var winCommandDescriptions = map[string]string{
	"Get-Process":      "查看进程状态",
	"Get-Service":      "查看服务状态",
	"Get-Volume":       "查看磁盘卷使用情况",
	"Get-EventLog":     "查看系统事件日志",
	"Get-CimInstance":  "查询 CIM/WMI 实例",
	"Get-NetTCPConnection": "查看 TCP 连接状态",
	"Get-NetRoute":     "查看路由表",
	"Get-NetAdapter":   "查看网络适配器",
	"Get-Counter":      "查看性能计数器",
	"Get-ComputerInfo": "查看系统信息",
	"Get-Command":      "查找可用命令",
	"Get-ChildItem":    "列出目录内容",
	"Get-Content":      "查看文件内容",
	"Get-HotFix":       "查看已安装补丁",
	"Remove-Item":      "删除文件或目录",
	"Stop-Computer":    "关闭系统",
	"Restart-Computer": "重启系统",
	"Stop-Service":     "停止服务",
	"Stop-Process":     "停止进程",
	"Format-Volume":    "格式化磁盘卷",
	"Clear-Disk":       "清除磁盘数据",
	"ipconfig":         "查看网络配置",
	"systeminfo":       "查看系统信息",
	"tasklist":         "查看进程列表",
	"whoami":           "查看当前用户",
	"powershell":       "PowerShell 命令行",
	"cmd":              "Windows 命令行",
	"diskpart":         "磁盘分区管理工具",
}
