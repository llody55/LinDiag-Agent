package platform

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/LinDiag-Agent/internal/output"
)

var defaultTimeoutSeconds = 30

func SetDefaultTimeout(seconds int) {
	defaultTimeoutSeconds = seconds
}

func GetDefaultTimeout() int {
	return defaultTimeoutSeconds
}

func ExecuteCommand(cmd string) (string, error) {
	return ExecuteCommandWithTimeout(cmd, defaultTimeoutSeconds)
}

func ExecuteCommandWithTimeout(cmd string, timeoutSeconds int) (string, error) {
	if isBackgroundCommand(cmd) {
		c := newShellCommand(cmd)
		err := c.Start()
		if err != nil {
			return "", err
		}
		return "命令已在后台启动", nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSeconds)*time.Second)
	defer cancel()

	c := newShellCommandContext(ctx, cmd)
	// 设置进程组（平台特定实现）
	setProcessGroup(c)

	var out []byte
	var err error
	done := make(chan struct{})

	go func() {
		out, err = c.CombinedOutput()
		close(done)
	}()

	// 进度提示：只在命令执行超过 5 秒时才显示
	// 同时监听 ctx.Done()，确保超时后立即停止（不会泄漏）
	go func() {
		select {
		case <-done:
			return
		case <-ctx.Done():
			return
		case <-time.After(5 * time.Second):
		}

		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		elapsed := 5
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				elapsed += 5
				output.Inplacef("   ⏳ 已执行 %ds...", elapsed)
			}
		}
	}()

	select {
	case <-done:
		output.ClearLine()
		return string(out), err
	case <-ctx.Done():
		output.ClearLine()
		// 杀掉整个进程组（平台特定实现）
		killProcessGroup(c)
		// 等待 CombinedOutput 返回，避免 goroutine 泄漏
		select {
		case <-done:
		case <-time.After(3 * time.Second):
		}
		return string(out) + fmt.Sprintf("\n[超时] 命令执行超过%d秒，已自动终止", timeoutSeconds), ctx.Err()
	}
}

func ExecuteCommandWithProgress(cmd string) (string, error) {
	output.Statusln("Running: %s", cmd)
	return ExecuteCommand(cmd)
}

func ExecuteCommandWithProgressAndTimeout(cmd string, timeoutSeconds int) (string, error) {
	output.Statusln("Running: %s", cmd)
	return ExecuteCommandWithTimeout(cmd, timeoutSeconds)
}

func GetHostname() string {
	hostname, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(hostname)
}

func GetIPAddress() string {
	return getIPAddress()
}
