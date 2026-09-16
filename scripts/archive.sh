#!/usr/bin/env bash
set -e

VERSION=$(grep 'version = ' cmd/root.go | sed -E 's/.*"([^"]+)".*/\1/')
[ -z "$VERSION" ] && VERSION="0.1.0"

mkdir -p bin/windows bin/linux bin/darwin
mkdir -p "bin/v$VERSION/windows" "bin/v$VERSION/linux" "bin/v$VERSION/darwin"

echo "=== [Agate 跨平台多架构版本构建与归档] ==="
echo "构建版本: v$VERSION ..."

# 1. Windows x64
echo "[1/4] 构建 Windows x64 (bin/windows/)..."
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -mod=vendor -ldflags="-s -w" -o "bin/v$VERSION/windows/agate_v${VERSION}_windows_amd64.exe" main.go
cp -f "bin/v$VERSION/windows/agate_v${VERSION}_windows_amd64.exe" "bin/v$VERSION/windows/agate.exe"
cp -f "bin/v$VERSION/windows/agate_v${VERSION}_windows_amd64.exe" "bin/windows/agate_windows_amd64.exe"
cp -f "bin/v$VERSION/windows/agate_v${VERSION}_windows_amd64.exe" "bin/windows/agate.exe"

# 2. Linux x64
echo "[2/4] 构建 Linux x64 (bin/linux/)..."
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -mod=vendor -ldflags="-s -w" -o "bin/v$VERSION/linux/agate_v${VERSION}_linux_amd64" main.go
cp -f "bin/v$VERSION/linux/agate_v${VERSION}_linux_amd64" "bin/v$VERSION/linux/agate"
cp -f "bin/v$VERSION/linux/agate_v${VERSION}_linux_amd64" "bin/linux/agate_linux_amd64"
cp -f "bin/v$VERSION/linux/agate_v${VERSION}_linux_amd64" "bin/linux/agate"

# 3. macOS Intel x64
echo "[3/4] 构建 macOS x64 (bin/darwin/)..."
CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -mod=vendor -ldflags="-s -w" -o "bin/v$VERSION/darwin/agate_v${VERSION}_darwin_amd64" main.go
cp -f "bin/v$VERSION/darwin/agate_v${VERSION}_darwin_amd64" "bin/darwin/agate_darwin_amd64"

# 4. macOS Apple Silicon ARM64
echo "[4/4] 构建 macOS ARM64 (bin/darwin/)..."
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -mod=vendor -ldflags="-s -w" -o "bin/v$VERSION/darwin/agate_v${VERSION}_darwin_arm64" main.go
cp -f "bin/v$VERSION/darwin/agate_v${VERSION}_darwin_arm64" "bin/darwin/agate_darwin_arm64"

chmod +x bin/linux/* bin/darwin/* "bin/v$VERSION/linux"/* "bin/v$VERSION/darwin"/* 2>/dev/null || true

echo "=== 归档完成，所有平台可执行文件已分类存放至 bin/windows/、bin/linux/、bin/darwin/ ==="
