# scripts/archive.ps1 - Agate 自动化版本构建与归档工具
$ErrorActionPreference = "Stop"

# 1. 探测可用 Go 编译器
$goCmd = "go"
if ($env:GO_BIN -and (Test-Path $env:GO_BIN)) {
    $goCmd = $env:GO_BIN
} elseif (Get-Command go -ErrorAction SilentlyContinue) {
    $goCmd = "go"
} elseif ($env:GOROOT -and (Test-Path "$env:GOROOT\bin\go.exe")) {
    $goCmd = "$env:GOROOT\bin\go.exe"
}

# 2. 解析 cmd/root.go 中的版本号
$version = "0.1.0"
$lines = Get-Content -Path "cmd/root.go"
foreach ($line in $lines) {
    if ($line -match 'version\s*=\s*"([^"]+)"') {
        $version = $matches[1]
        break
    }
}

$winVerDir = "bin/v$version/windows"

if (!(Test-Path $winVerDir)) {
    New-Item -ItemType Directory -Force -Path $winVerDir | Out-Null
}

Write-Host "=== [Agate Windows 版本构建与归档] ===" -ForegroundColor Cyan
Write-Host "正在构建版本: v$version ..." -ForegroundColor Gray

& $goCmd build -mod=vendor -ldflags="-s -w" -o "$winVerDir/agate_v${version}_windows_amd64.exe" main.go
Copy-Item -Force "$winVerDir/agate_v${version}_windows_amd64.exe" "$winVerDir/agate.exe"

Write-Host "[+] 成功归档至: $winVerDir/agate.exe" -ForegroundColor Green
Write-Host "=== 归档完成 ===" -ForegroundColor Cyan
