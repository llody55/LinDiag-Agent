package agent

import (
	"fmt"
	"sort"
	"sync"
)

// DiagnosticMode 是诊断模式的插件化接口。
// 每个模式定义自己的系统提示、快照命令、轮次上限。
// 通过 RegisterMode 可注册第三方模式，无需修改核心代码。
type DiagnosticMode interface {
	// ID 唯一标识，用于历史记录恢复和命令行参数选择
	ID() string
	// Name 显示名称（中文友好）
	Name() string
	// Description 一句话描述模式用途，用于模式选择菜单
	Description() string
	// SystemPrompt 返回该模式的系统提示词（已包含输出格式规则）
	SystemPrompt() string
	// SnapshotCmds 返回初始快照采集命令列表。
	// envInfo 可为 nil（保留扩展点：未来根据环境动态生成命令）。
	SnapshotCmds(envInfo *EnvInfo) []string
	// MaxRounds 返回最大诊断轮次，0 表示无限循环（智能模式）
	MaxRounds() int
}

// EnvInfo 环境信息结构，用于快照命令动态生成。
// 由 DetectEnvInfo() 一次性填充，贯穿模式选择→快照生成→会话初始化。
type EnvInfo struct {
	HasDocker     bool
	DockerRunning bool // 客户端已安装且 docker info 可连通
	HasKubernetes bool
	K8sConnected  bool // kubectl 已安装且 cluster-info 可连通
	HasHelm       bool
	HasNali       bool
	// AbnormalServices 检测到异常的服务列表（已安装但运行异常）
	AbnormalServices []string
}

// === 模式注册表 ===

var (
	modeRegistry = make(map[string]DiagnosticMode)
	registryMu   sync.RWMutex
)

// RegisterMode 注册一个诊断模式。重复 ID 会 panic（启动期错误，及早暴露）。
// 应在 init() 中调用，用于注册内置模式；第三方扩展可在自己的 init 中注册。
func RegisterMode(m DiagnosticMode) {
	registryMu.Lock()
	defer registryMu.Unlock()
	id := m.ID()
	if id == "" {
		panic("模式 ID 不能为空")
	}
	if _, exists := modeRegistry[id]; exists {
		panic(fmt.Sprintf("模式 ID %q 已注册", id))
	}
	modeRegistry[id] = m
}

// GetMode 按 ID 获取模式，不存在返回 nil
func GetMode(id string) DiagnosticMode {
	registryMu.RLock()
	defer registryMu.RUnlock()
	return modeRegistry[id]
}

// ListModes 返回所有已注册模式，按 ID 排序（菜单展示稳定有序）
func ListModes() []DiagnosticMode {
	registryMu.RLock()
	defer registryMu.RUnlock()
	modes := make([]DiagnosticMode, 0, len(modeRegistry))
	for _, m := range modeRegistry {
		modes = append(modes, m)
	}
	sort.Slice(modes, func(i, j int) bool {
		return modes[i].ID() < modes[j].ID()
	})
	return modes
}

// GetModeByMenuIndex 根据菜单序号（1-based）获取模式。
// 用于兼容 main.go 现有的数字选择交互。
func GetModeByMenuIndex(index int) DiagnosticMode {
	modes := ListModes()
	if index < 1 || index > len(modes) {
		if len(modes) > 0 {
			return modes[0]
		}
		return nil
	}
	return modes[index-1]
}
