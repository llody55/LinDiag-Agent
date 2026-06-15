package platform

import (
	"fmt"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/net"
)

// SystemInfo 系统信息结构体
type SystemInfo struct {
	Hostname        string
	OS              string
	Platform        string
	PlatformVersion string
	CPU             string
	Memory          string
	Disk            string
	Network         string
}

// GetSystemInfo 获取系统信息
func GetSystemInfo() *SystemInfo {
	info := &SystemInfo{}

	// 获取主机信息
	if hostInfo, err := host.Info(); err == nil {
		info.Hostname = hostInfo.Hostname
		info.OS = hostInfo.OS
		info.Platform = hostInfo.Platform
		info.PlatformVersion = hostInfo.PlatformVersion
	}

	// 获取CPU信息
	if cpuInfo, err := cpu.Info(); err == nil && len(cpuInfo) > 0 {
		info.CPU = fmt.Sprintf("%s %d cores", cpuInfo[0].ModelName, cpuInfo[0].Cores)
	}

	// 获取内存信息
	if memInfo, err := mem.VirtualMemory(); err == nil {
		info.Memory = fmt.Sprintf("Total: %.2f GB, Used: %.2f GB, Free: %.2f GB",
			float64(memInfo.Total)/1024/1024/1024,
			float64(memInfo.Used)/1024/1024/1024,
			float64(memInfo.Available)/1024/1024/1024)
	}

	// 获取磁盘信息
	if diskInfo, err := disk.Usage("/"); err == nil {
		info.Disk = fmt.Sprintf("Total: %.2f GB, Used: %.2f GB, Free: %.2f GB",
			float64(diskInfo.Total)/1024/1024/1024,
			float64(diskInfo.Used)/1024/1024/1024,
			float64(diskInfo.Free)/1024/1024/1024)
	}

	// 获取网络信息
	if netInfo, err := net.Interfaces(); err == nil {
		var networks []string
		for _, iface := range netInfo {
			if iface.Name != "lo" {
				for _, addr := range iface.Addrs {
					networks = append(networks, fmt.Sprintf("%s: %s", iface.Name, addr.String()))
				}
			}
		}
		info.Network = fmt.Sprintf("%v", networks)
	}

	return info
}
