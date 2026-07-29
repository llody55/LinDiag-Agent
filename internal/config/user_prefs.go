package config

import (
	"encoding/json"
	"os"
	"sync"

	"github.com/LinDiag-Agent/internal/paths"
)

// UserPreferences 用户可调的行为偏好，与主配置分离以便独立写入。
//
// 存放位置（Phase 3 Task 9 改造）：$XDG_CONFIG_HOME/lindiag/user_prefs.json
// （旧版相对 CWD 写入 user_prefs.json，会随启动目录漂移导致偏好丢失）。
type UserPreferences struct {
	AutoConfirmLowRisk    bool `json:"auto_confirm_low_risk"`
	AutoConfirmMediumRisk bool `json:"auto_confirm_medium_risk"`
}

var userPrefs = &UserPreferences{}

// prefsMu 保护 userPrefs 的并发读写
var prefsMu sync.RWMutex

// LoadUserPreferences 从 paths 包指定的位置加载用户偏好。
// 找不到文件不视为错误（首次启动 / 升级场景）。
func LoadUserPreferences() {
	data, err := os.ReadFile(paths.UserPrefsFile())
	if err != nil {
		return
	}
	prefsMu.Lock()
	defer prefsMu.Unlock()
	_ = json.Unmarshal(data, userPrefs)
}

// SaveUserPreferences 持久化偏好到 paths 包指定的位置。
// 写入前确保目录存在；写入失败被忽略以保持与旧版行为一致
// （偏好丢失不应阻塞当前命令执行流程）。
func SaveUserPreferences() {
	if err := paths.EnsureConfigDir(); err != nil {
		return
	}
	prefsMu.RLock()
	data, _ := json.Marshal(userPrefs)
	prefsMu.RUnlock()
	_ = os.WriteFile(paths.UserPrefsFile(), data, 0644)
}

func GetUserPreferences() *UserPreferences {
	prefsMu.RLock()
	defer prefsMu.RUnlock()
	return userPrefs
}

func SetAutoConfirmLowRisk(enabled bool) {
	prefsMu.Lock()
	userPrefs.AutoConfirmLowRisk = enabled
	prefsMu.Unlock()
	SaveUserPreferences()
}

func SetAutoConfirmMediumRisk(enabled bool) {
	prefsMu.Lock()
	userPrefs.AutoConfirmMediumRisk = enabled
	prefsMu.Unlock()
	SaveUserPreferences()
}
