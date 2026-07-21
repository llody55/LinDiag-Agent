package llm

// degraded_state.go 提供降级状态的快照/恢复机制。
//
// 背景：client.go 中有一个全局变量 degradedFormat，记录 API 格式降级
// 记忆（一旦 API 不支持 json_schema，后续整个会话都不再尝试）。
// 旧实现中 CallAISimple 直接调 CallAI，辅助调用（如高危命令解释）
// 的格式不兼容会永久污染主诊断会话的降级状态。
//
// 现引入 withIsolatedDegradedState：进入时快照主会话状态并临时切换到
// 主会话当前状态（共享起点），defer restore() 时恢复主会话状态。辅助
// 调用过程中底层 callWith* 修改的全局变量会在 restore 时被丢弃。
//
// 当前会话是单线程模型，调用串行化即可；若未来引入并发，应改为
// 基于上下文传递独立 state 变量。

import "sync"

// stateLock 保护 degradedFormat 的快照/恢复临界区。
// 仅在 CallAISimple 等辅助调用路径加锁；主会话 CallAI 不加锁，保持零开销。
var stateLock sync.Mutex

// isolatedStateSave 用于在 defer 中恢复主会话降级状态。
type isolatedStateSave struct {
	saved ResponseFormatType
}

// restore 恢复主会话降级状态到调用前的快照值。
func (s *isolatedStateSave) restore() {
	degradedFormat = s.saved
	stateLock.Unlock()
}

// withIsolatedDegradedState 进入一个辅助调用临界区：
//   - 加锁
//   - 快照当前 degradedFormat
//   - 返回用于 defer restore 的句柄
//
// 调用方必须在 defer 中调用 .restore()。
//
// 在临界区内调用 CallAI 会正常读写 degradedFormat；restore 时这些
// 修改会被丢弃，主会话降级状态保持进入前值。
func withIsolatedDegradedState() *isolatedStateSave {
	stateLock.Lock()
	return &isolatedStateSave{saved: degradedFormat}
}
