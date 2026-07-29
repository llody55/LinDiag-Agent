package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/LinDiag-Agent/internal/paths"
)

// ResponseFormatType 控制 LLM 输出格式约束方式
type ResponseFormatType string

const (
	// ResponseFormatJSONSchema 使用 Structured Outputs（json_schema），最强约束，需模型支持
	ResponseFormatJSONSchema ResponseFormatType = "json_schema"
	// ResponseFormatJSONObject 使用 JSON Mode（json_object），保证合法 JSON 但不约束 schema
	ResponseFormatJSONObject ResponseFormatType = "json_object"
	// ResponseFormatNone 不做格式约束，仅靠提示词引导（兜底方案）
	ResponseFormatNone ResponseFormatType = "none"
)

// Config 应用配置结构
type Config struct {
	LLM     LLMConfig     `json:"llm"`
	Command CommandConfig `json:"command"`
}

// LLM 配置
type LLMConfig struct {
	APIURL         string             `json:"api_url"`         // OpenAI 兼容 API 地址
	APIKey         string             `json:"api_key"`         // API 密钥
	ModelName      string             `json:"model_name"`      // 模型名称
	ResponseFormat ResponseFormatType `json:"response_format"` // 输出格式约束方式
}

// 命令执行配置
type CommandConfig struct {
	TimeoutSeconds int `json:"timeout_seconds"`
}

// 默认配置
var defaultConfig = Config{
	LLM: LLMConfig{
		APIURL:         "",
		APIKey:         "",
		ModelName:      "",
		ResponseFormat: ResponseFormatJSONSchema, // 默认使用最强约束
	},
	Command: CommandConfig{
		TimeoutSeconds: 30,
	},
}

// LoadConfig 加载配置，环境变量优先级高于配置文件
func LoadConfig() (*Config, error) {
	cfg := defaultConfig

	loadFromEnv(&cfg)
	if err := loadFromFile(&cfg); err != nil {
		return nil, fmt.Errorf("加载配置文件失败: %w", err)
	}

	return &cfg, nil
}

// loadFromEnv 从环境变量加载配置（优先级最高）
func loadFromEnv(cfg *Config) {
	if v := os.Getenv("LINDIAG_LLM_API_URL"); v != "" {
		cfg.LLM.APIURL = v
	}
	if v := os.Getenv("LINDIAG_LLM_API_KEY"); v != "" {
		cfg.LLM.APIKey = v
	}
	if v := os.Getenv("LINDIAG_LLM_MODEL_NAME"); v != "" {
		cfg.LLM.ModelName = v
	}
	if v := os.Getenv("LINDIAG_LLM_RESPONSE_FORMAT"); v != "" {
		cfg.LLM.ResponseFormat = ResponseFormatType(v)
	}
}

// loadFromFile 从配置文件加载配置。
// 路径由 internal/paths 包统一管理（$XDG_CONFIG_HOME/lindiag/config.json）。
func loadFromFile(cfg *Config) error {
	configFile := paths.ConfigFile()

	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		return nil // 配置文件不存在不算错误
	}

	file, err := os.Open(configFile)
	if err != nil {
		return err
	}
	defer file.Close()

	var fc Config
	if err := json.NewDecoder(file).Decode(&fc); err != nil {
		return fmt.Errorf("解析配置文件失败: %w", err)
	}

	// 仅在环境变量未设置时，才用文件值覆盖
	// 环境变量优先级最高，已在 loadFromEnv 中设置
	if fc.LLM.APIURL != "" && os.Getenv("LINDIAG_LLM_API_URL") == "" {
		cfg.LLM.APIURL = cleanValue(fc.LLM.APIURL)
	}
	if fc.LLM.APIKey != "" && os.Getenv("LINDIAG_LLM_API_KEY") == "" {
		cfg.LLM.APIKey = cleanValue(fc.LLM.APIKey)
	}
	if fc.LLM.ModelName != "" && os.Getenv("LINDIAG_LLM_MODEL_NAME") == "" {
		cfg.LLM.ModelName = cleanValue(fc.LLM.ModelName)
	}
	if fc.LLM.ResponseFormat != "" && os.Getenv("LINDIAG_LLM_RESPONSE_FORMAT") == "" {
		cfg.LLM.ResponseFormat = fc.LLM.ResponseFormat
	}
	if fc.Command.TimeoutSeconds > 0 {
		cfg.Command.TimeoutSeconds = fc.Command.TimeoutSeconds
	}
	return nil
}

// cleanValue 清理配置值，移除空格和反引号
func cleanValue(v string) string {
	return strings.ReplaceAll(strings.TrimSpace(v), "`", "")
}

// SaveConfig 保存配置到文件（路径由 paths 包统一管理）。
func SaveConfig(cfg *Config) error {
	clean := *cfg
	clean.LLM.APIURL = cleanValue(clean.LLM.APIURL)
	clean.LLM.APIKey = cleanValue(clean.LLM.APIKey)
	clean.LLM.ModelName = cleanValue(clean.LLM.ModelName)

	if err := paths.EnsureConfigDir(); err != nil {
		return err
	}

	data, err := json.Marshal(clean)
	if err != nil {
		return err
	}
	// 原子写：临时文件 + rename，避免崩溃时留下半截文件
	// 权限 0600：配置含 API Key，仅 owner 可读写
	tmp, err := os.CreateTemp(paths.ConfigDir(), "config.*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	// 格式化：写入前先缩进
	var indented bytes.Buffer
	if err := json.Indent(&indented, data, "", "  "); err == nil {
		if _, err := tmp.WriteAt(indented.Bytes(), 0); err == nil {
			tmp.Truncate(int64(len(indented.Bytes())))
		}
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Chmod(tmpName, 0600); err != nil {
		os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, paths.ConfigFile())
}
