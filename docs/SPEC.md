# Agate (Agent Gate) 协议与规范标准手册

## 一、 AI 协同规约标准 (SKILL.md / SSOT)

Agate 遵循**单一事实源（Single Source of Truth, SSOT）**原则：所有面向 Agent（Cursor、Codex、Antigravity、Claude Code、Windsurf 等）的核心协同与安全规约均由统一模板文件维护，并由 Go `//go:embed` 静态编译内置于可执行文件中，禁止在各处文档中重复硬编码全文以防规约漂移。

完整规约模板源码请直接参阅：[internal/templates/SKILL.md](../internal/templates/SKILL.md)

### 1.1 规约体系核心板块纲要

规约模板收敛为以下 7 大核心工程协同基线：

1. **中文原生与代码完整性**：
   - 交互思考、排查分析、注释与 Git Commit Message 规范使用简体中文；
   - 代码必须完整可运行，严禁使用 `// ... 保持不变` 等偷懒省略占位符。
2. **两阶段硬门禁与指令语义扩写 (Two-Phase Gate & Intent Expansion)**：
   - **双轨自适应分流**：模糊指令须先调用 `agate prompt` 锚定四源物理现场并补齐隐式工程项；清晰明确指令免调命令并遵循最小影响原则收敛展开（防过度扩写）；
   - **分级放行与局部纠偏闭环**：首轮强制输出 `【指令语义解码与隐式需求展开】` 四要素块；纯只读查询（A 级）直通回答，核心修改（C 级）输出精简 Plan 后提示用户确认“执行”或指出解码偏差；收到局部修正意见时严禁抢跑改代码，必须重新解码对齐并再次等待“执行”；
   - **请求类型与诊断连续性**：每个新任务或改变目标的请求重新确认目标和动作类型。普通状态询问不重复完整流程。纠偏重新核对目标与已有证据。用户确认关联已有方案，不把“执行”本身当作新故障，也不扩大授权范围。锚定置信度 `explicit` 只说明文件或模块明确，不说明根因已查明。纯分析请求只读检查后交付诊断结论，不强推实施 Plan；修复故障须先形成有证据的诊断结论再出方案；原因未明时交付排查计划，不把假设写成已证实根因；新功能或明确改动说明现状与预期差异，不虚构故障。故障类诊断须交付现象与预期、位置与证据、因果解释、不确定项、方案依据五要素；诊断状态分为未诊断、待验证假设、已形成有证据的诊断结论，不能仅凭解码标题、`explicit` 或一份 Plan 变更诊断状态。
3. **3-Hop 寻路规范 (3-Hop Navigation)**：
   - 检索路径严格遵循：`MAP.md` (空间地图) -> `contexts/context.md` (技术契约) -> 目标源码，严禁无序全仓盲扫。
4. **闭环验证与两振熔断 (Fail-Fast)**：
   - 落盘后必须主动运行 `agate verify`（或 `agent-verify`）并以绿色 `[PASS]` 物证交付；
   - 同一错误连续 2 次自检未过，强制触发两振物理熔断（`IsFinal=true`），停步移交主控权。
5. **绝对安全红线与动作外溢公理 (The Explicit Mandate Rule)**：
   - 动作二分：本地闭环动作（读取、编辑、本地测试、自检）在确认方案后可执行；外溢动作（Git 提交/推送/标签、Release 上传、包发布、CI 触发、数据库 DDL、破坏性删除）**默认绝对物理死锁**；
   - 显式授权排他性：唯有用户指令出现指向该动作的专属动词（如“提交代码”、“推送远程”）时方可单次解锁代跑，严禁以“顺便完成”为由越权代跑。
6. **跨 Agent 任务接力协议 (Relay Protocol)**：
   - 唤醒时执行 `agate task resume` 继承在途断点与接力四要素；交接前执行 `agate task handover` 固化物证，交付执行 `agate task done` 归档。
7. **记忆资产神圣公理与防泛化防线 (Memory Governance Rule)**：
   - 长期记忆 `MEMORY.md` 默认只读，严禁私自直写；临时暗坑仅记录于 `TASK.md` 随任务归档自然消亡；通用暗坑遵循“提议制”。

---

## 二、 隐形隔离名单标准规范

执行 `agate isolate` 或 `agate init` 时，系统会自动将以下清单写入本地 `.git/info/exclude`，静默忽略 AI 专有文件：

```text
# --- agate private tracking start ---
.env
.env.*
.env.local
.gemini/
.cursor/
.agents/
.ai-memory/
.cursorrules
.windsurfrules
CLAUDE.md
TASK.md
MEMORY.md
.ignore
*.agate.bak
.vscode/
*.log
logs/
# --- agate private tracking end ---
```

### 隔离原则与幂等性保证 (M5 规范)
1. **公共共享资产（进入 Git 跟踪）**：
   - `MAP.md`：项目架构与模块拓扑地图（供所有人及 AI 共享）；
   - `contexts/context.md`：团队约定的技术契约与运行约束。
2. **私有状态资产（本地排除）**：
   - 个人 AI 会话记录、Memory 记忆卡片、临时 Task 进度追踪、本地环境变量（`.env*`）等。
3. **幂等性与零污染契约**：
   - **块级防破坏追加**：写入 `.git/info/exclude` 时必须使用包含标记符（`# --- agate private tracking start ---` 与 `# --- agate private tracking end ---`）的专用受管块，新追加项精准插入在 end 标记内侧，绝不覆写用户原有的排除配置，并通过 `WriteFileAtomic` 原子落盘；
   - **完全本地生效**：`.git/info/exclude` 仅在开发者本地 `.git` 目录生效，不进版本库，不修改项目公共 `.gitignore`，杜绝协同 Diff 噪音。

---

## 三、 Git Hook 门禁安装与防篡改策略 (M4 规范)

### 3.1 拦截阶段与职责
- **`pre-commit` 门禁**：在提交暂存区代码前强制调用 `agate verify --staged`。直接从 Git Index（暂存区）提取待提交文件并通过 `git show :<path>` 针对性审查，毫秒级快速就绪，精准拦截私有密钥、超大文件、机器硬编码绝对路径、未完工占位符及 `vendor/` 污染等违规内容，工作区脏文件不产生误拦；
- **`pre-push` 物理阻断**：严格阻止 AI 在未经用户指令时执行 `git push` 偷跑代码至远端。在非交互式终端、检测到 Agent/CI 环境变量指纹（含 `CLAUDE_CODE`、`ANTIGRAVITY_IDE`、`CURSOR_VERSION`、`CODEX_VERSION`、`CODEX_SHELL` 等）或检测到 `AGATE_SESSION_CONTEXT` 包含执行者身份（大小写无关匹配 `runner=`/`agent=`）时，唯有显式声明 `ALLOW_AUTOMATED_PUSH=1` 或 `true` 才准予进入全量验证并放行。这道门独立于原生 Hook：会话有效时，原生 Hook 在诊断检查之前放行 Shell，不解析命令文本。

### 3.2 Hook 安装与规约分发防篡改策略
1. **非破坏性备份**：若检测到开发者已有同名 Hook，必须自动重命名备份（如 `pre-commit.agate.bak`），严禁静默覆盖；
2. **标记行识别**：写入的脚本必须带有 `# --- agate hook: <type> ---` 专用标识行；
3. **精准清理还原**：在执行卸载或移除时，仅精准移除由 agate 管理的钩子并还原原备份脚本，完好保留用户自定义钩子；
4. **受管区块合并与原子安全落盘**：向工作区分发规约（如 `AGENTS.md`、`.cursorrules`、`CLAUDE.md` 等）时，统一使用 `<!-- agate:<agent>-rules:start/end -->` 受管区块合并，保留区块外用户原有内容与既有 `.agate.bak`；对已受 Git 跟踪的规则文件（含大小写不敏感文件系统下通过 `os.SameFile` 识别的同名物理文件）须显式传入 `--allow-tracked` 方可更新，并通过临时文件 + `fsync` + `rename` 真原子落盘。

### 3.3 状态可观测性
- 支持通过 `agate hook status` 结构化查询 `core.hooksPath`、`pre-commit` 与 `pre-push` 托管状态，提供开箱即用的门禁可视看板。

---

## 四、 三层策略配置规范 (全局 / 项目 / 本机私有)

Agate 支持个人全局（`~/.agate/config.toml` 或 `AGATE_GLOBAL_CONFIG`）、团队共享项目级（`.agate/config.toml`）与本机私有项目覆写（`.agate/config.local.toml`，自动加入 `.git/info/exclude`）三层配置合并。目标选择优先级为：命令行 `-t` 参数 > 配置中的 `agent.targets` > 当前 Agent 环境指纹（内置与自定义档案 `env_keys` 强指纹优先于宿主终端 `term_programs` 弱指纹） > 项目已有规则入口；空白未知环境返回错误要求显式选择，不猜测默认工具。

```toml
[agent]
targets = ["codex", "cursor"]

[agent.profiles.example-tool]
path = ".example-tool/rules/agate.mdc"
format = "frontmatter-md"
confirmed = true
source = "工具手册与本地路径核对记录"
version = "本机实际版本"
reads = ["AGENTS.md"]
env_keys = ["EXAMPLE_AGENT"]
term_programs = ["example-ide"]

[guard]
strict = true
todo = "warning"
absolute_path = "error"
large_file_mb = 20

[hooks]
install = true
```

`agent.targets`、`agent.profiles.<name>`、`guard.strict`、`guard.todo`（`warning`/`error`/`off`）、`guard.absolute_path`（`error`/`warning`/`off`）、`guard.large_file_mb` 和 `hooks.install` 均已全量接入生效；本机私有 `.agate/config.local.toml` 对安全门禁遵循**单调收紧（Monotonic Tightening，只升不降）**，不能放宽团队已启用的安全红线。配置语法错误、未知字段或越界/保留路径会阻止初始化/验证并返回错误。

---

### 4.2 开放式第三方 Agent 接入约定

Agate 采用“**双地图为规范本体，主流预置与开放式档案分发，物理门禁透明兜底**”的开闭原则（Open-Closed Principle）：
1. **解耦原则**：工程认知本体沉淀于根目录公共文件（`MAP.md` 与 `contexts/context.md`），而非强绑定在某一商业 IDE 格式；
2. **长尾软件零代码扩展契约**：对于 GitHub Copilot、Cline、Trae、Continue、Aider 等未预置目标，开发者可通过 `agate profile add <name>`（或配置 `[agent.profiles.<name>]`）声明 `markdown`、`frontmatter-md`（`.mdc`）或 `pointer` 三大范式规则入口及 `env_keys` / `term_programs` 环境指纹，获得与内置工具同等的自动识别、受管合并与 `.code-workspace` 独立视图支持；
3. **安全底座零死角**：`.git/info/exclude` 隐形忽略、`pre-commit` 暂存区审计与 `pre-push` 物理防偷跑钩子在操作系统与 Git 进程级透明运行，任何非预置 Agent 或第三方编辑器均无权绕过。

### 4.3 跨平台宿主感知与 Windows 原生采集标准

为杜绝在 Windows、Linux 等不同操作系统下因宿主进程链不同而出现身份漏判与越权提权，系统确立如下采集契约：
1. **Editor 与 Runner 角色严格解耦**：宿主编辑器（Editor）仅代表运行环境，执行 Agent（Runner）代表具有修改任务意图的智能体。本地 Editor 进程证据只作用于 Editor 判定，严禁无条件升级为 Runner 或自动化执行凭据；
2. **Windows 原生快照规范**：Windows 平台采用 Win32 Unicode API（`CreateToolhelp32Snapshot`、`OpenProcess(PROCESS_QUERY_LIMITED_INFORMATION)`、`QueryFullProcessImageNameW`、`GetProcessTimes`），默认采集深度为 8（硬上限 8）；
3. **安全与防环防线**：查询父子进程创建时间，若父进程创建时间晚于子进程（疑似 PID 复用）或链路成环，立即有界停止；普通权限运行，遇“Access is denied”时优雅降级并保留已核验节点；
4. **精确匹配与脱敏**：仅精确匹配已确认的本地可执行文件（`Cursor.exe` -> `cursor`，`Code.exe` / `Code - Insiders.exe` -> `vscode`），证据说明仅展示 PID、进程名与层级，绝不输出完整安装路径或私有命令行。

---

## 五、 架构地图骨架标准 (MAP.md & contexts/context.md)

### 5.1 MAP.md 规范骨架
```markdown
# 项目模块地图 (Agent Navigation)

## 一、 项目架构与工程定位
本项目为 **{{.ProjectName}}**。

---

## 二、 核心入口与端口矩阵
| 服务 / 模块 | 职责与技术栈 | 开发环境入口 / 端口 | 关键配置 / 依赖 |
| :--- | :--- | :--- | :--- |
| **核心服务** | 主业务入口 | 本地端口 / URL | application.yml / vite.config.ts 等 |

---

## 三、 模块拓扑与职责分工
### 1. 核心模块
- 模块说明与核心逻辑路径。
```

### 5.2 contexts/context.md 规范骨架
```markdown
# 项目工程上下文 (Operational Context)

## 一、 技术栈与运行环境
- **基础运行环境**: Node.js / JDK / Go / Python 等主要版本
- **核心框架**: 补充框架名称与关键组件版本

---

## 二、 核心业务流与规则基线
- 核心业务流程：请求发起 -> 中间件校验 -> 核心 Service -> 数据持久化
- 异常与返回体规范：标准统一返回体格式与错误码映射
```

---

## 六、 退出码与物证协议标准 (L1 规范)

### 6.1 CLI 退出码契约（Exit Codes）
作为可被 CI、Git Hook 及自动化 Agent 调用的标准化工具，明确如下三态退出码：
| 退出码 | 标识 | 语义 | AI 与 CI 处理策略 |
| :--- | :--- | :--- | :--- |
| **`0`** | `[PASS]` | 验证全部通过，安全合规且测试绿灯 | 准予提交或交工，向用户呈现通过物证 |
| **`1`** | `[FAIL]` | 触发安全红线拦截、编译失败或单测报错 | 阻断提交，触发诊断修复；若连续第 2 次为 1，立即触发两振熔断 |
| **`2`** | `[USAGE_ERR]` | CLI 命令参数解析错误、未知子命令或非法位置参数 | 阻断流程，检查命令调用传参合法性 |

### 6.2 交付物证格式
Agent 向用户交付时，聊天框必须包含类似下述格式的实机物证回显：
```text
=== [agate verify] 闭环自检完成 ===
[PASS] 本地交付物校验通过，结构完备 (耗时: 264ms)
所有修改符合门禁要求，已具备交付条件。
```

### 6.3 两振熔断机制协议 (Two-Strike Circuit Breaker)
1. **物理计数器**：系统在隐形隔离区 `.ai-memory/.verify_streak` 中原子维护连续自检失败的次数（ASCII 格式数字）；
2. **重置触发**：任意一次自检（含 `--resume-after-break` 恢复自检、`task handover` 与 `task done` 自检）成功获得退出码 `0` 时，计数器文件被原子清理并归零；若恢复自检依然失败，计数器继续累加并保持熔断锁死；
3. **熔断与红线警示**：当连续失败次数 `>= 2` 时：
   - CLI 强制向终端标准错误/输出打印醒目红色熔断框；
   - Agent 必须将当次轮次标记为最终轮次（`IsFinal=true`），严禁继续盲目猜测与死循环修改代码；
   - 必须主动向用户提交错误堆栈并移交控制权。

### 6.4 提示词编译、宿主原生 Hook 与诊断状态机标准
针对 IDE/宿主（如 Cursor）的自动化 Hook 集成与诊断防跳步机制，规范如下协议：
1. **参数与退出码**：
   - 未知/不支持的 `--host` 或 `--event`，或者非法参数组合（如 `--host` 与 `--json` 混用、`--scope` 空白或非法）严格返回退出码 `2` (`[USAGE_ERR]`)；
   - 宿主原生 Hook 模式下，协议内的业务拦截/拒绝向标准输出输出符合宿主协议的 JSON（如 `{"continue":false,...}` 或 `{"permission":"deny",...}`），诊断提示走 stderr，返回非零退出码（`1`），标准输出绝不倾倒信封正文；
2. **上下文产物落盘与精确读取印证**：
   - `beforeSubmitPrompt` 事件中编译信封并落盘至私有目录 `.git/agate/requests/<requestID>.context.md`，同时维护会话级活跃请求指针（`sessions/<hash>.active`）；
   - `postToolUse` 事件精确校验工具读取路径是否与当前请求对应的 `<requestID>.context.md` 产物相匹配，仅匹配成功时才将交付状态流转为 `observed_read` 并记录 `DeliveryEvidence`；
3. **写入前门禁与诊断连续性**：
   - 文件编辑（`Write`/`Edit`）在 `preToolUse` 前必须已完成上下文交付，且诊断状态为 `confirmed` 或 `not_applicable`，并经过用户单独的「执行」和已确认文件范围核验。未诊断（`undiagnosed`）或待验证假设（`hypothesis`）阻断文件编辑；
   - Shell 在会话有效后、进入诊断检查之前放行，不按命令文本筛选。没有会话标识的 Shell 拒绝。MCP 不能借用该授权。该放行不替代上文的 Git `pre-push`。行为见 [多宿主统一 Native Hook 契约](../contexts/context.md#多宿主统一-native-hook-契约)；
4. **受校验的诊断状态更新**：
   - 支持通过 `agate prompt --set-diagnosis <status> --request-id <id> --evidence "<文本>"` 进行状态更新；若指定 `confirmed`，必须通过五要素文本结构校验（现象与预期、位置与证据、因果解释、不确定项、方案依据），拦截有 Plan 无原因说明的违规跳步。
5. **Antigravity IDE 2.8.0**：
   - 拒绝时标准输出仅为 `{"decision":"deny","reason":"..."}`，不混入信封正文；
   - `PostInvocation` 未携带 `response` 时，从 `transcriptPath` 读取 `step_index` 不大于本次 `initialNumSteps` 的最后一条 `PLANNER_RESPONSE`，只从中解析一个 `agate-plan`；
   - 确认轮没有 TurnID 时绑定当前快照的 `ActiveTurnID`。没有可信围栏时不登记候选方案，未确认或超出方案文件范围的写入必须拒绝。行为与实机边界见 [Antigravity IDE 原生 Hook](../contexts/context.md#antigravity-ide-原生-hook)。

---

## 七、 自包含单文件 HTML 审查报告规范 (Self-contained Transcript)

### 7.1 审查载体设计规范
针对人类审查、合规归档与异步复盘场景，`agate export` 与 `agate view` 遵循如下标准：
1. **零外部网络依赖**：禁止引用外部 CDN 样式、字体或远程脚本，单文件必须内嵌完整 CSS 与轻量 JS，双击即开；
2. **长文本与日志折叠**：原生使用 `<details>/<summary>` 对任务目标、测试控制台日志、技术契约与 Diff 差异进行层级折叠；
3. **红绿对比、自动脱敏与安全体检**：高维直观呈现本次改动的 `git diff`（支持暂存区/工作区差异）以及已执行敏感信息脱敏的 Phase 0 7 大安全卡口扫描细节，明确区分“未执行动态测试”与“动态测试未通过”；
4. **隐形存储隔离**：默认写入 `.ai-memory/reviews/review-<timestamp>.html`，天然受 `.git/info/exclude` 隐形隔离，保障业务仓库 100% 零污染。

---

## 八、 跨 Agent 任务接力与记忆治理规范 (Agate Relay)

### 8.1 双模任务接力单规范 (TASK.md)
1. **YAML Frontmatter 状态机**：包含 `task_id`、`title`、`status`、`current_agent`、`next_agent`、`current_role`、`next_role`、`receipt_html`、`snapshot_digest` 与 `updated_at`。各状态转换契约如下：
   | 动作命令 | 前置状态 | 目标状态 | 附带动作与副作用 |
   | :--- | :--- | :--- | :--- |
   | `agate task claim` | `TODO` / `HANDOVER_READY` | `CLAIMED` | 抢占创建 `.ai-memory/locks/task.lock`（租约 2 小时，记录 `role` 与 `session_id`），当指定 `--role` 时生成 `.ai-memory/sessions/active-role.md` 会话角色卡 |
   | `agate task handover` | `CLAIMED` / `IN_PROGRESS` | `HANDOVER_READY` | 强制执行 Phase 0 与动态测试门禁（联动 `guard.strict` 与两振熔断）、绑定代码快照摘要 `snapshot_digest`、编译 HTML 物证并向正文追加交接时间线 |
   | `agate task resume` | `HANDOVER_READY` / `BLOCKED` | `IN_PROGRESS` | 核验指定 `next_agent` 与 `next_role` 接棒权限、核对 `snapshot_digest` 代码快照一致性（`test`/`ops` 角色发现快照过期或物证缺失时输出阻塞级复验提示）、继承前任接力四要素、刷新持锁人信息与当前时间戳 |
   | `agate task done` | `CLAIMED` / `IN_PROGRESS` / `HANDOVER_READY` | `DONE` | 执行全量自检闭环（联动 `guard.strict` 与两振熔断，`ops` 角色强制校验审查物证与最新 `snapshot_digest` 绑定）、解绑并清空互斥锁、打上完成时间戳 |
2. **接力四要素 (Handover Quartet)**：正文必须结构化包含【已完成事项】、【在途断点】、【接棒建议与下一步】以及【暗坑警示】；
3. **不可篡改流转审计时间线 (Handover Timeline)**：
   - 表头格式：`| 交接时间 | 交接者 (From) | 接棒者 (To) | 审查物证凭单 | 核心备忘批注 |`；
   - 截断与转义防护：解析时严格防御 Markdown 表格对正文“暗坑警示”四要素的污染，备注内 `|` 自动转义为 `\|`；首行统一进行 CRLF 归一化。
4. **互斥软租约锁与过渡守护**：多 Agent 与同工具多会话施工通过 `.ai-memory/locks/task.lock` 互斥保护（含 `session_id` 隔离，默认 2 小时租约），加锁（`AcquireLock`）与释放锁（`ReleaseLock`）均由 `.task.lock.transition` 原子排他守护（带 30s 自动过期原子重命名自愈），彻底消除并发抢锁与释放竞态；`handover` 与 `done` 强制校验持锁所有权，`DONE` 终态任务严禁重新认领。

### 8.2 记忆资产分层治理 (Memory Governance)
1. **L0 任务级临时记忆**：偶发报错、临时网络状态与在途断点仅允许记录于 `TASK.md`，交接或任务归档后自然消亡，严禁全局泛化；
2. **L1 长期工程记忆**：`MEMORY.md` 仅允许人类显式指令沉淀，Agent 默认绝对只读；遇到重大通用架构暗坑仅可发起“提议”，由人类确认后单次追加。

---

## 九、 命名空间与专属品牌规范 (Namespace & Brand Standard)

### 9.1 唯一专有命名空间
系统在架构设计、命令行接口、Git Hook 门禁及配置路径上全面统一使用 `agate`（Agent Gate）专有品牌与命名空间，严禁保留历史旧别名：
1. **CLI 命令与二进制**：全局仅使用 `agate` 独立可执行二进制；Git Hook 物理门禁（`pre-commit` / `pre-push`）仅识别并调用 `agate`；
2. **私有隔离标识**：`.git/info/exclude` 仅使用 `# --- agate private tracking start ---` 与 `# --- agate private tracking end ---` 规范标记块；
3. **全局规约路径**：规约探测引擎标准候选路径为 `~/.config/agate/rules.md` 与 `~/.agate/rules.md`。

---

## 十、 可选 Jev 语义判断适配规范（规划）

Jev Skills 是 Agate 之外的可选判断能力，用于为语义模糊的检查点提供结构化建议；其不是 Agate 的运行时前置依赖，也不是安全、授权或合并决策边界。

1. **适用范围**：仅可用于验证失败分流、任务交接完整度、候选 Agent/工具路由和审查优先级等无法由确定性规则充分表达的问题；
2. **默认关闭与显式同意**：仅在用户明确启用时调用。API 模式必须先确认用户同意并仅检查密钥是否存在，严禁索取、回显、记录或提交密钥；
3. **最小化与脱敏**：发送到外部服务的上下文必须去除密钥、个人路径、私有配置和无关源码，只保留完成当前判断所需的证据与候选项；
4. **建议不等于授权**：Jev 输出不得放行 `pkg/guard` 拦截、跳过测试、解除 Hook、修改 CI 结果，亦不得直接触发提交、推送、发布、部署或合并；
5. **不确定性处理**：`needs_review`、空值、未知结果、API 错误或上下文不足，必须转为人工确认或补充证据，不得静默重试、模拟或自动放行；
6. **模拟模式标识**：在无 API 调用的 Agent 模拟模式中，输出必须明确标记为模拟，不得伪造 Jev 概率、置信度或调用回执。

---

## 十一、 开放式工具扩展与独立窗口视图规范 (Open Profile & Workspace View)

### 11.1 三大规则渲染范式与工具档案契约
1. **`markdown`**：以工具专属 HTML 注释受管区块（`<!-- agate:<target>-rules:start/end -->`）合并完整规约，严格保留受管区块外的用户自定义文本与换行符；
2. **`frontmatter-md`**：面向 `.mdc` 等带 YAML 头部的规则文件，新建时自动注入顶部 YAML Frontmatter（默认 `description: Agate Engineering Guardrails` 与 `alwaysApply: true`，或读取 `frontmatter` 自定义配置），更新时严格保留已存在的第 1 行用户 YAML Frontmatter；
3. **`pointer`**：面向轻量或具备跨文件读取能力的工具，仅在受管区块内写入 3-Hop 寻路、两阶段硬门禁与 `agate verify` 引导块，并内嵌 `<!-- agate:rules-sha256:<hex> -->` 完整规约摘要以确保版本一致性盘点；
4. **动态环境指纹**：支持在档案中声明 `env_keys`（强指纹）与 `term_programs`（弱指纹），通过 `agate profile add <name>` 落盘至个人全局（`~/.agate/config.toml`）、团队项目（`.agate/config.toml`）或本机私有（`.agate/config.local.toml`）。

### 11.2 三层配置合并与安全单调收紧规范
1. **加载顺序**：`~/.agate/config.toml`（或 `AGATE_GLOBAL_CONFIG`） $\rightarrow$ `.agate/config.toml` $\rightarrow$ `.agate/config.local.toml`；
2. **单调收紧（Monotonic Tightening）**：本机私有 `.agate/config.local.toml` 仅允许收紧安全红线，禁止将团队已启用的 `guard.strict = true` 或 `hooks.install = true` 降级为 `false`，亦禁止降低 `guard.todo` 与 `guard.absolute_path` 的拦截等级。

### 11.3 窗口级独立视图规范 (`agate workspace`)
1. **物理存储与零侵入**：视图文件固定生成于 `.ai-memory/workspaces/<target>.code-workspace`，严禁修改团队共享 `.vscode/settings.json` 或删除、移动任何物理规则文件；
2. **精确过滤边界**：仅在当前工具窗口的 `files.exclude` 与 `search.exclude` 中隐藏其他工具的规则文件路径，保留当前工具规则及 `MAP.md`、`contexts/context.md`、`TASK.md`、`MEMORY.md`，严禁整体隐藏 `.gemini` 或 `.cursor` 目录；
3. **显式免责声明**：CLI 必须向用户提示“隐藏仅影响当前窗口展示，不代表停止客户端加载底层物理文件”；对不支持 `.code-workspace` 的纯 CLI 工具（如 `codex`、`claude`）明确降级跳过。

---

## 十二、 多源身份感知与跨会话安全信封规范 (Multi-Source Context & Session Envelope)

### 12.1 宿主编辑器与执行者角色解耦规范
1. **双角色分离**：统一区分**宿主编辑器 (`Editor`)** 与**实际执行者 (`Runner`)**，禁止将仅处于某 IDE 集成终端内的普通 Shell 命令无条件等同于自动化 Agent 正在执行；
2. **三态冲突仲裁 (`known` / `unknown` / `ambiguous`)**：全量汇总所有候选特征而非顺序短路盲选；若同一角色命中多个不同工具的强特征，状态标记为 `ambiguous` 并阻断自动回退；
3. **装配与任务调用解耦**：`EffectiveTarget()` 用于规则装配时，优先采信已确知的 `Runner`（即使底层 `Editor` 存在历史残留冲突）；`agate task` 写命令仅采信确知的 `Runner` 身份，无确知 `Runner` 时强制要求显式传入 `--agent <name>`。

### 12.2 远程服务端路径白名单与受限祖先进程探针规范
1. **白名单路径对照**：识别 `/.cursor-server/` 与 `/.vscode-server/` 远程服务端目录特征，严格区分**当前会话专有路径变量**（如 `VSCODE_GIT_ASKPASS_NODE`、`VSCODE_IPC_HOOK_CLI`）与**全局残余 `PATH`**；纯 `PATH` 命中必须具备同厂商活跃终端互证且无多服务端并存冲突方可升级为 `known`；
2. **受限祖先进程探针 (`ProcessInspector`)**：在 Linux 环境下沿 PPID 向上最多采集 4 层（硬上限 8 层，内置 `visited` 环路防护与多实例去重），仅读取 `/proc/<pid>/stat` 与 `/proc/<pid>/exe` 符号链接，**严禁读取完整命令行参数 (`/proc/<pid>/cmdline`)** 以防泄露命令行凭据；非 Linux 平台或权限受限时静默安全降级；
3. **活跃进程优先约束**：当存在明确属于某编辑器的活跃父进程证据时，自动压制环境变量中跨宿主的残留会话变量与残留 `PATH` 并记录告警。

### 12.3 `AGATE_SESSION_CONTEXT` 跨会话安全信封规范
针对 SSH 远程转发、`tmux` 复用或子 Shell 丢失父级环境变量的场景，支持通过 `AGATE_SESSION_CONTEXT` 传递显式会话声明（格式：`v=1;editor=<name>;runner=<name>;src=<source>;iat=<unix>;exp=<unix>;sid=<id>`），并强制执行五重防线：
1. **长度与格式边界**：总长度严格 `<= 512` 字节，拒绝任何缺失 `=` 的畸形片段或未知扩展字段；
2. **版本锁定**：强制要求 `v=1`；
3. **凭据防火墙**：内置高危键名（`token`/`key`/`secret`/`password`/`authorization`/`bearer`/`cookie`）与常见 API 密钥前缀（`sk-`/`ghp_`/`gho_`/`ak_` 等）扫描，命中即整段作废；
4. **时效与时钟偏移强校验**：强制要求提供 `exp` 过期时间戳（允许指定 `iat`，拒绝 `exp < iat` 时序倒挂与超过 `MaxClockSkew` 的未来签发时间）；
5. **受控来源白名单与会话隔离**：`src` 仅允许 `terminal`、`ssh`、`extension`、`ide`、`ci` 五类受控来源（拒绝 `unknown`、`custom` 或含路径/特殊字符的伪造来源）；当信封携带 `sid` 且与当前终端现场 `AGATE_SESSION_ID`/`TERM_SESSION_ID` 不一致时立即拒绝；`SessionID` 在 JSON 序列化（`json:"-"`）与 `agate detect --explain` 终端输出中强制脱敏隐藏。

---

## 十三、 提示词物理锚定与双轨纠偏门禁规范 (`agate prompt`)

### 13.1 四源物理现场采集与安全过滤规范
1. **四源聚合**：`agate prompt` 确定性收集 Git 活跃现场（分支、Commit、脏文件列表与 `.verify_streak` 连续失败计数）、`MAP.md` 关联模块切片、`TASK.md` 非 `DONE` 在途断点（提取接力四要素中的在途断点与下一步建议）以及 `MEMORY.md` 准入卡片（严格按卡片 `生效范围` 精确匹配，严禁准入缺少生效范围的章节或无关模块卡片）；
2. **候选路径安全过滤**：扫描指令或 `--scope` 中的路径时，强制过滤 URL 协议（`http://`、`file://`、`git@`）、路径穿越（`../`）、绝对路径/Windows 盘符以及非源码后缀的纯中文斜杠词对（如“输入/输出”），并自动剥离孤立的顶层容器目录（`pkg`、`internal`）以防切出全仓所有子包。

### 13.2 三级锚定置信度与双轨分流规范
1. **`explicit`（高置信度精确锚定）**：当指令已包含明确文件路径/模块或通过 `-s/--scope` 显式指定时触发。信封自动折叠无关工作区脏文件列表（仅保留脏文件总数摘要），并将 `agate-intent` 第 3 项切换为“收敛模式”（仅核对直接调用方签名与单测，无额外连带影响直接填“无（指令边界已明确）”）；
2. **`inferred_dirty`（低置信度脏文件推测）**：当指令未指明路径且未启用 `--no-dirty`，仅靠当前 Git 活跃脏文件兜底关联时触发。信封必须标注 `[推测锚定-待核验]` 警示，要求宿主模型核对指令语义与脏文件是否一致，不一致时严禁强行采信；
3. **`unresolved`（未决待澄清）**：当未命中任何路径且工作区干净（或启用 `--no-dirty`）时触发，引导宿主模型结合上下文与 3-Hop 双地图向用户反问澄清。

### 13.3 显式纠偏参数、Hook 载荷与退出码规范
1. **CLI 纠偏参数**：支持 `-s/--scope`（支持逗号分隔或多次传入，自动去重排序）显式锁定物理目标，支持 `--no-dirty` 忽略工作区无关脏文件兜底；
2. **Hook 模式安全提取 (`--hook`)**：从标准输入读取纯文本或 IDE Hook JSON 载荷（提取 `prompt`/`user_prompt`/`message`/`instruction`/`content`/`text` 字段）；若输入为合法 JSON 对象但未包含有效非空指令文本，严禁将原始 JSON（可能含 `session_id` 等敏感元数据）回退当作指令输出；
3. **用法错误拦截 (退出码 `2`)**：空指令、显式空白 `-s ""`、越界/非法 `--scope` 路径或单独指定顶层容器目录（`-s pkg` / `-s internal`）统一返回 `*cmd.UsageError`（退出码 `2`）。
