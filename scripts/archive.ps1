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
$version = "0.2.1"
$lines = Get-Content -Path "cmd/root.go"
foreach ($line in $lines) {
    if ($line.Contains("version = ")) {
        $start = $line.IndexOf('"') + 1
        $end = $line.LastIndexOf('"')
        if ($start -gt 0 -and $end -gt $start) {
            $version = $line.Substring($start, $end - $start)
            break
        }
    }
}

$baseDir = "bin/v$version"
$winVerDir = "$baseDir/windows"
$linuxVerDir = "$baseDir/linux"
$darwinVerDir = "$baseDir/darwin"

if (!(Test-Path $winVerDir)) { New-Item -ItemType Directory -Force -Path $winVerDir | Out-Null }
if (!(Test-Path $linuxVerDir)) { New-Item -ItemType Directory -Force -Path $linuxVerDir | Out-Null }
if (!(Test-Path $darwinVerDir)) { New-Item -ItemType Directory -Force -Path $darwinVerDir | Out-Null }

Write-Host "=== [Agate 跨平台版本构建与归档] ===" -ForegroundColor Cyan
Write-Host "归档目标: $baseDir ..." -ForegroundColor Gray

# 1. Windows amd64
Write-Host "[1/3] 构建 Windows amd64 ($winVerDir/agate.exe)..." -ForegroundColor Gray
$env:CGO_ENABLED = "0"
$env:GOOS = "windows"
$env:GOARCH = "amd64"
& $goCmd build -mod=vendor -ldflags="-s -w" -o "$winVerDir/agate.exe" main.go
if ($LASTEXITCODE -ne 0) {
    throw "Windows amd64 构建失败，退出码: $LASTEXITCODE"
}

# 2. Linux amd64
Write-Host "[2/3] 构建 Linux amd64 ($linuxVerDir/agate)..." -ForegroundColor Gray
$env:CGO_ENABLED = "0"
$env:GOOS = "linux"
$env:GOARCH = "amd64"
& $goCmd build -mod=vendor -ldflags="-s -w" -o "$linuxVerDir/agate" main.go
if ($LASTEXITCODE -ne 0) {
    throw "Linux amd64 构建失败，退出码: $LASTEXITCODE"
}

# 3. macOS Intel & ARM64
Write-Host "[3/3] 构建 macOS Intel & ARM64 ($darwinVerDir/)..." -ForegroundColor Gray
$env:CGO_ENABLED = "0"
$env:GOOS = "darwin"
$env:GOARCH = "amd64"
& $goCmd build -mod=vendor -ldflags="-s -w" -o "$darwinVerDir/agate_amd64" main.go
if ($LASTEXITCODE -ne 0) {
    throw "macOS amd64 构建失败，退出码: $LASTEXITCODE"
}

$env:CGO_ENABLED = "0"
$env:GOOS = "darwin"
$env:GOARCH = "arm64"
& $goCmd build -mod=vendor -ldflags="-s -w" -o "$darwinVerDir/agate_arm64" main.go
if ($LASTEXITCODE -ne 0) {
    throw "macOS arm64 构建失败，退出码: $LASTEXITCODE"
}

# 清理环境变量
Remove-Item Env:\CGO_ENABLED -ErrorAction SilentlyContinue
Remove-Item Env:\GOOS -ErrorAction SilentlyContinue
Remove-Item Env:\GOARCH -ErrorAction SilentlyContinue

# 同步刷新根目录 bin/agate.exe
Copy-Item -Path "$winVerDir/agate.exe" -Destination "bin/agate.exe" -Force

Write-Host "`n[+] 成功归档各平台产物:" -ForegroundColor Green
Write-Host "  - Windows: $winVerDir/agate.exe" -ForegroundColor Gray
Write-Host "  - Linux:   $linuxVerDir/agate" -ForegroundColor Gray
Write-Host "  - macOS:   $darwinVerDir/ (agate_amd64 / agate_arm64)" -ForegroundColor Gray
Write-Host "=== 归档完成 ===" -ForegroundColor Cyan
