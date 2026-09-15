@echo off
chcp 65001 >nul

echo === [Agate 本地自检] ===

:: 1. 核心架构文件完备性检查
if not exist "main.go" (
    echo [FAIL] 缺少核心架构文件: main.go
    exit /b 1
)
if not exist "go.mod" (
    echo [FAIL] 缺少核心架构文件: go.mod
    exit /b 1
)
if not exist "vendor\modules.txt" (
    echo [FAIL] 缺少核心架构依赖: vendor\modules.txt
    exit /b 1
)
if not exist "AGENTS.md" (
    echo [FAIL] 缺少导航地图文件: AGENTS.md
    exit /b 1
)

:: 2. Auto detect Go compiler (Support custom GO_BIN, system PATH or GOROOT)
set "HAS_GO=0"
if not "%GO_BIN%"=="" (
    set "GO_CMD=%GO_BIN%"
    set "HAS_GO=1"
) else (
    where go >nul 2>&1
    if not errorlevel 1 (
        set "GO_CMD=go"
        set "HAS_GO=1"
    ) else if not "%GOROOT%"=="" (
        if exist "%GOROOT%\bin\go.exe" (
            set "GO_CMD=%GOROOT%\bin\go.exe"
            set "HAS_GO=1"
        )
    )
)

if "%HAS_GO%"=="1" (
    echo 正在运行单元测试套件...
    "%GO_CMD%" test -mod=vendor -v ./pkg/...
    if %errorlevel% neq 0 (
        echo [FAIL] 单元测试执行失败
        exit /b 1
    )
) else (
    echo 提示: 未检测到 Go 编译器，跳过单测调度，静态结构完整
)

echo [PASS] 本地交付物校验通过，结构完备
exit /b 0
