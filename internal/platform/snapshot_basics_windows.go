//go:build windows

package platform

// basicSnapshotCmds 返回采集系统基础信息的命令集（Windows 实现）。
// 用 PowerShell cmdlet 和 CIM/WMI 查询替代 Unix 命令。
func basicSnapshotCmds() []string {
	return []string{
		// 操作系统版本、构建号、架构
		"Get-CimInstance Win32_OperatingSystem | Select-Object Caption, Version, BuildNumber, OSArchitecture | Format-List",
		// 计算机名称、域、制造商
		"Get-CimInstance Win32_ComputerSystem | Select-Object Name, Manufacturer, Model, TotalPhysicalMemory | Format-List",
		// PowerShell 版本信息
		"$PSVersionTable | Format-List",
		// CPU 信息
		"Get-CimInstance Win32_Processor | Select-Object Name, NumberOfCores, NumberOfLogicalProcessors, MaxClockSpeed | Format-List",
		// 系统运行时长（替代 uptime）+ 本地时间，供报告引擎提取 LastBootUpTime
		"Get-CimInstance Win32_OperatingSystem | Select-Object LastBootUpTime, LocalDateTime | Format-List",
	}
}
