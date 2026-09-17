package adapter

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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

// TargetMapping 映射 Agent 至对应的规约文件物理路径
var TargetMapping = map[AgentTarget]string{
	TargetAntigravity: filepath.Join(".gemini", "GEMINI.md"),
	TargetCursor:      ".cursorrules",
	TargetClaude:      "CLAUDE.md",
	TargetWindsurf:    ".windsurfrules",
}

// DetectExistingTargets 自动嗅探工作区已存在的 Agent 配置
func DetectExistingTargets(root string) []AgentTarget {
	var detected []AgentTarget

	// 探测 Cursor
	if _, err := os.Stat(filepath.Join(root, ".cursorrules")); err == nil {
		detected = append(detected, TargetCursor)
	} else if fi, err := os.Stat(filepath.Join(root, ".cursor")); err == nil && fi.IsDir() {
		detected = append(detected, TargetCursor)
	}

	// 探测 Antigravity
	if fi, err := os.Stat(filepath.Join(root, ".gemini")); err == nil && fi.IsDir() {
		detected = append(detected, TargetAntigravity)
	}

	// 探测 Claude
	if _, err := os.Stat(filepath.Join(root, "CLAUDE.md")); err == nil {
		detected = append(detected, TargetClaude)
	}

	// 探测 Windsurf
	if _, err := os.Stat(filepath.Join(root, ".windsurfrules")); err == nil {
		detected = append(detected, TargetWindsurf)
	}

	return detected
}

// ResolveTargets 解析最终待挂载的目标 Agent 清单
func ResolveTargets(explicitTargets []string, root string) []AgentTarget {
	var targets []AgentTarget
	hasAll := false

	for _, t := range explicitTargets {
		lower := strings.ToLower(strings.TrimSpace(t))
		if lower == "all" {
			hasAll = true
			break
		}
		if lower != "" {
			targets = append(targets, AgentTarget(lower))
		}
	}

	if hasAll {
		return []AgentTarget{TargetCursor, TargetAntigravity, TargetClaude, TargetWindsurf}
	}

	// 若用户显式指定了 targets，按指定走
	if len(targets) > 0 {
		return targets
	}

	// 优先嗅探已有环境
	existing := DetectExistingTargets(root)
	if len(existing) > 0 {
		return existing
	}

	// 空白工程默认挂载最主流的两个工具，避免全量轰炸
	return []AgentTarget{TargetCursor, TargetAntigravity}
}

// DistributeRules 将规约内容分发至各 Agent 目标文件
func DistributeRules(customRules []byte, targets []AgentTarget) error {
	content := customRules
	if len(content) == 0 {
		content = templates.DefaultSkill
	}

	if len(targets) == 0 {
		targets = ResolveTargets(nil, ".")
	}

	for _, target := range targets {
		destPath, ok := TargetMapping[target]
		if !ok {
			continue
		}

		// 检查目标文件是否已存在既有规则
		if existing, err := os.ReadFile(destPath); err == nil {
			// 若内容已经完全相同，无需重复写入或备份
			if bytes.Equal(existing, content) {
				fmt.Printf("  \033[90m[-] %-11s 规约已最新，跳过 -> %s\033[0m\n", target, destPath)
				continue
			}
			// 若存在非空既有内容，执行安全备份保护
			if len(bytes.TrimSpace(existing)) > 0 {
				backupPath := destPath + ".agate.bak"
				if err := harness.CopyFile(destPath, backupPath); err != nil {
					return fmt.Errorf("备份已有规约文件失败 [%s -> %s]: %w", destPath, backupPath, err)
				}
				fmt.Printf("  \033[93m[!] 发现既有 %s 规约 [%s]，已自动安全备份至 %s\033[0m\n", target, destPath, backupPath)
			}
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
