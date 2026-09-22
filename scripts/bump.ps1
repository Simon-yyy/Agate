# scripts/bump.ps1 - Agate semantic version bump and release script (PowerShell)
param(
    [string]$Type = "patch",
    [switch]$Release
)

$ErrorActionPreference = "Stop"

$rootFile = "cmd/root.go"
if (!(Test-Path $rootFile)) {
    Write-Error "cmd/root.go not found. Please run this script from the project root."
}

# 1. Extract current version
$currentVersion = "0.1.0"
$lines = Get-Content -Path $rootFile
foreach ($line in $lines) {
    if ($line.Contains("version = ")) {
        $start = $line.IndexOf('"') + 1
        $end = $line.LastIndexOf('"')
        if ($start -gt 0 -and $end -gt $start) {
            $currentVersion = $line.Substring($start, $end - $start)
            break
        }
    }
}

# 2. Calculate target version
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
            Write-Error "Invalid bump type: $Type (supported: patch, minor, major or explicit version like 0.2.5)"
        }
    }
}

Write-Host "=== [Agate Version Bump] ===" -ForegroundColor Cyan
Write-Host "Current version: v$currentVersion" -ForegroundColor Gray
Write-Host "Target version:  v$newVersion" -ForegroundColor Green

# 3. Update cmd/root.go
$resolvedPath = (Resolve-Path $rootFile).Path
$utf8NoBom = New-Object System.Text.UTF8Encoding($false)
$content = [System.IO.File]::ReadAllText($resolvedPath, [System.Text.Encoding]::UTF8)
$newContent = $content.Replace("version = `"$currentVersion`"", "version = `"$newVersion`"")
[System.IO.File]::WriteAllText($resolvedPath, $newContent, $utf8NoBom)
Write-Host "[+] Updated $rootFile to version $newVersion" -ForegroundColor Green

# 4. Trigger cross-platform build and packaging
Write-Host "[+] Building cross-platform archives..." -ForegroundColor Cyan
& powershell.exe -NoProfile -ExecutionPolicy Bypass -File "scripts/archive.ps1"
if ($LASTEXITCODE -ne 0) {
    Write-Error "Archive packaging failed with exit code: $LASTEXITCODE"
}

Write-Host "`n[PASS] Version bump and archive completed: v$newVersion" -ForegroundColor Green

if ($Release) {
    Write-Host "[+] Committing and pushing Release tag (v$newVersion)..." -ForegroundColor Cyan
    git add $rootFile scripts/
    git commit -m "chore(release): bump version to v$newVersion"
    git tag -a "v$newVersion" -m "release: v$newVersion"
    $env:ALLOW_AUTOMATED_PUSH = "1"
    git push origin main
    git push origin "v$newVersion"
    Remove-Item Env:\ALLOW_AUTOMATED_PUSH -ErrorAction SilentlyContinue
    Write-Host "[PASS] Release tag v$newVersion pushed successfully!" -ForegroundColor Green
} else {
    Write-Host "[INFO] To publish this release to GitHub Releases, run:" -ForegroundColor Yellow
    Write-Host "   git add cmd/root.go scripts/" -ForegroundColor Gray
    Write-Host "   git commit -m `"chore(release): bump version to v$newVersion`"" -ForegroundColor Gray
    Write-Host "   git tag -a `"v$newVersion`" -m `"release: v$newVersion`"" -ForegroundColor Gray
    Write-Host '   $env:ALLOW_AUTOMATED_PUSH=1; git push origin main; git push origin v' -NoNewline -ForegroundColor Gray
    Write-Host $newVersion -ForegroundColor Gray
}
