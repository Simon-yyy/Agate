# Agate (Agent Gate) 协议与规范标准手册

## 一、 AI 协同规约标准 (SKILL.md / SSOT)

Agate 遵循**单一事实源（Single Source of Truth, SSOT）**原则：所有面向 Agent（Cursor、Codex、Antigravity、Claude Code、Windsurf 等）的核心协同与安全规约均由统一模板文件维护，并由 Go `//go:embed` 静态编译内置于可执行文件中，禁止在各处文档中重复硬编码全文以防规约漂移。

完整规约模板源码请直接参阅：[internal/templates/SKILL.md](../internal/templates/SKILL.md)

### 1.1 规约体系核心板块纲要

规约模板收敛为以下 7 大核心工程协同基线：

1. **中文原生与代码完整性**：
   - 交互思考、排查分析、注释与 Git Commit Message 规范使用简体中文；
   - 代码必须完整可运行，严禁使用 `// ... 保持不变` 等偷懒省略占位符。
2. **两阶段硬门禁 (Two-Phase Gate)**：
   - 探查与修改任务必须先输出只读诊断与精简 Plan，提示“👉 请确认方案，确认请回复‘执行’”，严格等待人类确认后方可在次轮落地改动。
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
   - **块级防破坏追加**：写入 `.git/info/exclude` 时必须使用包含标记符（`# --- agate private tracking start ---` 与 `# --- agate private tracking end ---`）的专用受管块，新追加项精准插入在 end 标记内侧，绝不覆写用户原有的排除配置；
   - **完全本地生效**：`.git/info/exclude` 仅在开发者本地 `.git` 目录生效，不进版本库，不修改项目公共 `.gitignore`，杜绝协同 Diff 噪音。

---

## 三、 Git Hook 门禁安装与防篡改策略 (M4 规范)

### 3.1 拦截阶段与职责
- **`pre-commit` 门禁**：在提交暂存区代码前强制调用 `agate verify --staged`。直接从 Git Index（暂存区）提取待提交文件并通过 `git show :<path>` 针对性审查，毫秒级快速就绪，精准拦截私有密钥、超大文件、机器硬编码绝对路径、未完工占位符等违规内容，工作区脏文件不产生误拦；
- **`pre-push` 物理阻断**：严格阻止 AI 在未经用户指令时执行 `git push` 偷跑代码至远端。在非交互式终端中，唯有显式声明 `ALLOW_AUTOMATED_PUSH=1` 或 `true` 环境变量白名单才准予进入全量验证并放行。

### 3.2 Hook 安装与防篡改策略
1. **非破坏性备份**：若检测到开发者已有同名 Hook，必须自动重命名备份（如 `pre-commit.agate.bak`），严禁静默覆盖；
2. **标记行识别**：写入的脚本必须带有 `# --- agate hook: <type> ---` 专用标识行；
3. **精准清理还原**：在执行卸载或移除时，仅精准移除由 agate 管理的钩子并还原原备份脚本，完好保留用户自定义钩子；
4. **规约与原子安全落盘**：向工作区分发规约（如 `.cursorrules`）时，内容一致免写，存在不同既有内容时自动备份为 `[文件名].agate.bak`，并通过临时文件 + `fsync` + `rename` 真原子落盘（崩溃安全）。

### 3.3 状态可观测性
- 支持通过 `agate hook status` 结构化查询 `core.hooksPath`、`pre-commit` 与 `pre-push` 托管状态，提供开箱即用的门禁可视看板。

---

## 四、 项目级配置规范 (`.agate/config.toml`)

项目可以通过 `.agate/config.toml` 覆盖默认 Agent 目标、严格验证和 Hook 安装策略。优先级为：命令行参数 > 项目配置 > 当前 Agent 自动识别 > 项目已有配置 > 内置默认值。

```toml
[agent]
targets = ["codex", "cursor"]

[guard]
strict = true
todo = "warning"
absolute_path = "error"
large_file_mb = 20

[hooks]
install = true
```

`agent.targets`、`guard.strict` 和 `hooks.install` 当前生效。`guard.todo`、`guard.absolute_path` 与 `guard.large_file_mb` 当前作为可解析占位字段，Agate 会提示暂未启用对应策略，不会静默改变审计行为。配置语法错误或未知字段会阻止初始化/验证并返回错误。

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
| **`2`** | `[USAGE_ERR]` | CLI 命令参数解析错误或运行时内部异常 | 阻断流程，检查命令调用传参合法性 |

### 6.2 交付物证格式
Agent 向用户交付时，聊天框必须包含类似下述格式的实机物证回显：
```text
=== [agate verify] 闭环自检完成 ===
[PASS] 本地交付物校验通过，结构完备 (耗时: 264ms)
所有修改符合门禁要求，已具备交付条件。
```

### 6.3 两振熔断机制协议 (Two-Strike Circuit Breaker)
1. **物理计数器**：系统在隐形隔离区 `.ai-memory/.verify_streak` 中原子维护连续自检失败的次数（ASCII 格式数字）；
2. **重置触发**：任意一次自检成功获得退出码 `0` 时，计数器文件被原子清理并归零；
3. **熔断与红线警示**：当连续失败次数 `>= 2` 时：
   - CLI 强制向终端标准错误/输出打印醒目红色熔断框；
   - Agent 必须将当次轮次标记为最终轮次（`IsFinal=true`），严禁继续盲目猜测与死循环修改代码；
   - 必须主动向用户提交错误堆栈并移交控制权。

---

## 七、 自包含单文件 HTML 审查报告规范 (Self-contained Transcript)

### 7.1 审查载体设计规范
针对人类审查、合规归档与异步复盘场景，`agate export` 与 `agate view` 遵循如下标准：
1. **零外部网络依赖**：禁止引用外部 CDN 样式、字体或远程脚本，单文件必须内嵌完整 CSS 与轻量 JS，双击即开；
2. **长文本与日志折叠**：原生使用 `<details>/<summary>` 对任务目标、测试控制台日志、技术契约与 Diff 差异进行层级折叠；
3. **红绿对比与安全体检**：高维直观呈现本次改动的 `git diff`（支持暂存区/工作区差异）以及 Phase 0 7 大安全卡口扫描细节；
4. **隐形存储隔离**：默认写入 `.ai-memory/reviews/review-<timestamp>.html`，天然受 `.git/info/exclude` 隐形隔离，保障业务仓库 100% 零污染。

---

## 八、 跨 Agent 任务接力与记忆治理规范 (Agate Relay)

### 8.1 双模任务接力单规范 (TASK.md)
1. **YAML Frontmatter 状态机**：必须包含 `task_id`、`title`、`status`、`current_agent`、`next_agent`、`receipt_html` 与 `updated_at`。各状态转换契约如下：
   | 动作命令 | 前置状态 | 目标状态 | 附带动作与副作用 |
   | :--- | :--- | :--- | :--- |
   | `agate task claim` | `TODO` / `HANDOVER_READY` | `CLAIMED` | 抢占创建 `.ai-memory/locks/task.lock`（租约 2 小时） |
   | `agate task handover` | `CLAIMED` / `IN_PROGRESS` | `HANDOVER_READY` | 触发自检门禁、编译 HTML 物证并向正文追加交接时间线 |
   | `agate task resume` | `HANDOVER_READY` / `BLOCKED` | `IN_PROGRESS` | 继承前任接力四要素、刷新持锁人信息与当前时间戳 |
   | `agate task done` | `CLAIMED` / `IN_PROGRESS` / `HANDOVER_READY` | `DONE` | 执行全量自检闭环、解绑并清空互斥锁、打上完成时间戳 |
2. **接力四要素 (Handover Quartet)**：正文必须结构化包含【已完成事项】、【在途断点】、【接棒建议与下一步】以及【暗坑警示】；
3. **不可篡改流转审计时间线 (Handover Timeline)**：
   - 表头格式：`| 交接时间 | 交接者 (From) | 接棒者 (To) | 审查物证凭单 | 核心备忘批注 |`；
   - 截断防护：解析时严格防御 Markdown 表格对正文“暗坑警示”四要素的污染；首行统一进行 CRLF 归一化。
4. **互斥软租约锁与过渡守护**：多 Agent 施工通过 `.ai-memory/locks/task.lock` 互斥保护（默认 2 小时租约），读写之间由 `.task.lock.transition` 原子排他守护（带 30s 自动过期自愈），彻底消除并发抢锁竞态；handover 与 done 强制校验持锁所有权，DONE 终态任务严禁重新认领。

### 8.2 记忆资产分层治理 (Memory Governance)
1. **L0 任务级临时记忆**：偶发报错、临时网络状态与在途断点仅允许记录于 `TASK.md`，交接或任务归档后自然消亡，严禁全局泛化；
2. **L1 长期工程记忆**：`MEMORY.md` 仅允许人类显式指令沉淀，Agent 默认绝对只读；遇到重大通用架构暗坑仅可发起“提议”，由人类确认后单次追加。

---

## 九、 命名空间与专属品牌规范 (Namespace & Brand Standard)

### 9.1 唯一专有命名空间
系统在架构设计、命令行接口、Git Hook 门禁及配置路径上全面统一使用 `agate`（Agent Gate）专有品牌与命名空间，严禁保留历史旧别名：
1. **CLI 命令与二进制**：全局仅使用 `agate` 独立可执行二进制；Git Hook 物理门禁（`pre-commit` / `pre-push`）仅识别并调用 `agate`；
2. **私有隔离标识**：`.git/info/exclude` 仅使用 `# agate private tracking start` 与 `# agate private tracking end` 规范标记块；
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


