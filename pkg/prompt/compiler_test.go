package prompt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCompilePrompt_EmptyInstruction(t *testing.T) {
	_, err := CompilePrompt(CompileOptions{
		Instruction: "   ",
	})
	if err == nil {
		t.Fatalf("期望空指令报错，但没有返回错误")
	}
}

func TestCompilePrompt_OralInstructionWithWorkspace(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "agate-prompt-test-*")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(tempDir)

	mapContent := `# 项目模块地图 (MAP.md)

## 一、 项目架构与工程定位
测试项目定位说明。

## 三、 模块拓扑与职责分工
### 1. 命令行交互层 (cmd/)
CLI 交互入口与命令分发。
`
	if err := os.WriteFile(filepath.Join(tempDir, "MAP.md"), []byte(mapContent), 0644); err != nil {
		t.Fatalf("写入 MAP.md 失败: %v", err)
	}

	res, err := CompilePrompt(CompileOptions{
		RootDir:     tempDir,
		Instruction: "帮我修改一下代码",
	})
	if err != nil {
		t.Fatalf("CompilePrompt 失败: %v", err)
	}

	if res.RawInstruction != "帮我修改一下代码" {
		t.Errorf("原始指令不匹配: %s", res.RawInstruction)
	}
	if !strings.Contains(res.TargetDescription, "未在指令中显式指明") && !strings.Contains(res.TargetDescription, "未指定具体文件") {
		t.Errorf("目标定位未识别出口语化缺省状态: %s", res.TargetDescription)
	}
	if !strings.Contains(res.CompiledPrompt, "1. 原始指令:") {
		t.Errorf("CompiledPrompt 未包含对齐后的四要素编号 1. 原始指令")
	}
}

func TestCompilePrompt_PathSecurityAndFalsePositiveRejection(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "agate-prompt-test-*")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// 测试：指令包含 URL、路径穿越、中文词对斜杠
	instruction := "请排查 http://127.0.0.1:8080/api 和 ../../etc/passwd 以及 输入/输出 逻辑"
	res, err := CompilePrompt(CompileOptions{
		RootDir:     tempDir,
		Instruction: instruction,
	})
	if err != nil {
		t.Fatalf("CompilePrompt 失败: %v", err)
	}

	// 验证：URL、越界路径和纯中文词对绝不能被当作 matched_files
	for _, f := range res.MatchedFiles {
		if strings.Contains(f, "http") || strings.Contains(f, "passwd") || strings.Contains(f, "输入") {
			t.Errorf("安全漏洞或误判：不应将非法或非代码词对识别为匹配文件: %s", f)
		}
	}
}

func TestCompilePrompt_TaskStatusAndQuartetExtraction(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "agate-prompt-test-*")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// 1. 测试处于 [DONE] 终态的任务不应被标为活跃
	doneContent := `---
task_id: "T-DONE"
title: "历史已完成任务"
status: "DONE"
---
# 正文
`
	taskFile := filepath.Join(tempDir, "TASK.md")
	if err := os.WriteFile(taskFile, []byte(doneContent), 0644); err != nil {
		t.Fatalf("写入 TASK.md 失败: %v", err)
	}

	resDone, err := CompilePrompt(CompileOptions{
		RootDir:     tempDir,
		Instruction: "开始下一阶段开发",
	})
	if err != nil {
		t.Fatalf("CompilePrompt 失败: %v", err)
	}
	if resDone.TaskActive {
		t.Errorf("[DONE] 终态任务不应被标为 TaskActive = true")
	}

	// 2. 测试活跃任务及其接力四要素断点提取
	activeContent := `---
task_id: "T-002"
title: "重构网络连接池"
status: "IN_PROGRESS"
current_role: "dev"
next_role: "test"
---
## 一、 任务目标与背景
实现连接复用。

## 二、 接力四要素（跨 Agent 核心资产）
- **在途断点与未决问题**: 已完成握手协议，待实现心跳保活
- **避坑警示与环境暗坑**: Windows 环境下需显式回收句柄
- **下一步执行建议**: 编写心跳单测并验证
- **交付物证与验证记录**: 待自检
`
	if err := os.WriteFile(taskFile, []byte(activeContent), 0644); err != nil {
		t.Fatalf("更新 TASK.md 失败: %v", err)
	}

	resActive, err := CompilePrompt(CompileOptions{
		RootDir:     tempDir,
		Instruction: "继续编写连接池代码",
	})
	if err != nil {
		t.Fatalf("CompilePrompt 失败: %v", err)
	}

	if !resActive.TaskActive || resActive.TaskTitle != "重构网络连接池" {
		t.Errorf("活跃任务未正确识别: active=%v, title=%s", resActive.TaskActive, resActive.TaskTitle)
	}
	if !strings.Contains(resActive.TaskBreakpoint, "待实现心跳保活") {
		t.Errorf("未正确提取四要素断点: %s", resActive.TaskBreakpoint)
	}
	if !strings.Contains(resActive.TaskNextStep, "编写心跳单测") {
		t.Errorf("未正确提取四要素下一步建议: %s", resActive.TaskNextStep)
	}
}

func TestCompilePrompt_MemoryEmptyScopeRejectedAndMultiScopeMatched(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "agate-prompt-test-*")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(tempDir)

	memContent := `# 跨会话协同记忆 (MEMORY.md)

### [ID: No-Scope-Section]
- **角色**: Diagnostic
未填写生效范围的普通标题

### [ID: Auth-Guard]
- **角色**: Constraint
- **生效范围**：` + "`" + `pkg/auth/*` + "`" + `, ` + "`" + `cmd/auth/*` + "`" + `
- **核心结论**: 严禁明文存储凭证。

### [ID: DB-Guard]
- **角色**: Constraint
- **生效范围**: pkg/db/*
- **核心结论**: 必须配置连接超时。
`
	if err := os.WriteFile(filepath.Join(tempDir, "MEMORY.md"), []byte(memContent), 0644); err != nil {
		t.Fatalf("写入 MEMORY.md 失败: %v", err)
	}

	res, err := CompilePrompt(CompileOptions{
		RootDir:     tempDir,
		Instruction: "修改 pkg/auth/token.go",
	})
	if err != nil {
		t.Fatalf("CompilePrompt 失败: %v", err)
	}

	for _, card := range res.MemoryCards {
		if strings.Contains(card, "No-Scope-Section") {
			t.Errorf("红线违约：缺少生效范围的卡片绝不应被准入: %s", card)
		}
		if strings.Contains(card, "DB-Guard") {
			t.Errorf("红线违约：无关范围的卡片绝不应被注入: %s", card)
		}
	}

	foundAuth := false
	for _, card := range res.MemoryCards {
		if strings.Contains(card, "Auth-Guard") {
			foundAuth = true
		}
	}
	if !foundAuth {
		t.Errorf("应该命中支持多生效范围且含中文冒号的 Auth-Guard 卡片")
	}
}

func TestCompilePrompt_PreciseSubpackageSliceAndMultilineQuote(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "agate-prompt-test-*")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(tempDir)

	mapContent := `# 项目模块地图 (MAP.md)

## 三、 模块拓扑与职责分工
### 1. 规约适配与分发 (pkg/adapter/)
规约分发器。

### 2. 提示词物理锚定与编译器 (pkg/prompt/)
提示词编译器。
`
	if err := os.WriteFile(filepath.Join(tempDir, "MAP.md"), []byte(mapContent), 0644); err != nil {
		t.Fatalf("写入 MAP.md 失败: %v", err)
	}

	res, err := CompilePrompt(CompileOptions{
		RootDir:     tempDir,
		Instruction: "修改 pkg/prompt/compiler.go\n处理 \"引号\" 换行",
	})
	if err != nil {
		t.Fatalf("CompilePrompt 失败: %v", err)
	}

	// 验证不会泛化切出 pkg/adapter/
	if len(res.MapExcerpts) != 1 || !strings.Contains(res.MapExcerpts[0], "pkg/prompt/") {
		t.Fatalf("应仅精准切出 pkg/prompt/ 章节，实际切出: %#v", res.MapExcerpts)
	}
	for _, m := range res.MatchedModules {
		if m == "pkg" {
			t.Fatalf("不应保留泛化顶层容器模块 pkg: %#v", res.MatchedModules)
		}
	}
	// 验证多行与引号转义
	if !strings.Contains(res.CompiledPrompt, `1. 原始指令: "修改 pkg/prompt/compiler.go 处理 \"引号\" 换行"`) {
		t.Fatalf("多行带引号指令未正确转义:\n%s", res.CompiledPrompt)
	}
	if res.AnchorConfidence != ConfidenceExplicit {
		t.Fatalf("明确路径指令预期置信度为 explicit，实际为: %s", res.AnchorConfidence)
	}
}

func TestCompilePrompt_ExplicitScopesAndIgnoreDirtyAndCorrectionProtocol(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "agate-prompt-scope-test-*")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(tempDir)

	mapContent := `# 项目模块地图 (MAP.md)

## 三、 模块拓扑与职责分工
### 1. 跨 Agent 任务接力与防撞车锁 (pkg/relay/)
任务租约锁与状态机。

### 2. 规约适配与分发 (pkg/adapter/)
规约分发器。
`
	if err := os.WriteFile(filepath.Join(tempDir, "MAP.md"), []byte(mapContent), 0644); err != nil {
		t.Fatalf("写入 MAP.md 失败: %v", err)
	}

	// 1. 测试通过 Scopes 显式纠偏与重复去重（即使指令口语模糊也能精确锁定 pkg/relay/lock.go）
	resScope, err := CompilePrompt(CompileOptions{
		RootDir:     tempDir,
		Instruction: "帮我改下超时逻辑",
		Scopes:      []string{"pkg/relay/lock.go", "pkg/relay/lock.go"},
		IgnoreDirty: true,
	})
	if err != nil {
		t.Fatalf("带 Scopes 的 CompilePrompt 失败: %v", err)
	}
	if len(resScope.ExplicitScopes) != 1 {
		t.Errorf("重复 Scopes 应去重为 1 项，实际为: %#v", resScope.ExplicitScopes)
	}
	if resScope.AnchorConfidence != ConfidenceExplicit {
		t.Errorf("显式 Scopes 预期置信度为 explicit，实际为: %s", resScope.AnchorConfidence)
	}
	if !strings.Contains(resScope.TargetDescription, "[显式纠偏锚定]") || !strings.Contains(resScope.TargetDescription, "pkg/relay/lock.go") {
		t.Errorf("TargetDescription 未包含显式纠偏锚定信息: %s", resScope.TargetDescription)
	}
	if len(resScope.MapExcerpts) != 1 || !strings.Contains(resScope.MapExcerpts[0], "pkg/relay/") {
		t.Errorf("显式 Scopes 应精准切出 pkg/relay/ 地图切片，实际为: %#v", resScope.MapExcerpts)
	}
	if !strings.Contains(resScope.CompiledPrompt, "局部纠偏与重锚定闭环 (Correction Loop)") {
		t.Errorf("CompiledPrompt 缺少局部纠偏与重锚定闭环协议")
	}
	if !strings.Contains(resScope.CompiledPrompt, "收敛模式：仅限核对直接调用方签名与对应单测") {
		t.Errorf("显式 explicit 模式下应包含防画蛇添足收敛提示")
	}

	// 2. 测试非法越界 Scopes 与孤立容器目录 pkg/internal 被拒绝
	_, err = CompilePrompt(CompileOptions{
		RootDir:     tempDir,
		Instruction: "帮我改下代码",
		Scopes:      []string{"../../etc/passwd"},
	})
	if err == nil {
		t.Fatalf("非法越界 --scope 预期返回错误，实际未报错")
	}
	_, err = CompilePrompt(CompileOptions{
		RootDir:     tempDir,
		Instruction: "帮我改下代码",
		Scopes:      []string{"pkg"},
	})
	if err == nil {
		t.Fatalf("孤立容器目录 --scope pkg 预期返回错误，实际未报错")
	}

	// 3. 测试 IgnoreDirty 模式下未指定路径时的 unresolved 状态
	resNoDirty, err := CompilePrompt(CompileOptions{
		RootDir:     tempDir,
		Instruction: "帮我改下代码",
		IgnoreDirty: true,
	})
	if err != nil {
		t.Fatalf("IgnoreDirty CompilePrompt 失败: %v", err)
	}
	if resNoDirty.AnchorConfidence != ConfidenceUnresolved {
		t.Errorf("IgnoreDirty + 模糊指令预期置信度为 unresolved，实际为: %s", resNoDirty.AnchorConfidence)
	}
	if !strings.Contains(resNoDirty.TargetDescription, "--no-dirty") {
		t.Errorf("IgnoreDirty 模式描述未体现 --no-dirty: %s", resNoDirty.TargetDescription)
	}
}
