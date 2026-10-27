package adapter

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	agentinfo "agate/pkg/agent"
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
var TargetMapping = func() map[AgentTarget]string {
	result := make(map[AgentTarget]string)
	for kind, profile := range agentinfo.BuiltinProfiles() {
		if kind != TargetCodex {
			result[kind] = filepath.FromSlash(profile.Path)
		}
	}
	return result
}()

// DetectExistingTargets 自动嗅探工作区已存在的 Agent 配置
func DetectExistingTargets(root string) []AgentTarget {
	var detected []AgentTarget

	// 探测 Cursor
	if _, err := os.Stat(filepath.Join(root, ".cursorrules")); err == nil {
		detected = append(detected, TargetCursor)
	} else if fi, err := os.Stat(filepath.Join(root, ".cursor", "rules")); err == nil && fi.IsDir() {
		detected = append(detected, TargetCursor)
	}

	// 探测 Antigravity
	if fi, err := os.Stat(filepath.Join(root, TargetMapping[TargetAntigravity])); err == nil && fi.Mode().IsRegular() {
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
	return ResolveTargetsWithProfiles(explicitTargets, root, detected, detectedOK, nil)
}

// ResolveTargetsWithProfiles 支持项目声明档案；无法识别且没有已有入口时要求显式选择。
func ResolveTargetsWithProfiles(explicitTargets []string, root string, detected AgentTarget, detectedOK bool, custom map[string]agentinfo.Profile) ([]AgentTarget, error) {
	profiles, err := agentinfo.Profiles(custom)
	if err != nil {
		return nil, err
	}
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
				continue
			}
			target := AgentTarget(clean)
			if parsed, ok := agentinfo.Parse(clean); ok {
				target = parsed
			}
			_, ok := profiles[target]
			if !ok {
				return nil, fmt.Errorf("未知 Agent 目标 %q；请显式选择已支持目标或在 .agate/config.toml 中声明工具档案", t)
			}
			if !seen[target] {
				seen[target] = true
				targets = append(targets, target)
			}
		}

		if hasAll {
			all := []AgentTarget{TargetCursor, TargetAntigravity, TargetClaude, TargetWindsurf, TargetCodex}
			names := make([]string, 0, len(custom))
			for name := range custom {
				names = append(names, name)
			}
			sort.Strings(names)
			for _, name := range names {
				all = append(all, AgentTarget(name))
			}
			return all, nil
		}

		if len(targets) > 0 {
			return targets, nil
		}
	}

	if detectedOK {
		if _, ok := profiles[detected]; !ok {
			return nil, fmt.Errorf("当前 Agent %q 没有可用档案，请显式指定 --targets", detected)
		}
		return []AgentTarget{detected}, nil
	}

	// 优先嗅探已有环境
	existing := DetectExistingTargets(root)
	if data, err := os.ReadFile(filepath.Join(root, "AGENTS.md")); err == nil && strings.Contains(string(data), codexRulesStart) {
		existing = append(existing, TargetCodex)
	}
	var customNames []string
	for name, profile := range custom {
		if info, err := os.Stat(filepath.Join(root, filepath.FromSlash(profile.Path))); err == nil && info.Mode().IsRegular() {
			customNames = append(customNames, name)
		}
	}
	sort.Strings(customNames)
	for _, name := range customNames {
		existing = append(existing, AgentTarget(name))
	}
	if len(existing) > 0 {
		return existing, nil
	}

	return nil, fmt.Errorf("未识别当前 Agent，也未发现已有规则入口；请通过 --targets 选择工具，未知工具需先声明档案")
}

// DistributeRules 将规约内容分发至各 Agent 目标文件
func DistributeRules(customRules []byte, targets []AgentTarget) error {
	if len(targets) == 0 {
		var err error
		targets, err = ResolveTargets(nil, ".")
		if err != nil {
			return err
		}
	}

	plan, err := BuildPlan(".", customRules, targets, nil, false)
	if err != nil {
		return err
	}
	return plan.Apply()
}

const (
	codexRulesStart = "<!-- agate:codex-rules:start -->"
	codexRulesEnd   = "<!-- agate:codex-rules:end -->"
)

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
