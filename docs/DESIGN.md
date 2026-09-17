# Agate (Agent Gate) 系统架构与技术设计文档

## 一、 核心问题与设计考量

### 1.1 现实痛点
在实际混合使用 Cursor、Antigravity、Claude Code 等 AI 工具时，主要有三个工程痛点：

- **工具生态分裂**：每款工具各认各的规则路径（`.cursorrules`、`GEMINI.md`、`CLAUDE.md`）。换个环境就得重新教一遍，久而久之各处规则严重漂移。
- **团队代码库被污染**：AI 生成的会话缓存、任务清单（`TASK.md`）和临时文件经常被误提交。直接改团队公共 `.gitignore` 又会把个人偏好强加给同事。
- **代码变动缺乏闭环证据**：AI 经常未经方案确认就大幅覆写代码，改坏后又盲目乱试，甚至提交编译不通过的半成品。

### 1.2 核心防线
针对这些问题，`agate` 确立了三条务实的工程防线：

1. **规约单一事实源**：全局只维护一份核心规范，由工具自动分发到各个编辑器的对应目录，杜绝规则多头维护；
2. **本地隐形隔离**：利用 Git 原生的本地私有机制（`.git/info/exclude`）静默隐藏 AI 专有文件，对团队公共提交树完全透明；
3. **两阶段门禁与自检闭环**：代码改动前必须输出方案等待确认；改动后必须在终端执行自检并拿到绿色的通过物证；同一错误连续两次未过强制熔断交还主控权。


---

## 二、 架构总览与核心分层

```
+-------------------------------------------------------------------------------+
|                             AI Agent 交互层 (LLM Context)                      |
|  - 两阶段硬门禁 (Two-Phase Gate)          - 3-Hop 寻路协议 (AGENTS -> Context) |
|  - 中文原生 & 代码完整性保证               - 两振硬熔断机制 (Two-Strike Failsafe)|
+---------------------------------------+---------------------------------------+
                                        | 规则分发 & 约束对齐
+---------------------------------------v---------------------------------------+
|                      agate 核心分发与适配引擎 (CLI Engine)                    |
|  - 模板自包含引擎 (go:embed)               - 跨平台软链/拷贝自适应 (Symlink Fallback)|
|  - 索引防爆仓生成 (.ignore)               - 目标环境探测 (Adapter Matrix)          |
+---------------------------------------+---------------------------------------+
                                        | 挂载与拦截
+---------------------------------------v---------------------------------------+
|                            本地 Git & 物理文件系统层                          |
|  - 私有跟踪隔离 (.git/info/exclude)        - 提交硬门禁 (.git/custom-hooks)        |
|  - 闭环验证执行器 (agate verify)           - 架构地图资产 (AGENTS.md / contexts)    |
+-------------------------------------------------------------------------------+
```

### 2.1 Agent 交互层
约束 AI 行为的规则引擎，无论用户接入何种 IDE 或模型，均统一注入以下基线：
- **A 级只读诊断**：遇疑问句、报错信息或模糊需求，强制只读分析，严禁调用写操作工具；
- **C 级实施确认**：实施前必须输出精简 Plan，等待用户显式回复“执行”；
- **交付闭环**：修改后自动调起自检命令，获取 `[PASS]` 退出码与物证方可交付。

### 2.2 CLI 核心引擎
采用 Go 编写的单二进制可执行文件：
- **`pkg/adapter`**：统一规约分发器，将内嵌或全局规约映射到 `.gemini/GEMINI.md`、`.cursorrules`、`CLAUDE.md` 等目标文件，支持非破坏性备份与原子写入；
- **`pkg/git`**：Git 仓库探查器，管理 `.git/info/exclude` 受管块、Git Worktree 寻径与本地 Hook 生命周期；
- **`pkg/guard`**：Phase 0 静态合规与安全审计引擎，执行敏感凭据、超大文件、断点残留、绝对路径等 7 大红线拦截，支持暂存区定向审查与大仓库内存忽略查询；
- **`pkg/relay`**：跨 Agent 任务接力与状态机中枢，管理双模 TASK.md、租约互斥锁与接力四要素；
- **`pkg/reporter`**：自包含单文件 HTML 审查报告引擎，汇聚安全审计、Diff 对比、任务看板与物证日志，支持跨平台浏览器自动弹出；
- **`pkg/harness`**：地图骨架生成器、跨平台文件操作、软链回退与真原子写入机制；
- **`internal/templates`**：通过 `go:embed` 将标准规约、地图骨架、任务模板与审查 HTML 模板固化进可执行二进制中，支持脱网离线运行。

### 2.3 底层 Git 与执行层
- **隐形隔离机制**：利用 Git 内置的 `.git/info/exclude` 本地私有忽略文件，采用受管标记块（Managed Block）增量注入，避免修改团队共用的 `.gitignore`；
- **双重物理 Hook 路径**：通过 `git config --local core.hooksPath .git/custom-hooks` 同时挂载私有 `pre-commit`（暂存区定向自检）与 `pre-push`（物理阻断 AI/脚本自动化偷跑推流，锁定提交权 100% 归人类），不破坏用户全局或其他三方 Hook。

---

## 三、 详细模块设计

### 3.1 目录结构规划
```
agate/
├── cmd/
│   ├── root.go             # 根命令定义、版本信息与全局 Flag
│   ├── init.go             # agate init 护栏编排逻辑
│   ├── verify.go           # agate verify 自检引擎 (--staged, --strict, --skip-guard, --report)
│   ├── export.go           # agate export 自包含 HTML 审查报告导出
│   ├── view.go             # agate view 快速浏览器预览审查报告
│   ├── task.go             # agate task 跨 Agent 任务接力 (status/claim/handover/resume)
│   ├── isolate.go          # agate isolate 隐形隔离逻辑
│   ├── scan.go             # agate scan 拓扑扫描与确定性回填
│   └── hook.go             # agate hook install / uninstall / status
├── pkg/
│   ├── adapter/
│   │   └── adapter.go      # 多 Agent 目标定义、规约分发与非破坏性备份
│   ├── git/
│   │   ├── exclude.go      # .git/info/exclude 受管块管理、去重与根目录寻径
│   │   ├── hook.go         # Git Hooks 安装、激活、卸载、状态查询与防偷跑
│   │   └── diff.go         # 原生 Git Diff 提取、分支元数据读取
│   ├── guard/
│   │   └── preflight.go    # Phase 0 静态安全红线、敏感信息脱敏与暂存区定向审计
│   ├── relay/
│   │   ├── manifest.go     # TASK.md Frontmatter 双模解析与格式化
│   │   ├── lock.go         # .ai-memory/locks/task.lock 租约互斥锁
│   │   └── relay.go        # 认领、自检交接与唤醒接棒业务编排
│   ├── reporter/
│   │   ├── report.go       # 数据汇聚、HTML 模板编译落盘与历史查找
│   │   └── browser.go      # 跨平台系统浏览器拉起实现 (xdg-open / open / start)
│   └── harness/
│       ├── fs.go           # 跨平台软链、拷贝降级与崩溃安全原子写 (WriteFileAtomic)
│       └── scaffold.go     # AGENTS.md / contexts / TASK / MEMORY 骨架初始化
├── internal/
│   └── templates/
│       ├── embed.go        # go:embed 静态资源声明
│       ├── review.html.tpl # 自包含 HTML 审查报告模板 (零 CDN 外链依赖)
│       ├── task.tpl        # YAML Frontmatter + 接力四要素任务看板模板
│       ├── SKILL.md        # 核心 AI 协同规约 (SSOT)
│       ├── ignore.tpl      # .ignore 索引防爆仓模板
│       ├── agents.tpl      # AGENTS.md 初始骨架模板
│       ├── context.tpl     # contexts/context.md 技术契约模板
│       └── memory.tpl      # MEMORY.md 长期记忆模板
├── vendor/                 # 离线打包依赖
├── go.mod
├── go.sum
└── main.go
```

### 3.2 跨平台软链与优雅降级算法
在 Windows 环境下，普通用户通常没有 `SeCreateSymbolicLinkPrivilege` 权限。若盲目调用 `os.Symlink` 会抛出 `A required privilege is not held by the client` 错误。

`agate` 采用如下两段式容错机制：
```go
func LinkOrCopy(src, dst string) error {
    // 1. 尝试符号链接 (Linux / macOS / 开启开发者模式的 Windows)
    err := os.Symlink(src, dst)
    if err == nil {
        return nil
    }

    // 2. 捕获权限不足或 Windows 特殊错误，平滑降级为物理文件拷贝
    if runtime.GOOS == "windows" || isPrivilegeError(err) {
        return copyFile(src, dst)
    }

    return err
}
```

### 3.3 验证调度状态机 (`agate verify`)
`agate verify` 为 Agent 提供确定性的交付验收门禁，其执行遵循严格的两阶段防御：

```mermaid
graph TD
    Start[执行 agate verify] --> Phase0[Phase 0: 仓库纯净度与安全合规前置拦截]
    Phase0 --> CheckClean{通过合规扫描?<br/>- 绝对路径泄露<br/>- 二进制/大图片<br/>- 未隔离私有文件<br/>- vendor上游污染}
    CheckClean -->|违规报错| ExitFail[输出 FAIL 退出码 1 熔断拦截]
    CheckClean -->|合规通过| CheckCustom{是否存在自定义脚本?}
    CheckCustom -->|存在 verify.sh 或 verify.cmd| RunCustom[执行项目专属验证脚本]
    CheckCustom -->|不存在| CheckBuild{探测常见构建配置文件?}
    CheckBuild -->|pom.xml| RunMaven[mvn test-compile -q]
    CheckBuild -->|package.json| RunNpm[npm test --if-present]
    CheckBuild -->|go.mod| RunGo[go test ./...]
    CheckBuild -->|无上述文件| RunFallback[通用轻量语法/文件完备性扫描]
    RunCustom --> ResultCheck{退出码 == 0 ?}
    RunMaven --> ResultCheck
    RunNpm --> ResultCheck
    RunGo --> ResultCheck
    RunFallback --> ResultCheck
    ResultCheck -->|是| ExitPass[输出 PASS 退出码 0]
    ResultCheck -->|否| ExitFail
```

### 3.4 关键工程优化与安全设计

#### 1. 崩溃安全原子写入 (`pkg/harness/fs.go:WriteFileAtomic`)
规约分发或模板更新时，直接调用 `os.WriteFile` 在遇到断电或强杀时容易造成半截文件损坏。Agate 采用标准的崩溃安全原子模式：
- 在目标文件同卷目录下创建临时文件；
- 将内容完全写入后调用 `file.Sync()` 强制将数据刷入物理磁盘介质；
- 调用操作系统原生原子重命名（`os.Rename`）瞬间替换目标文件，确保任何意外场景下原文件完整无损。

#### 2. 大仓库批量内存忽略优化 (`pkg/guard/preflight.go:newGitContext`)
在包含数万未跟踪文件的大型仓库中，对每个文件调用一次 `git check-ignore` 子进程会产生秒级甚至十秒级的进程创建开销。
Agate 在初始化扫描上下文时，单次执行 `git -c core.quotepath=false ls-files --others -i --exclude-standard` 批量读取被忽略路径集合并缓存在内存中；遍历时通过纯内存层级路径前缀判定，将 Git 忽略检测系统调用降为 0，扫描在 10ms 内极速完成。

#### 3. 暂存区精准审查 (`agate verify --staged`)
解决工作区脏代码与实际准备提交内容脱节的痛点：
- 调用 `git diff --cached --name-only --diff-filter=ACMR` 定向获取暂存文件列表；
- 优先通过 `git show :<path>` 直接读取 Git Index 中的暂存内容，而非工作区物理文件；
- 仅当暂存区内容触发安全红线时阻断，未暂存的本地探索性脏代码互不干扰。

---

## 四、 跨平台与技术选型论证

### 4.1 为什么选择 Go 而非 Node.js (npx) / Python / Shell？

| 考量维度 | Go 单二进制方案 | Node.js (npx) 方案 | Shell 脚本方案 |
| :--- | :--- | :--- | :--- |
| **外部运行时依赖** | **零依赖**。开箱即用，无环境绑架 | 强依赖宿主机已安装 Node.js/npm | 依赖 Bash/PowerShell，语法差异大 |
| **Git Hook 启动耗时** | **< 10ms**，无感知极速拦截 | 200~800ms，每次提交明显卡顿 | 10~50ms，但在 Windows 上极不稳定 |
| **跨平台路径与权限** | 标准库原生抽象抹平 OS 差异 | 需要第三方 npm 模块拼接处理 | Windows 与 Linux 脚本逻辑完全分裂 |
| **分发与更新** | 单文件拷贝、Homebrew、Scoop | `npm install -g` 或 `npx` | 手动下载 `curl | bash`，维护繁琐 |
| **后端/运维无前端场景** | 100% 友好，纯 Java/Go/C++ 项目无负担 | 纯后端服务器需被迫额外安装 Node 运行时 | 勉强支持，但跨系统难以移植 |

结论：对于底层 Git 治理与轻量门禁工具，编译型单二进制（Go）在启动性能、通用性与系统侵入性上具有压倒性优势。

---

## 五、 后续演进路线图 (Roadmap)

### Phase 1: 核心基础（已完成落地）
- [x] CLI 根命令骨架与嵌入式模板（`go:embed`）；
- [x] 多 Agent 规则分发与软链降级拷贝；
- [x] `.git/info/exclude` 自动化去重注入与隔离；
- [x] `agate verify` 自检门禁与本地 `pre-commit` 拦截。

### Phase 2: 动态架构地图与自适应探查
- [x] `agate scan`：自动解析项目 `pom.xml`、`package.json`、`go.mod`、路由配置文件，自动丰富 `AGENTS.md` 端口拓扑；
- [ ] 跨工程全局规约同步机制（支持从 `~/.agate/rules.md` 同步最新规则至当前项目）。

### Phase 3: CI/CD 接入与云端协同
- [ ] GitHub Actions / GitLab CI 门禁插件集成；
- [ ] 规则一致性校验（`agate check --strict`），防止本地规约被手动意外篡改。
