package rules

import (
	"strings"
	"testing"

	"github.com/LinDiag-Agent/internal/diagnosis"
)

// TestEngine_Run_BuiltinRules 覆盖内置规则集对一份含多项异常的快照的命中行为。
// 这不是覆盖率刷分测试：它锁定的契约是"规则引擎在没有 LLM 的情况下也能给出
// SRE 关心的基本诊断结论"。后续修改阈值或新增规则时，本测试保证不回退。
func TestEngine_Run_BuiltinRules(t *testing.T) {
	snap := strings.Join([]string{
		"=== 系统初始快照 ===",
		"$ uptime",
		" 10:00:00 up 30 days, load average: 5.20, 4.10, 2.80",
		"",
		"$ free -h",
		"              total        used        free      shared  buff/cache   available",
		"Mem:          7.7Gi       6.2Gi       134Mi      120Mi   1.3Gi        200Mi",
		"Swap:         2.0Gi       1.9Gi       100Mi",
		"",
		"$ df -h",
		"Filesystem      Size  Used Avail Use% Mounted on",
		"/dev/sda1       50G   48G   2G   96% /",
		"/dev/sdb1       100G  95G  5G   95% /data",
		"",
		"$ df -i",
		"Filesystem     Inodes IUsed IFree IUse% Mounted on",
		"/dev/sda1      1000000 950000 50000 95% /",
		"",
		"$ dmesg | tail -n 30",
		"[123] Out of memory: Killed process 1234 (java)",
		"[124] invoked oom-killer",
		"",
		"$ ps -eo stat,pid,cmd",
		"STAT PID CMD",
		"S    1   init",
		"Z    123 defunct",
		"Z    124 defunct",
		"",
		"$ systemctl --failed --no-legend",
		"foo.service loaded failed failed Some fail",
		"bar.service loaded failed failed Another",
		"",
		// conntrack 实际命令是单条 shell 用 ; 分隔两个 cat，输出两行
		"$ cat /proc/sys/net/netfilter/nf_conntrack_count 2>/dev/null; cat /proc/sys/net/netfilter/nf_conntrack_max 2>/dev/null",
		"1950000",
		"2000000",
		"",
	}, "\n")

	eng := NewEngine()
	issues := eng.Run(snap)
	if len(issues) == 0 {
		t.Fatalf("期望命中多条规则，实际 0 条")
	}

	wantTitles := map[string]bool{
		"系统1分钟平均负载过高": true,
		"内存压力告警":      true,
		"磁盘容量严重不足: /": true,
		// /data 95% 同样触发 Critical（≥95%），不是 High 级别的"告警"
		"磁盘容量严重不足: /data":      true,
		"inode 容量告警: /":        true,
		"存在僵尸进程":               true,
		"内核近期触发过 OOM Killer":   true,
		"连接跟踪表(conntrack)使用率高": true,
		"存在失败的 systemd 服务":     true,
	}
	got := make(map[string]bool, len(issues))
	for _, is := range issues {
		got[is.Title] = true
	}
	for want := range wantTitles {
		if !got[want] {
			t.Errorf("缺失预期 Issue: %q\n实际命中: %v", want, got)
		}
	}

	// 严重度排序契约：第一条不能比后一条更不紧急
	for i := 0; i < len(issues)-1; i++ {
		if diagnosis.SeverityRank(issues[i].Severity) >
			diagnosis.SeverityRank(issues[i+1].Severity) {
			t.Errorf("Issue[%d]=%s 比 Issue[%d]=%s 更不紧急，排序错误",
				i, issues[i].Severity, i+1, issues[i+1].Severity)
		}
	}
}

// TestEngine_Run_HealthySnapshot 验证健康快照不产生误报。
// 这是规则引擎保守性的核心契约：宁可漏报不可误报（避免噪声 Issue 让 LLM 分心）。
func TestEngine_Run_HealthySnapshot(t *testing.T) {
	snap := strings.Join([]string{
		"=== 系统初始快照 ===",
		"$ uptime",
		" 10:00:00 up 30 days, load average: 0.50, 0.40, 0.30",
		"",
		"$ free -h",
		"              total        used        free      shared  buff/cache   available",
		"Mem:          7.7Gi       2.0Gi       4.5Gi      120Mi   1.2Gi        5.4Gi",
		"Swap:         2.0Gi       0B          2.0Gi",
		"",
		"$ df -h",
		"Filesystem      Size  Used Avail Use% Mounted on",
		"/dev/sda1       50G   20G   30G   40% /",
		"",
		"$ df -i",
		"Filesystem     Inodes IUsed IFree IUse% Mounted on",
		"/dev/sda1      1000000 50000 950000 5% /",
		"",
		"$ dmesg | tail -n 30",
		"[100] device eth0 link up",
		"",
		"$ ps -eo stat,pid,cmd",
		"STAT PID CMD",
		"S    1   init",
		"S    2   kthreadd",
		"",
	}, "\n")

	eng := NewEngine()
	issues := eng.Run(snap)
	if len(issues) != 0 {
		t.Errorf("健康快照不应产生 Issue，实际命中 %d 条:", len(issues))
		for _, is := range issues {
			t.Logf("  - [%s] %s", is.Severity, is.Title)
		}
	}
}
