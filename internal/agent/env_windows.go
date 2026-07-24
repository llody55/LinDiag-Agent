//go:build windows

package agent

import (
	"context"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// envCheckDuration 将 envCheckTimeout（秒）转为 time.Duration。
func envCheckDuration() time.Duration {
	return time.Duration(envCheckTimeout) * time.Second
}

// probeDocker 检测 Docker 安装与运行状态（Windows 实现）。
// Windows 下用 Get-Command 替代 which，用 docker info 判断服务状态。
func probeDocker(info *EnvInfo, mu *sync.Mutex) {
	if !checkCmd("Get-Command docker -ErrorAction SilentlyContinue") {
		return
	}
	info.HasDocker = true
	// docker info 在 Windows 上行为一致（Docker Desktop / Docker Engine）
	if checkCmd("docker info") {
		info.DockerRunning = true
	} else {
		mu.Lock()
		info.AbnormalServices = append(info.AbnormalServices, "Docker")
		mu.Unlock()
	}
}

// probeKubernetes 检测 K8s 客户端与集群连通性（Windows 实现）。
func probeKubernetes(info *EnvInfo, mu *sync.Mutex) {
	if !checkCmd("Get-Command kubectl -ErrorAction SilentlyContinue") {
		return
	}
	info.HasKubernetes = true
	if checkCmd("kubectl cluster-info") {
		info.K8sConnected = true
	} else {
		mu.Lock()
		info.AbnormalServices = append(info.AbnormalServices, "Kubernetes")
		mu.Unlock()
	}
}

// probeHelmNoAbn 检测 Helm CLI 是否安装（Windows 实现）。
func probeHelmNoAbn(info *EnvInfo, _ *sync.Mutex) {
	if checkCmd("Get-Command helm -ErrorAction SilentlyContinue") {
		info.HasHelm = true
	}
}

// probeNaliNoAbn 检测 nali IP 归属地工具是否安装（Windows 实现）。
// nali 在 Windows 上大概率未安装，检测保持同构。
func probeNaliNoAbn(info *EnvInfo, _ *sync.Mutex) {
	if checkCmd("Get-Command nali -ErrorAction SilentlyContinue") {
		info.HasNali = true
	}
}

// checkCmd 运行命令，成功（exit 0）返回 true。带超时。
// Windows 下用 PowerShell 执行。
func checkCmd(cmd string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), envCheckDuration())
	defer cancel()
	err := exec.CommandContext(ctx, "powershell", "-NoProfile", "-NonInteractive", "-Command", cmd).Run()
	return err == nil
}

// runOutput 运行命令并返回输出。带超时。
func runOutput(cmd string) string {
	ctx, cancel := context.WithTimeout(context.Background(), envCheckDuration())
	defer cancel()
	out, _ := exec.CommandContext(ctx, "powershell", "-NoProfile", "-NonInteractive", "-Command", cmd).CombinedOutput()
	return string(out)
}

// prometheusDeployed 检测 K8s 集群中是否部署了 Prometheus（Windows 实现）。
func prometheusDeployed() bool {
	out := runOutput("kubectl get ns | Select-String -Pattern 'prometheus' -Quiet")
	return strings.TrimSpace(out) == "True"
}
