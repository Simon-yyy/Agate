package adapter

import (
	"fmt"
	"os"
	"path/filepath"

	"agate/internal/templates"
	"agate/pkg/harness"
)

// AgentTarget 目标 Agent 类型
type AgentTarget string

const (
	TargetAntigravity AgentTarget = "antigravity"
	TargetCursor      AgentTarget = "cursor"
	TargetClaude      AgentTarget = "claude"
	TargetWindsurf    AgentTarget = "windsurf"
)

// TargetMapping 目标文件映射关系
var TargetMapping = map[AgentTarget]string{
	TargetAntigravity: filepath.Join(".gemini", "GEMINI.md"),
	TargetCursor:      ".cursorrules",
	TargetClaude:      "CLAUDE.md",
	TargetWindsurf:    ".windsurfrules",
}

// DistributeRules 将规约内容分发至各 Agent 目标文件
func DistributeRules(customRules []byte, targets []AgentTarget) error {
	content := customRules
	if len(content) == 0 {
		content = templates.DefaultSkill
	}

	if len(targets) == 0 {
		targets = []AgentTarget{
			TargetAntigravity,
			TargetCursor,
			TargetClaude,
			TargetWindsurf,
		}
	}

	for _, target := range targets {
		destPath, ok := TargetMapping[target]
		if !ok {
			continue
		}

		if err := harness.WriteFileAtomic(destPath, content, 0644); err != nil {
			return fmt.Errorf("挂载 %s 规约失败 [%s]: %w", target, destPath, err)
		}
		fmt.Printf("  \033[92m[+] 已挂载 %-11s 规约 -> %s\033[0m\n", target, destPath)
	}
	return nil
}

// LoadGlobalRules 尝试加载用户全局规约，按顺序探测 ~/.agate/rules.md、~/.adh/rules.md、~/my_skills/SKILL.md 等，未找到则返回 nil
func LoadGlobalRules() ([]byte, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	candidatePaths := []string{
		filepath.Join(homeDir, ".agate", "rules.md"),
		filepath.Join(homeDir, ".adh", "rules.md"),
		filepath.Join(homeDir, "my_skills", "SKILL.md"),
		filepath.Join(homeDir, ".gemini", "config", "SKILL.md"),
	}

	for _, p := range candidatePaths {
		if _, err := os.Stat(p); err == nil {
			return os.ReadFile(p)
		}
	}
	return nil, nil
}
