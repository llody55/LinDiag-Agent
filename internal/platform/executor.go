package platform

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
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
	if strings.HasSuffix(strings.TrimSpace(cmd), "&") {
		c := exec.Command("sh", "-c", cmd)
		err := c.Start()
		if err != nil {
			return "", err
		}
		return "命令已在后台启动", nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSeconds)*time.Second)
	defer cancel()

	c := exec.CommandContext(ctx, "sh", "-c", cmd)

	var out []byte
	var err error
	done := make(chan struct{})

	go func() {
		out, err = c.CombinedOutput()
		close(done)
	}()

	go func() {
		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()
		count := 0
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				count++
				if count <= 3 {
					fmt.Printf("   (%ds)...\r", count*3)
				} else if count%3 == 0 {
					messages := []string{"正在努力处理中...", "数据量较大，请耐心等待...", "马上就好，请稍等..."}
					fmt.Printf("   %s\r", messages[(count/3-1)%3])
				}
			}
		}
	}()

	select {
	case <-done:
		fmt.Print("                           \r")
		return string(out), err
	case <-ctx.Done():
		fmt.Print("                           \r")
		if c.Process != nil {
			c.Process.Kill()
		}
		return string(out) + fmt.Sprintf("\n[超时] 命令执行超过%d秒，已自动终止", timeoutSeconds), ctx.Err()
	}
}

func ExecuteCommandWithProgress(cmd string) (string, error) {
	fmt.Printf("Running: %s\n", cmd)
	return ExecuteCommand(cmd)
}

func ExecuteCommandWithProgressAndTimeout(cmd string, timeoutSeconds int) (string, error) {
	fmt.Printf("Running: %s\n", cmd)
	return ExecuteCommandWithTimeout(cmd, timeoutSeconds)
}

func GetHostname() string {
	hostname, err := exec.Command("hostname").Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(hostname))
}

func GetIPAddress() string {
	cmd := "hostname -I | awk '{print $1}'"
	out, err := exec.Command("sh", "-c", cmd).CombinedOutput()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}
