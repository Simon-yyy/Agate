#!/usr/bin/env bash
set -e

VERSION=$(grep 'version = ' cmd/root.go | sed -E 's/.*"([^"]+)".*/\1/')
[ -z "$VERSION" ] && VERSION="0.1.0"

TARGET_DIR="bin/v$VERSION"
mkdir -p "$TARGET_DIR"

echo "=== [Agate 跨平台多架构版本构建与归档] ==="
echo "构建版本: v$VERSION ..."

# 1. Windows x64
echo "[1/4] 构建 Windows x64 (agate.exe)..."
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -mod=vendor -ldflags="-s -w" -o "$TARGET_DIR/agate_v${VERSION}_windows_amd64.exe" main.go
cp -f "$TARGET_DIR/agate_v${VERSION}_windows_amd64.exe" "$TARGET_DIR/agate.exe"
cp -f "$TARGET_DIR/agate_v${VERSION}_windows_amd64.exe" "bin/agate_windows_amd64.exe"
cp -f "$TARGET_DIR/agate_v${VERSION}_windows_amd64.exe" "bin/agate.exe"

# 2. Linux x64
echo "[2/4] 构建 Linux x64 (agate)..."
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -mod=vendor -ldflags="-s -w" -o "$TARGET_DIR/agate_v${VERSION}_linux_amd64" main.go
cp -f "$TARGET_DIR/agate_v${VERSION}_linux_amd64" "$TARGET_DIR/agate"
cp -f "$TARGET_DIR/agate_v${VERSION}_linux_amd64" "bin/agate_linux_amd64"
cp -f "$TARGET_DIR/agate_v${VERSION}_linux_amd64" "bin/agate"

# 3. macOS Intel x64
echo "[3/4] 构建 macOS x64 (agate_darwin_amd64)..."
CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -mod=vendor -ldflags="-s -w" -o "$TARGET_DIR/agate_v${VERSION}_darwin_amd64" main.go
cp -f "$TARGET_DIR/agate_v${VERSION}_darwin_amd64" "bin/agate_darwin_amd64"

# 4. macOS Apple Silicon ARM64
echo "[4/4] 构建 macOS ARM64 (agate_darwin_arm64)..."
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -mod=vendor -ldflags="-s -w" -o "$TARGET_DIR/agate_v${VERSION}_darwin_arm64" main.go
cp -f "$TARGET_DIR/agate_v${VERSION}_darwin_arm64" "bin/agate_darwin_arm64"

chmod +x bin/agate* "$TARGET_DIR"/agate* 2>/dev/null || true

echo "=== 归档完成，所有平台可执行文件已生成至 bin/ 与 $TARGET_DIR/ ==="
