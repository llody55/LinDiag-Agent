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

// WindowsPlatform Windows平台实现
type WindowsPlatform struct{}

// NewWindowsPlatform 创建Windows平台实例
func NewWindowsPlatform() *WindowsPlatform {
	return &WindowsPlatform{}
}

// ExecuteCommand 执行命令并获取详细结果
func (p *WindowsPlatform) ExecuteCommand(cmd string) (string, error) {
	// 处理PowerShell命令
	if strings.HasPrefix(strings.TrimSpace(cmd), "Get-") || strings.Contains(cmd, "Select-Object") || strings.Contains(cmd, "Where-Object") {
		// 使用PowerShell执行命令
		c := exec.Command("powershell", "-Command", "chcp 65001 | Out-Null; "+cmd)
		// 设置编码相关环境变量
		c.Env = append(os.Environ(), "PYTHONIOENCODING=utf-8")
		out, err := c.CombinedOutput()
		return string(out), err
	}

	// 处理Linux shell命令（sh -c）
	processedCmd := cmd
	if strings.HasPrefix(strings.TrimSpace(cmd), "sh -c ") {
		// 提取sh -c后面的命令
		innerCmd := strings.TrimSpace(strings.TrimPrefix(cmd, "sh -c "))
		// 移除引号
		innerCmd = strings.Trim(innerCmd, `"'`)
		processedCmd = innerCmd
	}

	// 转换Linux命令为Windows等效命令
	windowsCmd := p.convertLinuxToWindowsCmd(processedCmd)

	// 检查是否是后台运行的命令（以 & 结尾）
	if strings.HasSuffix(strings.TrimSpace(windowsCmd), "&") {
		// 对于后台运行的命令，使用 Start() 而不是 CombinedOutput()
		c := exec.Command("cmd", "/c", "chcp 65001 >nul && "+windowsCmd)
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

	// 构建命令，先设置编码为UTF-8
	execCmd := "cmd"
	execArgs := []string{"/c", "chcp 65001 >nul && " + windowsCmd}

	c := exec.CommandContext(ctx, execCmd, execArgs...)

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

// convertLinuxToWindowsCmd 将Linux命令转换为Windows等效命令
func (p *WindowsPlatform) convertLinuxToWindowsCmd(cmd string) string {
	// 基本命令映射
	cmdMap := map[string]string{
		// IP 相关命令
		"ip addr show": "ipconfig",
		"hostname -I":  "ipconfig | findstr IPv4",
		"ifconfig":     "ipconfig",

		// 系统信息命令
		"uname -a": "ver",
		"uptime":   "systeminfo | findstr /B /C:\"System Boot Time\"",
		"free -h":  "wmic OS get FreePhysicalMemory /value",
		"df -h":    "wmic logicaldisk get size,freespace,caption",
		"ps aux":   "tasklist",
		"top":      "tasklist | sort /+60",

		// 文件系统命令
		"ls -la": "dir /a",
		"pwd":    "cd",
		"cd":     "cd",
		"mkdir":  "mkdir",
		"rm":     "del",
		"cp":     "copy",
		"mv":     "move",
	}

	// 检查是否有直接映射
	if windowsCmd, ok := cmdMap[cmd]; ok {
		return windowsCmd
	}

	// 处理管道命令
	if strings.Contains(cmd, "|") {
		// 简单处理管道命令，转换为Windows的管道
		parts := strings.Split(cmd, "|")
		for i, part := range parts {
			trimmed := strings.TrimSpace(part)
			// 转换常见的Linux命令
			if strings.HasPrefix(trimmed, "grep") {
				// 转换grep为findstr
				grepArgs := strings.TrimSpace(strings.TrimPrefix(trimmed, "grep"))
				parts[i] = "findstr " + grepArgs
			} else if strings.HasPrefix(trimmed, "head") {
				// 转换head为head命令（Windows PowerShell或Git Bash可能有）
				parts[i] = trimmed
			} else if mappedCmd, ok := cmdMap[trimmed]; ok {
				parts[i] = mappedCmd
			}
		}
		return strings.Join(parts, " | ")
	}

	// 如果没有映射，直接返回原命令
	return cmd
}

// GetHostname 获取主机名
func (p *WindowsPlatform) GetHostname() string {
	if hostInfo, err := host.Info(); err == nil {
		return hostInfo.Hostname
	}
	return "unknown"
}

// GetIPAddress 获取IP地址
func (p *WindowsPlatform) GetIPAddress() string {
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
func (p *WindowsPlatform) GetSystemLanguage() string {
	// 使用PowerShell命令获取系统语言
	c := exec.Command("powershell", "-Command", "Get-WinSystemLocale | Select-Object -ExpandProperty Name")
	c.Env = append(os.Environ(), "CHCP=65001")
	out, err := c.CombinedOutput()
	if err == nil {
		language := strings.TrimSpace(string(out))
		if language != "" {
			return language
		}
	}

	// 尝试从环境变量获取
	if lang := os.Getenv("LANG"); lang != "" {
		return lang
	}

	// 尝试使用gopsutil获取
	if hostInfo, err := host.Info(); err == nil {
		return hostInfo.OS
	}

	return "unknown"
}

// IsDangerousCommand 检查命令是否危险
func (p *WindowsPlatform) IsDangerousCommand(cmd string) bool {
	// 危险命令列表
	dangerousCommands := []string{
		// 文件系统操作
		"format ",
		"fdisk ",
		"diskpart",
		"del /f /s /q",
		"rmdir /s /q",
		"erase /f /s /q",
		"xcopy /h /e /k /o",
		"robocopy /mir",

		// 系统操作
		"shutdown ",
		"restart-computer",
		"stop-computer",
		"init 0",
		"init 6",

		// 网络操作
		"netsh advfirewall set allprofiles state off",
		"ipconfig /release",
		"ipconfig /renew",
		"route delete",
		"net stop",

		// 权限操作
		"takeown /f",
		"icacls /grant",
		"icacls /deny",
		"runas ",
		"sudo",

		// 进程操作
		"taskkill /f /im",
		"taskkill /f /t",
		"wmic process where",

		// 注册表操作
		"reg delete",
		"reg add",
		"reg import",
		"reg export",
		"reg load",
		"reg save",
		"reg restore",
		"reg query",
		"reg copy",
		"reg compare",
		"reg flush",
		"reg hive",
		"reg import",

		// 其他危险操作
		"bcdedit /set",
		"bootcfg",
		"sysedit",
		"msconfig",
		"sfc /scannow",
		"chkdsk /f",
		"defrag /f",
		"disk cleanup",
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
func (p *WindowsPlatform) GetCommandDescription(cmd string) string {
	// 命令描述映射
	cmdDescriptions := map[string]string{
		// 注册表操作
		"reg delete":  "删除注册表项，可能导致系统功能异常",
		"reg add":     "添加或修改注册表项，可能导致系统功能异常",
		"reg import":  "导入注册表文件，可能覆盖系统设置",
		"reg export":  "导出注册表文件，可能泄露系统配置",
		"reg load":    "加载注册表配置单元，可能影响系统稳定性",
		"reg save":    "保存注册表配置单元，可能泄露系统配置",
		"reg restore": "恢复注册表配置，可能导致系统故障",

		// 文件系统操作
		"format ":      "格式化磁盘，会删除所有数据",
		"diskpart":     "磁盘分区工具，可能导致数据丢失",
		"del /f /s /q": "强制删除文件，无法恢复",

		// 系统操作
		"shutdown ":        "关闭或重启系统",
		"restart-computer": "重启计算机",
		"stop-computer":    "关闭计算机",

		// 网络操作
		"netsh advfirewall set allprofiles state off": "关闭防火墙，降低系统安全性",
		"ipconfig /release":                           "释放IP地址，可能导致网络连接中断",

		// 权限操作
		"takeown /f":    "获取文件所有权，可能导致权限混乱",
		"icacls /grant": "修改文件权限，可能导致安全问题",
		"runas ":        "以其他用户身份运行程序，可能提升权限",

		// 进程操作
		"taskkill /f /im": "强制终止进程，可能导致程序异常",
		"taskkill /f /t":  "强制终止进程及其子进程，可能导致系统不稳定",
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
