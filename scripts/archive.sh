#!/usr/bin/env bash
set -e

VERSION=$(grep 'version = ' cmd/root.go | sed -E 's/.*"([^"]+)".*/\1/')
[ -z "$VERSION" ] && VERSION="0.1.0"

TARGET_DIR="bin/v$VERSION"
mkdir -p "$TARGET_DIR/windows" "$TARGET_DIR/linux" "$TARGET_DIR/darwin"

echo "=== [Agate 跨平台版本构建与归档] ==="
echo "归档目标: $TARGET_DIR ..."

# 1. Windows x64
echo "[1/3] 构建 Windows x64 ($TARGET_DIR/windows/agate.exe)..."
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -mod=vendor -ldflags="-s -w" -o "$TARGET_DIR/windows/agate.exe" main.go

# 2. Linux x64
echo "[2/3] 构建 Linux x64 ($TARGET_DIR/linux/agate)..."
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -mod=vendor -ldflags="-s -w" -o "$TARGET_DIR/linux/agate" main.go

# 3. macOS Intel & ARM64
echo "[3/3] 构建 macOS ($TARGET_DIR/darwin/)..."
CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -mod=vendor -ldflags="-s -w" -o "$TARGET_DIR/darwin/agate_amd64" main.go
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -mod=vendor -ldflags="-s -w" -o "$TARGET_DIR/darwin/agate_arm64" main.go

chmod +x "$TARGET_DIR/linux"/* "$TARGET_DIR/darwin"/* 2>/dev/null || true

echo "=== 归档完成，产物为纯净单入口文件: ==="
echo "  - Windows: $TARGET_DIR/windows/agate.exe"
echo "  - Linux:   $TARGET_DIR/linux/agate"
echo "  - macOS:   $TARGET_DIR/darwin/ (agate_amd64 / agate_arm64)"
