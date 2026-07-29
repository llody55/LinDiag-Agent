package report

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/LinDiag-Agent/internal/output"
)

// GeneratePDFReport 生成 PDF 报告。
//
// 策略（无 Go 库依赖，复用已生成的 HTML，运行时探测外部转换器）：
//  1. 先调 GenerateHTMLReport 生成同名 HTML 中转文件
//  2. 按 wkhtmltopdf → chromium-family headless 顺序探测系统中可用的转换器
//  3. 若探测到任一转换器，调用它把 HTML 真转成 PDF 二进制
//  4. 若都没有，则回退到旧行为：保留 HTML，提示用户手动转 PDF
//
// 这样在装了 wkhtmltopdf 或 chromium 的环境（CI/容器/开发机）能真出 PDF；
// 在裸环境仍保留可用性（HTML 可读 + 给出转换命令）。
func GeneratePDFReport(filename string, data ReportData) {
	htmlFilename := strings.Replace(filename, ".pdf", ".html", 1)
	GenerateHTMLReport(htmlFilename, data)
	output.InfoMessage("已生成HTML报告: " + htmlFilename)

	if conv, kind := detectPDFConverter(); conv != "" {
		output.InfoMessage(fmt.Sprintf("检测到 %s，正在转换为 PDF...", kind))
		if err := runConverter(conv, kind, htmlFilename, filename); err != nil {
			output.ErrorMessage("PDF 转换失败: " + err.Error())
			output.InfoMessage(fmt.Sprintf("HTML 仍可用: %s", htmlFilename))
			return
		}
		output.SuccessMessage("已生成 PDF 报告: " + filename)
		return
	}

	// 回退：无外部转换器
	output.WarningMessage("未检测到 wkhtmltopdf 或 chromium，无法自动转 PDF")
	output.InfoMessage(fmt.Sprintf("请使用浏览器打开 HTML 并另存为 PDF: %s", htmlFilename))
	output.InfoMessage(fmt.Sprintf("或安装转换器后重试: apt install wkhtmltopdf  |  apt install chromium"))
}

// detectPDFConverter 探测可用的 PDF 转换器，返回 (路径, 类型)。
// 路径为空表示未找到。
func detectPDFConverter() (string, string) {
	if p, err := exec.LookPath("wkhtmltopdf"); err == nil {
		return p, "wkhtmltopdf"
	}
	for _, name := range []string{
		"chromium",
		"chromium-browser",
		"google-chrome",
		"google-chrome-stable",
		"chromium-headless-shell",
		"chrome-headless-shell",
	} {
		if p, err := exec.LookPath(name); err == nil {
			return p, "chromium"
		}
	}
	return "", ""
}

// runConverter 调用指定转换器把 htmlPath 转成 pdfPath。
func runConverter(conv, kind, htmlPath, pdfPath string) error {
	var cmd *exec.Cmd
	switch kind {
	case "wkhtmltopdf":
		// wkhtmltopdf: 直接接受两个路径参数，HTML → PDF
		cmd = exec.Command(conv, htmlPath, pdfPath)
	case "chromium":
		// chromium headless:
		//   --headless               新版（>=112）默认新模式，无需 --headless=old
		//   --no-sandbox             容器/裸环境通常无 sandbox namespace
		//   --disable-gpu            无显卡环境避免崩溃
		//   --print-to-pdf=PATH file://...  打印到指定路径
		cmd = exec.Command(conv,
			"--headless",
			"--no-sandbox",
			"--disable-gpu",
			"--disable-dev-shm-usage",
			"--print-to-pdf="+pdfPath,
			"file://"+htmlPath,
		)
	default:
		return fmt.Errorf("unknown converter kind: %s", kind)
	}
	// CombinedOutput 让转换器的 stderr 错误信息进入日志，便于排查
	// PDF 转换设置 60 秒超时，避免外部工具卡死
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd = exec.CommandContext(ctx, cmd.Args[0], cmd.Args[1:]...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %w (stderr: %s)", kind, err, string(out))
	}
	return nil
}
