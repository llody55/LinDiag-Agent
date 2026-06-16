package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// Config 应用配置结构
type Config struct {
	// LLM 配置
	LLM struct {
		APIURL    string `json:"api_url"`
		APIKey    string `json:"api_key"`
		ModelName string `json:"model_name"`
	} `json:"llm"`

	// 命令执行配置
	Command struct {
		TimeoutSeconds int `json:"timeout_seconds"`
	} `json:"command"`
}

// 默认配置
var defaultConfig = Config{
	LLM: struct {
		APIURL    string `json:"api_url"`
		APIKey    string `json:"api_key"`
		ModelName string `json:"model_name"`
	}{
		APIURL:    "", // 留空，用户必须通过配置文件设置
		APIKey:    "", // 留空，用户必须通过配置文件设置
		ModelName: "", // 留空，用户必须通过配置文件设置
	},
	Command: struct {
		TimeoutSeconds int `json:"timeout_seconds"`
	}{
		TimeoutSeconds: 30, // 默认超时时间30秒
	},
}

// LoadConfig 加载配置
func LoadConfig() (*Config, error) {
	// 创建配置实例
	config := defaultConfig

	// 1. 从环境变量加载配置
	loadFromEnv(&config)

	// 2. 从配置文件加载配置
	loadFromFile(&config)

	return &config, nil
}

// loadFromEnv 从环境变量加载配置
func loadFromEnv(config *Config) {
	// LLM 配置
	if apiURL := os.Getenv("LINDIAG_LLM_API_URL"); apiURL != "" {
		config.LLM.APIURL = apiURL
	}

	if apiKey := os.Getenv("LINDIAG_LLM_API_KEY"); apiKey != "" {
		config.LLM.APIKey = apiKey
	}

	if modelName := os.Getenv("LINDIAG_LLM_MODEL_NAME"); modelName != "" {
		config.LLM.ModelName = modelName
	}
}

// loadFromFile 从配置文件加载配置
func loadFromFile(config *Config) {
	// 配置文件路径：~/.config/lindiag/config.json
	configDir := filepath.Join(os.Getenv("HOME"), ".config", "lindiag")
	configFile := filepath.Join(configDir, "config.json")

	// 检查配置文件是否存在
	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		return
	}

	// 读取配置文件
	file, err := os.Open(configFile)
	if err != nil {
		return
	}
	defer file.Close()

	// 解析配置文件
	var fileConfig Config
	if err := json.NewDecoder(file).Decode(&fileConfig); err != nil {
		return
	}

	// 更新配置
	if fileConfig.LLM.APIURL != "" {
		// 清理 API URL，移除所有空格和反引号
		config.LLM.APIURL = strings.ReplaceAll(strings.TrimSpace(fileConfig.LLM.APIURL), "`", "")
	}

	if fileConfig.LLM.APIKey != "" {
		// 清理 API Key，移除所有空格和反引号
		config.LLM.APIKey = strings.ReplaceAll(strings.TrimSpace(fileConfig.LLM.APIKey), "`", "")
	}

	if fileConfig.LLM.ModelName != "" {
		// 清理模型名称，移除所有空格和反引号
		config.LLM.ModelName = strings.ReplaceAll(strings.TrimSpace(fileConfig.LLM.ModelName), "`", "")
	}

	if fileConfig.Command.TimeoutSeconds > 0 {
		config.Command.TimeoutSeconds = fileConfig.Command.TimeoutSeconds
	}
}

// SaveConfig 保存配置到文件
func SaveConfig(config *Config) error {
	// 清理配置值
	cleanConfig := *config
	cleanConfig.LLM.APIURL = strings.ReplaceAll(strings.TrimSpace(cleanConfig.LLM.APIURL), "`", "")
	cleanConfig.LLM.APIKey = strings.ReplaceAll(strings.TrimSpace(cleanConfig.LLM.APIKey), "`", "")
	cleanConfig.LLM.ModelName = strings.ReplaceAll(strings.TrimSpace(cleanConfig.LLM.ModelName), "`", "")

	// 配置文件路径：~/.config/lindiag/config.json
	configDir := filepath.Join(os.Getenv("HOME"), ".config", "lindiag")
	configFile := filepath.Join(configDir, "config.json")

	// 创建目录
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return err
	}

	// 写入配置文件
	file, err := os.Create(configFile)
	if err != nil {
		return err
	}
	defer file.Close()

	// 编码配置
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(cleanConfig)
}
