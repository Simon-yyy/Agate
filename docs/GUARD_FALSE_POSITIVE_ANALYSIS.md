# `agate verify` 前置护栏误报分析报告

> **审查对象**：HarnessGate（agate CLI）verify 命令
> **审查链路**：`cmd/verify.go`（Phase 0 调度）→ `pkg/guard/preflight.go`（全部审计规则）
> **关联文档**：[CODE_REVIEW.md](CODE_REVIEW.md)（整体代码审查报告）
> **核心结论**：会误报，且一旦命中 ERROR 即 `os.Exit(1)` 整体拦截，无任何逃生开关。

---

## 0. 一句话结论

`agate verify` 的 Phase 0 前置审计（`guard.RunPreflightAudit`）当前确认存在 **7 类真实误报场景**、**2 类环境性误触发**、**1 处潜伏逻辑 bug**。其中 P0 级两类（`.github/` 目录误拦、非 Git 目录全规则退化）在主流仓库布局与裸目录环境下命中率极高。由于审计结果无白名单机制、无 `--skip-guard` 应急旗标，任何一条 ERROR 都会在**测试尚未启动前**直接判死交付。

---

## 1. 拦截链路与结构性问题

### 1.1 Phase 0 的调用位置

```go
// cmd/verify.go
auditResult := guard.RunPreflightAudit(".")
auditResult.PrintReport()
if auditResult.HasErrors() {
    elapsed := time.Since(startTime).Round(time.Millisecond)
    fmt.Printf("\033[91m[FAIL] 触发安全护栏拦截，拒绝交付 (耗时: %v)\033[0m\n", elapsed)
    os.Exit(1)  // ← 任何单条 ERROR 即整体退出，测试/构建根本不会执行
}
```

### 1.2 三个结构性缺陷

| # | 缺陷 | 后果 |
|---|------|------|
| 1 | 无配置化白名单（如 `agate.toml` 的 `[guard] allow = [...]`） | 项目特例无法声明豁免，规则只能"一刀切" |
| 2 | 无 CLI 应急旗标（如 `verify --skip-guard`） | 误报发生时用户没有任何绕过手段，只能改源码 |
| 3 | 审计失败 = 拦截，与工具声明的"平滑降级"哲学冲突 | 环境/布局差异被当成违规处理 |

**判定数据流**：`Violation.Level == "ERROR"` → `HasErrors() == true` → `os.Exit(1)`。ERROR/WARN 的分级存在，但分级后的逃生语义没有闭环。

---

## 2. 已有的防误报设计（先说公道话）

代码里确实做了不少防御，以下场景**不会**误报：

| 已处理场景 | 实现位置 | 机制 |
|---|---|---|
| 文件已在 `.gitignore` / `.git/info/exclude` 中 | `isIgnored()` | 走 `git check-ignore`，ignored 且未 tracked 即放行 |
| guard 包自身源码含正则样例 | 路径审计 + 内容审计 | `pkg/guard/` 前缀豁免 |
| docs 目录中的路径示例 | `auditHardcodedPaths` | 跳过 `docs/` |
| 模板文件中的占位内容 | `auditContentHygiene` | 跳过 `internal/` |
| 注释中 `<YourUser>`、含"示例"字样的行 | `auditHardcodedPaths` | 行级白名单 |
| `node_modules`/`vendor`/`dist`/`build` 等目录 | `isCommonIgnoredDir()` | 目录名剪枝 |

问题在于：防御面留了几个大洞，且**全部无豁免通道**。以下按命中率降序详述。

---

## 3. 误报场景矩阵

### 3.1 🔴 [P0] `.github/` 目录不在剪枝清单 —— 社交预览图直接拦截

`isCommonIgnoredDir()` 的清单里有 `.git`，但 `.git` 是**精确匹配**，不包含 `.github`。后果：

```text
.github/assets/social-preview.png   （常见 100~500KB，tracked）
→ [ERROR] 大文件图片拦截 "图片文件过大，严禁直接入库"
→ 整个 verify 拒绝交付
```

`.github/` 放 issue 模板、CI workflow、社交预览图是主流开源仓库的标准布局，**本条命中率最高**。

**修复**：`isCommonIgnoredDir` 清单补 `.github`（1 行改动）。

---

### 3.2 🔴 [P0] 非 Git 目录 —— 全部规则退化为"见啥拦啥"

`newGitContext()` 探测失败后 `isGitRepo=false`，此时：

```go
func (c *gitContext) isTracked(slashPath string) bool {
    if !c.isGitRepo {
        return false
    }
    ...
}

func (c *gitContext) isIgnored(slashPath string) bool {
    if !c.isGitRepo {
        return false
    }
    ...
}
```

`isTracked()` 恒 false、`isIgnored()` 恒 false，则所有形如 `tracked || !ignored` 的判定**恒为 true**。在未 `git init` 的目录里跑 `agate verify`：

| 裸目录中的文件 | 判定结果 |
|---|---|
| 根目录 `.env` | ERROR |
| 任何 `.gz` / `.zip` / `.exe`（如解压项目自带的资源） | ERROR |
| 任何 `*.bak` / `temp_*` | ERROR |

命令的 Long 描述声称"无任何工程描述文件时执行通用完备性扫描"，但在裸目录里反而拦得最狠，属于**逻辑倒挂**：越是缺少 Git 元数据的场景，越应该降级宽容，而不是收紧。

**修复**：`RunPreflightAudit` 入口处，`!isGitRepo` 时全部降级为 WARN（或直接跳过内容审计，仅保留纯文本卫生检查）。一个分支即可。

---

### 3.3 🟠 [P1] `testdata/` 不在剪枝清单 —— Go 测试夹具全灭

Go 生态在 `testdata/` 目录放置 `.tar.gz`、`.zip`、`.png` 解析测试夹具是**官方推荐模式**（`go test` 天然忽略该目录），且必然 tracked：

```text
testdata/sample.tar.gz    → [ERROR] 二进制文件拦截
testdata/golden.png (60KB) → [ERROR] 大文件图片拦截
```

解析器、压缩库、图像处理库项目 100% 命中。

**修复**：`isCommonIgnoredDir` 或二进制审计处豁免 `testdata/`。

---

### 3.4 🟠 [P1] `docs/` 与根目录图片 >50KB —— 文档配图拦截

图片大小检查**不豁免 `docs/`**（路径审计豁免了 docs，二进制/大文件审计没有）：

```text
docs/img/architecture.png  → [ERROR] 大文件图片拦截（架构图截屏普遍 200KB+）
logo.png（根目录，60KB）   → [ERROR] 大文件图片拦截
```

文档型仓库必中。

**修复建议**：图片阈值分档——

| 条件 | 建议级别 |
|---|---|
| tracked 且 < 512KB | 放行或 WARN |
| untracked 且未被 ignore（疑似临时图） | ERROR |
| 任何位置 ≥ 512KB | ERROR |

---

### 3.5 🟠 [P1] 注释/测试中的 Windows 路径 —— 误判"机器路径泄露"

路径泄露正则形如：

```go
(?i)[a-zA-Z]:[\\/](?:Users|hclaw|code_files|Software|AppData)[\\/][^\s...]+
```

配合仅有的两个行级豁免（`<YourUser>`、`示例`）。以下**合法内容**全部 ERROR：

```go
// 用法: agate scan D:\code_files\demo     ← 注释里的用法示例
path := `C:\Users\test\fixture.txt`        // parser_test.go 测试夹具
```

两个问题：

1. **`_test.go` 不在豁免范围**——测试夹具路径是硬编码路径的合理存在形态；
2. **正则把 `hclaw`、`code_files` 写死进规则**，这是对开发机本机路径的**过拟合**：换成 `D:\work\` 反而漏检（假阴性），而别人注释里的合法示例照拦。

**修复**：
- 审计前剥离注释行（至少 `//`、`#` 起始的行降为 WARN）；
- `*_test.go` / `testdata/` 豁免；
- 路径特征词列表外置到配置，而非硬编码本机目录名。

---

### 3.6 🟠 [P1] `debugger` 裸词匹配 —— 注释与合法标识符误伤

内容卫生正则：

```go
\b(?:debugger|breakpoint\(\)|pdb\.set_trace\(\))
```

不剥离注释、不限语句形态：

```go
// TODO: 接入 debugger 后再验证   ← 注释 → ERROR
debugger := attach()               ← 合法变量名 → ERROR
```

**修复**：剥离注释后再匹配；对 `debugger` 要求独立语句形态（行首至行尾仅 `debugger;` / `debugger`）。

---

### 3.7 🟡 [P2] `temp_` / `tmp_` 前缀误伤正常文件名

`tempPrefixes = ["temp_", "tmp_", ...]` 对**任意后缀**文件生效：

```text
temp_sensor.c   ← 嵌入式温度传感器驱动 → ERROR
tmp_utils.py    ← 临时工具库（团队命名习惯） → ERROR
```

**修复**：前缀规则收窄为「前缀 + 脚本类后缀（.sh/.py/.ps1/.js 等）」，或干脆只保留扩展名判定（`.tmp/.bak/.orig/.swp`），放弃前缀启发式。

---

### 3.8 🟡 [P2] 有意跟踪的 `TASK.md` 无法放行

- `.env` 检查只覆盖两个变体，`.env.development` / `.env.local` 等**漏检**（假阴性）；
- 反过来，`TASK.md` 若是团队**有意入库**的公共任务清单，没有任何机制豁免——规则隐含假设了"所有项目都按 agate 的私有化哲学组织文件"，这与开源协作现实冲突。

**修复**：治本方案仍是白名单配置（见第 6 节）。

---

## 4. 环境性误触发（内容完全合法也会拦）

这类问题不出在审计规则，而出在**脚本调度层**（`detectCustomScript()`）：

### 4.1 Windows 下有 `verify.sh` 但未装 bash

`detectCustomScript()` 在 Windows 上返回 `bash verify.sh`，若系统无 bash（Git Bash 未加入 PATH），`runShellCommand` 找不到可执行文件：

```text
[FAIL] 自检未通过，拒绝交付
```

**测试根本没跑就被判死**——这与 verify 对"未安装 mvn/npm/go 时平滑降级到框架探测"的设计自相矛盾：框架探测有降级链，自定义脚本调度却没有。

### 4.2 Unix 下 `./verify.sh` 丢失执行位

Windows 上提交的 shell 脚本在 clone 到 Unix 后没有 `+x` 位：

```text
exec: permission denied → [FAIL] 自检未通过
```

**修复（两条合一）**：
- Unix 也统一走 `bash verify.sh` 调度（不依赖执行位）；
- 调度失败（bash 不存在 / 脚本不可读）时打印明确警告并**降级到框架探测**，而非直接 FAIL。降级语义应与"找不到脚本"一致——"调度不了"和"没有脚本"对用户是同一件事。

---

## 5. 附带发现

### 5.1 `auditVendorCleanliness` 运算符优先级 bug

```go
strings.Contains(slashPath, "/site/") || strings.Contains(slashPath, "/doc/") && ext == ".md"
```

Go 中 `&&` 优先级高于 `||`，实际语义是：

```go
contains("/site/") || (contains("/doc/") && ext == ".md")
```

与代码注释意图（`/site/` 且 `.md`，或 `/doc/` 且 `.md`）不符。当前只产 WARN 不拦截，属**潜伏逻辑错误**——一旦有人把该类违规升级为 ERROR，误报面会瞬间扩大。

**修复**：补括号：`(contains("/site/") || contains("/doc/")) && ext == ".md"`。

### 5.2 假阴性问题（不拦截，但漏防，一并记录）

| 漏检项 | 说明 |
|---|---|
| `.env.development` / `.env.local` 等 | `.env` 变体只查两种 |
| 非 `Users/hclaw/code_files/Software/AppData` 开头的盘符路径 | 正则过拟合导致覆盖面窄 |
| 已 tracked 但后加入 ignore 的文件 | `isIgnored` 逻辑正确（tracked 优先），属设计预期 |

---

## 6. 修复建议与优先级路线图

| 优先级 | 修复项 | 改动位置 | 预估工作量 |
|---|---|---|---|
| **P0** | `isCommonIgnoredDir` 清单补 `.github` | `preflight.go` | 1 行 |
| **P0** | `!isGitRepo` 时全部降级 WARN 或跳过审计 | `RunPreflightAudit` 入口 | 1 个分支 |
| **P1** | 引入 `agate.toml` `[guard] allow = [...]` 白名单 + `verify --skip-guard` 应急旗标 | 新增配置解析 + `cmd/verify.go` | 中 |
| **P1** | 路径/断点/伪代码审计：剥离注释行、豁免 `*_test.go` 与 `testdata/` | 各审计函数加过滤 | 中 |
| **P1** | `temp_` 前缀规则收窄为「前缀 + 脚本后缀」，或仅保留扩展名判定 | `tempPrefixes` 逻辑 | 小 |
| **P1** | Unix 统一 `bash verify.sh` 调度；bash 缺失/脚本不可调度时降级框架探测 | `detectCustomScript` / `cmd/verify.go` | 小 |
| **P2** | 图片阈值分档（tracked <512KB → WARN） | `auditBinaryAndLargeFiles` | 小 |
| **P2** | `auditVendorCleanliness` 补括号修正优先级 | `preflight.go` | 1 行 |
| **P2** | `.env` 变体清单补全（`.env.*` 模式，排除 `.env.example`） | `.env` 检查 | 小 |

**P0 两项是"一行/一个分支"级别的改动**，能消掉命中率最高的两类拦截，建议立即合入。

**白名单机制是治本方案**。当前所有启发式规则的共性问题是：把"个人洁癖"当成了普适规范，且不给例外通道。一个 `[guard] allow = ["TASK.md", "docs/img/*.png"]` 风格的配置项，配合 `--skip-guard` 应急旗标，可以把"工具的规则"和"项目的现实"解耦。

---

## 7. 回归测试建议

修复时应同步建立防回归 fixture（当前 `pkg/guard` **零测试**，这本身就是 CODE_REVIEW 的 H2 项）。建议用 `t.TempDir()` + `git init` 构造最小仓库夹具，覆盖以下用例：

| # | 用例 | 期望 |
|---|---|---|
| 1 | `.github/assets/social-preview.png`（300KB，tracked） | 无 ERROR |
| 2 | 非 Git 目录 + 根下 `.env` / `.zip` | 无 ERROR（WARN 允许） |
| 3 | `testdata/sample.tar.gz`（tracked） | 无 ERROR |
| 4 | `docs/img/architecture.png`（200KB，tracked） | 无 ERROR 或仅 WARN |
| 5 | 注释行 `// see D:\code_files\demo` | 无 ERROR |
| 6 | `parser_test.go` 中 `` `C:\Users\test\fixture.txt` `` | 无 ERROR |
| 7 | 注释 `// TODO: 接入 debugger` / 变量 `debugger := attach()` | 无 ERROR |
| 8 | `temp_sensor.c` / `tmp_utils.py` | 无 ERROR |
| 9 | 白名单中的 `TASK.md` | 无 ERROR |
| 10 | `auditVendorCleanliness` 对 `site/foo.txt` 与 `doc/bar.md` | 与注释意图一致的分级 |
| 11 | Windows + `verify.sh` 存在 + 无 bash | 降级框架探测，不 FAIL |
| 12 | Unix + `verify.sh` 无执行位 | `bash verify.sh` 调度成功 |

---

## 8. 附录：误报快速自查表

如果你的仓库跑 `agate verify` 被 Phase 0 拦截，按此表对号入座：

```text
[ERROR] 大文件图片拦截
  ├─ 路径含 .github/ ................ 场景 3.1（P0，工具 bug）
  ├─ 路径含 testdata/ ............... 场景 3.3（P1，工具 bug）
  ├─ 路径在 docs/ 或根目录、>50KB ... 场景 3.4（P1，阈值一刀切）
  └─ 其他 ............................ 先确认是否真的该入库

[ERROR] 私有文件泄露（.env / TASK.md）
  ├─ TASK.md 是团队公共任务清单 ...... 场景 3.8（P2，无白名单）
  └─ .env 确属敏感 .................. 真阳性，按建议处理

[ERROR] 硬编码路径 / 机器路径泄露
  ├─ 内容在注释或 _test.go .......... 场景 3.5（P1，工具 bug）
  └─ 内容在业务代码 ................. 真阳性，按建议外置配置

[ERROR] 断点残留 / 伪代码
  ├─ 内容在注释或变量名 ............. 场景 3.6（P1，工具 bug）
  └─ 独立语句 debugger .............. 真阳性，删除后重试

[ERROR] 临时/草稿脚本文件
  ├─ 文件名以 temp_/tmp_ 开头但非临时 场景 3.7（P2，前缀过宽）
  └─ 确属遗留草稿 .................. 真阳性，删除后重试

[ERROR] 二进制文件拦截
  ├─ 在 testdata/ 或测试夹具 ........ 场景 3.3（P1）
  └─ 仓库根/源码目录 ................ 真阳性，用 LFS 或外置

[FAIL] 自检未通过（测试未跑就失败）
  ├─ Windows 且依赖 bash ............ 场景 4.1（环境性）
  └─ Unix 且脚本无 +x ............... 场景 4.2（环境性）
```

---

*报告完。如需直接落地 P0+P1 补丁（含 `--skip-guard` 旗标与白名单配置），见第 6 节路线图。*
