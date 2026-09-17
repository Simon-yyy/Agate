# Codex 项目规约入口

本文件仅承载 Codex 规约。项目架构地图请阅读 [MAP.md](MAP.md)，技术契约请阅读 [contexts/context.md](contexts/context.md)。处理项目任务时，必须先按此顺序读取地图和契约，再进入目标源码。

<!-- agate:codex-rules:start -->
## Agate Codex Guardrails

﻿# Agate (Agent Gate) 核心协同与安全规范

## 1. 中文原生与代码完整性

- 交互分析、排查思路、代码注释与 Git Commit Message 必须使用规范简体中文；技术术语与标识符保留原样。
- 改动处必须完整可运行，严禁偷懒使用 `// ... 保持不变` 等省略占位符。

## 2. 两阶段硬门禁 (Two-Phase Gate)

- 遇到任何排查、探查或业务修改任务，首轮仅输出根因分析、精简方案与拟改动文件清单，严禁直接改动代码。
- 必须停步显式提示：“👉 请确认方案，确认请回复‘执行’”，严格等待用户明确回复后方可在下一轮落盘。
- 豁免项：收到“完善/初始化 MAP.md 与 contexts/context.md”指令时，按冷启动协议直接扫描配置并填充落盘。

## 3. 寻路规范 (3-Hop Navigation)

- 寻路顺序严格遵循：MAP.md (架构地图) -> contexts/context.md (技术契约) -> 目标源码。严禁全仓盲扫。
- 文档泛化维护指令仅限变更此两份地图文件，严禁篡改业务自身的 README.md。

## 4. 闭环验证与两振熔断

- 代码落盘后，必须主动在终端调用 agent-verify 自检，拿到绿色 PASS 物证后方可交付。
- 同一错误若连续修复 2 次未果，必须强制熔断停止盲试，交还主控权。

## 5. 绝对安全红线

- 严禁擅自执行任何写状态 Git 命令（git add/commit/push 提交权 100% 归用户）。
- 严禁在代码或回复中回显任何 API Key、密码与私钥。
<!-- agate:codex-rules:end -->
