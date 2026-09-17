#!/usr/bin/env bash
set -e

echo "=== [Agate 本地自检] ==="

# 1. 核心架构文件完备性检查
for f in "main.go" "go.mod" "vendor/modules.txt" "AGENTS.md" "contexts/context.md"; do
    if [ ! -f "$f" ]; then
        echo "[FAIL] 缺少核心架构文件: $f"
        exit 1
    fi
done

# 2. 若存在 go 编译器，执行单元测试与主程序构建校验
if command -v go >/dev/null 2>&1; then
    echo "正在运行单元测试套件..."
    go test -mod=vendor -v ./...
    echo "正在验证主程序编译构建..."
    go build -mod=vendor -o /dev/null main.go
else
    echo "提示: 未检测到 Go 编译器，跳过单测调度，静态结构完整"
fi

echo "[PASS] 本地交付物校验通过，结构完备"
exit 0
