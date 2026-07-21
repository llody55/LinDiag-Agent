package agent

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/LinDiag-Agent/internal/output"
)

// envCheckTimeout 单个环境检测命令的超时时间
// 设为 3 秒：正常情况下本地命令瞬间完成，服务不可用时快速跳过
const envCheckTimeout = 3 * time.Second

// DetectEnvironment 检测当前运行环境（Docker/K8s/Helm 等）。
// 每个检测命令都有独立超时，服务挂掉时不会阻塞。
// 同时区分"已安装"和"可连接"两个状态，让用户知道服务是否正常。
//
// 历史接口：返回 []string 用于 TUI 展示。新代码应使用 DetectEnvInfo() 获取
// 结构化信息；本函数基于 EnvInfo 渲染，保持外向行为不变。
func DetectEnvironment() []string {
	info := DetectEnvInfo()
	envList := renderEnvInfo(info)
	if len(info.AbnormalServices) > 0 {
		output.WarningMessage(fmt.Sprintf("检测到 %d 个服务异常: %s（建议优先诊断）",
			len(info.AbnormalServices), strings.Join(info.AbnormalServices, ", ")))
	}
	return envList
}

// DetectEnvInfo 并行检测环境，返回结构化 EnvInfo。
// 这是智能快照采集的数据源：模式可通过 SnapshotCmds(envInfo) 据此动态生成命令。
// 所有检测并行执行，总耗时上限为 envCheckTimeout（3s）而非串行累加。
func DetectEnvInfo() *EnvInfo {
	var info EnvInfo
	var abnormalMu sync.Mutex

	type probe struct {
		name string
		fn   func(*EnvInfo, *sync.Mutex)
	}
	probes := []probe{
		{"docker", probeDocker},
		{"k8s", probeKubernetes},
		{"helm", probeHelmNoAbn},
		{"nali", probeNaliNoAbn},
	}

	done := make(chan struct{}, len(probes))
	for _, p := range probes {
		go func(p probe) {
			p.fn(&info, &abnormalMu)
			done <- struct{}{}
		}(p)
	}
	for i := 0; i < len(probes); i++ {
		<-done
	}
	return &info
}

// renderEnvInfo 将 EnvInfo 渲染为人可读的字符串列表（保持旧接口兼容）
func renderEnvInfo(info *EnvInfo) []string {
	var envList []string

	if info.HasDocker {
		envList = append(envList, "Docker 客户端已安装")
		if info.DockerRunning {
			envList = append(envList, "Docker 服务运行中")
		} else {
			envList = append(envList, "⚠️ Docker 服务不可用（可能已挂起或停止）")
		}
	}

	if info.HasKubernetes {
		envList = append(envList, "Kubernetes 客户端已安装")
		if info.K8sConnected {
			envList = append(envList, "K8s 集群已连接")
			if out := runOutput("kubectl get ns 2>/dev/null | grep -i prometheus | head -1 | awk '{print $1}'"); strings.TrimSpace(out) != "" {
				envList = append(envList, "Prometheus 已部署")
			}
		} else {
			envList = append(envList, "⚠️ K8s 集群不可连接（API Server 可能已挂起或停止）")
		}
	}

	if info.HasHelm {
		envList = append(envList, "Helm 客户端已安装")
	}

	if info.HasNali {
		envList = append(envList, "nali IP归属地查询工具")
	}

	return envList
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

// checkCmd 运行命令，成功（exit 0）返回 true。
// 带 3 秒超时，服务挂起时不会阻塞。
func checkCmd(cmd string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), envCheckTimeout)
	defer cancel()
	err := exec.CommandContext(ctx, "sh", "-c", cmd).Run()
	return err == nil
}

// runOutput 运行命令并返回输出。带 3 秒超时。
func runOutput(cmd string) string {
	ctx, cancel := context.WithTimeout(context.Background(), envCheckTimeout)
	defer cancel()
	out, _ := exec.CommandContext(ctx, "sh", "-c", cmd).CombinedOutput()
	return string(out)
}
