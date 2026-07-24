//go:build windows

package rules

import (
	"regexp"
	"strings"

	"github.com/LinDiag-Agent/internal/diagnosis"
)

// rules_builtin_windows.go Windows 平台内置阈值规则集。
// 解析 PowerShell cmdlet (Get-CimInstance / Get-Volume / Get-Process /
// Get-EventLog / Get-Service) 的 Format-List 或 Format-Table 输出文本。
// Windows 无 inode / conntrack / 僵尸进程概念，对应 Linux 规则不实现。

// newBuiltinRules 返回 Windows 平台的内置规则集。
func newBuiltinRules() []Rule {
	return []Rule{
		&winMemPressureRule{},
		&winDiskCapacityRule{},
		&winUnresponsiveProcessRule{},
		&winStoppedServicesRule{},
		&winCriticalEventRule{},
	}
}

// === 内存压力（Windows）===
// 解析 Get-CimInstance Win32_OperatingSystem 的 Format-List 输出。
// FreePhysicalMemory / TotalVisibleMemorySize 单位为 KB。
type winMemPressureRule struct{}

func (r *winMemPressureRule) ID() string { return "win_mem_pressure" }

func (r *winMemPressureRule) Match(snapshot string) []diagnosis.Issue {
	out := extractCommandOutput(snapshot, "Get-CimInstance Win32_OperatingSystem | Select-Object FreePhysicalMemory")
	if out == "" {
		return nil
	}
	// Format-List 输出形如:
	// FreePhysicalMemory     : 12345678  (KB)
	// TotalVisibleMemorySize : 16384000  (KB)
	// FreeVirtualMemory      : 2000000   (KB)
	// TotalVirtualMemorySize : 4000000   (KB)
	severity := diagnosis.SeverityInfo
	findings := []string{}

	var freePhys, totalPhys float64
	if m := regexp.MustCompile(`FreePhysicalMemory\s*:\s*(\d+)`).FindStringSubmatch(out); m != nil {
		freePhys = parseFloatSafe(m[1])
	}
	if m := regexp.MustCompile(`TotalVisibleMemorySize\s*:\s*(\d+)`).FindStringSubmatch(out); m != nil {
		totalPhys = parseFloatSafe(m[1])
	}
	if totalPhys > 0 && freePhys >= 0 {
		availRatio := freePhys / totalPhys
		if availRatio < 0.1 {
			if severityRank(diagnosis.SeverityHigh) < severityRank(severity) {
				severity = diagnosis.SeverityHigh
			}
			findings = append(findings, "物理内存可用仅 "+pctStr(availRatio)+
				"（FreePhysicalMemory "+itoa(int(freePhys))+"KB / TotalVisibleMemorySize "+
				itoa(int(totalPhys))+"KB），剩余不足 10%")
		} else if availRatio < 0.2 {
			if severityRank(diagnosis.SeverityMedium) < severityRank(severity) {
				severity = diagnosis.SeverityMedium
			}
			findings = append(findings, "物理内存可用 "+pctStr(availRatio)+
				"（FreePhysicalMemory "+itoa(int(freePhys))+"KB / TotalVisibleMemorySize "+
				itoa(int(totalPhys))+"KB），剩余偏低")
		}
	}

	// 虚拟内存（页面文件）使用率，相当于 Linux Swap
	var freeVirt, totalVirt float64
	if m := regexp.MustCompile(`FreeVirtualMemory\s*:\s*(\d+)`).FindStringSubmatch(out); m != nil {
		freeVirt = parseFloatSafe(m[1])
	}
	if m := regexp.MustCompile(`TotalVirtualMemorySize\s*:\s*(\d+)`).FindStringSubmatch(out); m != nil {
		totalVirt = parseFloatSafe(m[1])
	}
	if totalVirt > 0 && freeVirt >= 0 {
		commitRatio := (totalVirt - freeVirt) / totalVirt
		if commitRatio >= 0.9 {
			if severityRank(diagnosis.SeverityHigh) < severityRank(severity) {
				severity = diagnosis.SeverityHigh
			}
			findings = append(findings, "虚拟内存（页面文件）已使用 "+pctStr(commitRatio)+
				"，系统接近内存耗尽")
		} else if commitRatio >= 0.8 {
			if severityRank(diagnosis.SeverityMedium) < severityRank(severity) {
				severity = diagnosis.SeverityMedium
			}
			findings = append(findings, "虚拟内存（页面文件）已使用 "+pctStr(commitRatio)+
				"，内存压力上升")
		}
	}

	if len(findings) == 0 {
		return nil
	}
	return []diagnosis.Issue{{
		Severity: severity,
		Category: diagnosis.CategoryMemory,
		Title:    "内存压力告警",
		Evidence: "Get-CimInstance Win32_OperatingSystem:\n" + strings.Join(findings, "\n"),
		Suggestion: "1) `Get-Process | Sort-Object WS -Descending | Select-Object -First 15` 找出吃内存进程；" +
			"2) 评估是否需要扩容内存或增大页面文件；3) 检查有无内存泄漏（进程工作集持续增长）",
	}}
}

// === 磁盘容量（Windows）===
// 解析 Get-Volume 的 Format-Table 输出。
// SizeRemaining 和 Size 单位为字节。
type winDiskCapacityRule struct{}

func (r *winDiskCapacityRule) ID() string { return "win_disk_capacity" }

func (r *winDiskCapacityRule) Match(snapshot string) []diagnosis.Issue {
	out := extractCommandOutput(snapshot, "Get-Volume | Where-Object")
	if out == "" {
		return nil
	}
	// Format-Table 输出形如:
	// DriveLetter FileSystemLabel FileSystem SizeRemaining        Size
	// ----------  --------------- ---------- -------------        ----
	// C           System          NTFS      50000000000      100000000000
	// 数据行末尾两列是 SizeRemaining 和 Size（均为字节数）
	re := regexp.MustCompile(`(\d+)\s+(\d+)\s*$`)
	var issues []diagnosis.Issue
	for _, m := range re.FindAllStringSubmatch(out, -1) {
		sizeRemaining := parseFloatSafe(m[1])
		size := parseFloatSafe(m[2])
		if size <= 0 || sizeRemaining < 0 {
			continue
		}
		usedRatio := (size - sizeRemaining) / size
		pct := int(usedRatio*100 + 0.5)
		// 驱动器号无法从正则提取，用 Size 近似标识
		label := "卷(Size=" + itoa(int(size/1024/1024/1024)) + "GB)"
		switch {
		case pct >= 95:
			issues = append(issues, diagnosis.Issue{
				Severity: diagnosis.SeverityCritical,
				Category: diagnosis.CategoryDisk,
				Title:    "磁盘容量严重不足: " + label,
				Evidence: "Get-Volume: " + label + " 使用率 " + itoa(pct) + "% ≥ 95%",
				Suggestion: "1) 立即清理该卷下的日志/缓存/临时文件；" +
					"2) 业务影响风险：写入会失败，需优先处理",
			})
		case pct >= 90:
			issues = append(issues, diagnosis.Issue{
				Severity: diagnosis.SeverityHigh,
				Category: diagnosis.CategoryDisk,
				Title:    "磁盘容量告警: " + label,
				Evidence: "Get-Volume: " + label + " 使用率 " + itoa(pct) + "% ≥ 90%",
				Suggestion: "1) 检查该卷上的大文件和目录占用；2) 清理日志/临时文件",
			})
		}
	}
	return issues
}

// === 无响应进程（Windows 替代僵尸进程检测）===
// 解析 Get-Process | Where-Object { $_.Responding -eq $false } 的 Format-Table 输出。
type winUnresponsiveProcessRule struct{}

func (r *winUnresponsiveProcessRule) ID() string { return "win_unresponsive_proc" }

func (r *winUnresponsiveProcessRule) Match(snapshot string) []diagnosis.Issue {
	out := extractCommandOutput(snapshot, "Get-Process | Where-Object { $_.Responding")
	if out == "" {
		return nil
	}
	// Format-Table 输出形如:
	// Id ProcessName
	// -- -----------
	// 123 notepad
	// 数据行以数字（PID）开头
	re := regexp.MustCompile(`^\d+\s+\S`)
	count := 0
	for _, ln := range strings.Split(out, "\n") {
		if re.MatchString(strings.TrimSpace(ln)) {
			count++
		}
	}
	if count == 0 {
		return nil
	}
	severity := diagnosis.SeverityLow
	if count >= 10 {
		severity = diagnosis.SeverityMedium
	}
	return []diagnosis.Issue{{
		Severity: severity,
		Category: diagnosis.CategoryProcess,
		Title:    "存在无响应进程",
		Evidence: "Get-Process 统计发现 " + itoa(count) + " 个无响应进程（Responding=False）",
		Suggestion: "1) `Get-Process | Where-Object { $_.Responding -eq $false }` 查看具体进程；" +
			"2) 检查是否死锁或资源争抢；3) 必要时重启无响应进程",
	}}
}

// === 已停止的自动启动服务（Windows 替代 systemctl --failed）===
// 解析 Get-Service | Where-Object { $_.Status -eq 'Stopped' -and $_.StartType -eq 'Automatic' }
// 的 Format-Table 输出。
type winStoppedServicesRule struct{}

func (r *winStoppedServicesRule) ID() string { return "win_stopped_services" }

func (r *winStoppedServicesRule) Match(snapshot string) []diagnosis.Issue {
	out := extractCommandOutput(snapshot, "Get-Service | Where-Object { $_.Status -eq 'Stopped'")
	if out == "" {
		return nil
	}
	// Format-Table 输出形如:
	// Name        DisplayName                            Status
	// ----        -----------                            ------
	// Service1    Service 1                             Stopped
	// Status 列（最后一列）为 "Stopped" 的行即为停止的服务
	var stopped []string
	for _, ln := range strings.Split(out, "\n") {
		trimmed := strings.TrimSpace(ln)
		if strings.HasSuffix(trimmed, "Stopped") {
			fields := strings.Fields(trimmed)
			if len(fields) >= 1 {
				stopped = append(stopped, fields[0])
			}
		}
	}
	if len(stopped) == 0 {
		return nil
	}
	severity := diagnosis.SeverityMedium
	if len(stopped) >= 5 {
		severity = diagnosis.SeverityHigh
	}
	return []diagnosis.Issue{{
		Severity: severity,
		Category: diagnosis.CategoryService,
		Title:    "存在已停止的自动启动服务",
		Evidence: "Get-Service 列出 " + itoa(len(stopped)) + " 个自动启动但已停止的服务: " +
			strings.Join(stopped, ", "),
		Suggestion: "1) `Get-Service -Name '<service>' | Format-List *` 查看具体状态；" +
			"2) `Get-WinEvent -FilterHashtable @{LogName='System';ProviderName='Service Control Manager'} -MaxEvents 10` 查日志；" +
			"3) 按依赖顺序修复",
	}}
}

// === 关键系统事件（Windows 替代 dmesg/OOM 检测）===
// 解析 Get-EventLog -LogName System -Newest 30 的 Format-List 输出。
// 统计 EntryType 为 Error 或 Critical 的事件数量。
type winCriticalEventRule struct{}

func (r *winCriticalEventRule) ID() string { return "win_critical_events" }

func (r *winCriticalEventRule) Match(snapshot string) []diagnosis.Issue {
	out := extractCommandOutput(snapshot, "Get-EventLog -LogName System -Newest 30")
	if out == "" {
		return nil
	}
	// Format-List 输出，每个事件条目形如:
	// TimeGenerated : 7/23/2026 10:00:00 AM
	// EntryType     : Error
	// Source        : Service Control Manager
	// Message       : ...
	errRe := regexp.MustCompile(`(?i)EntryType\s*:\s*(Error|Critical)`)
	matches := errRe.FindAllStringSubmatch(out, -1)
	if len(matches) == 0 {
		return nil
	}
	// 取第一个匹配行作为证据
	first := ""
	for _, ln := range strings.Split(out, "\n") {
		if errRe.MatchString(ln) {
			first = strings.TrimSpace(ln)
			break
		}
	}
	severity := diagnosis.SeverityMedium
	if len(matches) >= 10 {
		severity = diagnosis.SeverityHigh
	}
	return []diagnosis.Issue{{
		Severity: severity,
		Category: diagnosis.CategoryKernel,
		Title:    "系统事件日志中存在多个错误/严重事件",
		Evidence: "Get-EventLog 含 " + itoa(len(matches)) + " 条 Error/Critical 事件（最近30条中），首行：\n" + first,
		Suggestion: "1) `Get-EventLog -LogName System -EntryType Error -Newest 20` 查看完整错误事件；" +
			"2) 关注 Source 和 Message 字段定位具体组件；3) 按事件来源逐项排查",
	}}
}
