# scripts/bump.ps1 - Agate 语义化版本升级与发布脚本 (PowerShell)
param(
    [string]$Type = "patch",
    [switch]$Release
)

$ErrorActionPreference = "Stop"

$rootFile = "cmd/root.go"
if (!(Test-Path $rootFile)) {
    Write-Error "未找到 $rootFile，请在项目根目录下执行"
}

# 1. 提取当前版本号
$currentVersion = "0.1.0"
$lines = Get-Content -Path $rootFile
foreach ($line in $lines) {
    if ($line -match 'version\s*=\s*"([^"]+)"') {
        $currentVersion = $matches[1]
        break
    }
}

# 2. 计算新版本号
$parts = $currentVersion.Split('.')
[int]$major = if ($parts.Length -ge 1) { [int]$parts[0] } else { 0 }
[int]$minor = if ($parts.Length -ge 2) { [int]$parts[1] } else { 0 }
[int]$patch = if ($parts.Length -ge 3) { [int]$parts[2] } else { 0 }

switch ($Type.ToLower()) {
    "patch" {
        $patch++
        $newVersion = "$major.$minor.$patch"
    }
    "minor" {
        $minor++
        $patch = 0
        $newVersion = "$major.$minor.$patch"
    }
    "major" {
        $major++
        $minor = 0
        $patch = 0
        $newVersion = "$major.$minor.$patch"
    }
    default {
        $cleanVer = $Type.TrimStart('v')
        if ($cleanVer -match '^\d+\.\d+\.\d+') {
            $newVersion = $cleanVer
        } else {
            Write-Error "无效的版本升级类型: $Type (支持: patch, minor, major 或指定版本号如 0.2.0)"
        }
    }
}

Write-Host "=== [Agate 版本升级] ===" -ForegroundColor Cyan
Write-Host "当前版本: v$currentVersion" -ForegroundColor Gray
Write-Host "升级目标: v$newVersion" -ForegroundColor Green

# 3. 回填更新 cmd/root.go
$content = Get-Content -Path $rootFile -Raw
$newContent = $content -replace "version = `"$currentVersion`"", "version = `"$newVersion`""
Set-Content -Path $rootFile -Value $newContent -NoNewline
Write-Host "[+] 已更新 $rootFile 版本号至 $newVersion" -ForegroundColor Green

# 4. 执行构建归档
Write-Host "[+] 正在触发 Windows 平台构建与归档..." -ForegroundColor Cyan
& powershell.exe -NoProfile -ExecutionPolicy Bypass -File "scripts/archive.ps1"

Write-Host "`n[✓] 本地版本升级完毕: v$newVersion" -ForegroundColor Green

if ($Release) {
    Write-Host "[+] 正在提交并推送 Release 标签 (v$newVersion)..." -ForegroundColor Cyan
    git add $rootFile scripts/
    git commit -m "chore(release): bump version to v$newVersion"
    git tag -a "v$newVersion" -m "release: v$newVersion"
    $env:ALLOW_AUTOMATED_PUSH = "1"
    git push origin main
    git push origin "v$newVersion"
    Write-Host "[✓] Release 标签 v$newVersion 已推送，GitHub Actions 已自动触发云端发布！" -ForegroundColor Green
} else {
    Write-Host "💡 提示：如需将此版本自动发布至 GitHub Releases，可执行：" -ForegroundColor Yellow
    Write-Host "   git add cmd/root.go; git commit -m `"chore(release): bump version to v$newVersion`"" -ForegroundColor Gray
    Write-Host "   git tag -a `"v$newVersion`" -m `"release: v$newVersion`"" -ForegroundColor Gray
    Write-Host "   `$env:ALLOW_AUTOMATED_PUSH=`"1`"; git push origin main; git push origin `"v$newVersion`"" -ForegroundColor Gray
    Write-Host "   或下次直接运行: .\scripts\bump.cmd -Release 自动完成全流程发布。" -ForegroundColor Yellow
}
