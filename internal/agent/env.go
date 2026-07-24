package agent

import (
	"fmt"
	"strings"
	"sync"

	"github.com/LinDiag-Agent/internal/output"
)

// envCheckTimeout 单个环境检测命令的超时时间
// 设为 3 秒：正常情况下本地命令瞬间完成，服务不可用时快速跳过
const envCheckTimeout = 3 // 移除 time 包依赖，实际超时值由平台实现使用

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
			if prometheusDeployed() {
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
