# 寻路导航 (AGENTS.md)

## 1. 核心架构与技术栈
- 技术栈：Go 1.21+ / spf13/cobra CLI 框架 / GoReleaser 自动化跨平台构建
- 运行环境：Windows / Linux / macOS (单二进制零运行时依赖)

## 2. 常用构建与验证命令
- 校验：`agent-verify` (或 `agate verify`)
- 构建（默认自动引用 vendor 离线构建）：`go build -mod=vendor -o agate main.go`
- 测试：`go test -mod=vendor -v ./...`
- 打包发布：`goreleaser release --snapshot --clean`

## 3. 核心目录分工 (3-Hop Rule)
- `cmd/`：Cobra CLI 命令定义（root, init, verify, isolate, hook, scan）
- `pkg/adapter/`：多 Agent 目标规约分发器与单测（adapter_test.go）
- `pkg/git/`：Git 私有排除隔离与 Hook 拦截及单测（exclude_test.go）
- `pkg/harness/`：架构地图骨架生成与跨平台文件工具及单测（fs_test.go）
- `internal/templates/`：内置嵌入式模板资源（go:embed: SKILL.md, ignore.tpl, agents.tpl）
- `vendor/`：已完整打包的第三方依赖源码包（cobra, pflag 等，免联网开箱即用）
- `verify.cmd` / `verify.sh`：本地原生闭环自检调度脚本
- `docs/`：系统架构白皮书 (DESIGN.md) 与协议标准手册 (SPEC.md)
- `contexts/context.md`：工程业务概念地图与协议基线



