package platform

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/LinDiag-Agent/internal/output"
)

var defaultTimeoutSeconds atomic.Int32

func init() {
	defaultTimeoutSeconds.Store(30)
}

func SetDefaultTimeout(seconds int) {
	defaultTimeoutSeconds.Store(int32(seconds))
}

func GetDefaultTimeout() int {
	return int(defaultTimeoutSeconds.Load())
}

func ExecuteCommand(cmd string) (string, error) {
	return ExecuteCommandWithTimeout(cmd, int(defaultTimeoutSeconds.Load()))
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

	timeout := int(defaultTimeoutSeconds.Load())
	if timeoutSeconds > 0 {
		timeout = timeoutSeconds
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer cancel()

	c := newShellCommandContext(ctx, cmd)
	setProcessGroup(c)

	type result struct {
		out []byte
		err error
	}
	done := make(chan result, 1)

	go func() {
		o, e := c.CombinedOutput()
		done <- result{o, e}
	}()

	// 进度提示：只在命令执行超过 5 秒时才显示
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
	case r := <-done:
		output.ClearLine()
		return string(r.out), r.err
	case <-ctx.Done():
		output.ClearLine()
		killProcessGroup(c)
		// 等待 CombinedOutput 返回，避免 goroutine 泄漏
		var r result
		select {
		case r = <-done:
		case <-time.After(3 * time.Second):
		}
		return string(r.out) + fmt.Sprintf("\n[超时] 命令执行超过%d秒，已自动终止", timeout), ctx.Err()
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
