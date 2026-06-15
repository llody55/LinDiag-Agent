package platform

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/LinDiag-Agent/internal/config"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/net"
)

// LinuxPlatform Linux平台实现
type LinuxPlatform struct{}

// NewLinuxPlatform 创建Linux平台实例
func NewLinuxPlatform() *LinuxPlatform {
	return &LinuxPlatform{}
}

// ExecuteCommand 执行命令并获取详细结果
func (p *LinuxPlatform) ExecuteCommand(cmd string) (string, error) {
	// 检查是否是后台运行的命令（以 & 结尾）
	if strings.HasSuffix(strings.TrimSpace(cmd), "&") {
		// 对于后台运行的命令，使用 Start() 而不是 CombinedOutput()
		c := exec.Command("sh", "-c", cmd)
		// 设置完整的环境变量
		c.Env = os.Environ()
		err := c.Start()
		if err != nil {
			return "", err
		}
		// 立即返回，不等待命令完成
		return "命令已在后台启动", nil
	}

	// 获取命令执行超时时间（秒）
	timeout := 30 // 默认值
	if cfg, err := config.LoadConfig(); err == nil {
		timeout = cfg.Timeout.CommandTimeout
	}

	// 对于前台命令，添加超时机制
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer cancel()

	c := exec.CommandContext(ctx, "sh", "-c", cmd)
	// 设置完整的环境变量
	c.Env = os.Environ()

	// 使用通道来处理命令执行
	var out []byte
	var err error
	done := make(chan struct{})

	go func() {
		out, err = c.CombinedOutput()
		close(done)
	}()

	// 等待命令完成或超时
	select {
	case <-done:
		// 命令正常完成
		return string(out), err
	case <-ctx.Done():
		// 命令超时
		// 确保命令被终止
		if c.Process != nil {
			c.Process.Kill()
		}
		return string(out) + fmt.Sprintf("\n[超时] 命令执行超过%d秒，已自动终止", timeout), ctx.Err()
	}
}

// GetHostname 获取主机名
func (p *LinuxPlatform) GetHostname() string {
	if hostInfo, err := host.Info(); err == nil {
		return hostInfo.Hostname
	}
	return "unknown"
}

// GetIPAddress 获取IP地址
func (p *LinuxPlatform) GetIPAddress() string {
	if netInfo, err := net.Interfaces(); err == nil {
		for _, iface := range netInfo {
			if iface.Name != "lo" {
				for _, addr := range iface.Addrs {
					// 提取IPv4地址
					if strings.Contains(addr.String(), ".") {
						return strings.Split(addr.String(), "/")[0]
					}
				}
			}
		}
	}
	return "unknown"
}

// GetSystemLanguage 获取系统默认语言
func (p *LinuxPlatform) GetSystemLanguage() string {
	// 首先尝试从环境变量获取
	if lang := os.Getenv("LANG"); lang != "" {
		return lang
	}
	if lang := os.Getenv("LC_ALL"); lang != "" {
		return lang
	}
	if lang := os.Getenv("LC_MESSAGES"); lang != "" {
		return lang
	}

	// 尝试使用gopsutil获取
	if hostInfo, err := host.Info(); err == nil {
		return hostInfo.OS
	}

	return "unknown"
}

// IsDangerousCommand 检查命令是否危险
func (p *LinuxPlatform) IsDangerousCommand(cmd string) bool {
	// 危险命令列表
	dangerousCommands := []string{
		// 文件系统操作
		"rm -rf",
		"format",
		"mkfs",
		"dd if=",
		"mv /",
		"cp /",

		// 系统操作
		"reboot",
		"shutdown",
		"poweroff",
		"init 0",
		"init 6",

		// 网络操作
		"iptables -F",
		"iptables -X",
		"ifconfig down",
		"ip link set down",

		// 权限操作
		"chmod 777",
		"chown root:",
		"sudo",
		"su -",

		// 进程操作
		"kill -9",
		"pkill",
		"killall",

		// 其他危险操作
		"rm -f",
		"rmdir",
		"unlink",
		"ln -sf",
		"dd bs=",
		"cat > /etc/passwd",
		"cat > /etc/shadow",
	}

	// 转换为小写进行比较
	cmdLower := strings.ToLower(cmd)

	// 检查命令是否包含危险操作
	for _, dangerousCmd := range dangerousCommands {
		if strings.Contains(cmdLower, strings.ToLower(dangerousCmd)) {
			return true
		}
	}

	return false
}

// GetCommandDescription 获取命令的详细说明
func (p *LinuxPlatform) GetCommandDescription(cmd string) string {
	// 命令描述映射
	cmdDescriptions := map[string]string{
		// 文件系统操作
		"rm -rf": "递归强制删除文件和目录，无法恢复",
		"format": "格式化磁盘，会删除所有数据",
		"mkfs":   "创建文件系统，会删除磁盘上的所有数据",
		"dd if=": "低级磁盘操作，可能导致数据丢失",

		// 系统操作
		"reboot":   "重启系统",
		"shutdown": "关闭系统",
		"poweroff": "关闭电源",
		"init 0":   "关机",
		"init 6":   "重启",

		// 网络操作
		"iptables -F":      "清空防火墙规则，降低系统安全性",
		"iptables -X":      "删除用户定义的防火墙链",
		"ifconfig down":    "禁用网络接口，可能导致网络连接中断",
		"ip link set down": "禁用网络接口，可能导致网络连接中断",

		// 权限操作
		"chmod 777":   "设置所有用户可读写执行权限，存在安全风险",
		"chown root:": "修改文件所有者为root，可能导致权限混乱",
		"sudo":        "以超级用户身份执行命令，可能执行危险操作",
		"su -":        "切换到root用户，可能执行危险操作",

		// 进程操作
		"kill -9": "强制终止进程，可能导致数据丢失",
		"pkill":   "终止匹配的进程，可能影响系统稳定性",
		"killall": "终止所有匹配的进程，可能影响系统稳定性",

		// 其他危险操作
		"rm -f":             "强制删除文件，无法恢复",
		"cat > /etc/passwd": "修改密码文件，可能导致系统无法登录",
		"cat > /etc/shadow": "修改影子密码文件，可能导致系统无法登录",
	}

	// 转换为小写进行比较
	cmdLower := strings.ToLower(cmd)

	// 查找命令描述
	for cmdPattern, description := range cmdDescriptions {
		if strings.Contains(cmdLower, strings.ToLower(cmdPattern)) {
			return description
		}
	}

	return "该命令可能执行危险操作，建议谨慎使用"
}
