package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
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

	// 副模型配置（用于命令安全分析）
	SafetyLLM struct {
		APIURL    string `json:"api_url"`
		APIKey    string `json:"api_key"`
		ModelName string `json:"model_name"`
	} `json:"safety_llm"`

	// 超时配置
	Timeout struct {
		CommandTimeout int `json:"command_timeout"` // 命令执行超时（秒）
		HTTPTimeout    int `json:"http_timeout"`    // HTTP请求超时（秒）
	} `json:"timeout"`
}

// getConfigDir 获取跨平台的配置目录
// Linux/macOS: ~/.config/lindiag
// Windows: %APPDATA%\lindiag
func getConfigDir() string {
	var baseDir string
	if runtime.GOOS == "windows" {
		baseDir = os.Getenv("APPDATA")
	} else {
		baseDir = os.Getenv("HOME")
	}
	if baseDir == "" {
		return "."
	}
	if runtime.GOOS == "windows" {
		return filepath.Join(baseDir, "lindiag")
	}
	return filepath.Join(baseDir, ".config", "lindiag")
}

// getConfigFile 获取配置文件路径
func getConfigFile() string {
	return filepath.Join(getConfigDir(), "config.json")
}

// 默认配置
var defaultConfig = Config{
	LLM: struct {
		APIURL    string `json:"api_url"`
		APIKey    string `json:"api_key"`
		ModelName string `json:"model_name"`
	}{
		APIURL:    "https://dashscope.aliyuncs.com/compatible-mode/v1/chat/completions",
		APIKey:    "",
		ModelName: "qwen-flash-character-2026-02-26",
	},

	SafetyLLM: struct {
		APIURL    string `json:"api_url"`
		APIKey    string `json:"api_key"`
		ModelName string `json:"model_name"`
	}{
		APIURL:    "https://api.siliconflow.cn/v1/chat/completions",
		APIKey:    "",
		ModelName: "Qwen/Qwen3-8B",
	},

	Timeout: struct {
		CommandTimeout int `json:"command_timeout"`
		HTTPTimeout    int `json:"http_timeout"`
	}{
		CommandTimeout: 30,  // 默认命令执行超时30秒
		HTTPTimeout:    180, // 默认HTTP请求超时180秒
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

// ValidateConfig 验证配置是否完整
func ValidateConfig(config *Config) []string {
	var missing []string

	if config.LLM.APIKey == "" {
		missing = append(missing, "LLM API Key")
	}

	if config.SafetyLLM.APIKey == "" {
		missing = append(missing, "Safety LLM API Key")
	}

	return missing
}

// GetConfigHelpMessage 获取配置帮助信息
func GetConfigHelpMessage() string {
	return `配置缺失提示：

可配置项说明：

LLM（主模型）配置：
  - LINDIAG_LLM_API_URL      - API地址
  - LINDIAG_LLM_API_KEY      - API密钥（必填）
  - LINDIAG_LLM_MODEL_NAME   - 模型名称

Safety LLM（安全分析模型）配置：
  - LINDIAG_SAFETY_LLM_API_URL      - API地址
  - LINDIAG_SAFETY_LLM_API_KEY      - API密钥（必填）
  - LINDIAG_SAFETY_LLM_MODEL_NAME   - 模型名称

超时配置（单位：秒）：
  - LINDIAG_TIMEOUT_COMMAND   - 命令执行超时时间（默认30秒）
  - LINDIAG_TIMEOUT_HTTP      - HTTP请求超时时间（默认180秒）

配置方式：

1. 使用环境变量：
   # Linux/macOS
   export LINDIAG_LLM_API_URL="https://api.example.com/v1/chat/completions"
   export LINDIAG_LLM_API_KEY="your-api-key"
   export LINDIAG_LLM_MODEL_NAME="your-model-name"
   export LINDIAG_SAFETY_LLM_API_URL="https://api.example.com/v1/chat/completions"
   export LINDIAG_SAFETY_LLM_API_KEY="your-api-key"
   export LINDIAG_SAFETY_LLM_MODEL_NAME="your-model-name"
   export LINDIAG_TIMEOUT_COMMAND=60
   export LINDIAG_TIMEOUT_HTTP=300

   # Windows (PowerShell)
   $env:LINDIAG_LLM_API_URL="https://api.example.com/v1/chat/completions"
   $env:LINDIAG_LLM_API_KEY="your-api-key"
   $env:LINDIAG_LLM_MODEL_NAME="your-model-name"
   $env:LINDIAG_SAFETY_LLM_API_URL="https://api.example.com/v1/chat/completions"
   $env:LINDIAG_SAFETY_LLM_API_KEY="your-api-key"
   $env:LINDIAG_SAFETY_LLM_MODEL_NAME="your-model-name"
   $env:LINDIAG_TIMEOUT_COMMAND=60
   $env:LINDIAG_TIMEOUT_HTTP=300

2. 使用配置文件：
   # Linux/macOS: 创建 ~/.config/lindiag/config.json
   # Windows: 创建 %APPDATA%\lindiag\config.json
   
   文件内容如下：
   {
     "llm": {
       "api_url": "https://dashscope.aliyuncs.com/compatible-mode/v1/chat/completions",
       "api_key": "YOUR_API_KEY_HERE",
       "model_name": "qwen-flash-character-2026-02-26"
     },
     "safety_llm": {
       "api_url": "https://api.siliconflow.cn/v1/chat/completions",
       "api_key": "YOUR_API_KEY_HERE",
       "model_name": "Qwen/Qwen3-8B"
     },
     "timeout": {
       "command_timeout": 30,
       "http_timeout": 180
     }
   }

配置文件模板可参考：internal/config/config.json.example

配置优先级：环境变量 > 配置文件 > 默认值
`
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

	// Safety LLM 配置
	if apiURL := os.Getenv("LINDIAG_SAFETY_LLM_API_URL"); apiURL != "" {
		config.SafetyLLM.APIURL = apiURL
	}

	if apiKey := os.Getenv("LINDIAG_SAFETY_LLM_API_KEY"); apiKey != "" {
		config.SafetyLLM.APIKey = apiKey
	}

	if modelName := os.Getenv("LINDIAG_SAFETY_LLM_MODEL_NAME"); modelName != "" {
		config.SafetyLLM.ModelName = modelName
	}

	// 超时配置
	if timeout := os.Getenv("LINDIAG_TIMEOUT_COMMAND"); timeout != "" {
		if t, err := strconv.Atoi(timeout); err == nil && t > 0 {
			config.Timeout.CommandTimeout = t
		}
	}

	if timeout := os.Getenv("LINDIAG_TIMEOUT_HTTP"); timeout != "" {
		if t, err := strconv.Atoi(timeout); err == nil && t > 0 {
			config.Timeout.HTTPTimeout = t
		}
	}
}

// loadFromFile 从配置文件加载配置
func loadFromFile(config *Config) {
	// 配置文件路径：
	// Linux/macOS: ~/.config/lindiag/config.json
	// Windows: %APPDATA%\lindiag\config.json
	configFile := getConfigFile()

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

	// 更新 Safety LLM 配置
	if fileConfig.SafetyLLM.APIURL != "" {
		// 清理 API URL，移除所有空格和反引号
		config.SafetyLLM.APIURL = strings.ReplaceAll(strings.TrimSpace(fileConfig.SafetyLLM.APIURL), "`", "")
	}

	if fileConfig.SafetyLLM.APIKey != "" {
		// 清理 API Key，移除所有空格和反引号
		config.SafetyLLM.APIKey = strings.ReplaceAll(strings.TrimSpace(fileConfig.SafetyLLM.APIKey), "`", "")
	}

	if fileConfig.SafetyLLM.ModelName != "" {
		// 清理模型名称，移除所有空格和反引号
		config.SafetyLLM.ModelName = strings.ReplaceAll(strings.TrimSpace(fileConfig.SafetyLLM.ModelName), "`", "")
	}

	// 更新超时配置
	if fileConfig.Timeout.CommandTimeout > 0 {
		config.Timeout.CommandTimeout = fileConfig.Timeout.CommandTimeout
	}

	if fileConfig.Timeout.HTTPTimeout > 0 {
		config.Timeout.HTTPTimeout = fileConfig.Timeout.HTTPTimeout
	}
}

// SaveConfig 保存配置到文件
func SaveConfig(config *Config) error {
	// 清理配置值
	cleanConfig := *config
	cleanConfig.LLM.APIURL = strings.ReplaceAll(strings.TrimSpace(cleanConfig.LLM.APIURL), "`", "")
	cleanConfig.LLM.APIKey = strings.ReplaceAll(strings.TrimSpace(cleanConfig.LLM.APIKey), "`", "")
	cleanConfig.LLM.ModelName = strings.ReplaceAll(strings.TrimSpace(cleanConfig.LLM.ModelName), "`", "")

	// 清理 Safety LLM 配置值
	cleanConfig.SafetyLLM.APIURL = strings.ReplaceAll(strings.TrimSpace(cleanConfig.SafetyLLM.APIURL), "`", "")
	cleanConfig.SafetyLLM.APIKey = strings.ReplaceAll(strings.TrimSpace(cleanConfig.SafetyLLM.APIKey), "`", "")
	cleanConfig.SafetyLLM.ModelName = strings.ReplaceAll(strings.TrimSpace(cleanConfig.SafetyLLM.ModelName), "`", "")

	// 配置文件路径：
	// Linux/macOS: ~/.config/lindiag/config.json
	// Windows: %APPDATA%\lindiag\config.json
	configDir := getConfigDir()
	configFile := getConfigFile()

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
