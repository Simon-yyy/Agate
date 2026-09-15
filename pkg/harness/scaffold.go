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

// EnsureAgentsMap 生成架构地图骨架 AGENTS.md
func EnsureAgentsMap(projectName string) (bool, error) {
	if _, err := os.Stat("AGENTS.md"); err == nil {
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

	if err := WriteFileAtomic("AGENTS.md", buf.Bytes(), 0644); err != nil {
		return false, fmt.Errorf("写入 AGENTS.md 失败: %w", err)
	}
	return true, nil
}

// EnsureContext 生成工程技术上下文基线 contexts/context.md
func EnsureContext() (bool, error) {
	target := filepath.Join("contexts", "context.md")
	if _, err := os.Stat(target); err == nil {
		return false, nil
	}

	defaultContent := `# 项目工程上下文 (Operational Context)

## 一、 技术栈与运行环境
- **基础运行环境**: 运行环境与主要版本 (如 JDK 17 / Node.js 20 / Go 1.22)
- **核心框架**: 补充框架名称与关键组件版本

---

## 二、 核心业务流与规则基线
- 核心业务流程：请求路由 -> 权限/校验中间件 -> 业务处理 -> 持久化
- 异常与返回体规范：标准统一返回体格式与错误码映射
`

	if err := WriteFileAtomic(target, []byte(defaultContent), 0644); err != nil {
		return false, fmt.Errorf("写入 contexts/context.md 失败: %w", err)
	}
	return true, nil
}
