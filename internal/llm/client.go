package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/LinDiag-Agent/internal/config"
	"github.com/LinDiag-Agent/internal/output"
	openai "github.com/sashabaranov/go-openai"
	"github.com/sashabaranov/go-openai/jsonschema"
)

var (
	appConfig *config.Config
	client    *openai.Client
	// degradedFormat 记忆降级状态：一旦 API 不支持某格式，后续不再尝试
	degradedFormat ResponseFormatType

	// configMu 保护 appConfig 和 degradedFormat 的并发读写
	configMu sync.RWMutex
)

// ResponseFormatType 别名，避免循环导入
type ResponseFormatType = config.ResponseFormatType

// Init 初始化 LLM 模块
func Init() error {
	cfg, err := config.LoadConfig()
	if err != nil {
		return err
	}
	configMu.Lock()
	appConfig = cfg
	configMu.Unlock()

	// 配置 OpenAI 兼容客户端（支持自定义 base_url，适配 DeepSeek 等中转）
	clientCfg := openai.DefaultConfig(cfg.LLM.APIKey)
	if cfg.LLM.APIURL != "" {
		// SDK 的 BaseURL 只需要 base 部分（如 https://api.deepseek.com/v1），
		// 它会自动拼接 /chat/completions。
		// 但用户配置可能是完整端点 URL，需要兼容处理。
		clientCfg.BaseURL = normalizeBaseURL(cfg.LLM.APIURL)
	}
	client = openai.NewClientWithConfig(clientCfg)
	return nil
}

// normalizeBaseURL 将完整端点 URL 转换为 SDK 所需的 base URL。
// 例如 https://api.deepseek.com/v1/chat/completions → https://api.deepseek.com/v1
func normalizeBaseURL(url string) string {
	// 移除末尾斜杠
	url = strings.TrimSuffix(url, "/")
	// 移除 /chat/completions 后缀（SDK 会自动拼接）
	if strings.HasSuffix(url, "/chat/completions") {
		url = strings.TrimSuffix(url, "/chat/completions")
	}
	return url
}

// GetConfig 获取当前配置
func GetConfig() *config.Config {
	configMu.RLock()
	defer configMu.RUnlock()
	return appConfig
}

// SaveConfig 保存配置
func SaveConfig(cfg *config.Config) error {
	configMu.Lock()
	appConfig = cfg
	configMu.Unlock()
	return config.SaveConfig(cfg)
}

// CallAI 调用 LLM 并返回结构化响应。
// 支持 json_schema / json_object / none 三种模式自动降级。
// context 用于超时和取消控制。
func CallAI(ctx context.Context, messages []Message) (AgentResponse, error) {
	configMu.RLock()
	if appConfig == nil {
		configMu.RUnlock()
		if err := Init(); err != nil {
			return AgentResponse{}, fmt.Errorf("初始化配置失败: %w", err)
		}
		configMu.RLock()
	}
	cfg := appConfig
	if cfg.LLM.APIKey == "" || cfg.LLM.APIKey == "你的API_KEY" {
		configMu.RUnlock()
		return AgentResponse{}, errors.New("请先配置正确的 API_KEY")
	}
	if client == nil {
		configMu.RUnlock()
		return AgentResponse{}, errors.New("LLM 客户端未初始化")
	}

	// 默认上下文超时 180 秒（LLM 推理可能较慢）
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 180*time.Second)
		defer cancel()
	}

	oaiMessages := make([]openai.ChatCompletionMessage, 0, len(messages))
	for _, m := range messages {
		oaiMessages = append(oaiMessages, openai.ChatCompletionMessage{
			Role:    m.Role,
			Content: m.Content,
		})
	}

	// 根据降级记忆决定使用哪种格式
	format := cfg.LLM.ResponseFormat
	if degradedFormat == config.ResponseFormatJSONObject && format == config.ResponseFormatJSONSchema {
		format = config.ResponseFormatJSONObject
	} else if degradedFormat == config.ResponseFormatNone {
		format = config.ResponseFormatNone
	}
	configMu.RUnlock()

	switch format {
	case config.ResponseFormatJSONSchema:
		return callWithJSONSchema(ctx, oaiMessages)
	case config.ResponseFormatJSONObject:
		return callWithJSONObject(ctx, oaiMessages)
	default:
		return callWithNone(ctx, oaiMessages)
	}
}

// callWithJSONSchema 使用 Structured Outputs（最强约束），自动从 Go struct 生成 schema
func callWithJSONSchema(ctx context.Context, msgs []openai.ChatCompletionMessage) (AgentResponse, error) {
	schema, err := jsonschema.GenerateSchemaForType(AgentResponse{})
	if err != nil {
		// schema 生成失败，降级到 json_object
		output.Degradef("生成 JSON Schema 失败 (%v)，降级到 JSON Mode", err)
		return callWithJSONObject(ctx, msgs)
	}

	for attempt := 0; attempt < 3; attempt++ {
		output.StatusTimef(output.Cyan, "正在连接 AI (尝试 %d/3, Structured Outputs)...", attempt+1)

		configMu.RLock()
		modelName := appConfig.LLM.ModelName
		configMu.RUnlock()

		req := openai.ChatCompletionRequest{
			Model:    modelName,
			Messages: msgs,
			ResponseFormat: &openai.ChatCompletionResponseFormat{
				Type: openai.ChatCompletionResponseFormatTypeJSONSchema,
				JSONSchema: &openai.ChatCompletionResponseFormatJSONSchema{
					Name:   "agent_response",
					Schema: schema,
					Strict: true,
				},
			},
		}

		resp, err := client.CreateChatCompletion(ctx, req)
		if err != nil {
			if isRetryableError(err) && attempt < 2 {
				output.WarningInplacef("%v", err)
				time.Sleep(retryBackoff(attempt))
				continue
			}
			// 若 API 不支持 json_schema，降级到 json_object
			if isUnsupportedFormatError(err) {
				output.Degradef("当前 API 不支持 Structured Outputs，降级到 JSON Mode（后续不再尝试）")
				configMu.Lock()
				degradedFormat = config.ResponseFormatJSONObject
				configMu.Unlock()
				return callWithJSONObject(ctx, msgs)
			}
			return AgentResponse{}, fmt.Errorf("AI 调用失败: %w", err)
		}

		if len(resp.Choices) == 0 || strings.TrimSpace(resp.Choices[0].Message.Content) == "" {
			output.WarningInplacef("AI 返回空内容，重试中...")
			time.Sleep(2 * time.Second)
			continue
		}

		var result AgentResponse
		if err := schema.Unmarshal(resp.Choices[0].Message.Content, &result); err != nil {
			// schema 解析失败，尝试普通 JSON 解析
			if jsonErr := json.Unmarshal([]byte(resp.Choices[0].Message.Content), &result); jsonErr != nil {
				return AgentResponse{}, fmt.Errorf("解析 AI 结构化响应失败: %w", jsonErr)
			}
		}
		return result, nil
	}
	return AgentResponse{}, errors.New("AI 调用失败（重试3次后放弃）")
}

// callWithJSONObject 使用 JSON Mode（中等约束），保证输出合法 JSON
func callWithJSONObject(ctx context.Context, msgs []openai.ChatCompletionMessage) (AgentResponse, error) {
	// 在消息中注入格式引导提示
	guidedMsgs := injectJSONGuidance(msgs)

	for attempt := 0; attempt < 3; attempt++ {
		output.StatusTimef(output.Cyan, "正在连接 AI (尝试 %d/3, JSON Mode)...", attempt+1)

		configMu.RLock()
		modelName := appConfig.LLM.ModelName
		configMu.RUnlock()

		req := openai.ChatCompletionRequest{
			Model:    modelName,
			Messages: guidedMsgs,
			ResponseFormat: &openai.ChatCompletionResponseFormat{
				Type: openai.ChatCompletionResponseFormatTypeJSONObject,
			},
		}

		resp, err := client.CreateChatCompletion(ctx, req)
		if err != nil {
			if isRetryableError(err) && attempt < 2 {
				output.WarningInplacef("%v", err)
				time.Sleep(retryBackoff(attempt))
				continue
			}
			if isUnsupportedFormatError(err) {
				output.Degradef("当前 API 不支持 JSON Mode，降级到无格式约束模式（后续不再尝试）")
				configMu.Lock()
				degradedFormat = config.ResponseFormatNone
				configMu.Unlock()
				return callWithNone(ctx, msgs)
			}
			return AgentResponse{}, fmt.Errorf("AI 调用失败: %w", err)
		}

		if len(resp.Choices) == 0 || strings.TrimSpace(resp.Choices[0].Message.Content) == "" {
			output.WarningInplacef("AI 返回空内容，重试中...")
			time.Sleep(2 * time.Second)
			continue
		}

		content := strings.TrimSpace(resp.Choices[0].Message.Content)
		var result AgentResponse
		if err := json.Unmarshal([]byte(content), &result); err != nil {
			// 尝试从可能的 markdown 代码块中提取 JSON
			cleaned := extractJSONFromMarkdown(content)
			if jsonErr := json.Unmarshal([]byte(cleaned), &result); jsonErr != nil {
				return AgentResponse{Analysis: content}, nil // 至少返回原始文本作为分析
			}
		}
		return result, nil
	}
	return AgentResponse{}, errors.New("AI 调用失败（重试3次后放弃）")
}

// callWithNone 无格式约束（兜底），靠提示词引导 + 尽力解析
func callWithNone(ctx context.Context, msgs []openai.ChatCompletionMessage) (AgentResponse, error) {
	guidedMsgs := injectJSONGuidance(msgs)

	for attempt := 0; attempt < 3; attempt++ {
		output.StatusTimef(output.Cyan, "正在连接 AI (尝试 %d/3)...", attempt+1)

		configMu.RLock()
		modelName := appConfig.LLM.ModelName
		configMu.RUnlock()

		req := openai.ChatCompletionRequest{
			Model:    modelName,
			Messages: guidedMsgs,
		}

		resp, err := client.CreateChatCompletion(ctx, req)
		if err != nil {
			if isRetryableError(err) && attempt < 2 {
				output.WarningInplacef("%v", err)
				time.Sleep(retryBackoff(attempt))
				continue
			}
			return AgentResponse{}, fmt.Errorf("AI 调用失败: %w", err)
		}

		if len(resp.Choices) == 0 || strings.TrimSpace(resp.Choices[0].Message.Content) == "" {
			output.WarningInplacef("AI 返回空内容，重试中...")
			time.Sleep(2 * time.Second)
			continue
		}

		content := strings.TrimSpace(resp.Choices[0].Message.Content)
		// 尽力从自由文本中提取 JSON
		cleaned := extractJSONFromMarkdown(content)
		var result AgentResponse
		if err := json.Unmarshal([]byte(cleaned), &result); err != nil {
			// 解析失败，把原始文本作为分析返回
			return AgentResponse{Analysis: content}, nil
		}
		return result, nil
	}
	return AgentResponse{}, errors.New("AI 调用失败（重试3次后放弃）")
}

// injectJSONGuidance 在消息末尾注入 JSON 格式引导提示
func injectJSONGuidance(msgs []openai.ChatCompletionMessage) []openai.ChatCompletionMessage {
	if len(msgs) == 0 {
		return msgs
	}
	guidance := "\n\n[重要] 请严格以 JSON 格式回复，结构如下：\n" +
		`{"analysis":"你的分析说明","commands":[{"command":"要执行的命令","purpose":"命令目的","expected_risk":"safe|low|medium|high|critical"}],"issues":[{"severity":"critical|high|medium|low|info","category":"cpu|memory|disk|network|process|kernel|service|container|kubernetes|security|other","title":"问题一句话标题","evidence":"引用实际命令输出作为证据","suggestion":"修复建议与步骤"}],"is_final":false}` + "\n" +
		"不要输出 JSON 以外的任何内容。无需执行命令时 commands 为空数组；未识别到明确问题时 issues 为空数组。"

	last := msgs[len(msgs)-1]
	if last.Role == "user" {
		msgs[len(msgs)-1] = openai.ChatCompletionMessage{
			Role:    last.Role,
			Content: last.Content + guidance,
		}
	} else {
		msgs = append(msgs, openai.ChatCompletionMessage{
			Role:    "user",
			Content: guidance,
		})
	}
	return msgs
}

// extractJSONFromMarkdown 从可能包含 markdown 代码块的文本中提取 JSON
func extractJSONFromMarkdown(content string) string {
	// 尝试提取 ```json ... ``` 代码块
	re := regexp.MustCompile("(?s)```(?:json)?\\s*([\\s\\S]*?)```")
	if m := re.FindStringSubmatch(content); len(m) > 1 {
		return strings.TrimSpace(m[1])
	}
	// 尝试提取第一个 { 到最后一个 } 之间的内容
	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start >= 0 && end > start {
		return content[start : end+1]
	}
	return content
}

// isRetryableError 判断是否为可重试错误（网络超时、5xx 等）
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "timeout") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "eof") ||
		strings.Contains(msg, "temporary") ||
		strings.Contains(msg, "rate limit") ||
		strings.Contains(msg, "429") ||
		strings.Contains(msg, "500") ||
		strings.Contains(msg, "502") ||
		strings.Contains(msg, "503")
}

// isUnsupportedFormatError 判断是否为格式不支持错误
func isUnsupportedFormatError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "response_format") ||
		strings.Contains(msg, "json_schema") ||
		strings.Contains(msg, "not supported")
}

// retryBackoff 返回第 attempt 次（0-based）重试的等待时间。
// 基础 1s，指数退避 ×2，上限 8s，加 ±200ms 抖动。
func retryBackoff(attempt int) time.Duration {
	base := time.Second << uint(attempt) // 1s, 2s, 4s, 8s, ...
	if base > 8*time.Second {
		base = 8 * time.Second
	}
	// 抖动：±200ms
	jitter := time.Duration(0)
	if base > 0 {
		jitter = time.Duration(int64(base) / 10) // ±10%
	}
	return base + jitter - 40*time.Millisecond + time.Duration(80*attempt)*time.Millisecond
}
