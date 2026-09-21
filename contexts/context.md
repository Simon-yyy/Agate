# 领域上下文地图 (Context Map)

## 1. 核心业务与领域模型
- **单一事实源 (SSOT)**: 核心规约由 `internal/templates/SKILL.md` 固化，通过 `pkg/adapter` 分发映射至各大 Agent 工具配置文件；Codex 使用 `AGENTS.md` 内的受管区块；项目架构拓扑独立收敛于 `MAP.md`，与 `contexts/context.md` 构成 3-Hop 寻路双地图。
- **智能体指纹识别 (Agent Fingerprinting)**: 通过 `pkg/agent` 集中感知当前环境中的 Agent 类型（Codex, Cursor, Antigravity, Claude, Windsurf）及 CI 自动化标志，保障策略注入与状态机流转的精准对接。
- **隐形隔离 (Invisible Isolation)**: AI 私有状态（`.agents/`, `.gemini/`, `.cursorrules`, `TASK.md`, `MEMORY.md` 等）仅注册在本地 `.git/info/exclude`，严禁污染团队公共 `.gitignore` 与 Git 提交树。
- **闭环自检与两振熔断**: 代码变更后强制执行 `agate verify`（或 `agent-verify`），两振未果强制退出交还主控权。
- **项目级配置**: 可选的 `.agate/config.toml` 覆盖 Agent 目标、严格测试模式、文件阈值和 Hook 安装策略；命令行参数优先级最高。
- **可选语义判断层（规划）**: Jev Skills 可为任务接力、失败分流与审查排序提供建议；其输出不具有授权效力，不能绕过 Agate 的安全扫描、测试、Git Hook 或 CI。

## 2. 关键 API 与环境约定
- **CLI 命令契约**:
  - `agate init`: 初始化项目 AI 研发护栏与地图资产；
  - `agate isolate`: 注入本地私有排除规则（17 项隐形隔离清单，含 .env* 与 *.agate.bak）；
  - `agate verify [--staged] [--strict] [--skip-guard] [--report] [--resume-after-break]`: 执行本地多级自检闭环，退出码严格约定：`0` 为 PASS，`1` 为 FAIL；连续失败 2 次触发 `.ai-memory/.verify_streak` 物理两振熔断，后续运行须由人类显式传入 `--resume-after-break` 才可恢复；支持 `--report` 自动生成 HTML 审查报告；
  - `agate ci verify`: CI/远程合并门禁入口，固定启用严格校验与 HTML 审查报告，适用于 GitHub Actions、GitLab CI 等流水线；
  - `agate export [-o <path>] [--open] [--staged]`: 汇聚安全审计、Diff 对比、任务与自检日志，编译自包含单文件 HTML 审查报告；
  - `agate view`: 在系统默认浏览器中一键预览最新生成的 HTML 审查报告；
  - `agate scan [--write]`: 智能扫描工程技术栈与端口矩阵，支持确定性排序回填 MAP.md 与 contexts/context.md；
  - `agate hook [install|uninstall|status]`: 管理本地 Git 双重门禁（pre-commit 提纯自检 + pre-push 物理防偷跑）与状态看板；
  - `agate task [status|claim|handover|resume|done]`: 跨 Agent 任务接力中枢，管理 TASK.md 状态机流转、租约互斥锁、交接四要素与流转审计时间线。
- **自举验证契约**:
  - 项目根目录具备 `verify.cmd` / `verify.sh`，优先保障本地验证开箱即走。
- **Jev 集成边界**:
  - 仅在用户显式启用时调用；API 模式必须先取得用户同意并检查密钥是否存在，严禁在日志或对话中回显密钥；
  - 仅传递脱敏后的最小必要上下文；`needs_review`、未知结果和 API 失败必须转人工处理，不得降级为自动放行；
  - Agent 模拟模式必须显式标注，不得伪造 Jev 概率、置信度或调用回执。
- **跨平台兼容与归档收敛**:
  - Windows 环境优先尝试软链接，权限受限时优雅降级为物理拷贝（`CopyFile`）；
  - 换行符与 Frontmatter 解析原生兼容 CRLF/LF，防御边界截断漂移；
  - 归档工具（`scripts/archive.ps1` / `scripts/archive.sh`）原生支持全平台零 CGO 交叉编译（Windows amd64, Linux amd64, macOS amd64/arm64），产物纯净收敛于 `bin/v<version>/` 对应目录。

## 3. 动作外溢与显式授权公理 (Explicit Mandate Principle)
- **动作二分基线**：
  - **本地闭环动作**（代码编辑、本地编译、单测运行与自检）：常规任务及方案确认后准予闭环探索；
  - **外溢与跃迁动作**（Git 提交/推送、Release 上传、包发布、部署推流、破坏性变更）：**默认物理死锁**。
- **授权铁律（指哪打哪）**：
  - 严禁以“顺带闭环”为由代跑任何外溢动作；
  - 必须且仅当用户指令中出现明确指向该外溢动作的专属动词（如“提交代码”、“上传Release”、“发布”）时，对应操作方可单次解锁代跑。

## 4. 跨 Agent 任务接力与记忆资产防线 (Relay & Memory Governance)
- **接力四要素与状态机**：`TASK.md` 采用标准 YAML Frontmatter + Markdown 结构；换工具前必须执行 `agate task handover` 固化物证，新工具启动首句执行 `agate task resume` 接棒，全量任务交付归档执行 `agate task done`。
- **并发租约锁与过渡守护**：`.ai-memory/locks/task.lock` 提供租约互斥，配对 `.transition` 原子守护排他创建，防止并发抢锁竞态；`handover` 与 `done` 强制校验当前 Agent 锁所有权，`DONE` 终态任务禁止重新认领。
- **长期记忆神圣公理**：`MEMORY.md` 默认只读，严禁 Agent 私自直写；在途偶发问题仅记录于当期 `TASK.md` 随任务自然消亡；重大架构暗坑须遵循“提议制”，由人类显式确认后方可沉淀。
