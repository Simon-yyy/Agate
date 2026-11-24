# HarnessGate 宿主自动识别与 Cursor 门禁修复实施方案

日期：2026-11-19（北京时间）

状态：部分实现，多宿主原生接入尚未通过验收。当前缺项见[问题与回归记录](harnessgate-session-findings-2026-11-19.md#912-当前未关闭问题与验收清单2026-11-20)。

**目标**：用户运行 `agate init` 时，Agate 尽可能识别当前宿主。有可靠的唯一结果和有效的工具档案时，自动写入对应规则入口。结果不足或冲突时，解释原因并提示 `agate init -t <工具名>`。新 Agent 可用可复用档案接入。Cursor 则分别验证请求交付、诊断状态、用户确认和写入前拦截。

**架构**：复用现有进程采集、工具档案和分发计划。采集器提供当前进程链证据，识别层汇总候选并仲裁，初始化命令依据解析结果决定是否生成写入计划。Cursor 延用现有请求记录，分别记录上下文读取、诊断和用户确认。

**技术栈**：Go、Cobra、现有 TOML 配置、Cursor 原生 Hook。建议先扩展已有的 `profile add`，让自定义进程条件通过现有配置机制端到端生效。

**依据**：[问题与回归记录](harnessgate-session-findings-2026-11-19.md) · [前一版实施方案](superpowers/plans/2026-11-19-harnessgate-init-and-diagnostics-implementation-plan.md)

## 1. 当前基线与实施边界

- 当前工作树已有 Antigravity 进程匹配、`ProcessNames` 字段和部分只读分类的未提交改动。开始实施前记录 `git status --short`、`git rev-parse HEAD` 和相关差异，基于当前工作树续改。
- 「灵动岛」的 `.gemini/GEMINI.md` 已核对正文与 SHA-256，均和新版全局规则一致。该项目剩余工作是验证 Antigravity 实际加载和行为。
- 当前 Cursor 项目配置注册了提交事件与工具调用后事件。观察到的请求记录缺少模型读取证据，写入前 Hook 尚未启用。
- 依据真实宿主信号登记进程名。进程名是当前 Editor 的证据，Runner 仍采用现有专用会话信号。保留显式 `-t`、项目或本机已配置的默认目标、规则路径校验、Git 跟踪保护及预览行为。
- 业务写入、任务状态写入、构建和测试均按已有授权处理。实机验收使用临时项目。更新用户安装的二进制、项目配置或客户端 Hook 时，核对对应操作的授权。

## 2. 任务一：贯通工具档案的进程自动识别

**涉及文件**：[配置加载与保存](../pkg/config/config.go#L343) · [档案命令](../cmd/profile.go#L117) · [档案定义与校验](../pkg/agent/profile.go#L12) · [进程匹配](../pkg/agent/collector.go#L50) · [候选仲裁](../pkg/agent/registry.go#L65)

- [ ] 在 `pkg/config/config.go` 增加 `process_names` 字段的 TOML 读写。复用原有数组解析及序列化逻辑，覆盖项目、本机和用户全局配置的加载与保存。旧档案缺少字段时，沿用环境变量和终端标识的识别行为。
- [ ] 在 `cmd/profile.go` 为 `profile add` 增加可重复指定的 `--process-name`，将其传入现有档案验证与持久化入口。验证字段接受可执行文件名，拒绝路径、通配符、控制字符和空值。匹配时按目标平台的规则规范化名称。
- [ ] 让进程采集和档案匹配分工。采集器继续获取当前祖先链，匹配器使用档案中经核验的进程条件。迁移重复维护的内置匹配项，保留 VS Code 等只有 Editor 身份、没有规则分发入口的识别能力。
- [ ] 将同一进程命中的所有不同工具加入候选集。去重按工具标识，同一进程命中多个不同档案时保留冲突证据，由现有仲裁逻辑结合 Runner 和 Editor 决定结果。排序只用于稳定输出。
- [ ] 补回归用例。验证 `profile add` 写入配置后能够重新加载，`process_names` 在三个配置层按现有合并行为生效。用模拟的实际祖先链验证内置 Antigravity、自定义进程名、同名冲突、通用 `TERM_PROGRAM=vscode`、相似文件名以及明确 Runner 优先的场景。

**通过条件**：用同一个二进制只增加一份可复用档案，便能让编译时未内置的工具通过进程证据参与识别。测试包含真实配置入口和 `init` 调用，避免仅构造内存档案验证匹配函数。

## 3. 任务二：可靠识别时分发，失败时提示手动参数

**涉及文件**：[初始化编排](../cmd/init.go#L67) · [目标解析](../pkg/adapter/adapter.go#L86) · [分发计划](../pkg/adapter/plan.go#L136)

- [ ] 保留显式 `-t` 与配置中的默认装配集合。两者都没有时，复用一次当前环境检测。只有得到可靠且唯一的目标和有效档案才建立自动分发计划。
- [ ] 将宿主未知、候选冲突、已识别但没有规则档案分开处理。诊断输出说明缺项和候选，给出 `agate init -t <工具名>` 用法。工具尚未注册时，提示先补档案，可用 `agate profile list` 查看已有目标。
- [ ] 调整 `ResolveTargetsWithProfiles` 和 `cmd/init.go` 的衔接。已有规则入口仅用于给手动选择提供线索。没有可靠目标时，在规则写入、脚手架初始化及 Git 配置更改之前结束。`--dry-run` 应显示同样的失败依据。
- [ ] 为受 Git 跟踪的规则保留原保护。只有冲突的实际原因是缺少覆盖授权时，才提示 `--allow-tracked`。受管区块不合法、入口碰撞等问题显示各自原因。
- [ ] 补命令层回归。覆盖可靠匹配自动装配、未识别且无已有入口、未识别但有多个已有入口、单个旧入口、候选冲突、缺档案、显式覆盖及默认目标。断言未知场景不会创建或修改文件，补充 `-t` 后只装配所选目标。

**通过条件**：无参数初始化在可靠命中时成功，在证据不足时返回具体的手动命令和候选。测试覆盖原来会沿用旧入口写盘的分支。

## 4. 任务三：验收 Cursor 的请求触发与上下文交付

**涉及文件**：[Cursor 事件处理](../pkg/prompt/cursor_hook.go#L75) · [请求状态](../pkg/prompt/workflow.go#L26) · [项目前两项 Hook](../.cursor/hooks.json#L4)

- [ ] 核对 Cursor 版本、项目实际加载的 Hook 配置和调用的二进制。保持提交事件触发编译、读取事件记录交付的协议。将实机验证所见的事件字段按最小范围记录。
- [ ] 补齐已获准修改的常驻项目规则，让模型能根据当前请求定位并读取本次上下文产物。核对会话、请求与工作区的关联方法，避免读取旧请求或另一工作区的产物。
- [ ] 在 `postToolUse` 只对实际命中当前产物的读取登记 `delivery_evidence`。触发失败、产物生成失败、产物尚未读取和读取关联失败分别给出可检查的结果。
- [ ] 增加或补齐事件测试。覆盖同会话第二条请求、重复事件、无效载荷、读取其他产物和工作区不同等场景。重复事件应保留真实已读结果，不能让另一条请求继承该结果。
- [ ] 重载真实 Cursor 后发两条独立请求。逐条保存提交事件、编译产物、实际模型读取和对应的 `delivery_evidence`。再记录一条失败路径及其可定位提示。

**通过条件**：每条真实请求都有各自的编译和交付记录。只有编译文件或仍为 `pending_read` 的记录时，对应验收项保留为待完成。

## 5. 任务四：精细化写入检查并绑定用户确认

**涉及文件**：[写入前检查](../pkg/prompt/cursor_hook.go#L136) · [Shell 分类](../pkg/prompt/cursor_hook.go#L252) · [诊断门禁](../pkg/prompt/workflow.go#L52)

- [ ] 用宿主提供的结构化只读工具支持前置调查，例如原生读取、目录浏览和搜索。Shell 分支只允许可可靠验证的具体只读操作。去掉对整类 `agate`、Git 或 `go test` 的只读前缀放行。不能可靠判定边界的 Shell 请求按普通受控操作处理。
- [ ] 将上下文交付、诊断状态和用户确认分别记录。用户确认绑定工作区、会话、当前方案标识和规范化目标范围。新任务、纠偏或方案范围改变时，使此前确认对变化后的方案失效。
- [ ] 核对客户端能否可靠取得用户确认事件。以可归因的宿主确认或现有可信批准入口登记确认，保留其范围和时间。诊断登记可以独立完成。若当前客户端无法提供可靠确认信号，真实客户端验收标为缺项，现有项目的 `preToolUse` 挂载留到该项通过后。
- [ ] 对原生编辑、Shell 和 MCP 分别定义检查点。能获得结构化文件路径时检查目标是否属于已确认的范围。缺少结构化范围的写入请求继续走明确的操作授权。Git 提交、推送、安装或部署等外溢动作按已有独立授权契约处理。
- [ ] 覆盖只读可用、没有交付证据、没有诊断、诊断仍是假设、只有诊断未获用户确认、已确认且目标在范围内、超范围、其他工作区、纠偏后确认失效等用例。Shell 测试还要覆盖允许的只读调用和会产生写入或执行项目代码的调用。
- [ ] 在获准的临时 Cursor 项目完成真实写入检查。先验证只读调查可继续，再验证未获确认和超范围写入被拦截，最后验证获准修改能执行。记录原生编辑、Shell 和 MCP 各自覆盖情况。真实链路通过后，再按授权更新实际项目的 Hook 配置。

**通过条件**：只读调查可执行，写入分别受交付、诊断和用户确认约束。对业务修改的确认只在当前方案范围内生效。

## 6. 验收执行顺序与证据

已有 Cursor 实测记录、客户端版本与专用二进制的覆盖范围见[问题文档第 8 节](harnessgate-session-findings-2026-11-19.md#8-cursor-真实会话验收)。

1. **完成任务一和任务二，再交付宿主识别。** 用同一新产物在真实 Cursor、Antigravity 及一个编译时未内置的 Agent 环境验证自动初始化。临时项目没有预设 `agent.targets` 或已有入口。先运行 `detect --explain` 和 `init --dry-run`，确认选择依据为「当前 Agent 环境」，再运行无参数 `agate init` 验证文件落盘。加测可靠识别失败、提示手动选择及显式 `-t` 重试。
2. **验证「灵动岛」加载。** 先核对本机会话使用的规则版本，再在新的 Antigravity 会话发出只读排查请求。记录实际可取得的加载证据。只能观察模型输出时，记录为行为测试。规则正文和 SHA-256 一致已核对，可直接进入该步骤。
3. **完成任务三后再验收任务四。** 保存真实会话的编译、交付、诊断、确认及工具决定。分别登记可观测的聊天答复约束和可实测的工具拦截。
4. **汇总交付。** 保存二进制路径和版本、客户端版本、工具档案来源、规则来源及摘要、脱敏输出和各用例结果。实现完成后更新 `MAP.md`、`contexts/context.md` 及实施状态。若某项不可验证，记录原因和实际覆盖范围。

获得实施与验证授权后，从仓库根目录执行：

```powershell
go test -mod=vendor ./pkg/agent ./pkg/config ./pkg/adapter ./pkg/prompt ./cmd
if ($LASTEXITCODE -ne 0) { throw '针对性测试未通过' }

$agateValidationDir = [IO.Directory]::CreateTempSubdirectory('harnessgate-validation-').FullName
$agateValidationExe = Join-Path $agateValidationDir 'agate.exe'
go build -mod=vendor -o $agateValidationExe .
if ($LASTEXITCODE -ne 0) { throw '临时 Agate 构建失败' }

& $agateValidationExe verify
if ($LASTEXITCODE -ne 0) { throw 'Agate 自检未通过' }
```

真实终端验收使用上述临时二进制的明确路径。只有通过相应真实宿主、工具及授权用例，才将该项登记为已通过。现有编码改动、测试通过和真实客户端结果分别记录。

## 7. 回退与后续接手

- 按任务维护独立变更与证据。出现回归时，只撤回该任务的实现、测试及相应配置，保留用户已有改动。
- 客户端 Hook 启用失败时，回退至实机验证之前的配置。已确认正确写入的项目规则可继续使用。
- 记录下一项测试的命令、环境及期望结果。计划执行前按项目门禁确认实施范围。


## 8. 多宿主原生 Hook 实施方案

> 执行者按任务顺序落实。获得实施授权后，使用 Superpowers 的 `executing-plans` 或 `subagent-driven-development`。复选框用于记录实际完成情况。

**目标**　让第 8.3 和 8.4 节所列工具都能通过各自的客户端原生事件接入 Agate，达到当前 Cursor 已实测的门禁效果。`agate init` 尽可能识别当前宿主。识别到已验收的客户端版本时，同时装配规则和原生 Hook。识别不出或存在冲突时提示 `-t`。

**架构**　复用 `pkg/agent` 的识别和 `pkg/adapter` 的规则分发。提取独立的请求状态与写入判定，由各宿主适配器转换原生事件及回复。原生 Hook 的版本、覆盖工具与装配状态单独记录。

**技术栈**　现有 Go、Cobra、TOML、JSON、Git 私有目录及客户端原生 Hook。使用仓库现有依赖。

**依据**　[项目地图](../MAP.md) · [技术契约](../contexts/context.md) · [0.6.0 原生入口](../pkg/prompt/cursor_hook.go#L54)

### 8.1 现状与交付要求

- 0.6.0 的 `init` 已支持按环境识别内置或已注册的工具。识别不足时提示 `-t`。已有规则文件只能用来提示候选。`profile add --process-name` 可为新工具添加进程证据。
- `agate prompt --hook --host <宿主> --event <事件>` 目前只接受 Cursor。编译、状态存储、文件范围判定和 Cursor JSON 协议集中在 `pkg/prompt`。已验证的 Cursor 流程包括本轮原生读取、用户「执行」、范围内写入和越界拒绝。
- **统一原生验收标准**。每个目标至少选定一个可运行的客户端版本和平台，验证以下完整流程：用户提交触发本轮上下文编译，客户端成功读取本轮上下文并产生回执，Agate 登记当前方案及精确文件范围，用户在同一会话回复「执行」，客户端在实际写入前调用 Agate 执行判定。确认前和越界写入应被客户端拦下。范围内写入在取得本轮回执和有效确认后应成功。事件名称、配置和返回格式由各宿主实现。
- **原生验收状态**。规则分发、Hook 配置落盘、客户端事件观察以及完整门禁实测各自记录。只完成前几项时，标明具体已完成项与原生接入缺项。宿主缺少可信用户提交、读取回执或写入前拦截时，在相应客户端版本上标为原生接入受阻，记录证据和可核验的替代官方接口。
- 现有 `init` 的 `--no-hook` 控制 Git 的 `pre-commit` 和 `pre-push`。保留其含义。客户端规则、原生 Hook 和 Git Hook 的配置状态各自报告。
- 既有 Cursor 配置及命令继续可用。已安装的 Hook 应能启动新一轮；升级前缓存中的待确认和已确认方案不授予升级后的写入权限。用户仍可重新提交方案并确认。
- 自定义工具档案继续只声明规则入口与识别证据。等该工具有验证过的原生协议适配器，再开启相应版本的写入门禁。任意档案字段、版本自报和存在同名配置文件均不能直接开启强制拦截。
- 交付以宿主、版本、平台和工具通道为单位。对一个宿主核验了 `Write` 或 `apply_patch`，就登记该通道的实测覆盖。Shell、MCP、子代理、远程会话和其他写文件通道各自核验。
- 本方案拟定首批顺序为 **Cursor 兼容回归 → Antigravity IDE → Claude Code、Codex、Windsurf**。扩展批次纳入 GitHub Copilot、TRAE、OpenCode、Qwen Code、Cline 和 Kiro，并分别核验 Antigravity CLI 与 Gemini CLI 的兼容。通过各自验收的客户端无需用户先执行 `profile add`。本文中的接口、文件名和新增参数为实施提案。

#### 方案选型

| 方式 | 适用条件与代价 |
| --- | --- |
| 各宿主独立实现整套流程 | 适合快速验证事件。规则变更、并发修复和回归测试会在各宿主重复，难以保证审批语义一致。 |
| 共用状态与判定，各宿主保留协议适配（采用） | 同一套逻辑判断已读、审批和目标范围，分别处理各宿主配置、输入字段和拒绝响应。可逐个装配并回退。 |
| 配置驱动映射所有事件 | 字段重命名容易扩展，但无法单靠配置解决可信用户提交、补丁多路径和各版本错误处理。后续仅为语义完全一致的协议开放声明式映射。 |

#### 实施时要先验证的失败场景

| 场景 | 预期 | 对应用例 |
| --- | --- | --- |
| 同一会话包含多次「执行」，或模型续跑重放了最近用户文本 | 只有真实新提交能消费当前方案 | 任务 1、3、4 |
| Git worktree 共用 Git 公共目录，或客户端返回多个工作区路径 | 授权只对实际工作区生效，根目录不明确时拒绝该写入 | 任务 1、3 |
| 补丁或编辑一次修改多个目标，包含重命名或链接 | 所有实际目标都在范围内才允许 | 任务 1、4 |
| 同名 Hook 已存在、配置变更或脚本在保存后启动失败 | 保留原配置和用户条目，报告冲突或未加载，并可定向回退 | 任务 5、7 |
| 客户端忽略错误、异步执行或某类工具绕过 Hook | 能力按实测通道报告，证据不足时保留规则分发或观察模式 | 任务 3、4、6、7 |
| 同一个编辑器运行不同 Agent，或新增内置名与已有自定义档案重合 | 根据实际运行的 Agent 选入口。标出冲突并保留已有配置 | 任务 6、7 |

### 8.2 统一协议与文件落点

| 路径 | 改动 |
| --- | --- |
| `pkg/prompt/native_hook.go`（新增） | 定义统一事件、判定和适配器注册表。分发给已注册的宿主。 |
| `pkg/prompt/native_workflow.go`、`native_store.go`（新增） | 复用编译器和诊断校验，管理会话快照、读取回执、待确认方案和文件范围。 |
| `pkg/prompt/cursor_hook.go`（修改） | 保持 Cursor 现有输入输出，业务状态和授权判断改用统一流程。保留原函数供现有调用者迁移。 |
| `pkg/prompt/antigravity_hook.go`、`claude_hook.go`、`codex_hook.go`、`windsurf_hook.go`（新增） | 解析各客户端字段，按已验证版本标记提交来源、响应、读取与写入，序列化宿主期待的返回值。 |
| `pkg/agent/native_runtime.go`（新增） | 从已识别的运行环境提取可核实的客户端版本与平台。返回证据来源或缺项原因。 |
| `pkg/adapter/native_plan.go`（新增）及 `pkg/adapter/plan.go` | 生成宿主 Hook 配置计划，复用现有快照检查、跟踪保护、锁和原子写入。宿主 JSON 结构由相应渲染函数负责。 |
| `cmd/prompt.go`、`cmd/init.go`、`cmd/hook.go`（修改） | 接线原生分发、初始化预览、加载状态和定向回退。现有 Git Hook 子命令继续走原入口。 |
| `pkg/prompt/testdata/native/`（新增） | 保留限定版本、脱敏后的事件输入输出样本。禁止添加真实对话、私有文件内容、系统凭据或完整本机路径。 |
| `MAP.md`、`contexts/context.md`、`docs/` 验收记录 | 完成交付时同步入口、项目契约和分宿主的证据。 |

#### 拟定接口

```go
type NativeEventKind string // submit, read_completed, response_completed, before_tool, observe
type NativeAction string    // read, edit, shell, mcp, other

type NativeEvent struct {
    Kind         NativeEventKind
    SessionID    string
    TurnID       string
    EventID      string
    ToolUseID    string
    ClaimedRoots []string
    Action       NativeAction
    Targets      []string
    Command      string
    Text         string
    TrustedUser  bool
    Success      bool
}

type NativeDecision struct {
    Block           bool
    Reason          string
    AdditionalContext string
}

type NativeAdapter interface {
    Decode(eventName string, payload []byte) (NativeEvent, error)
    Encode(eventName string, decision NativeDecision) (stdout []byte, stderr string, exitCode int, err error)
}

func HandleNativeEvent(opts NativeEventOptions) (NativeEventResult, error)
```

- `NativeEventOptions` 带宿主标识、原生事件名、可信启动目录和原始载荷。`NativeEventResult` 带 stdout、stderr 和宿主协议退出码。`cmd/prompt.go` 须让原生协议指定的退出码到达调用方，同时保留现有 CLI 用法错误语义。
- `TrustedUser` 由适配器根据已核验的事件来源赋值，禁止把载荷中自带的同名字段当授权。在只含最近用户文本的事件里，即使 `Text == "执行"`，默认仍为 `false`。工具成功状态由经过核对的成功事件或明确成功结果提供。
- 可信工作区以启动目录定位的项目根为准，并核对客户端返回的工作区。根目录有歧义、宿主载荷与项目不一致，或缺少关联授权所需的标识时，停止本轮放行判定，由适配器返回对应拒绝响应。
- 统一会话记录存于 Git 公共目录的 `agate/requests/v2/<host>/<workspace-hash>/`。工作区键取规范化后的 **worktree 根目录**，不能只取 Git 公共目录。会话快照含活跃轮、去重标识、回执、待确认方案以及已消费记录。用户文本不用于生成轮次标识。
- 状态更新以宿主、工作区、会话为锁粒度。复用现有原子写文件工具，在持锁期间读取、校验并更新一次会话快照。只有上下文产物及快照持久化后才返回成功。重复或过期事件不改写当前授权。锁超时、记录损坏和提交顺序无法判断时，用验证过的拒绝响应处理受控写入。
- `pending_read` 表示上下文已经生成。成功读到当前轮精确产物才登记 `observed_read`。通过宿主注入上下文时可记录注入情况，仍按对应验收要求判断是否取得读取回执。编译报错、读了旧文件、工具失败或只看到日志路径均不能登记已读。
- `response_completed` 只有关联到当前已读请求时才登记单个 `agate-plan`，记录回复摘要、诊断、精确目标文件和方案来源轮。实际用户提交单独的「执行」才可一次性消费这份方案。确认轮也必须读自己的上下文。新的真实用户指令、纠偏、停止或不确定的重连使旧确认失效。
- 文件判定复用并加强现有范围函数。规范化到工作区内相对路径，查验链接、盘符及 Windows 大小写。禁止权限扩大到目录或通配符。多目标操作要求源和目标全部通过，缺少任何准确路径即拒绝。Shell 在会话有效后、进入诊断检查之前放行，不按命令文本筛选。没有会话标识的 Shell 拒绝。MCP 不能借用该授权。
- 数据持久化仅保留判定所需字段和摘要。对原生输入限制长度，内部错误经脱敏后输出。`preToolUse` 必须验证宿主识别的拒绝结构或阻断退出码，不能用普通 CLI 错误替代。若客户端在超时等情况下允许继续，记录该限制并分别验收，不宣称具有完整强制边界。

### 8.3 分宿主事件表

下表说明原五个目标的接入依据。目标版本和能力上限由任务 3、4、6、7 的实机验收确定。

| 宿主 | 项目配置及事件 | 适配重点 |
| --- | --- | --- |
| Cursor | `.cursor/hooks.json`。`beforeSubmitPrompt`、`postToolUse`、`afterAgentResponse`、`preToolUse` | 保留 `conversation_id`、`generation_id` 和既有 JSON。普通读取后用 `additional_context` 返回本轮绝对路径。对原生 Read 的成功事件记回执。 |
| Antigravity IDE | `.agents/hooks.json`。`PreInvocation`、`PostToolUse`、`Stop`/`PostInvocation`、`PreToolUse` | 使用 `conversationId`、`workspacePaths`、`toolCall.name`/`args`。`invocationNum` 为 0 视为用户提交。IDE 2.8.0 的 `PostInvocation` 不带 `response`，从 `transcriptPath` 取不超出 `initialNumSteps` 的最后一条 `PLANNER_RESPONSE`。确认轮无 TurnID 时绑定 `ActiveTurnID`。`Stop` 与版本目录自动挂载尚未实机验收。 |
| Claude Code | `.claude/settings.json`。`UserPromptSubmit`、`PostToolUse`、`Stop`、`PreToolUse` | 从提交事件读取真实用户指令，`Stop` 提取最终回复。实测会话内多次提交、嵌套目录、原生 `Read`、`Write` 和 `Edit`，按其协议返回拒绝。 |
| Codex | `.codex/hooks.json`。`UserPromptSubmit`、`PostToolUse`、`Stop`、`PreToolUse` | 核对 `session_id`、`turn_id` 和事件来源，特别是续跑产生的提交信号。`apply_patch` 要提取全部目标。若当前客户端使用 Shell 读取上下文，仅识别已验收的精确单文件读取调用及其成功结果，关联调用 ID 后登记回执。 |
| Windsurf Cascade | `.windsurf/hooks.json`。`pre_user_prompt`、`post_read_code`、`post_cascade_response`、`pre_write_code` | 写入拒绝需使用该宿主认可的退出码。响应事件可异步到达，须核验源轮次和工作区。`pre_run_command` 与 MCP 事件分别作覆盖测试。 |

**协议来源**　[Cursor](https://cursor.com/docs/hooks) · [Antigravity IDE](https://antigravity.google/docs/hooks?tab=ide) · [Claude Code](https://code.claude.com/docs/en/hooks) · [Codex](https://developers.openai.com/codex/hooks) · [Windsurf Cascade](https://docs.windsurf.com/de/windsurf/cascade/hooks)

#### 预写入事件的输出契约

| 宿主 | Agate 拒绝动作 | Agate 放行的含义 |
| --- | --- | --- |
| Cursor | stdout 返回 `permission: "deny"` 及 `user_message`。 | 沿用 `permission: "allow"`。 |
| Antigravity IDE | stdout 返回 `decision: "deny"` 及 `reason`。 | 使用 `decision: "allow"`，交给后续 Hook 和客户端默认策略。 |
| Claude Code | stdout 返回 `hookSpecificOutput.hookEventName: "PreToolUse"`、`permissionDecision: "deny"` 和 `permissionDecisionReason`。 | 使用客户端文档约定的无判定返回，继续原权限流程。 |
| Codex | stdout 返回 `hookSpecificOutput.hookEventName: "PreToolUse"`、`permissionDecision: "deny"` 和 `permissionDecisionReason`。 | 使用客户端文档约定的无判定返回，保留沙箱和客户端审批。 |
| Windsurf | stderr 给出理由并以 `2` 退出。 | 退出 `0` 继续。 |

测试须验证理由字段可见、输出格式被客户端接受，以及后续宿主权限流程继续生效。操作级授权始终只代表通过 Agate 自身的判断。

### 8.4 新增内置工具

下表的 ID 和由 Agate 生成的文件名是拟定值。官方文档确认规则目录、项目配置或可用事件。客户端确认的生效范围以分版本实测为准。

| 工具与拟定 ID | 规则入口 | 原生接入与验收重点 |
| --- | --- | --- |
| **GitHub Copilot** `copilot` | `.github/copilot-instructions.md` | 候选项目 Hook 入口为 `.github/hooks/agate.json`。分别验收 VS Code 的 Copilot 会话和 Copilot CLI。按实际 Copilot 协议映射 `userPromptSubmitted`、`preToolUse`、`postToolUse`、`agentStop`，核对事件来源和超时行为。通用 `Code.exe` 只识别到编辑器。同一个 VS Code 窗口里的 Claude 或 Codex 会话仍走各自适配器。[Copilot 配置](https://docs.github.com/en/copilot/reference/custom-instructions-support) · [VS Code Hook 文档](https://code.visualstudio.com/docs/agent-customization/hooks) · [GitHub Hook 参考](https://docs.github.com/en/copilot/reference/hooks-reference) |
| **TRAE IDE** `trae` | 在 `.trae/rules/` 下生成 Agate 专属规则文件。按目标版确认始终生效的 Frontmatter。 | `.trae/hooks.json` 已有工具前后事件和拒绝响应。先验收国内版，记录当前会话对应的进程、事件、根目录和真实写入结果。国际版与独立运行模式分别验证。[规则文档](https://docs.trae.cn/ide_rules) · [Hook 文档](https://docs.trae.cn/ide_hook-configuration-reference) |
| **OpenCode** `opencode` | 优先使用项目配置的 `instructions` 引用 Agate 单独的 Markdown 文件。项目若明确使用跨工具共享的 `AGENTS.md`，按共享入口处理，保留原有 Codex 受管区块及 Git 跟踪保护。 | 通过官方本地插件接口桥接 Agate。按版本验证工具执行前后回调、最终回复、用户提交及插件启动失败时的处理。桥接文件由 Agate 生成，宿主信任流程按客户端要求执行。[指令文档](https://opencode.ai/docs/instructions/) · [配置](https://opencode.ai/docs/config/) · [插件接口](https://opencode.ai/docs/build/plugins/server) |
| **Qwen Code** `qwen-code` | 默认项目 `QWEN.md`。若设置了 `context.fileName`，预览有效入口后再分发。 | 合并项目 `.qwen/settings.json` 中的 Hook。映射 `UserPromptSubmit`、工具前后事件与结束信号，验证真人提交、只读结果及写入前拒绝。若不受信任的项目忽略项目配置，报告待加载。[配置文档](https://qwenlm.github.io/qwen-code-docs/en/users/configuration/settings/) · [Hook 文档](https://qwenlm.github.io/qwen-code-docs/en/users/features/hooks/) |
| **Cline** `cline` | 按目标版本在 `.cline/rules/` 或 `.clinerules/` 中选一个入口。盘点该客户端还会读取的现有规则。 | VS Code 扩展与 CLI 的 Hook 目录、脚本形式和载荷分别适配。`Code.exe` 无法单独标识 Cline。先验证项目规则加载，再验证工具前后的事件及审批来源。[规则文档](https://docs.cline.bot/customization/cline-rules) · [官方仓库中的扩展与 CLI Hook 示例](https://github.com/cline/cline/blob/main/docs/customization/clineignore.mdx) |
| **Kiro** `kiro` | 在 `.kiro/steering/` 下生成 Agate 规则，按目标版本验证自动包含方式。 | 项目 `.kiro/hooks/*.json`。IDE 与 CLI 按版本和运行形态分别验证提交、执行前后和结束事件。能力表按实际工具通道填写。[Steering 文档](https://kiro.dev/docs/steering/) · [Hook 事件](https://kiro.dev/docs/hooks/types/) |
| **Antigravity CLI** `antigravity-cli` | 按 CLI 文档和实际版本选择项目规则文件。独立记录 IDE 和 CLI 的规则配置来源。 | 验证 CLI 的事件与规则入口，再判断哪些代码能与 Antigravity IDE 共用。按实际运行形态选择适配器和版本记录。[CLI 功能](https://antigravity.google/docs/cli-features) · [Hook 文档](https://antigravity.google/docs/hooks?tab=cli) |
| **Gemini CLI** `gemini-cli`（兼容旧环境） | 默认项目 `GEMINI.md`，以项目已配置的上下文文件名为准。 | 在可运行的 Gemini CLI 版本上，用项目 `.gemini/settings.json` 中的 `BeforeAgent`、`BeforeTool`、`AfterTool`、`AfterAgent` 接入并验收完整原生门禁。现有 `-t gemini` 继续指向 Antigravity，Gemini CLI 使用完整 ID。Google 已建议迁移至 Antigravity CLI。缺少可用实机环境时，单独记录 Gemini CLI 的原生验收阻塞项。[规则文件](https://geminicli.com/docs/cli/gemini-md/) · [Hook 文档](https://geminicli.com/docs/hooks/reference/) · [Google 迁移说明](https://blog.google/innovation-and-ai/technology/developers-tools/google-io-2026-developer-highlights/) |

**实施批次**　先开发 Copilot、TRAE、OpenCode 和 Qwen Code，再开发 Cline、Kiro、Antigravity CLI 与 Gemini CLI。所有目标均执行同一份原生验收矩阵。一个目标完成原生链路后，可单独交付其已实测的版本和平台。

### 8.5 实施步骤

#### 任务 0　锁定基线和测试环境

**涉及位置**　`cmd/root.go`、`pkg/prompt/cursor_hook_test.go`、`cmd/init_test.go`、`pkg/prompt/testdata/native/`。

- [ ] 记录工作区现有差异、源码版本、`agate --version`、PATH 实际路径和用于测试的构建产物 SHA-256。检查现有 Cursor Hook 和用户配置。使用临时 Git 仓库、虚拟内容和独立会话跑客户端测试。
- [ ] 用固定的 Cursor 事件回放建立基线。记录提交、无关文件读取、本轮读取、最终方案、确认、范围内写入和越界写入的 JSON 与退出码。加一条乱序或重放事件。
- [ ] 执行 `go test -mod=vendor -count=1 ./pkg/prompt ./cmd`，保留实际结果。若测试受 Windows 沙箱临时文件替换影响，在获准的正常临时目录复验。基线测试通过后再修改协议。

#### 任务 1　提取状态流程与写入判定

**新建**　`native_hook.go`、`native_workflow.go`、`native_store.go` 及对应 `*_test.go`。  
**复用**　`CompilePrompt`、`ValidateDiagnosisOutput`、`RepairPlanAllowed`、`isFileInConfirmedScope`、`harness.WriteFileAtomic`。  
**交付**　`applyNativeEvent(store *nativeStore, event NativeEvent) (NativeDecision, error)`，`store` 由可信根、宿主和会话定位。

- [ ] 添加 `TestNativeApprovalIsBoundToCurrentTurn`。同一会话先发方案再发「执行」，只有确认轮已读后才允许范围内文件。其他会话、其他工作区、旧轮、第二次「执行」及用户纠偏均不能复用授权。模型文本含「执行」也不得生成批准。
- [ ] 添加 `TestNativeReceiptAndScopedTargets` 与 `TestNativeStoreFailureAndReplay`。读取失败、同名不同目录、缺少工作区根、链接或多文件越界应拒绝。并行读取与最终响应后应保留两者的有效状态。模拟写盘失败和延迟事件，确保无半记录授权。
- [ ] 运行新增定向测试，确认旧实现无法完成相应断言。实现统一会话快照、状态流转、目标规范化与原子写入，再重跑 `go test -mod=vendor -count=1 -run '^TestNative' ./pkg/prompt`。预期全部通过。
- [ ] 升级时保留旧记录供核对。新的会话或升级后首次有效提交建立新快照。验证降级后旧二进制可继续建立新请求，确认记录不会跨版本继承。

#### 任务 2　接线命令并回归 Cursor

**修改**　`pkg/prompt/cursor_hook.go`、`cmd/prompt.go`、`pkg/prompt/cursor_hook_test.go`、`cmd/prompt_test.go`。  
**交付**　`HandleNativeEvent` 登记 Cursor 适配器，既有命令和项目配置直接工作。

- [ ] 为普通 `prompt`、普通 `--hook`、诊断登记、宿主不支持和原生事件输出写 CLI 兼容断言。保留现有 `--host cursor` 和事件名。拒绝动作按 Cursor 所需 JSON 输出，stdout 不混入日志。
- [ ] 把 Cursor 输入转换和结果序列化留在 `cursor_hook.go`。请求编译、已读、方案登记和写入决定调用任务 1 的逻辑。将诊断命令按记录 ID 查找对应会话，审批状态不随诊断更新改变。
- [ ] 跑任务 0 的事件回放和 `go test -mod=vendor -count=1 ./pkg/prompt ./cmd`。在临时项目重跑 Windows Cursor 的只读、确认前拒绝、范围内通过和越界拒绝。一并验证关闭或恢复本地 Hook 后重新进入会话能正确识别新一轮。

#### 任务 3　接入 Antigravity IDE

**新增**　`pkg/prompt/antigravity_hook.go`、`antigravity_hook_test.go`、`testdata/native/antigravity/`。  
**变更范围**　独立临时项目中的 `.agents/hooks.json`。正式项目的自动装配由任务 5 负责。

- [x] 2026-11-24 在 Antigravity IDE 2.8.0、Windows 临时仓库、手工 `.agents/hooks.json` 上验证 `PreInvocation`、`PostToolUse`、`PostInvocation`、`PreToolUse`：方案登记、单独「执行」、范围内 `sample.txt` 放行、未确认的 `other.txt` 被 pre-tool hook 拒绝且文件不存在。
- [ ] 主动 `Stop` 尚未实机覆盖。版本目录仍为空，`agate init` 不会按已验收版本自动挂载，该项留在任务 5。
- [x] 单测落在 `TestAntigravityAdapterProtocol` 与 `TestNativeEmptyTurnIDConfirmAllowsInScopeWrite`，覆盖重复 `lastUserInput` 不重复提交、`transcriptPath` 回读和空 TurnID 绑定。计划中的 `TestAntigravityInvocationIsNotUserSubmit`、`TestAntigravityPreToolDenial` 没有单独成名。
- [x] `NativeAdapter` 已启用审批闭环。IDE 2.8.0 的 `PostInvocation` 不带 `response`，适配器读取 `transcriptPath` 中 `step_index` 不大于 `initialNumSteps` 的最后一条 `PLANNER_RESPONSE`。确认轮没有 TurnID 时绑定当前 `ActiveTurnID`。快照只保留方案摘要和文件列表，不写入日志正文。
- [ ] 通过的能力尚未登记进版本目录。缺项为 `Stop`、自动挂载，以及 Claude Code、Codex、Windsurf 与八个扩展宿主的实机矩阵。

#### 任务 4　接入 Claude Code、Codex 和 Windsurf

**新增**　各宿主的适配器、对应测试和脱敏事件样本。每个宿主单独完成下列步骤并记录版本证据。  
**交付**　每个宿主调用同一审批与写入判定逻辑，并按其协议输出结果。

- [ ] **Claude Code。** 从真实 `UserPromptSubmit` 接收审批，从 `Stop` 获取最终回复。添加 `TestClaudeStopAndEditScope`，覆盖多个工具事件、原生读写、同会话复用、自动继续和用户另起一轮。实机验证拒绝后目标文件字节不变。
- [ ] **Codex。** 使用 `turn_id` 核验事件关联。添加 `TestCodexPatchChecksAllTargets`，覆盖补丁的新增、修改、删除、移动及混合目标。添加 `TestCodexContinuationCannotConfirm`，验证由结束事件引出的续跑不能消费审批。检查纯读通道的起止和调用 ID。确认文档所述 Hook 错误或超时可能继续工具调用的版本行为，并在覆盖表中标注。
- [ ] **Windsurf。** 核验 `pre_user_prompt` 和异步 `post_cascade_response` 的关联字段。添加 `TestWindsurfDelayedResponseAndExitCode`，覆盖晚到响应、同文重复提交和用退出码拒绝预写事件。确认 Shell 写入需走对应的命令入口，文件授权不能代替命令授权。
- [ ] 执行 `go test -mod=vendor -count=1 -run '^Test(Claude|Codex|Windsurf)' ./pkg/prompt`。逐宿主通过测试及真实会话验收后再登记。若只能验收到部分能力，保留其观察或规则模式，并注明缺项。

#### 任务 5　让 init 根据项目和宿主装配 Hook

**新增**　`pkg/agent/native_runtime.go`、`native_runtime_test.go`、`pkg/adapter/native_plan.go`、`native_plan_test.go`。  
**修改**　`pkg/adapter/plan.go`、`cmd/init.go`、`cmd/init_test.go`、`cmd/hook.go`。  
**拟定接口**　`PlanNativeHooks(root string, target agent.Kind, runtime NativeRuntime, mode NativeMode) ([]NativeEntry, error)`。`NativeRuntime` 包含已确认的宿主、客户端版本、平台和工作区根。`NativeMode` 支持 `auto`、`off`、`require`。

- [ ] 将 `--native-hooks=auto|off|require` 加入 `init`。默认 `auto`。`off` 跳过本次原生装配。`require` 要求单一目标及已验证的原生能力，缺项时在任何写入前退出。`--no-hook` 继续只控制 Git。`-t` 用于选择目标，其本身不证明存在对应客户端或支持的版本。
- [ ] 复用一次 `InspectContextFull` 的结果。自动装配限定在实际匹配当前运行环境、版本和平台的宿主。`-t all` 可分发全部规则，只对当前已核实的宿主计划原生装配。无法可靠识别时继续按 0.6.0 提示 `-t`。识别到宿主但缺少经验证的版本时预览规则和具体缺项。
- [ ] 为每个受支持的宿主登记版本、平台、运行形态、已验证事件、覆盖工具和错误处理行为。Windows IDE 从已核实的祖先进程可执行文件获取产品版本。CLI 或远程宿主使用可绑定到当前进程的官方版本信息。文件名、PATH 上的同名程序和用户提供的版本字符串只作线索。版本未知、版本和平台未验收，或无法关联当前进程时，有确定目标便只分发规则。观察验证只在获准的隔离项目启用。
- [ ] 实现安全配置合并。保留用户已有事件、未知字段及顺序。Agate 仅管理能够确认归属的条目，使用版本化指纹记账，不向宿主 JSON 塞入未支持的元数据。遇到无效 JSON、重复键、链接、Git 跟踪限制或来源不明的同名条目时生成冲突。`--dry-run` 不创建目录、备份、锁和记录，也不修改 Git 配置。
- [ ] 装配前核对客户端启动环境实际解析到的 Agate 路径与版本。项目共享配置使用经过实机验证的可移植调用。本机绝对路径仅可用于按仓库约定隔离的本地配置，预览时提示客户端无法找到可执行文件的情况。
- [ ] 复用规划和落盘之间的快照检查。在锁内重查 Git 跟踪和文件摘要。先准备规则及 Hook 所需的本地入口，再启用新 Hook。发生部分失败时报告写入范围，只回滚本次原生变更。回退前重新检查摘要，保留用户在期间新增的内容。
- [ ] 增加 `agate hook native status` 与 `agate hook native rollback --host <宿主>`。状态至少区分「仅规则」「Hook 已写入，等待客户端确认」「已观测到事件」和「本机已通过指定通道验收」。回退只处理本项目的 Agate 条目，重新加载须由客户端自身完成。宿主要求信任或审查时沿用客户端流程。
- [ ] 添加 `TestInitNativeAutoFallback`、`TestInitNativeDryRunIsReadOnly`、`TestNativeMergeIdempotentAndPreservesUserEntries` 与 `TestNativeRollbackRetainsUserEdits`。运行 `go test -mod=vendor -count=1 ./pkg/agent ./pkg/adapter ./cmd`，预期全通过且旧 CLI 行为保持兼容。

#### 任务 6　新增工具的内置档案和原生适配

**修改**　`pkg/agent/registry.go`、`pkg/agent/profile.go`、`pkg/agent/collector.go`、`pkg/adapter/adapter.go`、`pkg/adapter/plan.go`、`cmd/init_test.go`。  
**新增**　按宿主协议拆分 `pkg/prompt` 下的适配器、测试与脱敏样本。OpenCode 的本地插件桥接模板放在 `internal/templates`。

- [ ] 每个目标先用官方文档和实际客户端核实规则入口、规则格式和身份信号。CLI 核验实际运行工具的信号。编辑器插件核验能区分当前 Agent 的会话信号。没有足够证据时，内置档案仍可通过 `-t <工具>` 选择。现有同名自定义档案要在预览中明确提示，保留用户配置并提供迁移指引。
- [ ] 将目标 ID、别名、规则入口与识别线索放入已有内置注册表。扩展 `-t all` 从有效的内置和自定义档案收集目标，同时维持稳定顺序与去重。对共用规则文件和配置路径预先检查写入计划，保留已有指令和规则。OpenCode 的引用配置须经过规则与配置两边的快照检查。
- [ ] 添加 `TestInitBuiltinAdditionalAgents`、`TestBuiltinAliasesStayCompatible`、`TestSharedEditorDoesNotSelectAnotherAgent`、`TestBuiltinProfileConflictPreservesUserConfig` 和 `TestAllIncludesNewBuiltinsWithoutDuplicateWrites`。覆盖默认安装时无需 `profile add`、共用进程、旧 `gemini` 别名、新 CLI ID、已有档案和同路径冲突。
- [ ] 逐宿主完成测试、适配和验证，沿用任务 1 的状态机、任务 5 的装配逻辑及任务 7 的实机矩阵。对 OpenCode 另验收本地插件加载、子进程协议和退出行为。对 Copilot、Cline、Kiro 逐项核对不同运行形态与版本。
- [ ] 执行 `go test -mod=vendor -count=1 ./pkg/agent ./pkg/adapter ./pkg/prompt ./cmd`。记录各目标规则分发通过项和独立的原生能力状态，实机验证后再开启自动装配。

#### 任务 7　真实客户端验收、文档和回退

**交付**　脱敏的宿主版本清单、验收记录、发行候选构建和分宿主回退说明。

- [ ] 每个宿主以无预设目标、无规则入口的临时项目进入真实客户端，运行 `agate detect --explain` 与 `agate init --dry-run`。核对预览中的目标依据、规则路径、原生入口和状态。随后运行无参数 `agate init`，核对实际落盘。新工具另用一份自定义档案验证此链路。用普通终端、弱信号和冲突信号验证手动提示。
- [ ] 为 8.3 和 8.4 节每个目标建立独立验收记录，执行下表的真实会话测试。记录日期、客户端版本、系统、安装程序哈希、实际 Hook 路径、脱敏轮次、工具类型、真实决定及目标文件摘要。注入失败和 Hook 超时单独记录。缺少真实客户端回执、审批绑定或拦截证据的目标继续标为原生接入待验收或受阻。
- [ ] 先复验 Cursor，再开放通过验收的其他宿主。对同一宿主未验证的平台、旧版本及远程模式继续报告能力限制。发布前在实际目标版本上完成「新配、已有用户 Hook、回退」三种测试。
- [ ] 执行 `go test -mod=vendor -count=1 ./...` 和 `agate verify`，核对退出码和最终 `[PASS]`。在 Windows 还需复验同一路径覆盖旧二进制并重开客户端。按现有脚本生成 Windows/Linux/macOS 归档，分别记录已运行的平台与仅完成交叉编译的平台。
- [ ] 更新双地图、命令说明和版本验收记录。任何提交、推送、发布和正式环境安装均按现有授权范围单独处理。

| 实机动作 | 必须保留的证据 |
| --- | --- |
| 普通只读调查，再读取本轮编译上下文 | 只读放行。当前轮具有读取回执，读旧轮或其他工作区的文件不能产生回执。 |
| 同一文件在确认前尝试修改 | 客户端返回拒绝或阻断退出码，文件 SHA-256 不变。 |
| 展示只改 `sample.txt` 的方案，用户在客户端单独发送「执行」 | 记录审批所绑定的宿主、工作区、当前方案、确认轮和时间。 |
| 在确认轮读取本轮上下文后，对 `sample.txt` 和 `outside.txt` 分别发起写入 | 当前范围内写入成功。越界动作被阻断，`outside.txt` 的文件摘要不变。 |
| 连续发「执行」、改目标、切会话或让模型续跑 | 旧审批无效且不能误绑定。多根项目和 Git worktree 各做一条隔离用例。 |
| Hook 超时、错误、未获客户端信任，或不走 Hook 的工具 | 记录客户端最终行为。只有实测覆盖的工具通道可以标为强制拦截。 |
| 保留用户 Hook 后回退 Agate 条目 | 用户 Hook 完整保留，原配置可重新加载。现有 Git Hook 状态仍可查询。 |

### 8.6 交付与停止条件

1. **通用能力通过**。初始化流程仍能自动识别与分发。未知、冲突及仅有旧入口时提示明确的 `-t`。自定义档案可扩展到新工具，`--dry-run` 可审阅原生装配与降级原因。
2. **原生接入逐个验收**。Cursor 向后兼容。第 8.3 和 8.4 节的每个新增宿主都应达到 Cursor 的原生门禁验收标准。逐一登记通过的版本、平台和工具通道。取得可信提交、独立读取回执、当前方案绑定、真实放行与拒绝的全部证据，才将对应目标标为原生接入完成。
3. **可回退**。遇到版本不兼容、审批错绑、作用范围不明或越界写入，停止启用该宿主的强制模式。保留证据并回退本次原生配置。下次从新会话、重新提交的方案开始验收。
4. **本轮交付为方案**。上面的新增文件、参数及验收命令是后续实施项目，测试结果以执行阶段证据为准。

## 9. 当前修复实施顺序

按[当前未关闭问题与验收清单](harnessgate-session-findings-2026-11-19.md#912-当前未关闭问题与验收清单2026-11-20)安排工作。每项的复现、影响和历次结果见问题文档。

| 顺序 | 修改位置和实施步骤 | 验证 |
| --- | --- | --- |
| 1. 稳定 Windows 请求状态保存 | 在 `pkg/prompt/workflow.go` 排查状态文件替换失败，核对目标、句柄和实际系统错误。依据证据调整写入和重试路径，保留旧状态的可读性。 | 验证正常覆盖、并发写入及保存失败。`TestCursorApprovalFollowsCurrentDeliveredPlan` 重复运行 10 次后，重跑全量 `agate verify`。 |
| 2. 修正 Antigravity 失败处理和轮次关联 | 在 `pkg/prompt/antigravity_hook.go` 按官方字段解码读取结果，并为拒绝返回正式的终止响应。沿 `pkg/prompt/native_workflow.go` 核对提交、回复、回执与写入的关联证据。 | 首次调用和真实新一轮各自交付上下文，续跑不重复交付，失败读取不产生已读回执，模型调用与工具拒绝分别按协议生效。 |
| 3. 贯通配置装配和版本登记 | 在 `pkg/adapter/native_plan.go`、`cmd/init.go` 按宿主渲染和合并 Hook 配置。落盘前复核快照，处理计划过期和写入期部分失败。在 `pkg/agent/native_runtime.go` 根据实机证据登记版本、平台和运行形态。 | 临时项目验证预览、初始化、跟踪文件保护、重复装配、用户配置保留和回退。已验收宿主可装配，未知版本的 `auto` 保留规则分发，`require` 在写入前拒绝。 |
| 4. 逐款实现新增接入 | 按第 8.4 节完成宿主识别、原生适配器、专用配置或插件桥接。每款分别启用已验证的版本与运行形态。各自保存规则加载和事件触发证据。 | 逐款运行无 `-t` 初始化、规则加载、上下文交付、确认前拒绝、范围内放行、越界拒绝以及旧确认失效用例。 |
| 5. 用同一候选完成交付验证 | 按第 6 节和第 8.5 节任务 7 核对二进制路径和 SHA-256、客户端版本、事件、回执及文件结果，记录受测平台和工具通道。 | 全量自检、独立协议回放和相应的真实客户端验收均通过后，在问题文档登记关闭证据。 |
