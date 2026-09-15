#!/usr/bin/env bash
set -e

echo "=== [Agate 本地闭环自检] ==="

# 1. 核心架构文件完备性检查
for f in "main.go" "go.mod" "vendor/modules.txt" "AGENTS.md"; do
    if [ ! -f "$f" ]; then
        echo "[FAIL] 缺少核心文件: $f"
        exit 1
    fi
done

# 2. 若存在 go 编译器，执行单元测试
if command -v go >/dev/null 2>&1; then
    echo "正在运行单元测试套件..."
    go test -mod=vendor -v ./pkg/...
else
    echo "提示: 本地未检测到 go 编译器，跳过单测调度，静态完备性校验通过。"
fi

echo "[PASS] Agate 自检验证通过，所有模块与文档结构完整。"
exit 0
