#!/bin/bash

# 跨平台构建脚本
# 同时构建适用于 Linux 和 Windows 的二进制文件

echo "=== LinDiag-Agent 跨平台构建 ==="

# 创建输出目录
mkdir -p output

# 构建 Linux 版本
echo "\n构建 Linux 版本..."
echo "正在构建 x86_64 静态二进制..."
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -ldflags '-s -w -extldflags "-static"' -o output/lindiag-agent_amd64_linux cmd/agent/main.go
if [ $? -eq 0 ]; then
    echo "✅ Linux x86_64 版本构建成功"
else
    echo "❌ Linux x86_64 版本构建失败"
    exit 1
fi
echo "正在构建 arm64 静态二进制..."
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -a -ldflags '-s -w -extldflags "-static"' -o output/lindiag-agent_arm64_linux cmd/agent/main.go
if [ $? -eq 0 ]; then
    echo "✅ Linux arm64 版本构建成功"
else
    echo "❌ Linux arm64 版本构建失败"
    exit 1
fi

# 构建 Windows 版本
echo "\n构建 Windows 版本..."
# 直接构建 Windows 版本，使用条件编译
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o output/lindiag-agent.exe cmd/agent/main.go
if [ $? -eq 0 ]; then
    echo "✅ Windows 版本构建成功"
else
    echo "❌ Windows 版本构建失败"
    exit 1
fi

# 显示构建结果
echo "\n=== 构建结果 ==="
echo "构建产物位于 output 目录："
ls -la output/

echo "\n✅ 跨平台构建完成"
echo "\n使用方法："
echo "  Linux x86_64: ./output/lindiag-agent_amd64_linux"
echo "  Linux arm64: ./output/lindiag-agent_arm64_linux"
echo "  Windows: ./output/lindiag-agent.exe"
