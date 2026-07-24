//go:build linux || darwin

package platform

// basicSnapshotCmds 返回采集系统基础信息的命令集（Unix 实现）。
// 包含内核版本、发行版信息、CPU 架构等，全部是 Unix/Linux 命令。
func basicSnapshotCmds() []string {
	return []string{
		"uname -a",
		"cat /etc/os-release 2>/dev/null || cat /etc/redhat-release 2>/dev/null || cat /etc/debian_version 2>/dev/null || echo 'OS info not found'",
		"lsb_release -a 2>/dev/null || echo 'lsb_release not available'",
		"arch",
	}
}
