package harness

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"text/template"

	"agate/internal/templates"
)

type TemplateContext struct {
	ProjectName string
}

// EnsureIgnore 生成防爆仓检索过滤文件 .ignore
func EnsureIgnore() (bool, error) {
	if _, err := os.Stat(".ignore"); err == nil {
		return false, nil // 已存在，不覆盖
	}

	err := WriteFileAtomic(".ignore", templates.DefaultIgnore, 0644)
	if err != nil {
		return false, fmt.Errorf("生成 .ignore 失败: %w", err)
	}
	return true, nil
}

// EnsureMap 生成项目架构地图骨架 MAP.md。
func EnsureMap(projectName string) (bool, error) {
	if _, err := os.Stat("MAP.md"); err == nil {
		return false, nil // 已存在
	}

	tmpl, err := template.New("agents").Parse(string(templates.DefaultAgentsTpl))
	if err != nil {
		return false, fmt.Errorf("解析 agents.tpl 失败: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, TemplateContext{ProjectName: projectName}); err != nil {
		return false, fmt.Errorf("渲染 agents.tpl 失败: %w", err)
	}

	if err := WriteFileAtomic("MAP.md", buf.Bytes(), 0644); err != nil {
		return false, fmt.Errorf("写入 MAP.md 失败: %w", err)
	}
	return true, nil
}

// EnsureAgentsMap 保留兼容旧调用方；新代码应使用 EnsureMap。
func EnsureAgentsMap(projectName string) (bool, error) { return EnsureMap(projectName) }

// EnsureContext 生成工程技术上下文基线 contexts/context.md
func EnsureContext() (bool, error) {
	target := filepath.Join("contexts", "context.md")
	if _, err := os.Stat(target); err == nil {
		return false, nil
	}

	if err := WriteFileAtomic(target, templates.DefaultContextTpl, 0644); err != nil {
		return false, fmt.Errorf("写入 contexts/context.md 失败: %w", err)
	}
	return true, nil
}

// EnsureTaskBoard 生成协同任务看板 TASK.md
func EnsureTaskBoard() (bool, error) {
	if _, err := os.Stat("TASK.md"); err == nil {
		return false, nil // 已存在
	}

	if err := WriteFileAtomic("TASK.md", templates.DefaultTaskTpl, 0644); err != nil {
		return false, fmt.Errorf("写入 TASK.md 失败: %w", err)
	}
	return true, nil
}

// EnsureMemory 生成跨会话协同记忆 MEMORY.md
func EnsureMemory() (bool, error) {
	if _, err := os.Stat("MEMORY.md"); err == nil {
		return false, nil // 已存在
	}

	if err := WriteFileAtomic("MEMORY.md", templates.DefaultMemoryTpl, 0644); err != nil {
		return false, fmt.Errorf("写入 MEMORY.md 失败: %w", err)
	}
	return true, nil
}
