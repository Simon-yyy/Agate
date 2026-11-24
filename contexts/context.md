# 领域上下文地图 (Context Map)

本页记录当前行为与运行边界。命令入口和对应源码见 [项目模块地图](../MAP.md)。

## 1. 核心业务与领域模型
- **规约来源与分发**。内置规约由 `internal/templates/SKILL.md` 提供。`agate init` 优先读取执行用户的全局规约，再通过 `pkg/adapter` 为选中的工具生成受管入口。提示词编译对清晰指令收敛范围，对模糊指令结合 Git 现场、`MAP.md`、`TASK.md` 和符合准入范围的 `MEMORY.md` 卡片补足锚定信息。
- **宿主与执行者识别**。`pkg/agent` 分别汇总宿主编辑器（Editor）与实际执行者（Runner）的候选，输出 `known`、`unknown` 或 `ambiguous`。唯一可信的 Runner 优先。Runner 未知时，才采用唯一可信的 Editor。
  - Windows 默认采集 8 层祖先进程，Linux 默认 4 层，硬上限 8 层。当前进程链上的精确文件名和白名单服务端路径可提供 Editor 证据。受限或中断时保留已经核实的证据。
  - 环境变量、远程路径及受校验的 `AGATE_SESSION_CONTEXT` 可提供补充证据。通用 `TERM_PROGRAM=vscode` 和全局 `PATH` 中的工具只提供环境线索。
  - `AGATE_SESSION_CONTEXT` 最长 512 字节，校验版本、时间和 `src` 来源。允许的来源为 `terminal`、`ssh`、`extension`、`ide`、`ci`。会话标识仅用于内部一致性校验，诊断输出会脱敏。
- **隐形隔离与窗口级视图过滤 (Invisible Isolation & Per-Tool Workspace View)**: AI 私有状态（`.agents/`, `.gemini/`, `.cursorrules`, `TASK.md`, `MEMORY.md`, `.agate/config.local.toml` 及自定义未跟踪规则路径等）仅注册在本地 `.git/info/exclude`，严禁污染团队公共 `.gitignore` 与 Git 提交树；同时通过 `agate workspace` 在 `.ai-memory/workspaces/<target>.code-workspace` 生成窗口独立的规则过滤视图，在不修改 `.vscode/settings.json` 与物理规则文件的前提下实现同仓多工具窗口展示隔离（注：隐藏仅影响当前窗口展示，不代表停止客户端加载）。
- **闭环自检与两振熔断**: 代码变更后强制执行 `agate verify`（或 `agent-verify`），两振未果强制退出交还主控权；`--resume-after-break` 仅在当次恢复自检真正通过后才清除 `.verify_streak` 计数，若再次失败则继续维持熔断锁死。
- **三层策略配置与安全单调收紧 (Three-Tier Config & Monotonic Tightening)**: 支持个人全局（`~/.agate/config.toml` 或 `AGATE_GLOBAL_CONFIG`）、团队项目级（`.agate/config.toml`）与本机私有（`.agate/config.local.toml`）三层配置合并；本机私有覆写对安全门禁（`guard.strict`、`hooks.install`、`guard.todo`、`guard.absolute_path`）强制执行“单调收紧（只升不降）”，严禁本机配置私自放宽团队安全红线。
- **可选语义判断层（规划）**: Jev Skills 可为任务接力、失败分流与审查排序提供建议；其输出不具有授权效力，不能绕过 Agate 的安全扫描、测试、Git Hook 或 CI。

## 2. 关键 API 与环境约定
- **CLI 命令契约**:
  - `agate init [-t <工具或all>] [--dry-run] [--allow-tracked] [--no-hook]`: 先生成和展示规则分发计划，检查无冲突后才装配护栏与地图资产；`--dry-run` 不写入任何项目文件或 Git 配置；`--allow-tracked` 仅允许更新计划中已跟踪的规则，不改变提交或推送授权；
  - `agate profile list` 列出内置和自定义工具档案。`agate profile add <名称> --path <相对路径>` 注册规则入口，可通过 `--process-name`、`--env` 和 `--term-program` 声明识别信号。`--global` 选择个人全局配置，`--local` 选择本机私有配置，默认写入项目配置。
  - `agate detect [--explain] [--json]`: 只读诊断当前宿主编辑器、执行 Agent、白名单路径、祖先进程与网络传输环境指纹（支持 `--explain` 脱敏详细证据链与 `SessionID` 抹除防护，以及 `--json` 机器格式输出）；
  - `agate prompt [<instruction>] [-s <scope>] [--no-dirty] [--json] [--hook]` 编译自然语言指令。`explicit` 表示锚定了目标。`--set-diagnosis <状态> --request-id <id> [--evidence <文本>]` 可单独登记诊断。登记 `confirmed` 时，`--evidence` 需要说明现象与预期、位置与证据、因果、不确定项和方案依据。空指令或无效范围返回退出码 `2`。
  - `agate workspace [plan|apply|restore|open <target>]`: 预览、生成、清理或直接启动 `.ai-memory/workspaces/<target>.code-workspace` 独立窗口视图（支持 `-t` 逗号分隔多目标与显式空白参数拦截），对纯 CLI 工具（`codex`、`claude`）安全降级跳过，未知子命令返回用法错误退出码 `2`；
  - `agate isolate`: 注入本地私有排除规则（17 项默认隐形隔离清单，含 `.env*` 与 `*.agate.bak`）；
  - `agate verify [--staged] [--strict] [--skip-guard] [--report] [--resume-after-break]`: 执行本地多级自检闭环，退出码严格约定：`0` 为 PASS，`1` 为 FAIL；连续失败 2 次触发 `.ai-memory/.verify_streak` 物理两振熔断，后续运行须由人类显式传入 `--resume-after-break` 且当次自检通过方可解除熔断；支持 `--report` 自动生成 HTML 审查报告；
  - `agate ci verify`: CI/远程合并门禁入口，固定启用严格校验与 HTML 审查报告，适用于 GitHub Actions、GitLab CI 等流水线；
  - `agate export [-o <path>] [--open] [--staged]`: 汇聚已脱敏安全审计、Diff 对比、任务与自检日志，编译自包含单文件 HTML 审查报告（未执行动态测试时显式标记为“未执行动态测试”，与“测试未通过”严格区分）；
  - `agate view`: 在系统默认浏览器中安全预览最新生成的 HTML 审查报告（Windows 下通过 `rundll32 url.dll,FileProtocolHandler` 启动，避免 `cmd.exe` 元字符注入风险）；
  - `agate scan [--write]`: 智能扫描工程技术栈与端口矩阵，支持确定性正斜杠路径排序与原子写入回填 MAP.md 与 contexts/context.md；
  - `agate hook [install|uninstall|status]`: 管理本地 Git 双重门禁（pre-commit 提纯自检 + pre-push 物理防偷跑）与状态看板，未知子命令严格返回用法错误退出码 `2`；
  - `agate task [status|claim|handover|resume|done]`: 跨 Agent 与多角色（`--role dev|test|ops`、`--next-role`）任务接力中枢，管理 TASK.md 状态机流转、租约互斥锁、会话角色卡片（`.ai-memory/sessions/active-role.md`）、交接四要素与流转审计时间线（全部写操作前置校验配置文件与非空白 `--agent` 参数，`handover` 与 `done` 强制联动 `.agate/config.toml` 的 `guard.strict`、Phase 0 静态审计与两振熔断，无输出失败测试同样阻断）。
- **三态退出码契约 (Exit Code Contract)**:
  - `0` (`[PASS]`): 命令成功执行或闭环验证全部通过；
  - `1` (`[FAIL]`): 触发安全红线拦截、测试未通过、状态冲突或运行时错误；
  - `2` (`[USAGE_ERR]`): CLI 命令行参数解析错误、非法位置参数或未知子命令（由 `*cmd.UsageError` 统一承载并映射）。
- **`.ai-memory/` 本地运行时状态拓扑**:
  - `.ai-memory/.verify_streak`: 两振硬熔断连续失败物理计数器；
  - `.ai-memory/locks/task.lock` & `.ai-memory/locks/task.lock.transition`: 跨 Agent / 多会话任务租约锁与原子排他过渡锁（30s 陈旧锁自动重命名回收）；
  - `.ai-memory/locks/rules-distribution.lock`: 规则分发串行化互斥锁；
  - `.ai-memory/reviews/review-<timestamp>.html`: 零外部依赖的自包含单文件 HTML 审查报告归档；
  - `.ai-memory/workspaces/<target>.code-workspace`: 按工具生成的窗口级独立视图配置；
  - `.ai-memory/sessions/active-role.md`: 当前会话角色（`dev`/`test`/`ops`）职责与权限边界卡片。
- **静态合规与忽略缓存契约**:
  - `pkg/guard` 预热忽略缓存时通过 `git ls-files --others -i --exclude-standard --directory` 仅将以 `/` 结尾的整目录忽略项写入 `ignoredDirs`，防止目录内单个被忽略普通文件污染父目录导致新创建的未跟踪违规文件漏检；
  - 全量审计与 `--staged` 暂存区审计统一覆盖 7 大卡口（含 `vendor/` 纯净度、`.env.*` 与 `*.agate.bak` 拦截），支持 `.agate/config.toml` 的 `guard.large_file_mb`、`guard.todo`（`warning`/`error`/`off`）、`guard.absolute_path`（`error`/`warning`/`off`）策略配置，且 `Violation.LineContent` 在构造时即完成敏感赋值脱敏。
- **自举验证契约**:
  - 项目根目录具备 `verify.cmd` / `verify.sh`，优先保障本地验证开箱即走。
- **开放式规则分发与并存契约**:
  - 目标选择顺序为显式 `-t`、有效配置中的 `agent.targets`、当前环境中可确定的工具。识别结果未知或冲突时，`init` 停止自动分发并提示用 `-t`，已有规则入口仅作为选择线索。显式的 `-t all` 选择全部内置分发目标和已声明的自定义工具。
  - 全局规约按执行用户家目录依次查找 `~/.agate/rules.md`、`~/.config/agate/rules.md`、`~/my_skills/SKILL.md`、`~/.gemini/config/SKILL.md`，使用第一个命中的规约文件。命中文件但无法读取时初始化终止。未找到时使用二进制内置正文。服务器按运行 Agate 的账号查找。检测到旧版内容时，差异提示供用户复核。
  - 自定义工具通过 `[agent.profiles.<名称>]` 或 `agate profile add` 声明 `path`、`format`、`confirmed = true`、`source` 和 `version`。可选字段包括 `reads`、`env_keys`、`term_programs`、`process_names` 和 `frontmatter`。`process_names` 只接受可执行文件名，采用不区分大小写的精确匹配。入口确认和真实客户端加载分别验证。
  - 自定义路径使用 `/` 分隔的项目内相对路径，支持 `.md`、`.mdc`、`.rules` 或隐藏 rules 文件；拒绝越界、绝对路径、系统保留名、配置与私有资产路径（含 `verify.cmd`、`verify.sh`、`verify.bat`、`.ignore`）、内置入口碰撞及符号链接。
  - 示例配置如下；示例工具为虚构名称，真实工具须按其实际手册核对路径和版本：

    ```toml
    [agent]
    targets = ["example-tool"]
    [agent.profiles.example-tool]
    path = ".example-tool/rules/agate.mdc"
    format = "frontmatter-md"
    confirmed = true
    source = "工具手册与本地路径核对记录"
    version = "本机实际版本"
    reads = ["AGENTS.md"]
    env_keys = ["EXAMPLE_AGENT"]
    term_programs = ["example-ide"]
    process_names = ["example-ide.exe"]
    ```

  - 显式目标仅更新所选入口；受管区块外的用户内容与已有 `.agate.bak` 保留（`frontmatter-md` 更新已有 `.mdc` 时严格保留第 1 行用户 YAML Frontmatter，`pointer` 写入轻量寻路引导块并内嵌 `<!-- agate:rules-sha256:<hex> -->` 保证盘点摘要一致），无法确认归属的旧正文追加区块并提示人工核验；含其他工具受管标记、重复／残缺标记、已跟踪文件未获允许（含大小写不敏感文件系统下通过 `git ls-files -z` 与 `os.SameFile` 识别的同名物理文件）或计划快照变化均阻断规则写入（若全部条目均为 `skip` 则无锁短路返回）。
  - `.ai-memory/locks/rules-distribution.lock` 串行化遵循分发接口的初始化；执行前复核全部入口快照及 Git 状态，逐个原子更新文件。不承诺跨文件事务、任意外部写入拦截或会话版本一致性；异常退出可能留下锁，须确认无初始化运行后人工处理。
  - 盘点与双向重叠诊断分别显示物理入口、Git 跟踪、受管正文及可能的读取关系；可通过 `agate workspace apply` 生成窗口级 `.code-workspace` 视图过滤其他工具规则，不写入 `.vscode/settings.json`，也不搬移其他工具入口。真实加载、行为效果与严格工作区隔离须独立验证。
- **Jev 集成边界**:
  - 仅在用户显式启用时调用；API 模式必须先取得用户同意并检查密钥是否存在，严禁在日志或对话中回显密钥；
  - 仅传递脱敏后的最小必要上下文；`needs_review`、未知结果和 API 失败必须转人工处理，不得降级为自动放行；
  - Agent 模拟模式必须显式标注，不得伪造 Jev 概率、置信度或调用回执。
- **跨平台兼容与归档收敛**:
  - Windows 环境优先尝试软链接，权限受限时优雅降级为物理拷贝（`CopyFile`），非 Windows 相对路径软链自动计算相对于目标父目录的路径；
  - 换行符与 Frontmatter 解析原生兼容 CRLF/LF，防御边界截断漂移；
  - `scripts/archive.ps1` 与 `scripts/archive.sh` 读取 `cmd/root.go` 中的版本，生成 Windows amd64、Linux amd64、macOS amd64 和 macOS arm64 的零 CGO 归档。输出位于 `bin/v<version>/`，Windows 脚本同时刷新 `bin/agate.exe`。
  - 归档和本机安装分别核验。Windows 可用 `Get-Command agate,agate-hooks` 查实际路径，分别运行 `--version`，并比对安装文件与归档的 SHA-256。若项目把 Cursor Hook 指向单独的程序，也要核对该程序。

### Cursor 原生 Hook

原生模式使用 `agate prompt --hook --host cursor --event <事件>`。标准输出只返回事件协议 JSON。`--json` 用于普通编译输出。项目可通过 `.cursor/hooks.json` 注册原生事件，`agate init` 的规则分发和 Git 门禁安装是独立步骤。

| 事件 | 行为 |
| --- | --- |
| `beforeSubmitPrompt` | 编译当前用户消息，在 Git 公共目录的 `agate/requests/` 下写入本轮上下文和请求记录，初始交付状态为 `pending_read`。收到单独的「执行」时，关联同会话、同工作区的当前待确认方案。 |
| `postToolUse` | 通过 `additional_context` 提供本轮上下文的绝对路径。原生读取命中本轮文件时，登记 `observed_read` 和 `delivery_evidence`。方案轮和确认轮分别读取自己的文件。 |
| `afterAgentResponse` | 从已交付请求的最终回复提取 `agate-plan`，记录诊断状态、方案标识及精确的仓库相对文件路径，等待用户确认。 |
| `preToolUse` | 只门禁结构化文件编辑：上下文已读、诊断、单独「执行」、轮次和目标路径。其余工具在会话有效后直接放行，不看命令文本、不看诊断。没有会话标识时，Read、Shell、MCP 和文件编辑都拒绝，理由为「缺少会话标识，拒绝工具调用」。统一流程与旧的 `HandleCursorEvent` 都执行这条规则。 |

请求记录可核对上下文交付和审批。实际写入结果以客户端事件和文件状态为准。

### Antigravity IDE 原生 Hook

原生模式使用 `agate prompt --hook --host antigravity --event <事件>`。项目挂载 `.agents/hooks.json`。拒绝时标准输出只返回 `{"decision":"deny","reason":"..."}`。

IDE 2.8.0 的 `PostInvocation` 不带 `response`。适配器改读 `transcriptPath`，只取 `step_index` 不大于本次 `initialNumSteps` 的最后一条 `PLANNER_RESPONSE`，从中解析 `agate-plan`。确认轮没有 TurnID 时，绑定当前快照的 `ActiveTurnID`，不另造会话。

| 事件 | 行为 |
| --- | --- |
| `PreInvocation` | `invocationNum` 为 0 时视为用户提交并编译上下文。同一 `lastUserInput` 的后续调用视为模型续跑，不重复当作新提交。 |
| `PostToolUse` | 读取成功且路径为本轮上下文时，登记 `observed_read`。失败读取不产生已读回执。 |
| `PostInvocation` | 从上面的回复来源登记方案。没有可信 `agate-plan` 时不登记候选方案。 |
| `PreToolUse` | 与其他宿主共用 `ApplyNativeWorkflow`。未确认或超出方案文件范围时拒绝，文件不落盘。 |
| `Stop` | 解码分支已接上，实机尚未覆盖主动停止。 |

2026-11-24 在 Antigravity IDE 2.8.0、Windows 临时仓库上完成实机矩阵：方案登记、「执行」确认、范围内 `sample.txt` 写入放行、未确认的 `other.txt` 被 pre-tool hook 拒绝且文件不存在。该次使用手工 Hook 与候选二进制，版本目录仍为空，`agate init` 还不会按已验收版本自动挂载。

### 多宿主统一 Native Hook 契约

- **统一 NativeAdapter 协议模型**：
  - 内部统一抽象为 `NativeEvent`（`Kind`: `submit`, `read_completed`, `response_completed`, `before_tool`, `observe`）与判定决策 `NativeDecision`（`Block`, `Reason`, `AdditionalContext`）；
  - 各宿主实现 `NativeAdapter` 接口（`Decode`/`Encode`），实现客户端私有协议与统一模型间的双向编解码；
  - 会话快照与锁统一隔离于 Git 公共目录的 `agate/requests/v2/<host>/<workspace-hash>/`，以“宿主 + 工作区 + 会话”为锁粒度；
  - 写入判定统一由 `ApplyNativeWorkflow` 驱动，且只作用于结构化文件编辑：上下文已读交付门禁 -> 诊断状态门禁 -> 真人显式“执行”方案锁定 -> 精确目标文件范围核验。其余工具在会话有效后直接放行，不看命令文本、不看诊断。没有会话标识时，Read、Shell、MCP 和文件编辑都拒绝，理由为「缺少会话标识，拒绝工具调用」。是否该查、该改、该提交由宿主 Agent 按规约判断。
- **预写入拒绝契约 (PreToolUse / Pre-Write)**：
  - Cursor：stdout 输出 `{"permission":"deny","user_message":"..."}`；
  - Antigravity IDE：stdout 输出 `{"decision":"deny","reason":"..."}`（挂载 `.agents/hooks.json`）；
  - Claude Code：stdout 输出 `hookSpecificOutput` 带 `permissionDecision: "deny"`（挂载 `.claude/settings.json`）；
  - Codex：stdout 输出 `hookSpecificOutput` 带 `permissionDecision: "deny"`（支持 `apply_patch` 多目标解析）；
  - Windsurf Cascade：stderr 打印理由，并以 **退出码 2** 强制阻断（挂载 `.windsurf/hooks.json`）。
- **扩展内置工具矩阵**：
  - 第一批：GitHub Copilot (`.github/copilot-instructions.md`), TRAE IDE (`.trae/rules/agate.mdc`), OpenCode (`.opencode/instructions.md`), Qwen Code (`QWEN.md`)；
  - 第二批：Cline (`.cline/rules/agate.md`), Kiro (`.kiro/steering/agate.md`), Antigravity CLI (`.gemini/GEMINI.md`), Gemini CLI (`GEMINI.md`)。
  - `agate init` 支持通过 `--native-hooks=auto|off|require` 自动合并生成宿主原生 Hook 配置。

## 3. 动作外溢与显式授权公理 (Explicit Mandate Principle)
- **动作二分基线**：
  - **本地闭环动作**（代码编辑、本地编译、单测运行与自检）：常规任务及方案确认后准予闭环探索；
  - **外溢与跃迁动作**（Git 提交/推送、Release 上传、包发布、部署推流、破坏性变更）：默认不代跑。
- **授权铁律（指哪打哪）**：
  - 严禁以“顺带闭环”为由代跑任何外溢动作；
  - 必须且仅当用户指令中出现明确指向该外溢动作的专属动词（如“提交代码”、“上传Release”、“发布”）时，对应操作方可单次解锁代跑。
- **与两道物理门的关系**：
  - 上面的动词约束交给 Agent 遵守。原生 Hook 不解析命令，也不用工具名名单判断调查或外溢。会话有效时，只有结构化文件编辑进入诊断与「执行」门。
  - Git `pre-push` 仍单独拦截自动化推送。Agent 或非交互环境需要 `ALLOW_AUTOMATED_PUSH=1` 才进入后续校验。

## 4. 跨 Agent / 多角色任务接力与记忆资产防线 (Relay, Multi-Role & Memory Governance)
- **接力四要素、多角色流转与状态机**：`TASK.md` 采用标准 YAML Frontmatter（含 `current_role`、`next_role` 多角色字段与 `snapshot_digest` 代码快照摘要）+ Markdown 结构；支持 `dev`（编码实现）、`test`（独立验收）、`ops`（运维审查）三类角色并在 `.ai-memory/sessions/active-role.md` 生成当前会话专属角色边界卡（零侵入根目录共享 `AGENTS.md`/`.cursorrules`）；换工具或换角色前必须执行 `agate task handover` 固化物证并记录代码快照摘要，新工具/新角色启动首句执行 `agate task resume` 接棒（校验指定 `next_agent`、`next_role` 身份，并在 `test`/`ops` 角色发现快照过期或物证缺失时输出阻塞级复验提示），全量任务交付归档执行 `agate task done`（`ops` 角色强制校验审查物证与最新 `snapshot_digest` 绑定）。
- **并发租约锁与过渡守护**：`.ai-memory/locks/task.lock` 提供租约互斥（支持 `role` 角色标识与 `session_id` 区分同一工具的不同会话窗口），配对 `.transition` 原子守护排他创建（覆盖 `AcquireLock`、`ReleaseLock` 及陈旧 `.transition` 的原子重命名清理），防止并发抢锁与释放竞态；`handover` 与 `done` 强制校验当前 Agent 及会话锁所有权，`DONE` 终态任务禁止重新认领。
- **长期记忆神圣公理**：`MEMORY.md` 默认只读，严禁 Agent 私自直写；在途偶发问题仅记录于当期 `TASK.md` 随任务自然消亡；重大架构暗坑须遵循“提议制”，由人类显式确认后方可沉淀。
