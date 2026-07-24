//go:build linux

package rules

import (
	"regexp"
	"strings"

	"github.com/LinDiag-Agent/internal/diagnosis"
)

// rules_builtin_linux.go Linux 平台内置阈值规则集。
// 每条规则只读快照文本，不执行命令；命中阈值时产出 diagnosis.Issue。

// newBuiltinRules 返回 Linux 平台的内置规则集。
func newBuiltinRules() []Rule {
	return []Rule{
		&loadAvgRule{},
		&memPressureRule{},
		&diskCapacityRule{},
		&diskInodeRule{},
		&zombieProcessRule{},
		&oomKernelRule{},
		&conntrackRule{},
		&failedServicesRule{},
	}
}

// === 负载过高 ===
type loadAvgRule struct{}

func (r *loadAvgRule) ID() string { return "loadavg" }

func (r *loadAvgRule) Match(snapshot string) []diagnosis.Issue {
	out := extractCommandOutput(snapshot, "uptime")
	if out == "" {
		return nil
	}
	// uptime 输出形如: ... load average: 1.52, 1.10, 0.80
	re := regexp.MustCompile(`load average:\s*([\d.]+),\s*([\d.]+),\s*([\d.]+)`)
	m := re.FindStringSubmatch(out)
	if m == nil {
		return nil
	}
	load1 := parseFloatSafe(m[1])
	// 规则只对 1 分钟负载 > 4 触发（保守阈值，避免开发机偶发峰值误报）
	if load1 <= 4 {
		return nil
	}
	return []diagnosis.Issue{{
		Severity: diagnosis.SeverityHigh,
		Category: diagnosis.CategoryCPU,
		Title:    "系统1分钟平均负载过高",
		Evidence: "uptime: load average " + m[1] + ", " + m[2] + ", " + m[3] +
			"（1分钟负载 " + m[1] + " > 4）",
		Suggestion: "1) `top`/`htop` 查看高 CPU 进程；2) 检查是否死循环或业务流量突增；" +
			"3) 确认 CPU 核数（`nproc`）；负载/核数 > 1 即过载",
	}}
}

// === 内存压力 ===
type memPressureRule struct{}

func (r *memPressureRule) ID() string { return "mem_pressure" }

func (r *memPressureRule) Match(snapshot string) []diagnosis.Issue {
	out := extractCommandOutput(snapshot, "free -h")
	if out == "" {
		return nil
	}
	// free -h 输出（典型）:
	//               total   used   free   shared  buff/cache  available
	// Mem:          7.7Gi   5.2Gi  234Mi  120Mi   2.3Gi       1.9Gi
	// Swap:         2.0Gi   1.8Gi  200Mi
	// 我们关心 Mem available 与 Swap used
	severity := diagnosis.SeverityInfo
	findings := []string{}

	// Swap 使用率：如果 used 接近 total，说明内存已经吃紧开始换页
	swapLineRe := regexp.MustCompile(`Swap:\s+(\S+)\s+(\S+)\s+(\S+)`)
	if m := swapLineRe.FindStringSubmatch(out); m != nil {
		swapTotal, swapUsed := parseHumanSize(m[1]), parseHumanSize(m[2])
		if swapTotal > 0 && swapUsed > 0 {
			ratio := swapUsed / swapTotal
			if ratio >= 0.9 {
				severity = diagnosis.SeverityHigh
				findings = append(findings, "Swap 已使用 "+pctStr(ratio)+
					"（"+m[2]+" / "+m[1]+"），系统处于频繁换页状态")
			} else if ratio >= 0.5 {
				if severityRank(diagnosis.SeverityMedium) < severityRank(severity) {
					severity = diagnosis.SeverityMedium
				}
				findings = append(findings, "Swap 已使用 "+pctStr(ratio)+
					"（"+m[2]+" / "+m[1]+"），内存压力上升")
			}
		}
	}

	// available < 10% ---- 更精确的 low-memory 指标
	memLineRe := regexp.MustCompile(`Mem:\s+(\S+)\s+\S+\s+\S+\s+\S+\s+\S+\s+(\S+)`)
	if m := memLineRe.FindStringSubmatch(out); m != nil {
		total, avail := parseHumanSize(m[1]), parseHumanSize(m[2])
		if total > 0 && avail > 0 {
			ratio := avail / total
			if ratio < 0.1 {
				if severityRank(diagnosis.SeverityHigh) < severityRank(severity) {
					severity = diagnosis.SeverityHigh
				}
				findings = append(findings, "Mem available 仅 "+pctStr(ratio)+
					"（"+m[2]+" / "+m[1]+"），剩余可用内存不足 10%")
			} else if ratio < 0.2 {
				if severityRank(diagnosis.SeverityMedium) < severityRank(severity) {
					severity = diagnosis.SeverityMedium
				}
				findings = append(findings, "Mem available "+pctStr(ratio)+
					"（"+m[2]+" / "+m[1]+"），剩余可用内存偏低")
			}
		}
	}

	if len(findings) == 0 {
		return nil
	}
	return []diagnosis.Issue{{
		Severity: severity,
		Category: diagnosis.CategoryMemory,
		Title:    "内存压力告警",
		Evidence: "free -h:\n" + strings.Join(findings, "\n"),
		Suggestion: "1) `ps --sort=-%mem | head` 找出吃内存进程；" +
			"2) 评估是否需要扩容或配置 Swap；3) 检查有无内存泄漏（进程 RSS 持续增长）",
	}}
}

// === 磁盘容量 ===
type diskCapacityRule struct{}

func (r *diskCapacityRule) ID() string { return "disk_capacity" }

func (r *diskCapacityRule) Match(snapshot string) []diagnosis.Issue {
	out := extractCommandOutput(snapshot, "df -h")
	if out == "" {
		return nil
	}
	// df -h 输出: Filesystem Size Used Avail Use% Mounted
	//                                  ^ 有 Use% 列
	re := regexp.MustCompile(`\s(\d+)%\s+(/\S*)`)
	var issues []diagnosis.Issue
	for _, m := range re.FindAllStringSubmatch(out, -1) {
		pct, ok := parsePercent(m[1] + "%")
		if !ok {
			continue
		}
		mount := m[2]
		switch {
		case pct >= 95:
			issues = append(issues, diagnosis.Issue{
				Severity: diagnosis.SeverityCritical,
				Category: diagnosis.CategoryDisk,
				Title:    "磁盘容量严重不足: " + mount,
				Evidence: "df -h: " + mount + " 使用率 " + m[1] + "% ≥ 95%",
				Suggestion: "1) 立即清理 " + mount + " 下的日志/缓存（`du -sh " + mount + "/*`）；" +
					"2) 业务影响风险：写入会失败，需优先处理",
			})
		case pct >= 90:
			issues = append(issues, diagnosis.Issue{
				Severity:   diagnosis.SeverityHigh,
				Category:   diagnosis.CategoryDisk,
				Title:      "磁盘容量告警: " + mount,
				Evidence:   "df -h: " + mount + " 使用率 " + m[1] + "% ≥ 90%",
				Suggestion: "1) `du -sh " + mount + "/*` 查看空间占用分布；2) 清理日志/临时文件",
			})
		}
	}
	return issues
}

// === 磁盘 inode ===
type diskInodeRule struct{}

func (r *diskInodeRule) ID() string { return "disk_inode" }

func (r *diskInodeRule) Match(snapshot string) []diagnosis.Issue {
	out := extractCommandOutput(snapshot, "df -i")
	if out == "" {
		return nil
	}
	re := regexp.MustCompile(`\s(\d+)%\s+(/\S*)`)
	var issues []diagnosis.Issue
	for _, m := range re.FindAllStringSubmatch(out, -1) {
		pct, ok := parsePercent(m[1] + "%")
		if !ok || pct < 90 {
			continue
		}
		mount := m[2]
		severity := diagnosis.SeverityHigh
		if pct >= 95 {
			severity = diagnosis.SeverityCritical
		}
		issues = append(issues, diagnosis.Issue{
			Severity: severity,
			Category: diagnosis.CategoryDisk,
			Title:    "inode 容量告警: " + mount,
			Evidence: "df -i: " + mount + " inode 使用率 " + m[1] + "% ≥ 90%",
			Suggestion: "inode 耗尽通常由海量小文件导致。`find " + mount +
				" -xdev -type f | wc -l` 看分布；清理大量小文件所在目录",
		})
	}
	return issues
}

// === 僵尸进程 ===
type zombieProcessRule struct{}

func (r *zombieProcessRule) ID() string { return "zombie" }

func (r *zombieProcessRule) Match(snapshot string) []diagnosis.Issue {
	out := extractCommandOutput(snapshot, "ps -eo stat")
	if out == "" {
		// 回退尝试另一种 ps 命令格式
		out = extractCommandOutput(snapshot, "ps -eo pid,ppid")
	}
	if out == "" {
		return nil
	}
	lines := strings.Split(out, "\n")
	zombieCount := 0
	for _, ln := range lines {
		trimmed := strings.TrimSpace(ln)
		if trimmed == "" {
			continue
		}
		// stat 列首字符 Z 表示僵尸；格式 "Z  1234 cmd"
		if strings.HasPrefix(trimmed, "Z") {
			zombieCount++
		}
	}
	if zombieCount == 0 {
		return nil
	}
	severity := diagnosis.SeverityLow
	if zombieCount >= 10 {
		severity = diagnosis.SeverityMedium
	}
	return []diagnosis.Issue{{
		Severity: severity,
		Category: diagnosis.CategoryProcess,
		Title:    "存在僵尸进程",
		Evidence: "ps 统计发现 " + itoa(zombieCount) + " 个僵尸进程（状态为 Z）",
		Suggestion: "1) 找到僵尸进程的父进程：`ps -o ppid= -p <zombie_pid>`；" +
			"2) 若父进程已知不回收，可发送 SIGCHLD 或重启父进程；" +
			"3) 少量僵尸进程不耗资源，仅大量堆积需关注",
	}}
}

// === 内核 OOM ===
type oomKernelRule struct{}

func (r *oomKernelRule) ID() string { return "oom_kernel" }

func (r *oomKernelRule) Match(snapshot string) []diagnosis.Issue {
	out := extractCommandOutput(snapshot, "dmesg")
	if out == "" {
		return nil
	}
	// 常见 OOM 标志：内核会打印 "Out of memory" 或 "Killed process"
	oomRe := regexp.MustCompile(`(?i)(out of memory|killed process|invoked oom-killer)`)
	matches := oomRe.FindAllString(out, -1)
	if len(matches) == 0 {
		return nil
	}
	// 取第一行命中作为证据
	first := ""
	for _, ln := range strings.Split(out, "\n") {
		if oomRe.MatchString(ln) {
			first = strings.TrimSpace(ln)
			break
		}
	}
	severity := diagnosis.SeverityHigh
	if len(matches) >= 3 {
		severity = diagnosis.SeverityCritical // 多次 OOM 说明还没解决，业务已受影响
	}
	return []diagnosis.Issue{{
		Severity: severity,
		Category: diagnosis.CategoryMemory,
		Title:    "内核近期触发过 OOM Killer",
		Evidence: "dmesg 含 " + itoa(len(matches)) + " 处 OOM 相关日志，首行：\n" + first,
		Suggestion: "1) `dmesg | grep -i oom` 查看完整 OOM 上下文与被杀进程；" +
			"2) 通常说明物理内存+Swap 不足；3) 扩容内存或限制业务内存使用",
	}}
}

// === conntrack 满 ===
type conntrackRule struct{}

func (r *conntrackRule) ID() string { return "conntrack" }

func (r *conntrackRule) Match(snapshot string) []diagnosis.Issue {
	// baseSnapshotCmds 用 cat /proc/sys/net/netfilter/nf_conntrack_count 与 max
	out := extractCommandOutput(snapshot, "cat /proc/sys/net/netfilter/nf_conntrack_count")
	if out == "" {
		return nil
	}
	lines := strings.Split(out, "\n")
	var count, maxv float64
	for _, ln := range lines {
		ln = strings.TrimSpace(ln)
		if ln == "" {
			continue
		}
		n, ok := firstNumber(ln)
		if !ok {
			continue
		}
		if count == 0 {
			count = n
		} else {
			maxv = n
		}
	}
	if maxv <= 0 || count <= 0 {
		return nil
	}
	ratio := count / maxv
	if ratio < 0.8 {
		return nil
	}
	severity := diagnosis.SeverityMedium
	if ratio >= 0.95 {
		severity = diagnosis.SeverityHigh
	}
	return []diagnosis.Issue{{
		Severity: severity,
		Category: diagnosis.CategoryNetwork,
		Title:    "连接跟踪表(conntrack)使用率高",
		Evidence: "nf_conntrack_count=" + ftoa(count) + " / max=" + ftoa(maxv) +
			"，使用率 " + pctStr(ratio),
		Suggestion: "1) `dmesg | grep conntrack` 看是否已经丢包告警；" +
			"2) 提高 `nf_conntrack_max`（注意内存开销，每条 ~300B）；" +
			"3) 业务侧减少短连接，多用长连接复用",
	}}
}

// === 失败的 systemd 服务 ===
type failedServicesRule struct{}

func (r *failedServicesRule) ID() string { return "failed_services" }

func (r *failedServicesRule) Match(snapshot string) []diagnosis.Issue {
	out := extractCommandOutput(snapshot, "systemctl --failed")
	if out == "" {
		return nil
	}
	// 输出形如:
	// UNIT          LOAD   ACTIVE SUB    DESCRIPTION
	// foo.service   loaded failed failed some desc
	// 0 loaded units listed.
	lines := strings.Split(out, "\n")
	var failed []string
	for _, ln := range lines {
		ln = strings.TrimSpace(ln)
		if ln == "" || strings.HasPrefix(ln, "UNIT") || strings.HasSuffix(ln, "loaded units listed.") {
			continue
		}
		// 取第一字段作为 unit 名
		fields := strings.Fields(ln)
		if len(fields) >= 1 {
			failed = append(failed, fields[0])
		}
	}
	if len(failed) == 0 {
		return nil
	}
	severity := diagnosis.SeverityMedium
	if len(failed) >= 5 {
		severity = diagnosis.SeverityHigh
	}
	return []diagnosis.Issue{{
		Severity: severity,
		Category: diagnosis.CategoryService,
		Title:    "存在失败的 systemd 服务",
		Evidence: "systemctl --failed 列出 " + itoa(len(failed)) + " 个失败单元: " +
			strings.Join(failed, ", "),
		Suggestion: "1) `systemctl status <unit>` 查看具体失败原因；" +
			"2) `journalctl -u <unit> -n 50` 查日志；3) 按依赖顺序修复",
	}}
}
