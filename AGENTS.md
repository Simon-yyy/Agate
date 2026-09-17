# 项目模块地图 (AGENTS.md)

## 一、 项目架构与工程定位
- **定位**：`agate`（Agent Gate）是一个零外部运行时依赖的单文件 Go 命令行工具，专为 AI 编程助手设计的工程护栏与安全门禁。
- **技术栈**：Go 1.21+ (标准库 + Cobra CLI + Pflag) / 零 CGO 静态编译。
- **运行平台**：Windows (amd64) / Linux (amd64) / macOS (amd64, arm64)。
- **核心契约**：单一事实源规约分发、Git 本地隐形隔离治理、双地图架构资产、两振熔断自检交付。

---

## 二、 关键入口与常用命令

### 1. 编译构建与归档
- **版本升级与归档**：`./scripts/bump.sh [patch|minor|major]`（支持加 `--release` 自动推送到 GitHub 发布）
- **本地开发构建**：`go build -mod=vendor -o agate main.go`
- **跨平台全量归档**：`./scripts/archive.sh`（生成 Windows/Linux/macOS 产物至 `bin/v<version>/`）
- **Windows 本地归档**：`.\scripts\archive.cmd` (调度 `scripts/archive.ps1`，生成全平台产物至 `bin/v<version>/` 的 windows/linux/darwin 目录)

### 2. 闭环自检与校验
- **执行本地自检**：`./verify.sh`（Linux/macOS）或 `verify.cmd`（Windows）
- **调度 CLI 门禁**：`agate verify`（优先调度脚本，无脚本时探测语言测试套件）
- **单元测试套件**：`go test -mod=vendor -v ./...`

### 3. 工程初始化与治理
- **工程护栏挂载**：`agate init`（可选 `-t all` 或 `-t codex,cursor,antigravity`）
- **技术拓扑回填**：`agate scan --write`（自动探测技术栈与端口，同步回填 AGENTS.md 与 contexts/context.md）
- **Git 私有隔离**：`agate isolate`（静默配置 `.git/info/exclude`）
- **门禁生命周期**：`agate hook [install|uninstall|status]`

### 4. 审查报告与物证导出
- **导出自包含 HTML 报告**：`agate export`（支持 `--open`, `--staged`, `-o <path>`）
- **浏览器极速预览**：`agate view`
- **自检联动报告生成**：`agate verify --report`

### 5. 跨 Agent 任务接力中枢
- **查看任务看板与状态**：`agate task status`
- **认领任务并加锁**：`agate task claim [--agent <name>] [--force]`
- **自检交接并生成物证**：`agate task handover [--to <agent>] [--note <text>]`
- **新 Agent 唤醒接棒**：`agate task resume`
- **终态交付归档与释放锁**：`agate task done [--note <text>]`

---

## 三、 模块拓扑与职责分工 (3-Hop Navigation)

### 1. 命令行交互层 (`cmd/`)
- [cmd/root.go](cmd/root.go)：CLI 根命令定义、版本号与全局通用配置。
- [cmd/init.go](cmd/init.go)：`agate init` 编排入口，按序驱动 .ignore、Git 隔离、规约分发、双地图骨架及本地 Hook 挂载。
- [cmd/verify.go](cmd/verify.go)：`agate verify` 自检引擎，执行 Phase 0 安全红线审计与测试套件调度。
- [cmd/scan.go](cmd/scan.go)：`agate scan` 静态技术栈嗅探器与端口提取器，支持双地图回填。
- [cmd/isolate.go](cmd/isolate.go)：`agate isolate` 命令行入口，独立触发 Git 隔离治理。
- [cmd/hook.go](cmd/hook.go)：`agate hook` 命令行入口，管理本地 Git 门禁挂载状态。
- [cmd/export.go](cmd/export.go)：`agate export` 命令行入口，导出自包含单文件 HTML 审查报告。
- [cmd/view.go](cmd/view.go)：`agate view` 命令行入口，在浏览器中打开最新生成的审查报告。
- [cmd/task.go](cmd/task.go)：`agate task` 命令行入口，跨 Agent 任务认领、状态机流转、交接与唤醒接棒。

### 2. 规约适配与分发 (`pkg/adapter/`)
- [pkg/adapter/adapter.go](pkg/adapter/adapter.go)：多 Agent 目标嗅探器与规约分发器。
  - 支持目标：Codex (`AGENTS.md` 受管区块)、Cursor (`.cursorrules`)、Antigravity (`.gemini/GEMINI.md`)、Claude Code (`CLAUDE.md`)、Windsurf (`.windsurfrules`)。
  - 支持优先加载用户全局规约，无全局时降级使用内置单一事实源。

### 3. Git 治理与安全门禁 (`pkg/git/`)
- [pkg/git/exclude.go](pkg/git/exclude.go)：基于 `.git/info/exclude` 的私有状态隐形隔离治理，支持 Git Worktree 动态寻径与规则幂等去重。
- [pkg/git/hook.go](pkg/git/hook.go)：管理本地 `pre-commit`（提纯自检）与 `pre-push`（防偷跑硬阻断）双物理钩子。
- [pkg/git/diff.go](pkg/git/diff.go)：提取分支、Commit 元数据与 Diff 差异行流。

### 4. 代码洁癖与安全审计 (`pkg/guard/`)
- [pkg/guard/preflight.go](pkg/guard/preflight.go)：Phase 0 静态合规拦截引擎，包含 7 大检查卡口：
  1. 私有密钥与草稿文件泄露拦截（.env, TASK.md 等）；
  2. 二进制与超大图片文件拦截；
  3. 机器绝对路径硬编码拦截（Windows/Unix 路径硬编码）；
  4. 临时残留与调试草稿拦截；
  5. 源码洁癖（Git 冲突标记、debugger 断点、TODO/占位符未完工拦截）；
  6. vendor 依赖目录纯净度；
  7. 3-Hop 双地图完备性校验（AGENTS.md 与 contexts/context.md 成对存在）。

### 5. 跨 Agent 任务接力与防撞车锁 (`pkg/relay/`)
- [pkg/relay/manifest.go](pkg/relay/manifest.go)：`TASK.md` 双模解析器（YAML Frontmatter 元数据 + 接力四要素 Markdown 正文）。
- [pkg/relay/lock.go](pkg/relay/lock.go)：`.ai-memory/locks/task.lock` 租约互斥锁，防止多 Agent 并发修改踩脚。
- [pkg/relay/relay.go](pkg/relay/relay.go)：认领（Claim）、安全自检交接（Handover）与唤醒接棒（Resume）编排总控。

### 6. 审查报告与物证导出 (`pkg/reporter/`)
- [pkg/reporter/report.go](pkg/reporter/report.go)：汇聚任务、审计结果与测试输出，编译自包含 HTML 报告。
- [pkg/reporter/browser.go](pkg/reporter/browser.go)：跨平台浏览器秒级打开实现。

### 7. 底层脚手架与跨平台工具 (`pkg/harness/`)
- [pkg/harness/fs.go](pkg/harness/fs.go)：原子性安全文件写入、跨平台物理拷贝与软链接平滑降级实现。
- [pkg/harness/scaffold.go](pkg/harness/scaffold.go)：初始化 `.ignore`、`AGENTS.md` 与 `contexts/context.md` 模板渲染与落盘。

### 8. 核心资产与模板资源 (`internal/templates/`)
- [internal/templates/SKILL.md](internal/templates/SKILL.md)：单一事实源通用规约（三层门禁、3-Hop 寻路协议、两阶段交互规则、接力协议与记忆防线）。
- [internal/templates/task.tpl](internal/templates/task.tpl)：升级版 YAML Frontmatter + 接力四要素任务看板模板。
- [internal/templates/review.html.tpl](internal/templates/review.html.tpl)：自包含 HTML 审查报告模板（零 CDN 外链依赖）。
- [internal/templates/agents.tpl](internal/templates/agents.tpl)：架构地图骨架模板。
- [internal/templates/context.tpl](internal/templates/context.tpl)：技术契约基线模板。
- [internal/templates/ignore.tpl](internal/templates/ignore.tpl)：防检索爆仓 .ignore 模板。
- [internal/templates/embed.go](internal/templates/embed.go)：Go 标准库 `//go:embed` 静态编译资源入口。

### 9. 智能体指纹与注册中心 (`pkg/agent/`)
- [pkg/agent/registry.go](pkg/agent/registry.go)：多 Agent 协同类型定义（Codex, Cursor, Antigravity, Claude, Windsurf）、自动化环境指纹嗅探器与类型注册表。

---

## 四、 3-Hop 寻路协议指南
AI 在处理本仓库任何编码与排查需求时，必须严格执行 3 跳寻径：
1. **第 1 跳 (全局拓扑)**：阅读 [AGENTS.md](AGENTS.md)，定位功能模块归属与关键文件；
2. **第 2 跳 (技术契约)**：阅读 [contexts/context.md](contexts/context.md)，核对运行环境、核心约束与业务契约；
3. **第 3 跳 (目标实现)**：直接进入对应子目录（如 `cmd/` 或 `pkg/guard/`）针对性检视源码，严禁全仓盲目扫描。
