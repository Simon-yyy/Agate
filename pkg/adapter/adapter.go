package adapter

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"agate/internal/templates"
	agentinfo "agate/pkg/agent"
	"agate/pkg/harness"
)

// AgentTarget 目标 Agent 类型
type AgentTarget = agentinfo.Kind

const (
	TargetCodex       AgentTarget = agentinfo.Codex
	TargetAntigravity AgentTarget = agentinfo.Antigravity
	TargetCursor      AgentTarget = agentinfo.Cursor
	TargetClaude      AgentTarget = agentinfo.Claude
	TargetWindsurf    AgentTarget = agentinfo.Windsurf
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

// SupportedTargets 定义所有合法受支持的 Agent 目标类型与规范化映射
var SupportedTargets = map[string]AgentTarget{
	"cursor":      TargetCursor,
	"antigravity": TargetAntigravity,
	"gemini":      TargetAntigravity,
	"claude":      TargetClaude,
	"windsurf":    TargetWindsurf,
	"codex":       TargetCodex,
}

// ResolveTargets 解析最终待挂载的目标 Agent 清单（包含合法性校验与去重）
func ResolveTargets(explicitTargets []string, root string) ([]AgentTarget, error) {
	return ResolveTargetsForEnvironment(explicitTargets, root, "", false)
}

// ResolveTargetsForEnvironment 按“显式参数 > 当前 Agent > 项目文件 > 默认值”解析目标。
// detectedOK 为 false 时不使用 detected，便于调用方在无法识别环境时自然回退。
func ResolveTargetsForEnvironment(explicitTargets []string, root string, detected AgentTarget, detectedOK bool) ([]AgentTarget, error) {
	if len(explicitTargets) > 0 {
		hasAll := false
		seen := make(map[AgentTarget]bool)
		var targets []AgentTarget

		for _, t := range explicitTargets {
			clean := strings.ToLower(strings.TrimSpace(t))
			if clean == "" {
				continue
			}
			if clean == "all" {
				hasAll = true
				break
			}
			target, ok := SupportedTargets[clean]
			if !ok {
				return nil, fmt.Errorf("不支持的 Agent 目标: '%s' (有效选项: cursor, antigravity, claude, windsurf, codex, all)", t)
			}
			if !seen[target] {
				seen[target] = true
				targets = append(targets, target)
			}
		}

		if hasAll {
			return []AgentTarget{TargetCursor, TargetAntigravity, TargetClaude, TargetWindsurf, TargetCodex}, nil
		}

		if len(targets) > 0 {
			return targets, nil
		}
	}

	if detectedOK {
		return []AgentTarget{detected}, nil
	}

	// 优先嗅探已有环境
	existing := DetectExistingTargets(root)
	if len(existing) > 0 {
		return existing, nil
	}

	// 空白工程默认挂载最主流的两个工具，避免全量轰炸
	return []AgentTarget{TargetCursor, TargetAntigravity}, nil
}

// DistributeRules 将规约内容分发至各 Agent 目标文件
func DistributeRules(customRules []byte, targets []AgentTarget) error {
	content := customRules
	if len(content) == 0 {
		content = templates.DefaultSkill
	}

	if len(targets) == 0 {
		var err error
		targets, err = ResolveTargets(nil, ".")
		if err != nil {
			return err
		}
	}

	for _, target := range targets {
		if target == TargetCodex {
			if err := updateCodexRules("AGENTS.md", content); err != nil {
				return err
			}
			fmt.Printf("  \033[92m[+] 已更新 %-11s 规约受管区块 -> AGENTS.md\033[0m\n", target)
			continue
		}

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

const (
	codexRulesStart = "<!-- agate:codex-rules:start -->"
	codexRulesEnd   = "<!-- agate:codex-rules:end -->"
)

// updateCodexRules 仅更新 AGENTS.md 中 Agate 受管区块，架构地图位于 MAP.md。
func updateCodexRules(path string, rules []byte) error {
	existingBytes, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("读取 Codex 指令文件失败 [%s]: %w", path, err)
	}
	existing := string(existingBytes)
	newline := "\n"
	if strings.Contains(existing, "\r\n") {
		newline = "\r\n"
	}
	body := strings.TrimSpace(string(rules))
	block := codexRulesStart + newline + "## Agate Codex Guardrails" + newline + body + newline + codexRulesEnd

	start := strings.Index(existing, codexRulesStart)
	end := strings.Index(existing, codexRulesEnd)
	var updated string
	switch {
	case start >= 0 && end > start:
		end += len(codexRulesEnd)
		updated = existing[:start] + block + existing[end:]
	case start >= 0 || end >= 0:
		return fmt.Errorf("Codex 指令受管区块标记不完整 [%s]", path)
	case strings.TrimSpace(existing) == "":
		updated = block + newline
	default:
		updated = strings.TrimRight(existing, "\r\n") + newline + newline + block + newline
	}

	if updated == existing {
		return nil
	}
	if err := harness.WriteFileAtomic(path, []byte(updated), 0644); err != nil {
		return fmt.Errorf("写入 Codex 指令文件失败 [%s]: %w", path, err)
	}
	return nil
}

// LoadGlobalRules 尝试加载用户全局规约，返回规约内容、实际匹配的来源物理路径以及错误
func LoadGlobalRules() ([]byte, string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, "", fmt.Errorf("获取用户家目录失败: %w", err)
	}
	return loadGlobalRulesFrom(homeDir)
}

// loadGlobalRulesFrom 从显式家目录加载全局规约，便于跨平台测试隔离。
func loadGlobalRulesFrom(homeDir string) ([]byte, string, error) {
	candidatePaths := []string{
		filepath.Join(homeDir, ".agate", "rules.md"),
		filepath.Join(homeDir, ".config", "agate", "rules.md"),
		filepath.Join(homeDir, "my_skills", "SKILL.md"),
		filepath.Join(homeDir, ".gemini", "config", "SKILL.md"),
	}

	for _, p := range candidatePaths {
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			content, readErr := os.ReadFile(p)
			if readErr != nil {
				return nil, p, fmt.Errorf("发现全局规约文件 [%s] 但读取失败: %w", p, readErr)
			}
			return content, p, nil
		}
	}
	return nil, "", nil
}

// CheckDeprecatedRulePatterns 检查规约内容中是否残留了老版本废弃标识
func CheckDeprecatedRulePatterns(content []byte) []string {
	var warnings []string
	text := string(content)
	if strings.Contains(text, "ai-dev-harness") {
		warnings = append(warnings, "包含旧品牌命名 'ai-dev-harness'")
	}
	if strings.Contains(text, "AGENTS.md (架构地图)") || strings.Contains(text, "AGENTS.md 与 context.md") {
		warnings = append(warnings, "寻路规约仍指向旧版 'AGENTS.md' 架构地图而非 'MAP.md'")
	}
	return warnings
}

