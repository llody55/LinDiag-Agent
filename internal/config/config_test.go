package config

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLoadConfig_FromFile 测试从配置文件加载
func TestLoadConfig_FromFile(t *testing.T) {
	// 创建临时配置目录
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	configDir := filepath.Join(tmpDir, ".config", "lindiag")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatal(err)
	}

	configContent := `{
		"llm": {
			"api_url": "https://api.test.com/v1",
			"api_key": "test-key-123",
			"model_name": "test-model",
			"response_format": "json_object"
		},
		"command": {
			"timeout_seconds": 45
		}
	}`
	configPath := filepath.Join(configDir, "config.json")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatal(err)
	}

	// 清除环境变量避免干扰
	os.Unsetenv("LINDIAG_LLM_API_URL")
	os.Unsetenv("LINDIAG_LLM_API_KEY")
	os.Unsetenv("LINDIAG_LLM_MODEL_NAME")
	os.Unsetenv("LINDIAG_LLM_RESPONSE_FORMAT")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig 失败: %v", err)
	}

	if cfg.LLM.APIURL != "https://api.test.com/v1" {
		t.Errorf("APIURL = %q, 期望 %q", cfg.LLM.APIURL, "https://api.test.com/v1")
	}
	if cfg.LLM.APIKey != "test-key-123" {
		t.Errorf("APIKey = %q, 期望 %q", cfg.LLM.APIKey, "test-key-123")
	}
	if cfg.LLM.ModelName != "test-model" {
		t.Errorf("ModelName = %q, 期望 %q", cfg.LLM.ModelName, "test-model")
	}
	if cfg.LLM.ResponseFormat != ResponseFormatJSONObject {
		t.Errorf("ResponseFormat = %q, 期望 %q", cfg.LLM.ResponseFormat, ResponseFormatJSONObject)
	}
	if cfg.Command.TimeoutSeconds != 45 {
		t.Errorf("TimeoutSeconds = %d, 期望 45", cfg.Command.TimeoutSeconds)
	}
}

// TestLoadConfig_EnvOverride 测试环境变量优先级高于配置文件
func TestLoadConfig_EnvOverride(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	configDir := filepath.Join(tmpDir, ".config", "lindiag")
	os.MkdirAll(configDir, 0755)
	configContent := `{"llm":{"api_url":"from-file","api_key":"from-file-key","model_name":"from-file-model"}}`
	os.WriteFile(filepath.Join(configDir, "config.json"), []byte(configContent), 0644)

	// 环境变量应覆盖文件配置
	os.Setenv("LINDIAG_LLM_API_URL", "from-env")
	defer os.Unsetenv("LINDIAG_LLM_API_URL")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig 失败: %v", err)
	}

	if cfg.LLM.APIURL != "from-env" {
		t.Errorf("APIURL = %q, 期望 from-env (环境变量应优先)", cfg.LLM.APIURL)
	}
	// 文件中的值应保留
	if cfg.LLM.APIKey != "from-file-key" {
		t.Errorf("APIKey = %q, 期望 from-file-key", cfg.LLM.APIKey)
	}
}

// TestLoadConfig_Defaults 测试默认值
func TestLoadConfig_Defaults(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	os.Unsetenv("LINDIAG_LLM_API_URL")
	os.Unsetenv("LINDIAG_LLM_API_KEY")
	os.Unsetenv("LINDIAG_LLM_MODEL_NAME")
	os.Unsetenv("LINDIAG_LLM_RESPONSE_FORMAT")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig 失败: %v", err)
	}

	if cfg.LLM.ResponseFormat != ResponseFormatJSONSchema {
		t.Errorf("默认 ResponseFormat = %q, 期望 %q", cfg.LLM.ResponseFormat, ResponseFormatJSONSchema)
	}
	if cfg.Command.TimeoutSeconds != 30 {
		t.Errorf("默认 TimeoutSeconds = %d, 期望 30", cfg.Command.TimeoutSeconds)
	}
}

// TestLoadConfig_CleanValue 测试配置值清理（移除反引号和空格）
func TestLoadConfig_CleanValue(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	configDir := filepath.Join(tmpDir, ".config", "lindiag")
	os.MkdirAll(configDir, 0755)
	// 包含反引号和多余空格
	configContent := "{\"llm\":{\"api_url\":\"  `https://api.test.com/v1`  \",\"api_key\":\"`key`\",\"model_name\":\"model\"}}"
	os.WriteFile(filepath.Join(configDir, "config.json"), []byte(configContent), 0644)

	os.Unsetenv("LINDIAG_LLM_API_URL")
	os.Unsetenv("LINDIAG_LLM_API_KEY")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig 失败: %v", err)
	}

	if cfg.LLM.APIURL != "https://api.test.com/v1" {
		t.Errorf("APIURL 清理后 = %q, 期望 https://api.test.com/v1", cfg.LLM.APIURL)
	}
	if cfg.LLM.APIKey != "key" {
		t.Errorf("APIKey 清理后 = %q, 期望 key", cfg.LLM.APIKey)
	}
}
