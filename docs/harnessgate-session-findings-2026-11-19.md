# HarnessGate 问题与回归记录

原始复盘日期：2026-11-19（北京时间）

原始核对范围：从本会话「灵动岛」初始化截图开始，覆盖规则来源、服务器分发、Windows 宿主识别、指令扩写和诊断流程。核对基线为 HarnessGate 提交 `fe78e72`，本机 PATH 中的 Agate 为 `v0.5.0`。本会话已更新用户全局规则和工作区内置模板。

[当前未关闭问题与验收清单](#912-当前未关闭问题与验收清单2026-11-20) · [实施方案与修复顺序](harnessgate-auto-init-and-cursor-remediation-plan-2026-11-19.md#9-当前修复实施顺序)

第 1 至 7 节记录初次排查时的现象与建议。第 8 节收录 Cursor 的早期实机证据，第 9 节记录后来构建的回归结果。

## 1. 预期目标与当前差距

**预期目标：`agate init` 默认尽可能从当前会话识别宿主。能够可靠确定工具和规则入口时，直接自动分发。无法可靠确定时，说明原因并提示用户用 `agate init -t <工具名>` 指定目标。** 已接入的旧 Agent 和后来出现的新 Agent 共用这套流程。新 Agent 可以通过宿主声明或可复用的工具档案接入。用户也可以主动指定目标，覆盖自动选择。

1. **建立可扩展的宿主识别与分发机制。** 把识别所需的环境证据、规则路径和格式纳入同一套可扩展工具档案。用已有工具覆盖回归，用编译时未内置的新 Agent 验证能否只增加接入信息、复用同一二进制。
2. **同步受影响项目并验证会话加载。** 本机全局规则和内置模板已经修正，「灵动岛」中已生成的 Antigravity 规则仍保留旧条款。规则分发会更新文件，实际会话加载需要单独验证。
3. **分别验收扩写、诊断和工具拦截。** Cursor 已有事件处理代码，当前项目只注册了提交和工具调用后的事件。真实会话的自动扩写与交付缺少完整记录，聊天答复自动检查尚未实现，写入前拦截尚未启用。

## 2. 首次截图证明了什么

截图中的项目是「灵动岛」。日志显示「目标选择依据：显式目标」，所选目标为 `antigravity`。Agate 使用 `C:\Users\Simon\my_skills\SKILL.md`，生成了项目的 `.gemini/GEMINI.md`，还报告生成了 `.ignore`、`MAP.md`、`contexts/context.md` 和 `TASK.md`。这是显式选定目标后的分发成功。这次日志没有覆盖无参数识别。`Git 跟踪=false` 描述的是当时该入口的 Git 状态。 [初始化记录输出](../cmd/init.go#L90)

日志里的提示有两个含义：

- **全局规约需复核、规则差异预览。** 本次命中的全局正文仍要求只读排查也先出方案，并缺少诊断连续性和诊断状态。预览只列出差异，分发仍然使用选中的全局正文。 [旧规则和差异检查](../pkg/adapter/adapter.go#L219)
- **客户端加载及会话刷新未验证。** Agate 确认了入口映射和落盘结果。实际 Antigravity 会话是否读到新版、是否遵循，仍需客户端证据。日志的「适用版本：未核验」也属于这条入口映射提示。 [客户端加载提示](../pkg/adapter/plan.go#L605)

## 3. 旧全局规则覆盖新版模板

### 原因与影响

`agate init` 在确定目标后，按顺序从执行账号的家目录查找全局规则：

1. `~/.agate/rules.md`
2. `~/.config/agate/rules.md`
3. `~/my_skills/SKILL.md`
4. `~/.gemini/config/SKILL.md`

Agate 使用第一份找到的全局文件。四处都没有文件时，使用当前二进制内嵌的 `internal/templates/SKILL.md`。`v0.5.0` 更新了仓库模板和 `AGENTS.md`。截图中的个人全局文件当时仍是旧版，优先级更高，所以新二进制继续向项目分发旧正文。 [规则读取顺序](../pkg/adapter/adapter.go#L189) · [初始化取源](../cmd/init.go#L103) · [模板嵌入](../internal/templates/embed.go#L5)

会话中核对过全局文件的来源。修订前文件属性显示它创建于 2026-09-15。同目录的 `SKILL.md.bak` 保留早期 `ai-dev-harness` 标题，仓库首个提交 `6ec4531` 已将 `~/my_skills/SKILL.md` 列为候选路径。这些记录支持它是早期个人规约沿用下来的版本。首次写入者及写入方式没有查实。

### 已完成与仍待处理

本会话已修正全局 `SKILL.md` 和仓库内置模板。只读排查先查证据、再给结论。证据不足时区分待验证假设和已有结论。核心修改继续走方案确认。交接、归档和 `TASK.md` 写入限于已进入接力流程且获准写入的任务。 [现行内置模板](../internal/templates/SKILL.md#L24)

本会话运行的 `agate verify` 返回 `[PASS]`。另用新编译的临时二进制，在两个临时项目分别验证了真实用户全局文件和空用户目录下的内置模板。两次生成的 `GEMINI.md` 正文均与各自来源一致，分发时没有出现旧规约警告。这验证了规则取源与写盘。「灵动岛」的 `E:\Tools\灵动岛\.gemini\GEMINI.md` 尚未重新分发，后续只读检查仍能找到旧条款。

### 建议的解决办法

1. 在「灵动岛」预览使用新版全局规则。当前 Antigravity 终端识别有缺口，可以用显式目标刷新现有规则。第 4 节同时验收自动选择与手动指定目标。

   ```powershell
   Set-Location -LiteralPath 'E:\Tools\灵动岛' -ErrorAction Stop
   agate init -t antigravity --no-hook --dry-run
   ```

   确认来源为已修正的全局文件、日志不再出现两条陈旧规约告警，且分发计划更新 `.gemini/GEMINI.md`、没有冲突。获得项目写入授权后，再执行同一命令并去掉 `--dry-run`。`--no-hook` 跳过 Git Hook 安装。实际初始化还会检查项目脚手架和本地 Git 排除配置。 [预览与写入逻辑](../cmd/init.go#L119)
2. 核对生成文件的受管区块和 `agate:rules-sha256`，在 Antigravity 新会话验证一条只读排查请求。优先使用客户端可观察的规则加载证据。只有模型输出能观察时，将结果记录为行为验证。 [受管规则摘要](../pkg/adapter/plan.go#L478)
3. 降低以后再次发生版本差异的概率。在 `cmd/init.go` 的来源提示和 `pkg/adapter` 的差异诊断中集中展示二进制版本、命中的规则路径、全局和内置规则的修订或摘要、各目标文件的计划变化。全局规则涉及其他项目，建议先查看差异，再确认合并需要升级的条款。 [现有差异预览](../pkg/adapter/adapter.go#L241)

### 服务器上的规则来源

在服务器上执行 `agate init` 时，使用的是服务器执行账号的家目录。账号预配了全局文件时，按相同优先级读取。没有候选文件时，使用服务器实际执行的二进制自带模板。仓库模板的本地改动要经过重新构建才会进入二进制。Windows 用户目录里的个人规则需要单独配置到服务器上才会被读取。

服务器可以用 `agate --version` 和 `agate init --dry-run` 核对实际版本、规则来源和目标。仅部署业务项目且部署流程未运行 `agate init` 时，不会触发规则分发。截图时未被 Git 跟踪的入口也不会随 Git 检出带过去。 [家目录取源](../pkg/adapter/adapter.go#L189) · [编译时嵌入](../internal/templates/embed.go#L5)

## 4. 让新旧 Agent 共用自动识别与手动兜底

### 现有能力与缺口

现有工具档案已经能声明项目规则路径、格式、专用环境变量和终端标识。带有可区分环境信号的新 Agent 可以先注册一份可复用的档案，再在项目中运行无参数 `agate init`。目前平台进程采集和规则分发虽可复用，本地可执行文件匹配仍写在 `collector.go` 里。只有进程证据的新工具无法通过现有自定义档案补充该匹配。一个此前完全未知的工具也需要有规则入口和格式的信息。 [工具档案字段](../pkg/agent/profile.go#L12) · [档案注册参数](../cmd/profile.go#L172) · [环境指纹与进程证据](../pkg/agent/registry.go#L65)

在之前失败的 Antigravity 集成终端里，祖先进程链含 `Antigravity IDE.exe`。`TERM_PROGRAM` 是通用的 `vscode`，当时未出现 Antigravity 专用环境变量。Windows 原生采集已经能提供祖先进程证据，当前的本地可执行文件精确匹配只覆盖 `Cursor.exe`、`Code.exe` 和 `Code - Insiders.exe`。因此，那个终端能取得的 Antigravity 进程证据没有转化成 Editor 识别结果。Antigravity 的规则入口映射已经存在，显式指定它时可以完成分发。 [Windows 采集](../pkg/agent/collector_windows.go#L30) · [本地匹配](../pkg/agent/collector.go#L50) · [Antigravity 工具档案](../pkg/agent/profile.go#L52)

`agate init` 的装配优先级依次是显式目标、项目默认装配集合、当前 Agent 环境、已有规则入口。当前实现已经会在无法识别且无已有入口时提示指定 `-t`。有旧入口的项目可能按历史文件自动分发，所以要同时核对日志的选择依据，以及自动识别失败时是否给出了可执行的提示。 [选择依据日志](../cmd/init.go#L90) · [当前失败提示](../pkg/adapter/adapter.go#L163)

原实施方案以 Cursor 为实机样本，并将当时缺少进程证据的 Antigravity 等宿主列为待核验。现已有 Antigravity 证据，可以沿用原来的 Windows 采集和匹配设计。之前观察到 Cursor 进程被识别，原复现终端使用新二进制完整通过无参数验收的记录仍不完整。 [原范围](superpowers/plans/2026-11-13-windows-host-auto-detection.md#L78) · [原实机验收](superpowers/plans/2026-11-13-windows-host-auto-detection.md#L347)

### 建议的通用接入机制

1. **采集当前会话的证据。** `pkg/agent/collector.go` 继续采集当前祖先进程证据。把内置的专用会话信号与档案提供的进程、终端和环境指纹交给 `pkg/agent/registry.go` 统一匹配和仲裁。进程证据用于识别宿主 Editor，执行者 Runner 仍用能识别实际 Agent 的会话信号。VS Code 系工具可能共享 `TERM_PROGRAM=vscode`，通用值只能辅助诊断。 [进程证据采集](../pkg/agent/collector.go#L197) · [现有身份仲裁](../pkg/agent/registry.go#L65)
2. **把进程匹配和规则映射放进工具档案。** 建议在 `pkg/agent/profile.go`、`pkg/config/config.go` 和 `cmd/profile.go` 扩展档案及其持久化。每个档案包含稳定工具标识、平台与角色、可核验的识别条件、规则相对路径及格式、信息来源和适用版本。内置工具与新增工具使用同一匹配流程。进程条件按操作系统匹配当前祖先链中的精确名称。Cursor 和 Antigravity 可分别用已核验的进程名作为旧工具样本。 [现有档案校验](../pkg/agent/profile.go#L143) · [档案持久化](../pkg/config/config.go#L462)
3. **允许新工具首次运行就接入。** 若宿主或其集成程序能在当前会话提供可核验的工具标识及规则约定，Agate 可按受支持的声明格式解析接入信息。若工具只暴露专用信号而未提供规则约定，则加载随 Agate 分发、由团队统一提供或用户一次注册的档案。已有全局档案可供后续项目复用。进程名至多提供身份线索，规则入口需要来自该工具的公开约定或已确认档案。 [现有档案入口](../cmd/profile.go#L172) · [现有会话采集](../pkg/agent/registry.go#L92)
4. **证据足够时自动分发。** `cmd/init.go` 复用一次探测结果，在得到可靠且唯一的身份和可用规则档案时，直接交给 `pkg/adapter` 校验路径、预览并写入受管规则。保留当前的显式 `-t` 与项目默认目标配置，用户主动设定时按配置执行，并在日志中显示实际来源。 [初始化选目标](../cmd/init.go#L74) · [现有分发保护](../pkg/adapter/plan.go#L301)
5. **识别失败时给出可执行的提示。** 在自动路径里，区分没有足够的会话证据、识别结果冲突、已识别工具但缺规则档案。把已有规则入口作为手动选择的线索，建议在环境无法确认时等待用户明确目标再写入。提示应列出可选的已注册工具或冲突候选，并给出 `agate init -t <工具名>`。工具尚未注册时，提示先补齐工具档案，再指定目标或重试自动识别。 [现有冲突提示](../cmd/init.go#L74) · [未注册目标提示](../pkg/adapter/adapter.go#L109)

### 验收如何证明可扩展性

- **旧工具回归。** 在真实 Antigravity 和 Cursor 终端使用同一新编译产物。临时项目无预设 `agent.targets` 和已有入口。预览显示「当前 Agent 环境」，随后直接运行 `agate init`，确认分别写入 `.gemini/GEMINI.md` 和 `.cursorrules`。保留普通终端、相似进程名、冲突身份以及 Cursor 中运行 Codex 的回归。 [目标身份优先级](../pkg/agent/context.go#L67) · [原无参数验收](superpowers/plans/2026-11-13-windows-host-auto-detection.md#L371)
- **新 Agent 验收。** 选择一个编译时未内置的 Agent，先核对其真实宿主信号与规则入口。保持 Agate 源码和二进制相同，仅增加一份可复用工具档案，或由 Agent 提供已支持的宿主声明。在两个干净项目中分别运行 `agate init`，确认自动得到正确工具和入口。假设该工具只能通过当前祖先进程识别，也应能用档案接入。若方案声称支持首次自动发现，还要在用户未预先注册档案的环境验证该宿主声明可被发现并直接使用。
- **失败提示和手动兜底。** 分别覆盖普通终端、只有通用终端标识、缺少规则档案和身份冲突。确认自动识别失败时提示原因及手动命令。选一个已注册目标，用 `agate init -t <工具名>` 重试，确认日志记录为「显式目标」，并生成对应入口。再验证已识别当前宿主时，显式参数仍可主动选择另一目标。记录宿主及 Agate 版本、脱敏识别证据、档案来源、选择依据和写入结果。 [现有初始化测试](../cmd/init_test.go#L287)

## 5. 自动触发扩写和上下文交付尚未闭环

### 已实现与存在的问题

Cursor 的 `beforeSubmitPrompt` 会编译请求并写出本次上下文产物。`postToolUse` 在观察到模型读取对应产物后记录交付证据。`preToolUse` 能在工具执行前检查交付和诊断条件。提交事件无法原地改写提示词，模型读取本次上下文是独立的一步。 [事件处理实现](../pkg/prompt/cursor_hook.go#L75)

本机仓库的 `.cursor/hooks.json` 只注册了 `beforeSubmitPrompt` 和 `postToolUse`，调用独立的 `agate-hooks.exe`。历史记录只证明手动调用过提交事件并返回成功。当前检查未找到真实请求上下文产物，也没有 Cursor 重载后「发送请求、自动编译、模型读取并记录」的完整验收证据。 [本机项目 Hook 配置](../.cursor/hooks.json#L4)

当前只有 Cursor 的原生事件协议已接入 Agate。Antigravity 目前走项目规则分发，是否拥有可接入的原生事件，要结合实际客户端版本和协议核验。`agate init` 自带的 Hook 安装指的是 Git 的 `pre-commit` 和 `pre-push`，由 Git 提交或推送时触发。 [宿主适配范围](../pkg/prompt/cursor_hook.go#L52) · [Git Hook 命令](../cmd/hook.go#L13)

### 建议的解决办法

1. 核对 Cursor 加载的 Hook 配置、二进制路径与版本，以及实际生效的常驻规则。需要时，在获准修改的项目规则中补齐读取本次上下文的引导，确保读取与当前请求标识对应。 [原方案的交付链路](superpowers/plans/2026-11-13-windows-host-auto-detection.md#L439)
2. 在重载后的真实 Cursor 会话中提交两条独立请求，逐条捕获原生提交事件、生成产物和匹配的读取证据。增加一个失败用例，确认可以定位未触发、编译失败或未完成交付。手动调用与单元测试保留为协议和代码层证据。 [客户端验收用例](superpowers/plans/2026-11-13-windows-host-auto-detection.md#L502)
3. 如果后续需要自动装配宿主 Hook，建议分别记录 Editor、Runner、规则入口和已验证的原生事件能力。在 `cmd/init.go` 预览中展示适配状态，再为已核验且获准的客户端装配 Hook。工具事件适配按实际发送事件的客户端选择。无参数识别可以独立交付。 [目标选择实现](../cmd/init.go#L74) · [原生适配约束](superpowers/plans/2026-11-13-windows-host-auto-detection.md#L451)

## 6. 诊断规则已修正，聊天回复检查和写入门禁仍有缺口

### 当前行为

故障修复的规约流程是「解码 → 只读查证 → 解释原因 → 给修复方案 → 确认后实施 → 验证」。纯问原因时直接交付诊断。原因待查时列出现有证据、待验证假设和下一项检查。新功能或明确改动说明现状与预期。`explicit` 只表示文件或模块已锚定。 [提示词编译逻辑](../pkg/prompt/compiler.go#L744) · [现行规则](../internal/templates/SKILL.md#L31)

代码会校验主动登记为 `confirmed` 的诊断文本，检查现象与预期、位置与证据、因果解释、不确定项及方案依据。这是文本要素校验，因果是否成立仍需依据证据判断。当前没有接入聊天回复检查。Cursor 的写入前处理代码检查上下文读取证据和诊断状态，当前项目配置尚未注册该事件。用户对具体方案的授权仍要单独确认。 [诊断文本检查](../pkg/prompt/workflow.go#L159) · [诊断状态登记](../pkg/prompt/workflow.go#L208) · [工具检查](../pkg/prompt/cursor_hook.go#L136)

### 建议的解决办法

1. 先在加载新版规则的真实会话检查纯问原因、原因待查、明确文件的故障、纠偏、新任务和确认实施。记录实际答复、证据来源和操作。自动回复检查需要先验证目标客户端是否公开支持观察最终回复的接口，并区分只能提示和能够阻止输出的能力。没有接口时，将回复顺序记录为由规则约束。
2. 启用写入前拦截之前，先验证诊断阶段能正常读取证据和登记诊断。当前实现将 `Shell`、`Bash` 统一纳入写入门禁，直接启用可能连只读排查命令也拦住。建议在客户端能可靠识别的范围内，分别定义只读证据采集、诊断登记和实际写入的边界，并将修改确认绑定到当前任务及方案。 [被检查的工具类型](../pkg/prompt/cursor_hook.go#L229) · [诊断放行条件](../pkg/prompt/workflow.go#L52)
3. 在真实宿主分别验证原生编辑、Shell 和 MCP。未读上下文或仍是待验证假设时，验证受覆盖的写入能被拦截。满足诊断条件并取得对应授权后，验证获准操作可执行。用户纠偏后，应针对新目标重新核对证据和授权。 [诊断验收场景](superpowers/plans/2026-11-13-windows-host-auto-detection.md#L484)

## 7. 实施与验收顺序

1. **通用宿主识别与手动兜底。** 用同一二进制验证旧工具和新 Agent 的自动分发，同时验证无可靠识别结果时会给出具体提示，用户可按提示添加 `-t` 完成初始化。
2. **规则同步。** 更新受影响的项目入口。每个客户端记录规则来源、写入结果、加载证据和行为结果。服务器另记执行账号和实际二进制版本。
3. **扩写交付。** 在真实 Cursor 会话逐条核验事件触发、编译产物、读取交付和失败提示。
4. **诊断与工具检查。** 分别验收聊天诊断质量、可用的回复检查能力、工具拦截及用户确认。对未覆盖的客户端或路径记录实测边界。
5. **补齐原方案的状态和证据。** 11 月 13 日的方案首页仍为「待评审、待实施」。建议按其三个工作包登记实际实施范围、测试结果、真实客户端验收和尚缺的能力。仓库自检、真实客户端测试和安装版确认各自给出结论。 [原方案完成标准](superpowers/plans/2026-11-13-windows-host-auto-detection.md#L388) · [独立验收要求](superpowers/plans/2026-11-13-windows-host-auto-detection.md#L518)

本报告依据本会话的截图、源码核对与验证输出、当前本机配置和原实施方案。编写期间未新增客户端或服务器验收结果。

## 8. Cursor 真实会话验收

### 2026-11-20 Cursor 本地验收

- 在 Windows 的 Cursor 3.30.14 中使用当前源码构建的专用 Hook 和独立 Git 项目。基线会话中，较早请求的记录保持 `pending_read`，实际使用原生 Read 打开本轮文件的请求 `d670268300` 写入 `observed_read`。新版用普通 Read 触发 `postToolUse.additional_context` 提供绝对路径，方案轮 `98b607bbc8` 和确认轮 `d6ad154040` 各自写入了指向本轮文件的 `delivery_evidence`。
- Cursor 记录了真实的 `afterAgentResponse` 和 `beforeSubmitPrompt`。待确认方案的目标是 `sample.txt`。确认轮记录了来源请求、方案回复摘要、确认时间以及 `sample.txt` 的单文件范围。随后收到另一生成轮的「执行」时，该轮未继承已消费的确认。
- Cursor 的 `preToolUse` 日志显示，原生 Read 返回 `allow`。确认前的原生 Write 返回 `deny`。确认后写 `outside.txt` 返回 `deny`，写 `sample.txt` 返回 `allow`，且写入后的事件返回成功。文件核对只发现 `sample.txt` 增加一行 `APPROVED`。
- 本轮真实写入验收覆盖 Cursor 原生文件工具。Shell 的只读白名单、写命令拒绝，以及 MCP 写入类工具拒绝由 Go 测试覆盖。对文件的确认只作用于原生文件编辑，Shell 与 MCP 的写入授权仍需独立入口。
- HarnessGate 本地项目接入了同一 SHA-256 的专用二进制，Cursor 日志确认加载四个项目 Hook。项目规则以原生读取 `MAP.md` 启动本轮上下文交付。本地生效证据为隔离会话结果和实际项目的配置加载记录。

## 9. 多宿主原生 Hook 回归问题与修复方案

### 9.1 回归基线与结论

2026-11-20 在 Windows 上用当前工作树重新构建 `agate-candidate.exe`。候选版本为 `0.6.0`，SHA-256 为 `C28215CF47AE52325551058719CB2476F599B3BF903BBD9AB42B3795C8BCCD78`。使用临时 Git 仓库和虚拟文件完成 CLI 回放。第 8 节的 Cursor 客户端验收记录属于此前的构建。

| 检查范围 | 结果 |
| --- | --- |
| `go test -mod=vendor -count=1 ./...` | 通过。 |
| 候选程序 `agate verify` | 退出码 `0`，输出 `[PASS]`。 |
| 新增八种内置工具的自动选择 | 模拟各宿主信号时，八个无 `-t` 的 `init --dry-run` 均选中预期规则入口。 |
| 原生事件回放与配置检查 | 29 项中 4 项符合预期、25 项不符合预期。 |
| 本机两个入口对照 | PATH 上的 `agate.exe` 复现 Cursor 普通读取无法拿到上下文路径。项目配置指向的 `agate-hooks.exe` 在相同回放中能返回路径。两者均自报 `0.6.0`。 |
| 本次候选的真实客户端验收 | 待验收。当前存在会话授权失效、配置覆盖和协议不匹配等阻断项。 |

**多宿主原生接入未通过验收。** 下文的「已复现」来自本次候选 CLI。「源码缺项」来自静态核对。并发、故障注入和实机测试列为修复后的验证项目。

### 9.2 P0　用户确认跨轮次生效

**已复现。** Cursor 已确认 `sample.txt` 后，旧轮次的写入请求仍返回 `allow`。再次单独发送「执行」，并完成新一轮读取后，也能沿用之前的批准范围。首次确认前拒绝、有效确认后范围内允许、范围外拒绝三条正反用例均通过。

**原因。** 新状态机在首次「执行」满足条件时写批准。随后未消费新方案的「执行」没有清除快照里的 `PlanConfirmed` 和 `ConfirmedFiles`，却切换了 `ActiveTurnID`。工具前检查没有核对事件轮次与批准轮。读回执和最终回复也缺少来源轮校验。会话标识缺失时使用 `default-session`，且未用 `ClaimedRoots` 核对工作区。[提交与预写检查](../pkg/prompt/native_workflow.go#L143) · [统一事件字段](../pkg/prompt/native_hook.go#L29)

**解决方案**

- 在 `pkg/prompt/native_workflow.go` 中，把批准绑定到宿主、规范化 worktree 根、会话、来源方案 ID、确认轮和精确文件范围。新用户提交先使旧批准失效，再判断是否消费当前待确认方案。重复的同一条提交保持幂等。独立的新一轮「执行」须消费新的方案。
- 在读回执、最终回复和预写入处理前核对事件来源。拒绝旧轮、异序、缺少稳定标识或根目录冲突的写入。移除受控操作的 `default-session` 兜底。没有可证明关联机制的宿主保留待验收状态。
- 在 `pkg/prompt/native_store.go` 将进程内 `sync.Mutex` 改为跨 Hook 进程的会话锁。锁住完整的读取、转换和保存，用唯一临时文件原子替换快照。锁失败、记录损坏或保存失败时拒绝当前受控写入。并发丢失更新为源码风险，尚未实测复现。[现有快照锁](../pkg/prompt/native_store.go#L145)

**通过条件。** CLI 子进程测试覆盖旧轮写入、重复确认、相同文本不同轮、迟到回复、失败回执、跨会话和跨 worktree。只有有效确认轮已读的范围内原生写入返回允许。两个独立进程并行处理同一会话时，批准、失效和回执按事件顺序更新。

### 9.3 P0　初始化覆盖用户已有 Hook

**已复现。** `init --dry-run` 保留配置字节。执行 `init` 时，一个已经被 Git 跟踪的测试 `.cursor/hooks.json` 被整文件替换，原有用户 Hook 和独立字段消失。生成配置也缺少现行项目使用的 `preToolUse.failClosed`。

**原因。** `PlanNativeHooks` 仅探测文件是否存在，仍生成全新的配置对象。`ExecuteNativeHookPlan` 直接覆盖该路径。规则分发的 Git 跟踪保护、快照检查和合并逻辑没有覆盖原生配置路径。[原生配置计划与落盘](../pkg/adapter/native_plan.go#L24) · [初始化执行顺序](../cmd/init.go#L161)

**解决方案**

- 在 `pkg/adapter/native_plan.go` 按各宿主结构合并现有配置，保留用户条目、未知字段、启停属性和条目顺序。Agate 只更新可确认归属的条目。Cursor 按已验证协议保留 `failClosed`。同名条目归属不明、非法 JSON、重复键或链接时报告冲突。
- 将原生配置纳入 `pkg/adapter/plan.go` 的预检、Git 跟踪保护和快照复核。获准更新已跟踪配置时也只改受管条目。`require` 的冲突须在规则和 Hook 任一文件写入前返回。
- 在 `cmd/hook.go` 增加 `agate hook native status` 与 `agate hook native rollback --host <宿主>`。记录变更前后的摘要与受管条目。回退前核对当前文件，保留用户后来加入的内容。

**通过条件。** 测试已有用户 Hook、跟踪文件、并发改动、损坏 JSON、重复初始化、只读预览和定向回退。未获授权时原始配置字节保持一致。允许合并时用户条目完整保留，受管条目唯一。用真实客户端确认合并后可以加载。

### 9.4 P1　Cursor 原有上下文交付在新 CLI 中中断

**已复现。** 提交事件生成上下文文件，但普通 `Read` 的 `postToolUse` 返回空对象，项目规则等不到 `additional_context`。本机专用 `agate-hooks.exe` 在相同回放中仍返回该字段。

**原因。** `cmd/prompt.go` 优先走新注册适配器。旧 `HandleCursorEvent` 在普通读取后提供路径的行为未迁移。新状态机只在提交时向内部判定填入路径，而 Cursor 提交响应只输出 `continue`，工具后流程也没有补发路径。[CLI 入口](../cmd/prompt.go#L102) · [新适配器](../pkg/prompt/cursor_hook.go#L111) · [旧交付分支](../pkg/prompt/cursor_hook.go#L326)

**解决方案。** 在 `pkg/prompt/native_workflow.go` 的成功工具后分支中，若事件关联当前轮且上下文仍待交付，返回该轮文件的绝对路径。通过 `CursorAdapter.Encode` 输出 `additional_context`。只有原生 Read 成功命中本轮文件才登记回执。保留临时仓库内的 CLI 回归用例。在新候选通过同样验收前，项目继续使用已验收的专用 Hook 接线。

**通过条件。** 用同一个候选从 CLI 发起两轮独立请求。普通 Read 后返回本轮路径。读取其他轮或同名不同目录文件不产生回执。隔离 Cursor 客户端可自动获取路径和读取回执，并通过确认前拒绝、确认后范围内允许、越界拒绝。

### 9.5 P1　四个已注册宿主的事件协议不一致

| 宿主 | 已复现的问题与代码原因 | 解决方案与通过条件 |
| --- | --- | --- |
| **Antigravity IDE** | 官方 `PreInvocation` 样本未获得 `injectSteps`，实现返回的是 `contextPath`。原生 `write_to_file` 在确认前返回 `allow`，因为未知工具被分类成读取。源码还将存在 `lastUserInput` 视为可信新提交。 | 在 [Antigravity 适配器](../pkg/prompt/antigravity_hook.go#L40) 按实际版本输出 `injectSteps`，显式映射原生读写工具和路径。确认成功时查验事件失败信息。区分真实用户提交与模型续跑。若当前版本缺少可信来源，按[实施方案第 8 节](harnessgate-auto-init-and-cursor-remediation-plan-2026-11-19.md#8-多宿主原生-hook-实施方案)核验官方入口或记录阻塞。真实写入在确认前必须被拒绝。[官方协议](https://antigravity.google/docs/hooks?tab=ide) |
| **Claude Code** | 官方下划线命名的 `PreToolUse` 编辑载荷未获得拒绝决定。实现从 `sessionId`、`toolName`、`toolInput` 取字段，空工具名随后进入读取分支。 | 在 [Claude 适配器](../pkg/prompt/claude_hook.go#L18) 按官方 `session_id`、`tool_name`、`tool_input` 解码。核验提交与 `Stop` 的轮次关联并读取最终文本字段。使用官方格式验证原生 `Read`、`Edit` 和 `Write` 的正反用例。[官方协议](https://code.claude.com/docs/en/hooks) |
| **Codex** | 提交响应只有空 JSON，模型拿不到上下文入口。`Stop` 的 `last_assistant_message` 未登记方案，代码只读取 `response`。源码还只在 `patch`、`diff`、`content` 查补丁，既有测试使用 Git diff。 | 在 [Codex 适配器](../pkg/prompt/codex_hook.go#L20) 采用 `UserPromptSubmit` 支持的上下文输出及官方 `Stop` 字段。按 `tool_input.command` 处理 `apply_patch` 原生数据，校验新增、修改、移动、删除和多目标。所有路径都能核验时才允许。实测续跑和真人提交的区分。[官方协议](https://developers.openai.com/codex/hooks) |
| **Windsurf Cascade** | 官方 `tool_info.user_prompt` 提交返回退出码 `1`。预写事件解析失败也返回 `1`，Windsurf 阻断需要 `2`。解码字段和 CLI 错误优先返回共同导致这一结果。 | 改 [Windsurf 适配器](../pkg/prompt/windsurf_hook.go#L18) 读取 `tool_info` 等官方字段，校验异步回复的来源轮。在 [CLI 入口](../cmd/prompt.go#L114) 按该事件保留宿主指定的预写拒绝退出码，普通命令保持现有退出语义。用子进程验证越界、损坏载荷与内部失败均使预写返回 `stderr` 和退出码 `2`。[官方协议](https://docs.windsurf.com/de/windsurf/cascade/hooks) |

各适配器使用明确的读、写、Shell、MCP 工具映射。写入前无法识别的工具应明确报告未知，按宿主可验证的策略处理。发布前还要核对生成的配置结构能被目标客户端接受，区分 Hook 拒绝、启动失败、超时和该工具未触发 Hook。

### 9.6 P1　原生装配缺少运行条件、版本与状态校验

**已复现。** 在模拟 Cursor 信号下，`init -t opencode --native-hooks=require` 返回成功并写入规则，即使 OpenCode 无原生适配器。`require` 也能为与检测结果无关的 Claude 生成计划。`agate hook native status` 返回用法错误。

**原因。** `DetectNativeRuntime` 只按目标名设置 `SupportsHook`，未填写 `ClientVer`。`PlanNativeHooks` 没有核对拟装配目标与实际宿主一致，CLI 未要求 `require` 具有单一目标且计划未跳过。`cmd/hook.go` 目前只注册 Git Hook 子命令。[运行时识别](../pkg/agent/native_runtime.go#L17) · [原生模式](../cmd/init.go#L161) · [现有 Hook 子命令](../cmd/hook.go#L89)

**解决方案。** 维护经实机验证的宿主、版本、平台、运行形态和覆盖通道清单。统一校验 `auto|off|require`。`auto` 仅装配匹配当前环境的已验收入口，证据不足时报告原因。`require` 在任何项目写入前要求单一目标、宿主匹配、已验证版本和有效配置计划。补齐第 9.3 节的状态与回退接口。

**通过条件。** 覆盖无适配器、宿主不匹配、未知版本、未验收平台、多目标和非法参数。`require` 失败后规则、Hook 和 Git 配置摘要保持一致。状态准确显示「仅规则」「待加载」「已观测」「原生闭环通过」。

### 9.7 P1　新增八个目标只有规则档案

**已复现。** Copilot、TRAE、OpenCode、Qwen Code、Cline、Kiro、Antigravity CLI、Gemini CLI 可以通过模拟信号选中规则计划，但八个原生调用全返回不支持。`init -t all` 仍只列原先五个内置目标。新增规则是否被真实客户端加载也需逐个核验。

**原因。** 新增项进入内置档案和识别注册表。原生适配器注册、Hook 配置渲染和 `all` 汇总未同步。OpenCode 的独立规则文件还需由客户端实际加载。[档案定义](../pkg/agent/profile.go#L85) · [原生适配器入口](../pkg/prompt/native_hook.go#L89) · [内置目标汇总](../pkg/adapter/adapter.go#L119)

**解决方案**

1. 依据[实施方案第 8.4 节](harnessgate-auto-init-and-cursor-remediation-plan-2026-11-19.md#84-新增内置工具)逐款补上原生适配和配置渲染，复用同一状态机。核验真实进程或会话信号。Copilot 与 Cline 不能单凭 `Code.exe` 判断。OpenCode 要核对 `instructions` 引用和本地插件桥接。
2. 由同一内置注册表生成 `all` 的稳定目标列表。保留已有自定义档案、旧 `gemini` 别名和显式选择行为。对共用路径生成单一且已验证兼容的写入计划。归属或内容冲突时报告原因并保留原文件。
3. 分别记录规则加载、原生事件触发和完整门禁验收。每款执行[实施方案第 8.1 节](harnessgate-auto-init-and-cursor-remediation-plan-2026-11-19.md#81-现状与交付要求)的统一标准。缺少可运行的目标版本或官方事件时，保留该工具的阻塞项。

**通过条件。** 每个新目标无需 `profile add` 即可在真实宿主内预览和装配。`-t all` 列出全部有效目标并正确处理共享路径。逐款实测确认前拒绝、确认后范围内写入、越界拒绝和旧授权失效。

### 9.8 修复顺序、回归补测与交付判定

| 顺序 | 实施内容 | 必须通过的检查 |
| --- | --- | --- |
| 1 | 修复轮次绑定、跨进程状态写入、Hook 配置合并及 Git 跟踪保护。 | 旧轮与无新方案的「执行」均不能放行。用户 Hook 保留。配置冲突时项目无部分写入。 |
| 2 | 恢复 Cursor 上下文交付。修正 `require` 与真实宿主、版本和能力的绑定。 | 通过实际 CLI 入口回放，再在隔离 Cursor 会话复验。缺少匹配能力时预检拒绝。 |
| 3 | 修正 Antigravity、Claude Code、Codex、Windsurf 官方协议。 | 核对官方或脱敏事件样本的 stdout、stderr 和退出码。逐宿主在隔离客户端完成正反用例。 |
| 4 | 接通新增八款的原生适配、规则加载与全部目标分发。 | 每款独立登记通过的版本、平台和工具通道。保留旧别名、用户档案和回退能力。 |
| 5 | 核对候选产物与项目实际使用的 Hook。 | `go test -mod=vendor -count=1 ./...`、候选程序 `verify`、独立协议回放和真实客户端矩阵均通过。分别核对 PATH 程序及项目 Hook 二进制，记录实际版本和 SHA-256。 |

现有 Cursor 测试多处直接调用旧 `HandleCursorEvent`，而实际 `agate prompt` 优先走新注册适配器。部分多宿主单测采用与实现字段相同的构造载荷。后续补测从真实 CLI 入口发起，保留官方成功与失败样本。重跑本节 29 项检查，确认前放行、旧轮授权复用或覆盖用户配置中的任何一项，都阻断验收。[Cursor 既有测试](../pkg/prompt/cursor_hook_test.go#L11) · [既有协议测试](../pkg/prompt/multi_host_test.go#L11) · [实施方案的多宿主实施步骤](harnessgate-auto-init-and-cursor-remediation-plan-2026-11-19.md#85-实施步骤)

### 9.9 修复后复验记录（2026-11-20）

**多宿主原生接入仍未通过验收。** 本次从修复后的工作树重新构建 Windows `agate 0.6.0` 候选。候选 SHA-256 为 `C9AA57AE0780DC49D65A45D86DC1314C577FA66092F70F4F7EB8DAB48E6E82DD`。CLI 回放和配置检查使用临时 Git 仓库。第 9.1 节保留修复前的 29 项基线，本次检查扩展为 49 项。

| 验证范围 | 通过 / 总数 | 结果 |
| --- | --- | --- |
| Go 全量单测和候选程序自检 | | `go test -mod=vendor -count=1 ./...` 通过，候选程序 `agate verify` 退出码为 `0` 并输出 `[PASS]`。 |
| 原生 CLI 事件回放 | 10 / 22 | Cursor、Windsurf、Antigravity IDE、Codex 各有一项协议回放失败，新增八款工具的原生调用全部返回不支持。 |
| 原生装配与配置检查 | 14 / 18 | 回退误删一条用户 Hook，未知版本 `auto`、未验收版本 `require` 和非法模式值三项检查失败。 |
| 无 `-t` 的自动选择与手动回退 | 9 / 9 | 使用 `init --dry-run --native-hooks=off`。注入八种宿主信号时均选中对应规则目标，无可用宿主信号时提示手动传入 `-t`。 |
| 独立 CLI 检查合计 | **33 / 49** | **16 项失败。** |

| 对应问题 | 已验证通过 | 仍有问题及后续验收条件 |
| --- | --- | --- |
| 9.2　确认跨轮次生效 | Cursor 确认前拒绝写入，确认后只允许范围内写入。旧轮写入被拒绝，另起一轮且没有新方案的「执行」也被拒绝。 | 补测旧确认重复送达、缺失轮次标识、迟到回复、失败回执、跨会话和跨 worktree。同一会话以两个独立 Hook 进程验证快照更新顺序，并注入锁与保存失败。现有同名并发测试使用同一进程内的 goroutine。 [授权与回执处理](../pkg/prompt/native_workflow.go#L250) · [并发测试](../pkg/prompt/native_workflow_test.go#L305) |
| 9.3　初始化覆盖用户 Hook | 预览保留文件字节。已跟踪配置未经 `--allow-tracked` 拒绝更新，且本次冲突场景没有写入规则。授权合并保留用户 Hook、未知字段和 Cursor `failClosed`。普通用户条目可保留到回退后。 | 回退会误删仅在命令文本中提到 `agate prompt --hook` 的用户条目。使用明确的受管条目标识或安装记录核验归属，回退前复核配置快照。补测用户新增条目、归属冲突和计划生成后文件被改动等场景，验收以用户配置完整保留为准。 [归属判定](../pkg/adapter/native_plan.go#L77) · [回退实现](../pkg/adapter/native_plan.go#L375) |
| 9.4　Cursor 上下文交付 | 提交会生成本轮文件。在回放中直接读取该文件，可以登记本轮读取回执。 | 普通 `Read` 后仍未返回 `additional_context`。当前流程只在读取命中本轮文件后返回路径。应在当前轮待交付时通过成功的普通读取事件提供路径，并在读取精确命中本轮文件时登记回执。以同一候选完成两轮 CLI 回放及隔离 Cursor 会话验证。 [工具后处理](../pkg/prompt/native_workflow.go#L328) |
| 9.5　已注册宿主协议 | Claude Code 官方格式的确认前 `Edit` 被拒绝。Windsurf 损坏的预写载荷返回退出码 `2`。Antigravity IDE 原生 `write_to_file` 在确认前被拒绝。Codex 能从 `Stop.last_assistant_message` 记录方案。 | Windsurf 官方嵌套提交载荷仍返回 `1`，适配器未读取用于会话关联的 `trajectory_id`。Antigravity IDE 官方 `PreInvocation` 样本返回结果缺少 `injectSteps`，需核验可信新提交及该版本支持的注入类型。Codex `UserPromptSubmit` 返回顶层 `additional_context`，应按协议输出 `hookSpecificOutput.additionalContext`。逐宿主用官方样本及真实客户端验证交付、读取、确认和写入结果。 [Windsurf 适配器](../pkg/prompt/windsurf_hook.go#L38) · [Antigravity 适配器](../pkg/prompt/antigravity_hook.go#L50) · [Codex Hook 协议](https://developers.openai.com/codex/hooks) |
| 9.6　装配条件和状态校验 | `require` 拒绝无原生适配器和宿主不匹配的场景，已验证无适配器失败时没有写入规则。`agate hook native status` 与 `rollback` 已可调用。 | `auto` 在未知版本时仍计划装配，`require` 接受未验收的版本字符串，非法 `--native-hooks` 值未被拒绝。入口先校验枚举，按宿主、版本、平台和运行形态核对已验收能力。证据不足时 `auto` 跳过 Hook 并说明原因，`require` 在写入前报错。状态还需结合客户端观测和闭环结果验证。 [参数入口](../cmd/init.go#L161) · [装配计划](../pkg/adapter/native_plan.go#L199) |
| 9.7　新增八款工具 | 八款目标的模拟识别均通过，`init -t all` 预览已包含新增目标。 | Copilot、TRAE、OpenCode、Qwen Code、Cline、Kiro、Antigravity CLI、Gemini CLI 的原生调用仍全部返回不支持。逐款补齐已核实的事件解码、配置渲染和门禁判定，再在真实宿主中验证自动识别、规则加载及原生门禁。保留无法核实的版本或通道的阻塞状态。 [能力注册](../pkg/agent/native_runtime.go#L28) · [当前阻塞测试](../pkg/adapter/native_plan_test.go#L175) |

**下一轮修复顺序。** 先修复回退误删和 Cursor 上下文交付，补测授权轮次及跨进程状态。随后收紧装配模式与版本检查，对照宿主协议完成现有适配，再逐款接通新增目标。每个交付候选重新执行 CLI 正反用例和真实客户端验收。

本机 `PATH` 解析到的 `agate.exe`、项目 `.cursor/hooks.json` 指向的 `agate-hooks.exe` 与本次候选三者的 SHA-256 均不同。本次候选的真实客户端闭环尚未验证。复验时应记录客户端版本、该会话加载的 Hook 路径与哈希，以及实际事件和文件结果。 [项目 Cursor Hook 配置](../.cursor/hooks.json#L6)

### 9.10 第二轮复验记录（2026-11-20）

**多宿主原生接入仍未通过验收。** 本轮从当前工作树重新构建了 Windows `agate 0.6.0`。候选 SHA-256 为 `A4DC85CD7983F113CF7987C7687153D3CA68F8C9A522573ED6D793C97CF40589`。第 9.9 节的 16 个失败项中，7 项已通过，9 项仍失败。本轮另外确认两处问题，分别是已验收版本清单为空和状态命令误判紧凑 JSON。

| 验证范围 | 本轮结果 |
| --- | --- |
| 全量 Go 单测与候选程序自检 | 通过。候选程序 `agate verify` 退出码为 `0`，输出 `[PASS]`。 |
| 原生 CLI 事件回放 | 13 / 22。Antigravity IDE 上下文交付及八款工具的原生调用仍失败。 |
| 原生装配与配置检查 | 16 / 18。本轮调整了配置用例，加入版本预检、回退归属和紧凑 JSON 状态检查。正向装配被空版本清单拦截，配置合并另有单测通过。 |
| 自动选择与手动回退 | 9 / 9。注入八种宿主信号时，无需 `-t` 即可选中规则目标。缺少可用信号时提示手动指定。 |

#### 本轮通过的项目

- Cursor 普通 `Read` 能返回本轮 `additional_context`。确认前、越界、旧轮和无新方案时再次发送「执行」的写入请求仍被拒绝。迟到回复、缺失轮次和跨会话检查通过。两个独立测试进程的锁串行化用例通过，覆盖的是锁内操作的串行化。 [工具后处理](../pkg/prompt/native_workflow.go#L328) · [双进程锁测试](../pkg/prompt/native_workflow_test.go#L524)
- Windsurf 可接收含 `trajectory_id` 和 `tool_info.user_prompt` 的提交载荷。Codex `UserPromptSubmit` 响应按 `hookSpecificOutput.additionalContext` 交付上下文，`Stop` 仍能登记方案。 [Windsurf 适配器](../pkg/prompt/windsurf_hook.go#L38) · [Codex 响应编码](../pkg/prompt/codex_hook.go#L125)
- 回退仅移除已标记且命令匹配的 Agate 条目。普通用户 Hook 和命令文本中仅提到 `agate prompt --hook` 的用户条目均保留。 `auto` 跳过未知版本，`require` 拒绝未验收版本，非法模式值返回错误。版本拒绝场景下测试仓库的规则和 Hook 配置保持原状。 [回退实现](../pkg/adapter/native_plan.go#L385) · [版本校验](../pkg/adapter/native_plan.go#L239) · [模式解析](../pkg/agent/native_runtime.go#L71)

#### 剩余问题与验收条件

| 问题 | 证据与影响 | 修复办法及通过条件 |
| --- | --- | --- |
| 原生装配缺少可用版本 | 发布候选的版本清单为空。以模拟 Cursor 3.30.14 信号运行时，`auto` 跳过 Hook，`require` 在配置合并前报错。第 8 节的真实 Cursor 3.30.14 记录使用较早的专用 Hook。 [版本清单](../pkg/agent/native_runtime.go#L43) | 用本轮或后续候选在隔离客户端复验，记录通过的宿主、版本、平台和运行形态，再登记对应能力。用实际 CLI 验证已登记环境能预览并装配，已跟踪配置仍需明确许可。未知或未验收环境继续降级或拒绝。 |
| Antigravity IDE 上下文交付 | 官方 `PreInvocation` 样本提供 `lastUserInput`、`invocationNum` 等字段。当前实现要求 `prompt` 才进入提交处理，回放结果缺少 `injectSteps`。确认前写入拒绝仍通过。 [适配器](../pkg/prompt/antigravity_hook.go#L49) · [官方 Hook 协议](https://antigravity.google/docs/hooks?tab=ide) | 核验当前版本可用于区分新提交与模型续跑的官方信号，采用该版本支持的 `injectSteps` 格式。用官方载荷和真实客户端分别验证首次交付、续跑不重复登记、实际读取回执和写入判定。缺少可信提交信号的版本继续标记为受阻。 |
| 八款工具的原生接入 | Copilot、TRAE、OpenCode、Qwen Code、Cline、Kiro、Antigravity CLI、Gemini CLI 的规则目标均可通过模拟信号选中，八款原生调用仍全部返回不支持。 [原生能力注册](../pkg/agent/native_runtime.go#L28) · [阻塞测试](../pkg/adapter/native_plan_test.go#L191) | 按[实施方案第 8.4 节](harnessgate-auto-init-and-cursor-remediation-plan-2026-11-19.md#84-新增内置工具)逐款接通官方事件及装配入口，用真实宿主独立验证自动识别、规则加载、事件交付和确认前后写入判定。规则分发可用时保留现有无 `-t` 路径。 |
| `hook native status` 误报 | 同一份受管配置换成紧凑 JSON 后，状态从「已托管」变为「无 Agate 条目」。现有实现匹配带固定空格的标记文本。 [状态实现](../pkg/adapter/native_plan.go#L374) | 解析 JSON，复用回退的按事件和命令核验归属逻辑。验证紧凑和缩进格式返回相同状态，并补测用户字段中的同名标记、空配置和无效 JSON。 |

本轮 CLI 回放及配置检查均在临时仓库完成。项目 Cursor 配置仍指向专用 `agate-hooks.exe`，其 SHA-256 与本轮候选不同。本轮候选的真实客户端闭环仍待验收。 [项目 Cursor Hook 配置](../.cursor/hooks.json#L6)

### 9.11 第三轮复验记录（2026-11-20）

**上轮四个问题中，状态误报已修复，其余三个仍影响原生接入验收。** 本轮从当前工作树重新构建 Windows `agate 0.6.0`，候选 SHA-256 为 `AEC59D7C4D6EAA99FDAD2C56C681D8F560CD9EE98CD7FD342A6293F11DFCA2AC`。

| 验证范围 | 本轮结果 |
| --- | --- |
| 全量 Go 单测与自检 | 通过。候选 `agate verify` 退出码为 `0`，输出 `[PASS]`。 |
| 原有 Hook 事件回放 | 13 / 22，与上轮相同。Cursor、Claude Code、Codex、Windsurf 的已通过用例未退化。Antigravity IDE 上下文交付及八款工具的原生调用仍失败。 |
| Antigravity IDE 专项回放 | 2 / 5。编号 `1` 的构造载荷会返回步骤，未更换用户输入的续跑未重复注入。官方首次调用编号 `0`、注入步骤格式、同一会话内更换用户输入三项仍失败。 |
| 状态、回退与装配检查 | 11 / 13。两项失败均涉及 Cursor 3.30.14 的原生装配，分别是 `auto` 预览和 `require` 落盘。 |
| 自动选择与手动回退 | 9 / 9。八种模拟宿主信号均无需 `-t` 即可选中规则目标，缺少可用信号时提示 `-t`。 |
| 无参数 `init` 实际落盘 | 4 / 4。分别注入 Cursor 和 Antigravity IDE 的宿主信号，在临时仓库实际运行 `agate init`，两者均写入规则并跳过未验收的原生 Hook。 |

#### 已修复的状态误报

`hook native status` 已按 JSON 结构检查受管条目。紧凑和缩进格式均识别为「已托管」。用户条目上的同名标记不会触发误报。空文件显示无 Agate 条目，损坏 JSON 显示阻塞。回退仍会保留命令文本中仅提到 Agate 的用户 Hook。 [状态解析](../pkg/adapter/native_plan.go#L349)

#### 剩余问题与通过条件

| 问题 | 本轮证据 | 修复办法与通过条件 |
| --- | --- | --- |
| 原生 Hook 装配缺少可用版本 | 已验收版本清单仍为空。模拟 Cursor 3.30.14 时，`auto` 跳过 Hook，`require` 在写入规则和 Hook 前返回错误。 [版本清单](../pkg/agent/native_runtime.go#L43) | 用同一候选完成隔离客户端闭环，再登记有实机证据的宿主、版本、平台和运行形态。已登记环境应能预览并装配，未知版本应继续跳过或拒绝。 |
| Antigravity IDE 上下文交付仍不符合协议 | 官方将 `invocationNum` 从 `0` 开始计数，候选只在编号 `1` 时用 `lastUserInput` 进入提交处理。编号 `0` 的回放没有 `injectSteps`。编号 `1` 返回的步骤使用 `{ "type": "read_file", "path": ... }`，不符合官方步骤格式。同一会话内更换用户输入的回放仍未收到上下文。 [事件解码](../pkg/prompt/antigravity_hook.go#L52) · [响应编码](../pkg/prompt/antigravity_hook.go#L134) · [官方 Hook 协议](https://antigravity.google/docs/hooks?tab=ide) | 区分缺失编号与合法的 `0`，根据可验证的宿主信号识别新一轮提交，并使用官方支持的注入格式。回归用例明确断言首次交付、后续独立提交、同一请求续跑不重复交付，以及读取回执和写入判定。客户端无法提供可信轮次关联时保留阻塞状态。 |
| 八款工具仍缺少原生接入 | Copilot、TRAE、OpenCode、Qwen Code、Cline、Kiro、Antigravity CLI、Gemini CLI 的模拟宿主检测通过，八款的原生 Hook 调用均返回不支持。 [原生能力注册](../pkg/agent/native_runtime.go#L28) | 按[实施方案第 8.4 节](harnessgate-auto-init-and-cursor-remediation-plan-2026-11-19.md#84-新增内置工具)逐款核对官方事件、装配格式和版本，验证真实宿主自动识别、规则加载、事件交付和确认前后写入行为。 |

本轮结果来自新候选和临时仓库。项目 Cursor 仍指向 SHA-256 不同的专用 `agate-hooks.exe`。新候选的真实客户端闭环仍待验收。 [项目 Hook 配置](../.cursor/hooks.json#L6)

### 9.12 当前未关闭问题与验收清单（2026-11-20）

第四轮复验使用从工作树构建的 Windows `agate 0.6.0`，候选 SHA-256 为 `77B60E9608E5A989AD1B29BB0E33393C4E9C01C97CCDBB4A146B02ED6A9D5575`。CLI 复验在临时 Git 仓库执行。

| 检查 | 第四轮结果 |
| --- | --- |
| 全量 `agate verify` | 输出 `[FAIL] 单元测试执行失败`。`TestCursorApprovalFollowsCurrentDeliveredPlan` 保存请求状态时，Windows 拒绝重命名。另跑 `pkg/adapter`、`pkg/agent`、`pkg/prompt` 均通过。单独重复失败用例 10 次，9 次通过、1 次复现同样错误。 |
| 原有 Hook 回放 | 14 / 22，通过项比上轮多 1 项。Antigravity 的首次上下文交付通过，8 个失败项均为新增工具的原生调用。 |
| Antigravity IDE 专项回放 | 5 / 7 符合各用例的预期。两项失败为读取失败回执和拒绝响应格式。编号递增时换输入的用例记录当前行为，该编号是否在真实客户端的新一轮重置仍待确认。 |
| 装配、识别、状态与回退 | 13 / 15。Cursor 3.30.14 的原生预览与强制装配仍失败。模拟 Cursor 信号运行无参数 `init` 会写入规则。八种模拟信号可自动选中对应规则目标，无可靠信号时提示 `-t`。紧凑 JSON 状态识别和回退保留用户 Hook 均通过。 |

#### 已复现的交付阻断

| 问题 | 证据和影响 | 修复位置与通过条件 |
| --- | --- | --- |
| Cursor 请求状态偶发写入失败 | 全量测试及单项复跑均在 `afterAgentResponse` 保存请求 JSON 时遇到 `Access is denied`。`SaveRequestState` 写固定临时文件，再调用 `os.Rename` 替换目标。具体由哪个进程或 Windows 条件导致此次拒绝，尚未查明。影响请求状态更新和全量自检稳定性。 [保存入口](../pkg/prompt/workflow.go#L86) · [复现用例](../pkg/prompt/cursor_hook_test.go#L149) | 先在临时项目核对句柄占用、目标状态和错误码，再根据证据修复 Windows 替换流程。复用仓库可用的原子写入机制，验证覆盖旧状态、实际保存失败时旧记录可读、重试和同一请求并发写入。失败用例重复 10 次全部通过，并重跑完整 `agate verify`。 |
| Antigravity IDE 读取失败被登记为已读 | 官方 `PostToolUse` 用 `error` 报告失败，适配器只检查可选的 `success`，缺失时当作成功。用真实协议字段回放失败读取后，快照仍从 `pending_read` 变为 `observed_read`。 [事件解码](../pkg/prompt/antigravity_hook.go#L87) · [读取回执](../pkg/prompt/native_workflow.go#L328) · [Google 官方协议](https://antigravity.google/docs/hooks?tab=ide) | 按官方结果字段解码，接入统一读取回执。失败、错路径、旧轮及缺少关联证据的读取都应保持未交付，只有可靠关联的成功读取才能登记。断言快照字段和后续门禁结果。 |
| Antigravity IDE 拒绝响应缺少终止字段 | 工作区不匹配的回放中，候选退出码为 `1`，返回 `continue:false` 和原因，未返回官方用于终止模型调用的 `terminationBehavior`。当前结果证明响应格式缺项，真实客户端是否终止仍待观察。 [响应编码](../pkg/prompt/antigravity_hook.go#L130) · [Google 官方协议](https://antigravity.google/docs/hooks?tab=ide) | 将 `PreInvocation` 拒绝映射为官方 `terminationBehavior: "terminate"`，按协议携带用户可见错误。用协议回放及真实会话验证拒绝后模型调用确实停止。保留 `PreToolUse` 的独立工具拒绝语义。 |
| 已验收版本清单为空 | 版本集合为空，已注册适配器也无法走通自动装配。模拟曾在第 8 节实测的 Cursor 3.30.14，`auto` 跳过 Hook，`require` 在写入前报错。第 8 节用的是较早的专用二进制。当前匹配只核验宿主和版本，平台已经采集但未参与匹配。 [版本清单](../pkg/agent/native_runtime.go#L43) · [装配预检](../pkg/adapter/native_plan.go#L199) | 先在隔离客户端用同一候选验收，再依据实际证据登记宿主、版本、平台和运行形态。补齐版本来源和匹配检查。已验收环境可自动装配，未知版本保留规则分发，`require` 遇不匹配时应在改动前拒绝。 |

#### 八款工具的原生接入缺项

八款工具已登记为内置规则目标。官方入口已存在，当前代码缺少对应的原生适配器和装配逻辑。`agate prompt --hook` 无法分发这八款的事件，`init` 中的原生装配预检也会拦住它们。现有测试明确要求这八款保持阻塞。为每款增加适配时，应同步补齐事件解码、结果编码、配置合并和回退，再用已验证的正向用例替换该款的阻塞预期。尚无完整证据的宿主或运行形态继续保持阻塞。 [规则档案](../pkg/agent/profile.go#L85) · [事件分发](../pkg/prompt/native_hook.go#L90) · [装配入口](../pkg/adapter/native_plan.go#L199) · [阻塞测试](../pkg/adapter/native_plan_test.go#L191)

| 工具 | 官方入口与 Agate 尚缺的工作 |
| --- | --- |
| GitHub Copilot | 为 Copilot CLI 生成并合并 `.github/hooks/*.json`，分别校对 CLI、VS Code Agent 会话的事件和响应。用会话证据识别实际运行者，覆盖用户提交、工具前后事件和停止事件。 [GitHub Hook 参考](https://docs.github.com/en/copilot/reference/hooks-reference) · [VS Code Hook 文档](https://code.visualstudio.com/docs/agent-customization/hooks) |
| TRAE IDE | 合并 `.trae/hooks.json`，适配国内版的提交和工具事件。用真实 `Write`、`Edit` 验证拒绝和范围内放行。国际版和其他运行形态分别记录。 [TRAE Hook 文档](https://docs.trae.cn/ide_hook-configuration-reference) |
| OpenCode | 通过官方服务端插件桥接提交、工具执行前后和结束信号。按目标插件 API 版本验证加载、权限判定和子进程失败处理。规则分发另需核验项目配置实际引用了 Agate 规则。 [OpenCode 插件接口](https://opencode.ai/docs/build/plugins/server) · [规则配置](https://opencode.ai/docs/instructions/) |
| Qwen Code | 合并 `.qwen/settings.json`。验证提交和工具前后事件的会话关联，以及工作区信任或项目 Hook 审批对加载的影响。 [Qwen Code Hook 文档](https://qwenlm.github.io/qwen-code-docs/en/users/features/hooks/) |
| Cline | 分别适配 VS Code 扩展的 `.clinerules/hooks/` 和 CLI 的 `.cline/hooks/`。各自核对脚本执行方式、载荷结构与支持平台。 [Cline 官方仓库](https://github.com/cline/cline/blob/main/docs/customization/clineignore.mdx) |
| Kiro | 按目标 IDE 或 CLI 版本生成 `.kiro/hooks/*.json`。验证指令提交、执行前阻断和工具结果事件。按运行形态确认事件和工具匹配范围。 [Kiro Hook 文档](https://kiro.dev/docs/hooks/) |
| Antigravity CLI | 用官方 `.agents/hooks.json` 格式接通 CLI 目标。评估与 IDE 共用协议层和配置时的条目归属，CLI 仍需独立的识别和闭环验收记录。 [Google CLI Hook 文档](https://antigravity.google/docs/hooks?tab=cli) |
| Gemini CLI | 合并 `.gemini/settings.json`，映射 `BeforeAgent`、`BeforeTool`、`AfterTool`、`AfterAgent`。按旧环境兼容目标测试规则加载、工具拒绝及已确认范围内放行。 [Gemini CLI Hook 参考](https://geminicli.com/docs/hooks/reference/) |

#### 源码核对和仍待完成的验收

- **已注册宿主的配置生成。** 当前生成器统一写 `hooks.<事件>`。Antigravity 官方格式在顶层以 Hook 名称分组，并为工具前后事件配置 matcher 和处理器。按目标宿主的实际结构实现格式校验、合并、状态和回退。在真实客户端确认生成配置被加载并触发。其他已注册宿主也须验收生成配置。 [当前生成器](../pkg/adapter/native_plan.go#L108) · [Antigravity 官方配置](https://antigravity.google/docs/hooks?tab=ide)
- **配置计划过期与部分写入。** `ExecuteNativeHookPlan` 备份实时内容，但随后写入生成计划时的旧内容，执行处缺少原生配置快照复核。`init` 先应用规则计划，再写 Hook。应在落盘前复核生成计划时的文件内容和归属，明确写入期故障的部分完成状态及回退方式。使用计划后改动配置、写入失败、重复初始化和回退保留用户新增条目等测试取证。 [执行原生计划](../pkg/adapter/native_plan.go#L316) · [初始化顺序](../cmd/init.go#L194)
- **同会话轮次和结果关联。** Antigravity 首次 `invocationNum=0` 已能通过 `ephemeralMessage` 返回有效上下文路径。第二轮回放仅在编号重置为 `0` 时交付新路径，真实客户端是否重置仍待取证。`Stop` / `PostInvocation` 当前读取 `response`，公开载荷未列出该字段。用实际事件或可核验的会话记录关联提交、模型回复、读回执和写入。对所有已注册宿主补测迟到回执、缺少轮次、存储失败及确认前后实际文件变化。 [Antigravity 适配器](../pkg/prompt/antigravity_hook.go#L51) · [会话处理](../pkg/prompt/native_workflow.go#L328) · [Google 官方协议](https://antigravity.google/docs/hooks?tab=ide)
- **真实宿主和交付产物。** 第四轮只有候选 CLI 和临时仓库证据。项目 Cursor 仍指向 SHA-256 不同的专用 `agate-hooks.exe`。应在隔离客户端核对实际调用路径、候选哈希、宿主版本、平台、运行形态及事件记录。复验真实 Cursor、Antigravity IDE 及其他目标的无 `-t` 初始化、规则实际加载和完整门禁。[实施方案第 6 节](harnessgate-auto-init-and-cursor-remediation-plan-2026-11-19.md#6-验收执行顺序与证据)另列的自定义工具自动识别和「灵动岛」规则加载须分别补取实机证据。 [既有实机验收范围](#8-cursor-真实会话验收) · [项目 Hook 配置](../.cursor/hooks.json#L6)

实施顺序为先查明并处理 Windows 请求状态保存失败，同时修正 Antigravity 的失败回执和拒绝协议。随后补齐配置生成、版本识别及实机验收，再逐款接入新增工具。关闭问题需保留全量测试、独立协议回放和对应真实客户端结果。

## 10. 2026-11-24 Antigravity IDE 2.8.0 实机验收

本节不改写上文 11-20 的历史结论。对 IDE 2.8.0，它替换「`PostInvocation` 只读 `response`」和「Antigravity 原生门禁尚未实机通过」这两条当时的开放项。

| 项 | 结果 |
| --- | --- |
| 宿主 | Antigravity IDE 2.8.0，Windows，临时 Git 仓库，手工 `.agents/hooks.json` 与临时候选 `agate.exe` |
| 回复来源 | `PostInvocation` 无 `response`。适配器读取 `transcriptPath`，只取 `step_index` 不大于 `initialNumSteps` 的最后一条 `PLANNER_RESPONSE` |
| 确认绑定 | 确认轮没有 TurnID 时绑定当前 `ActiveTurnID` |
| 方案登记 | 模型输出 `agate-plan` 且 `files` 为 `sample.txt` 后，快照出现 `plan_candidate`，`plan_confirmed` 仍为 false |
| 范围内写入 | 单独回复「执行」后，`write_file` 写入 `sample.txt`，内容为 `hello`，`PostToolUse` 的 `error` 为空 |
| 范围外拒绝 | 未确认时对 `other.txt` 的 `write_file` 被 pre-tool hook 拒绝，文件不存在。拒绝原因为诊断状态未诊断 |
| 仍未关闭 | 主动 `Stop` 未测。版本目录为空，`agate init` 不会自动挂载。候选二进制未安装到 PATH。Claude Code、Codex、Windsurf 与八个扩展宿主没有同一矩阵的实机证据 |

契约已写入 [Antigravity IDE 原生 Hook](../contexts/context.md#antigravity-ide-原生-hook)。未完成项记在 `TASK.md` 的 `TASK-20261124-NATIVE-HOOKS`。
