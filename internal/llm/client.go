package llm

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/LinDiag-Agent/internal/config"
	"github.com/LinDiag-Agent/internal/platform"
)

var (
	httpClient = &http.Client{Timeout: 180 * time.Second}
	appConfig  *config.Config
)

// Init 初始化 LLM 模块
func Init() error {
	// 加载配置
	cfg, err := config.LoadConfig()
	if err != nil {
		return err
	}
	appConfig = cfg
	return nil
}

// GetConfig 获取当前配置
func GetConfig() *config.Config {
	return appConfig
}

// SaveConfig 保存配置
func SaveConfig(cfg *config.Config) error {
	appConfig = cfg
	return config.SaveConfig(cfg)
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
}

type ChatResponse struct {
	Choices []struct {
		Message Message `json:"message"`
	} `json:"choices"`
}

// 调用AI API
func CallAI(messages []Message) string {
	if appConfig == nil {
		if err := Init(); err != nil {
			return "【错误】: 初始化配置失败"
		}
	}

	if appConfig.LLM.APIKey == "" || appConfig.LLM.APIKey == "你的API_KEY" {
		return "【错误】: 请先配置正确的 API_KEY"
	}

	for retry := 0; retry < 3; retry++ {
		fmt.Printf("\n[%s] 正在连接 AI (尝试 %d/3)...", time.Now().Format("15:04:05"), retry+1)

		reqBody, _ := json.Marshal(ChatRequest{Model: appConfig.LLM.ModelName, Messages: messages})
		req, _ := http.NewRequest("POST", appConfig.LLM.APIURL, bytes.NewBuffer(reqBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+appConfig.LLM.APIKey)

		resp, err := httpClient.Do(req)
		if err != nil {
			fmt.Printf("\n[网络错误] %v\n", err)
			if retry == 2 {
				return "【错误】: API 连接失败，请检查网络或 API_KEY"
			}
			time.Sleep(3 * time.Second)
			continue
		}
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusOK {
			fmt.Printf("\n[API 返回错误] HTTP %d: %s\n", resp.StatusCode, string(body))
			return fmt.Sprintf("【API 错误】HTTP %d", resp.StatusCode)
		}

		var res ChatResponse
		if err := json.Unmarshal(body, &res); err != nil {
			fmt.Printf("\n[解析错误] %v\n", err)
			return "【错误】: 解析 AI 响应失败"
		}
		if len(res.Choices) > 0 {
			content := strings.TrimSpace(res.Choices[0].Message.Content)
			if content == "" {
				fmt.Println("AI 返回空内容，重试中...")
				if retry < 2 {
					time.Sleep(2 * time.Second)
					continue
				}
				return ""
			}
			return content
		}
		fmt.Println("AI 返回空内容，重试中...")
		if retry < 2 {
			time.Sleep(2 * time.Second)
			continue
		}
		return ""
	}
	return "【错误】: AI 调用失败（重试3次后放弃）"
}

// 清理输入内容
func CleanInput(input string) string {
	reAnsi := regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)
	input = reAnsi.ReplaceAllString(input, "")
	reControl := regexp.MustCompile(`[\x00-\x1F\x7F]`)
	return reControl.ReplaceAllString(input, "")
}

// 截断输出内容
func TruncateOutput(output string, maxChars int) string {
	if len(output) <= maxChars {
		return output
	}
	lines := strings.Split(output, "\n")
	// 如果行数超过50，使用简单的截断方式，避免内存溢出
	if len(lines) > 50 {
		// 只保留前20行和后20行
		keep := 20
		head := strings.Join(lines[:keep], "\n")
		tail := strings.Join(lines[len(lines)-keep:], "\n")
		return head + fmt.Sprintf("\n\n... [输出过长，已截断 %d 行] ...\n\n", len(lines)-keep*2) + tail
	}
	// 如果行数不超过50，使用原来的截断方式
	keep := 18
	if len(lines) < keep*2 {
		keep = len(lines) / 2
	}
	head := strings.Join(lines[:keep], "\n")
	tail := strings.Join(lines[len(lines)-keep:], "\n")
	return head + fmt.Sprintf("\n\n... [输出过长，已截断 %d 行] ...\n\n", len(lines)-keep*2) + tail
}

// 加载默认聊天历史
func LoadDefaultChatHistory(reader *bufio.Reader, modeID int, systemPrompt string, snapshotCmds []string, rules string) []Message {
	fmt.Println("\n正在采集系统快照（请稍等）...")
	history := []Message{
		{Role: "system", Content: systemPrompt + rules},
		{Role: "user", Content: "初始系统快照:\n" + platform.GetSnapshot(snapshotCmds)},
	}

	fmt.Println("\n请输入现象描述/日志（输入 ok 结束，多行输入）:")
	var rawInput strings.Builder
	hasContent := false
	for {
		fmt.Print("> ")
		line, _ := reader.ReadString('\n')
		cleanLine := CleanInput(line)
		trimmed := strings.TrimSpace(cleanLine)

		if strings.EqualFold(trimmed, "ok") {
			break
		}

		if trimmed != "" {
			hasContent = true
			rawInput.WriteString(cleanLine)
			rawInput.WriteString("\n")
		} else if hasContent {
			rawInput.WriteString("\n")
		}
	}
	history = append(history, Message{Role: "user", Content: "用户需求:\n" + rawInput.String()})
	return history
}

// 修复连续的assistant消息
func FixConsecutiveAssistantMessages(messages []Message) []Message {
	if len(messages) < 2 {
		return messages
	}

	fixed := []Message{messages[0]}
	for i := 1; i < len(messages); i++ {
		current := messages[i]
		previous := fixed[len(fixed)-1]

		if current.Role == "assistant" && previous.Role == "assistant" {
			// 插入一个用户消息作为分隔
			fixed = append(fixed, Message{Role: "user", Content: "继续分析"})
		}
		fixed = append(fixed, current)
	}
	return fixed
}
