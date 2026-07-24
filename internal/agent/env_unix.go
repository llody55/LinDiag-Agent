//go:build linux || darwin

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

// probeDocker 检测 Docker 安装与运行状态
func probeDocker(info *EnvInfo, mu *sync.Mutex) {
	if !checkCmd("which docker >/dev/null 2>&1") {
		return
	}
	info.HasDocker = true
	if checkCmd("docker info >/dev/null 2>&1") {
		info.DockerRunning = true
	} else {
		mu.Lock()
		info.AbnormalServices = append(info.AbnormalServices, "Docker")
		mu.Unlock()
	}
}

// probeKubernetes 检测 K8s 客户端与集群连通性
func probeKubernetes(info *EnvInfo, mu *sync.Mutex) {
	if !checkCmd("kubectl version --client >/dev/null 2>&1") {
		return
	}
	info.HasKubernetes = true
	if checkCmd("kubectl cluster-info >/dev/null 2>&1") {
		info.K8sConnected = true
	} else {
		mu.Lock()
		info.AbnormalServices = append(info.AbnormalServices, "Kubernetes")
		mu.Unlock()
	}
}

// probeHelmNoAbn 检测 Helm CLI 是否安装（不涉及异常状态）
func probeHelmNoAbn(info *EnvInfo, _ *sync.Mutex) {
	if checkCmd("which helm >/dev/null 2>&1") {
		info.HasHelm = true
	}
}

// probeNaliNoAbn 检测 nali IP 归属地工具是否安装（不涉及异常状态）
func probeNaliNoAbn(info *EnvInfo, _ *sync.Mutex) {
	if checkCmd("which nali >/dev/null 2>&1") {
		info.HasNali = true
	}
}

// checkCmd 运行命令，成功（exit 0）返回 true。带超时。
func checkCmd(cmd string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), envCheckDuration())
	defer cancel()
	err := exec.CommandContext(ctx, "sh", "-c", cmd).Run()
	return err == nil
}

// runOutput 运行命令并返回输出。带超时。
func runOutput(cmd string) string {
	ctx, cancel := context.WithTimeout(context.Background(), envCheckDuration())
	defer cancel()
	out, _ := exec.CommandContext(ctx, "sh", "-c", cmd).CombinedOutput()
	return string(out)
}

// prometheusDeployed 检测 K8s 集群中是否部署了 Prometheus（Unix 实现）。
func prometheusDeployed() bool {
	out := runOutput("kubectl get ns 2>/dev/null | grep -i prometheus | head -1 | awk '{print $1}'")
	return strings.TrimSpace(out) != ""
}
