# 项目模块地图 (MAP.md)

## 一、 项目架构与工程定位

- **定位**：`agate`（Agent Gate）是一个零外部运行时依赖的单文件 Go 命令行工具，专为 AI 编程助手设计的工程护栏与安全门禁。
- **技术栈**：Go 1.21+ (标准库 + Cobra CLI + Pflag) / 零 CGO 静态编译。
- **运行平台**：Windows (amd64) / Linux (amd64) / macOS (amd64, arm64)。
- **核心契约**：单一事实源规约分发、Git 本地隐形隔离治理、双地图架构资产、两振熔断自检交付。

---

## 二、 关键入口与常用命令

### 1. 编译构建与归档

- **版本升级与归档 (Linux/macOS)**：`./scripts/bump.sh [patch|minor|major|<version>]`（支持加 `--release` 自动推送到 GitHub 发布）
- **版本升级与归档 (Windows)**：`.\scripts\bump.ps1 -Bump [patch|minor|major|<version>] [-Release]`
- **本地开发构建**：`go build -mod=vendor -o agate main.go`
- **跨平台全量归档**：`./scripts/archive.sh`（生成 Windows/Linux/macOS 产物至 `bin/v<version>/`）
- **Windows 本地归档**：`.\scripts\archive.cmd` (调度 `scripts/archive.ps1`，生成全平台产物至 `bin/v<version>/` 的 windows/linux/darwin 目录)

### 2. 闭环自检与校验

- **执行本地自检**：`./verify.sh`（Linux/macOS）或 `verify.cmd`（Windows）
- **调度 CLI 门禁**：`agate verify [--staged] [--strict] [--skip-guard] [--report]`（优先调度脚本，无脚本时探测语言测试套件）
- **熔断人工恢复**：`agate verify --resume-after-break`（由人类确认已排查修复后解除两振硬熔断并重试验证）
- **CI 远程门禁**：`agate ci verify`（强制严格模式并生成审查报告，供 GitHub Actions/GitLab CI 调用）
- **单元测试套件**：`go test -mod=vendor -v ./...`

### 3. 工程初始化与治理

- **工程护栏挂载**：`agate init`（可选 `-t all` 或 `-t codex,cursor,antigravity`；选择顺序为显式目标、项目默认装配集合、当前 Agent、已有入口；无法确定时要求显式选择）
- **分发计划预览**：`agate init -t <工具> --dry-run`（只读展示创建、更新、跳过与冲突，列出已有入口及潜在重叠读取）；Git 已跟踪规则需要 `--allow-tracked` 明确允许更新。
- **开放工具档案管理**：`agate profile [list|add <name>]`（支持 `--path`、`--format markdown|frontmatter-md|pointer`、`--env`、`--term-program`、`--reads`、`--frontmatter`、`--global`、`--local`，免改源码扩展任意新兴 AI 编程工具）
- **多工具独立窗口视图**：`agate workspace [plan|apply|restore|open <target>]`（在 `.ai-memory/workspaces/<target>.code-workspace` 生成窗口独立规则过滤视图，零侵入 `.vscode/settings.json`）
- **技术拓扑回填**：`agate scan --write`（自动探测技术栈与端口，同步回填 MAP.md 与 contexts/context.md）
- **三层策略配置**：支持 `~/.agate/config.toml`（个人全局）+ `.agate/config.toml`（团队共享）+ `.agate/config.local.toml`（本机私有覆写，安全门禁遵循单调收紧只升不降）
- **Git 私有隔离**：`agate isolate`（静默配置 `.git/info/exclude`）
- **门禁生命周期**：`agate hook [install|uninstall|status]`

### 4. 审查报告与物证导出

- **导出自包含 HTML 报告**：`agate export`（支持 `--open`, `--staged`, `-o <path>`）
- **浏览器极速预览**：`agate view`
- **自检联动报告生成**：`agate verify --report`

### 5. 跨 Agent 任务接力中枢

- **查看任务看板与状态**：`agate task status`
- **认领任务并加锁**：`agate task claim [--agent <name>] [--role dev|test|ops] [--force]`
- **自检交接并生成物证**：`agate task handover [--to <agent>] [--role dev|test|ops] [--next-role dev|test|ops] [--note <text>]`
- **新 Agent 唤醒接棒**：`agate task resume [--role dev|test|ops]`
- **终态交付归档与释放锁**：`agate task done [--role dev|test|ops] [--note <text>]`

---

## 三、 模块拓扑与职责分工 (3-Hop Navigation)

### 1. 命令行交互层 (`cmd/`)

- [cmd/root.go](cmd/root.go)：CLI 根命令定义、版本号与全局通用配置。
- [cmd/init.go](cmd/init.go)：`agate init` 编排入口，先预览并校验规则计划，再装配受管入口、双地图、协同资产和 Git 本地门禁。
- [cmd/verify.go](cmd/verify.go)：`agate verify` 自检引擎，执行 Phase 0 安全红线审计与测试套件调度。
- [cmd/ci.go](cmd/ci.go)：`agate ci verify` 远程门禁入口，固定启用严格校验与 HTML 物证报告。
- [cmd/scan.go](cmd/scan.go)：`agate scan` 静态技术栈嗅探器与端口提取器，支持双地图回填。
- [cmd/isolate.go](cmd/isolate.go)：`agate isolate` 命令行入口，独立触发 Git 隔离治理。
- [cmd/hook.go](cmd/hook.go)：`agate hook` 命令行入口，管理本地 Git 门禁挂载状态。
- [cmd/export.go](cmd/export.go)：`agate export` 命令行入口，导出自包含单文件 HTML 审查报告。
- [cmd/view.go](cmd/view.go)：`agate view` 命令行入口，在浏览器中打开最新生成的审查报告。
- [cmd/task.go](cmd/task.go)：`agate task` 命令行入口，跨 Agent 与多角色（`dev`/`test`/`ops`）任务认领、状态机流转、交接与唤醒接棒（`handover` 与 `done` 统一联动 `guard.strict` 配置、Phase 0 静态审计与两振熔断）。
- [cmd/profile.go](cmd/profile.go)：`agate profile` 命令行入口，列出与注册内置/自定义 AI 工具档案（支持项目级、本机私有与个人全局三层配置落盘）。
- [cmd/workspace.go](cmd/workspace.go)：`agate workspace` 命令行入口，预览、生成、恢复或直接启动按工具隔离的 `.code-workspace` 独立视图窗口。

### 2. 规约适配与分发 (`pkg/adapter/`)

- [pkg/adapter/adapter.go](pkg/adapter/adapter.go)：多 Agent 目标嗅探器与规约分发器。
  - 支持内置目标：Codex (`AGENTS.md` 受管区块)、Cursor (`.cursorrules`)、Antigravity (`.gemini/GEMINI.md`)、Claude Code (`CLAUDE.md`)、Windsurf (`.windsurfrules`)，并开放支持任意声明式自定义工具档案。
  - 支持优先加载用户全局规约，无全局时降级使用内置单一事实源。
- [pkg/adapter/plan.go](pkg/adapter/plan.go)：只读分发计划与执行、三大规则渲染范式（`markdown` 受管区块合并、`frontmatter-md` 保留或生成 YAML Frontmatter 的 `.mdc` 规则、`pointer` 轻量寻路引用引导并内嵌规则哈希摘要）、Git 跟踪保护（含大小写不敏感文件系统下基于 Git 索引与 `os.SameFile` 的物理文件校验）、路径与符号链接检查、过期快照检查、分发写入锁、双向重叠读取及正文版本诊断。诊断不等于客户端加载证明。

### 3. Git 治理与安全门禁 (`pkg/git/`)

- [pkg/git/exclude.go](pkg/git/exclude.go)：基于 `.git/info/exclude` 的私有状态隐形隔离治理，支持 Git Worktree 动态寻径、规则幂等去重与原子写入。
- [pkg/git/hook.go](pkg/git/hook.go)：管理本地 `pre-commit`（提纯自检）与 `pre-push`（防偷跑硬阻断，覆盖全量 Agent 与 CI 环境指纹）双物理钩子。
- [pkg/git/diff.go](pkg/git/diff.go)：提取分支（兼容尚无 Commit 的 Unborn HEAD 空仓库回退）、Commit 元数据与 UTF-8 路径 Diff 差异行流。

### 4. 代码洁癖与安全审计 (`pkg/guard/`)

- [pkg/guard/preflight.go](pkg/guard/preflight.go)：Phase 0 静态合规拦截引擎（支持工作区全量审计与 `--staged` 暂存区 Blob 增量审计对齐，基于 `--directory` 区分整目录忽略防父目录误判，并对违规代码行在构造期执行敏感信息脱敏），包含 7 大检查卡口：
  1. 私有密钥与草稿文件泄露拦截（`.env`, `.env.*`, `*.agate.bak`, `TASK.md`, `MEMORY.md` 等）；
  2. 二进制与超大图片文件拦截；
  3. 机器绝对路径硬编码拦截（Windows/Unix 路径硬编码）；
  4. 临时残留与调试草稿拦截；
  5. 源码洁癖（Git 冲突标记、debugger 断点、TODO/占位符未完工拦截）；
  6. vendor 依赖目录纯净度；
  7. 3-Hop 双地图完备性校验（MAP.md 与 contexts/context.md 成对存在）。

### 5. 跨 Agent 任务接力与防撞车锁 (`pkg/relay/`)

- [pkg/relay/manifest.go](pkg/relay/manifest.go)：`TASK.md` 双模解析器（YAML Frontmatter 元数据含 `current_role`、`next_role` 多角色字段与 `snapshot_digest` 工作区快照指纹 + 接力四要素 Markdown 正文）。
- [pkg/relay/lock.go](pkg/relay/lock.go)：`.ai-memory/locks/task.lock` 租约互斥锁（含 `role` 角色标识与 `session_id` 同工具多窗口会话隔离）与 `.transition` 过渡守护锁（覆盖加锁、释放锁及过期过渡锁原子重命名回收），防止多 Agent / 多会话并发修改踩脚。
- [pkg/relay/relay.go](pkg/relay/relay.go)：认领（Claim，生成 `.ai-memory/sessions/active-role.md` 独立会话角色边界卡）、安全自检交接（Handover，自动绑定工作区代码快照摘要与角色交接方向）、唤醒接棒（Resume，含指定 `next_agent`、`next_role` 核验与 `test`/`ops` 角色级代码快照过期阻断告警）与终态交付（Done，`ops` 角色强制核验最新快照与审查物证绑定）编排总控。

### 6. 审查报告与物证导出 (`pkg/reporter/`)

- [pkg/reporter/report.go](pkg/reporter/report.go)：汇聚任务、已脱敏审计结果与测试输出（区分“未执行动态测试”与“动态测试未通过”），编译自包含 HTML 报告。
- [pkg/reporter/browser.go](pkg/reporter/browser.go)：跨平台浏览器安全打开实现（Windows 采用 `rundll32 url.dll,FileProtocolHandler` 免 Shell 注入启动并异步回收子进程句柄）。

### 7. 底层脚手架与跨平台工具 (`pkg/harness/`)

- [pkg/harness/fs.go](pkg/harness/fs.go)：原子性安全文件写入、跨平台物理拷贝与相对路径自适应软链接平滑降级实现。
- [pkg/harness/scaffold.go](pkg/harness/scaffold.go)：初始化 `.ignore`、`MAP.md` 与 `contexts/context.md` 模板渲染与幂等落盘。

### 8. 核心资产与模板资源 (`internal/templates/`)

- [internal/templates/SKILL.md](internal/templates/SKILL.md)：单一事实源通用规约（三层门禁、3-Hop 寻路协议、两阶段交互规则、接力协议与记忆防线）。
- [internal/templates/task.tpl](internal/templates/task.tpl)：升级版 YAML Frontmatter + 接力四要素任务看板模板。
- [internal/templates/review.html.tpl](internal/templates/review.html.tpl)：自包含 HTML 审查报告模板（零 CDN 外链依赖）。
- [internal/templates/agents.tpl](internal/templates/agents.tpl)：架构地图骨架模板。
- [internal/templates/context.tpl](internal/templates/context.tpl)：技术契约基线模板。
- [internal/templates/memory.tpl](internal/templates/memory.tpl)：长期工程记忆库（`MEMORY.md`）初始骨架模板。
- [internal/templates/ignore.tpl](internal/templates/ignore.tpl)：防检索爆仓 .ignore 模板。
- [internal/templates/embed.go](internal/templates/embed.go)：Go 标准库 `//go:embed` 静态编译资源入口。

### 9. 智能体指纹与注册中心 (`pkg/agent/`)

- [pkg/agent/registry.go](pkg/agent/registry.go)：多 Agent 协同类型定义（Codex, Cursor, Antigravity, Claude, Windsurf 及开放自定义工具）、支持自定义档案的两阶段优先级环境指纹嗅探器（`DetectEnvironmentWithProfiles`：第一阶段强指纹 `env_keys` 优先于第二阶段宿主终端 `term_programs`/`gitIPC`）与类型注册表。
- [pkg/agent/profile.go](pkg/agent/profile.go)：内置入口档案、开放式声明档案（支持 `markdown`、`frontmatter-md`、`pointer` 三种渲染格式，`.md`/`.mdc`/`.rules` 路径后缀及 `env_keys`/`term_programs`/`frontmatter` 属性）、名称和跨平台相对路径校验（含 `verify.cmd`/`verify.sh`/`verify.bat`/`.ignore` 等保留资产保护）；自定义档案不能静默覆写内置工具或声明同路径目标。

### 10. 三层策略配置加载 (`pkg/config/`)

- [pkg/config/config.go](pkg/config/config.go)：支持个人全局（`~/.agate/config.toml` 或 `AGATE_GLOBAL_CONFIG`）、团队项目级（`.agate/config.toml`）与本机私有（`.agate/config.local.toml`）三层配置合并、工具档案持久化写入（`UpsertProfile`）及本机安全门禁单调收紧（Monotonic Tightening，本机配置仅可收紧不可放宽团队安全红线）。

### 11. 多工具独立窗口视图 (`pkg/workspace/`)

- [pkg/workspace/workspace.go](pkg/workspace/workspace.go)：生成与清理 `.ai-memory/workspaces/<target>.code-workspace` 窗口级独立视图文件，在不触碰团队 `.vscode/settings.json` 与物理规则文件的前提下，按工具窗口过滤其他工具规则并保留当前工具规则与 `MAP.md`、`contexts/context.md`、`TASK.md`、`MEMORY.md`。

### 12. 可选语义决策适配层（规划）

- **定位**：未来以独立适配器接入已安装的 Jev Skills，为任务接力、验证失败分流和审查排序提供语义建议。
- **边界**：Jev 只输出建议与待复核信号，不替代 `pkg/guard`、测试、Git Hook 或 CI 的确定性放行条件；尚未引入运行时代码或网络依赖。

### 13. 构建与发布自动化脚本 (`scripts/`)

- [scripts/archive.sh](scripts/archive.sh) / [scripts/archive.ps1](scripts/archive.ps1) / [scripts/archive.cmd](scripts/archive.cmd)：跨平台零 CGO 静态交叉编译归档脚本，生成 Windows/Linux/macOS 二进制产物并校验编译失败阻断。
- [scripts/bump.sh](scripts/bump.sh) / [scripts/bump.ps1](scripts/bump.ps1)：语义化版本号升级、源码版本常量同步与可选 Git Tag 自动化发布脚本。
- [scripts/archive_test.go](scripts/archive_test.go)：归档脚本在 `go build` 失败时的快速阻断回归测试。

---

## 四、 3-Hop 寻路协议指南

AI 在处理本仓库任何编码与排查需求时，必须严格执行 3 跳寻径：

1. **第 1 跳 (全局拓扑)**：阅读 [MAP.md](MAP.md)，定位功能模块归属与关键文件；
2. **第 2 跳 (技术契约)**：阅读 [contexts/context.md](contexts/context.md)，核对运行环境、核心约束与业务契约；
3. **第 3 跳 (目标实现)**：直接进入对应子目录（如 `cmd/` 或 `pkg/guard/`）针对性检视源码，严禁全仓盲目扫描。
