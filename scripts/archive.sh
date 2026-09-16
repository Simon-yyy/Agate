#!/usr/bin/env bash
set -e

VERSION=$(grep 'version = ' cmd/root.go | sed -E 's/.*"([^"]+)".*/\1/')
[ -z "$VERSION" ] && VERSION="0.1.0"

TARGET_DIR="bin/v$VERSION"
mkdir -p "$TARGET_DIR/windows" "$TARGET_DIR/linux" "$TARGET_DIR/darwin"

echo "=== [Agate 跨平台版本构建与归档] ==="
echo "归档目标: $TARGET_DIR ..."

# 1. Windows x64
echo "[1/3] 构建 Windows x64 ($TARGET_DIR/windows/)..."
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -mod=vendor -ldflags="-s -w" -o "$TARGET_DIR/windows/agate_v${VERSION}_windows_amd64.exe" main.go
cp -f "$TARGET_DIR/windows/agate_v${VERSION}_windows_amd64.exe" "$TARGET_DIR/windows/agate.exe"

# 2. Linux x64
echo "[2/3] 构建 Linux x64 ($TARGET_DIR/linux/)..."
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -mod=vendor -ldflags="-s -w" -o "$TARGET_DIR/linux/agate_v${VERSION}_linux_amd64" main.go
cp -f "$TARGET_DIR/linux/agate_v${VERSION}_linux_amd64" "$TARGET_DIR/linux/agate"

# 3. macOS Intel & ARM64
echo "[3/3] 构建 macOS ($TARGET_DIR/darwin/)..."
CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -mod=vendor -ldflags="-s -w" -o "$TARGET_DIR/darwin/agate_v${VERSION}_darwin_amd64" main.go
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -mod=vendor -ldflags="-s -w" -o "$TARGET_DIR/darwin/agate_v${VERSION}_darwin_arm64" main.go

chmod +x "$TARGET_DIR/linux"/* "$TARGET_DIR/darwin"/* 2>/dev/null || true

echo "=== 归档完成，所有产物已集中归档至 $TARGET_DIR/ ==="
