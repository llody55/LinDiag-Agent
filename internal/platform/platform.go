package platform

// Platform 定义平台相关操作的接口
type Platform interface {
	// ExecuteCommand 执行命令并获取详细结果
	ExecuteCommand(cmd string) (string, error)

	// GetHostname 获取主机名
	GetHostname() string

	// GetIPAddress 获取IP地址
	GetIPAddress() string

	// GetSystemLanguage 获取系统默认语言
	GetSystemLanguage() string

	// IsDangerousCommand 检查命令是否危险
	IsDangerousCommand(cmd string) bool

	// GetSnapshot 采集系统快照
	GetSnapshot(cmds []string) string
}
