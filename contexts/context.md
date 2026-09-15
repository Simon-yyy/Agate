# 领域上下文地图 (Context Map)

## 1. 核心业务与领域模型
- **单一事实源 (SSOT)**: 核心规约由 `internal/templates/SKILL.md` 固化，通过 `pkg/adapter` 分发映射至各大 Agent 工具配置文件。
- **隐形隔离 (Invisible Isolation)**: AI 私有状态（`.agents/`, `.gemini/`, `.cursorrules`, `TASK.md`, `MEMORY.md` 等）仅注册在本地 `.git/info/exclude`，严禁污染团队公共 `.gitignore` 与 Git 提交树。
- **闭环自检与两振熔断**: 代码变更后强制执行 `agate verify`（或 `agent-verify`），两振未果强制退出交还主控权。

## 2. 关键 API 与环境约定
- **CLI 命令契约**:
  - `agate init`: 初始化项目 AI 研发护栏与地图资产；
  - `agate isolate`: 注入本地私有排除规则；
  - `agate verify`: 执行本地多级自检闭环，退出码严格约定：`0` 为 PASS，`1` 为 FAIL；
  - `agate scan [--write]`: 智能扫描工程技术栈与端口矩阵，支持回填 AGENTS.md；
  - `agate hook [install|uninstall]`: 管理本地 Git pre-commit 钩子。
- **自举验证契约**:
  - 项目根目录具备 `verify.cmd` / `verify.sh`，优先保障本地验证开箱即走。
- **跨平台兼容**:
  - Windows 环境优先尝试软链接，权限受限时优雅降级为物理拷贝（`CopyFile`）；
  - 换行符统一使用 LF，防止跨平台 Shell 语法解析错误。

## 3. 动作外溢与显式授权公理 (Explicit Mandate Principle)
- **动作二分基线**：
  - **本地闭环动作**（代码编辑、本地编译、单测运行与自检）：常规任务及方案确认后准予闭环探索；
  - **外溢与跃迁动作**（Git 提交/推送、Release 上传、包发布、部署推流、破坏性变更）：**默认物理死锁**。
- **授权铁律（指哪打哪）**：
  - 严禁以“顺带闭环”为由代跑任何外溢动作；
  - 必须且仅当用户指令中出现明确指向该外溢动作的专属动词（如“提交代码”、“上传Release”、“发布”）时，对应操作方可单次解锁代跑。



