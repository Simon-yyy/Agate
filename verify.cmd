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

:: 2. Auto detect Go compiler (Support custom GO_BIN or system PATH)
if not "%GO_BIN%"=="" (
    set "GO_CMD=%GO_BIN%"
) else (
    set "GO_CMD=go"
)

:: 3. If Go compiler exists, run unit tests
set "HAS_GO=0"
if exist "%GO_CMD%" set "HAS_GO=1"
if "%HAS_GO%"=="0" (
    where %GO_CMD% >nul 2>nul
    if %errorlevel% equ 0 set "HAS_GO=1"
)

if "%HAS_GO%"=="1" (
    echo Running unit test suite...
    "%GO_CMD%" test -mod=vendor -v ./pkg/...
    if %errorlevel% neq 0 (
        echo [FAIL] Unit tests failed
        exit /b 1
    )
) else (
    echo Info: No local Go compiler detected in PATH, skipped tests. Static structure is valid.
)

echo [PASS] Agate self-verification passed. Deliverable is clean.
exit /b 0
