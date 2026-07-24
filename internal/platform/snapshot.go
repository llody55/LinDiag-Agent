package platform

import (
	"fmt"
	"strings"
	"sync"
)

// GetSnapshot 采集系统快照。
// 复用 executor 的超时与进程组保护，统一执行路径；采集失败时明确标注，
// 避免空输出被 AI 误判为"该指标正常"。
//
// 执行策略：所有命令并行执行（每条独立超时与进程组），但结果按输入顺序
// 拼接——既缩短总耗时（上限为单条最慢命令而非累加），又保持输出可读性。
func GetSnapshot(cmds []string) string {
	var b strings.Builder
	b.WriteString("=== 系统初始快照 ===\n")

	// 先收集最基础的系统信息（平台特定命令）
	basicCmds := basicSnapshotCmds()

	all := make([]string, 0, len(basicCmds)+len(cmds))
	all = append(all, basicCmds...)
	all = append(all, cmds...)

	results := make([]string, len(all))
	var wg sync.WaitGroup
	for i, c := range all {
		wg.Add(1)
		go func(i int, cmd string) {
			defer wg.Done()
			results[i] = runSnapshotCmd(cmd, 10)
		}(i, c)
	}
	wg.Wait()

	for _, r := range results {
		b.WriteString(r)
	}
	return b.String()
}

// runSnapshotCmd 执行单条快照命令并返回格式化结果。
// 失败时输出明确标注的 [采集失败]，而非静默空输出。
// 命令前缀由 snapshotPromptPrefix() 平台函数返回（Linux: "$ ", Windows: "> "）。
func runSnapshotCmd(cmd string, timeoutSeconds int) string {
	out, err := ExecuteCommandWithTimeout(cmd, timeoutSeconds)
	prefix := snapshotPromptPrefix()
	if err != nil {
		return fmt.Sprintf("%s%s\n[采集失败: %v]\n%s\n\n", prefix, cmd, err, out)
	}
	return fmt.Sprintf("%s%s\n%s\n\n", prefix, cmd, out)
}
