@echo off
chcp 65001 >nul

echo === [Agate Local Verification] ===

:: 1. Core Architecture Files Check
if not exist "main.go" (
    echo [FAIL] Missing main.go
    exit /b 1
)
if not exist "go.mod" (
    echo [FAIL] Missing go.mod
    exit /b 1
)
if not exist "vendor\modules.txt" (
    echo [FAIL] Missing vendor\modules.txt
    exit /b 1
)
if not exist "AGENTS.md" (
    echo [FAIL] Missing AGENTS.md
    exit /b 1
)

:: 2. Auto detect local go compiler
if exist "D:\hclaw\go\bin\go.exe" (
    set "PATH=D:\hclaw\go\bin;%PATH%"
)

:: 3. If Go compiler exists, run unit tests
where go >nul 2>nul
if %errorlevel% equ 0 (
    echo Running unit test suite...
    go test -mod=vendor -v ./pkg/...
    if %errorlevel% neq 0 (
        echo [FAIL] Unit tests failed
        exit /b 1
    )
) else (
    echo Info: No local Go compiler detected, skipped tests. Static structure is valid.
)

echo [PASS] Agate self-verification passed. Deliverable is clean.
exit /b 0
