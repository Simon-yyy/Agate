package workspace

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"agate/pkg/agent"
	"agate/pkg/harness"
)

// WorkspaceFolder VSCode .code-workspace 目录项
type WorkspaceFolder struct {
	Path string `json:"path"`
}

// WorkspaceSettings VSCode .code-workspace 配置项
type WorkspaceSettings struct {
	FilesExclude map[string]bool `json:"files.exclude"`
}

// WorkspaceConfig VSCode .code-workspace 顶层结构
type WorkspaceConfig struct {
	Folders  []WorkspaceFolder `json:"folders"`
	Settings WorkspaceSettings `json:"settings"`
}

// TargetView 单个工具的独立视图计划
type TargetView struct {
	Target        string          `json:"target"`
	Supported     bool            `json:"supported"`
	SelfRulePath  string          `json:"self_rule_path"`
	WorkspacePath string          `json:"workspace_path"`
	HiddenPaths   []string        `json:"hidden_paths"`
	VisiblePaths  []string        `json:"visible_paths"`
	Reason        string          `json:"reason,omitempty"`
	Config        WorkspaceConfig `json:"config"`
}

// ViewPlan 独立视图工作区总体计划
type ViewPlan struct {
	RootDir string       `json:"root_dir"`
	Views   []TargetView `json:"views"`
}

var protectedPaths = map[string]bool{
	"MAP.md":              true,
	"contexts/context.md": true,
	"TASK.md":             true,
	"MEMORY.md":           true,
}

var builtinRulePaths = map[string]string{
	"codex":       "AGENTS.md",
	"antigravity": ".gemini/GEMINI.md",
	"cursor":      ".cursorrules",
	"claude":      "CLAUDE.md",
	"windsurf":    ".windsurfrules",
}

// NormalizeTarget 归一化工具名称
func NormalizeTarget(target string) string {
	t := strings.ToLower(strings.TrimSpace(target))
	switch t {
	case "gemini":
		return "antigravity"
	case "claude-code":
		return "claude"
	default:
		return t
	}
}

func isSafeTargetName(target string) bool {
	if target == "" || target == "." || target == ".." {
		return false
	}
	if strings.ContainsAny(target, `/\:*?"<>|`) || strings.Contains(target, "..") {
		return false
	}
	return true
}

// SupportsWorkspaceView 判定目标工具是否支持生成 GUI .code-workspace 独立视图
// cursor、antigravity（及 gemini）、windsurf 以及自定义档案支持生成 .code-workspace；
// codex、claude 为纯 CLI / 非 VSCode 工作区工具，明确标记为不支持 GUI .code-workspace 过滤。
func SupportsWorkspaceView(target string) bool {
	t := NormalizeTarget(target)
	if !isSafeTargetName(t) || t == "all" {
		return false
	}
	switch t {
	case "codex", "claude":
		return false
	case "cursor", "antigravity", "windsurf":
		return true
	default:
		return true
	}
}

func cleanRulePath(p string) string {
	s := filepath.ToSlash(strings.TrimSpace(p))
	s = strings.TrimPrefix(s, "./")
	s = strings.TrimPrefix(s, "/")
	return s
}

func isProtectedOrDirectoryPath(p string) bool {
	if p == "" {
		return true
	}
	if protectedPaths[p] {
		return true
	}
	// 严禁隐藏整个工具目录或核心目录
	switch strings.TrimSuffix(p, "/") {
	case ".", ".gemini", ".cursor", ".vscode", ".agate", ".ai-memory", "contexts":
		return true
	}
	return false
}

func mergeRulePaths(profiles map[agent.Kind]agent.Profile) map[string]string {
	merged := make(map[string]string, len(builtinRulePaths)+len(profiles))
	for k, v := range builtinRulePaths {
		merged[k] = cleanRulePath(v)
	}
	for k, prof := range profiles {
		name := NormalizeTarget(string(k))
		if !isSafeTargetName(name) {
			continue
		}
		if cleaned := cleanRulePath(prof.Path); cleaned != "" {
			merged[name] = cleaned
		}
	}
	return merged
}

func resolveRequestedTargets(targets []string, known map[string]string) ([]string, error) {
	var rawList []string
	for _, item := range targets {
		for _, part := range strings.Split(item, ",") {
			trimmed := strings.TrimSpace(part)
			if trimmed != "" {
				rawList = append(rawList, trimmed)
			}
		}
	}

	if len(rawList) == 0 {
		rawList = []string{"cursor", "antigravity", "windsurf"}
	}

	seen := make(map[string]bool)
	var resolved []string
	for _, raw := range rawList {
		t := NormalizeTarget(raw)
		if t == "all" {
			var allKeys []string
			for k := range known {
				allKeys = append(allKeys, k)
			}
			sort.Strings(allKeys)
			for _, k := range allKeys {
				if !seen[k] {
					seen[k] = true
					resolved = append(resolved, k)
				}
			}
			continue
		}
		if !isSafeTargetName(t) {
			return nil, fmt.Errorf("非法的目标工具名称 %q", raw)
		}
		if _, ok := known[t]; !ok {
			return nil, fmt.Errorf("未知的目标工具 %q (未在内建档案或自定义 profiles 中定义)", raw)
		}
		if !seen[t] {
			seen[t] = true
			resolved = append(resolved, t)
		}
	}
	return resolved, nil
}

// BuildViewPlan 计算各目标工具的私有 .code-workspace 视图过滤计划
func BuildViewPlan(root string, targets []string, profiles map[agent.Kind]agent.Profile) (*ViewPlan, error) {
	if strings.TrimSpace(root) == "" {
		root = "."
	}
	knownRules := mergeRulePaths(profiles)
	resolvedTargets, err := resolveRequestedTargets(targets, knownRules)
	if err != nil {
		return nil, err
	}

	plan := &ViewPlan{
		RootDir: root,
		Views:   make([]TargetView, 0, len(resolvedTargets)),
	}

	for _, target := range resolvedTargets {
		selfPath := knownRules[target]
		if !SupportsWorkspaceView(target) {
			plan.Views = append(plan.Views, TargetView{
				Target:       target,
				Supported:    false,
				SelfRulePath: selfPath,
				VisiblePaths: []string{selfPath, "MAP.md", "contexts/context.md", "TASK.md", "MEMORY.md"},
				Reason:       fmt.Sprintf("[%s] 为纯 CLI / 非 VSCode 工作区工具，不支持 GUI .code-workspace 视图过滤，已安全降级跳过", target),
			})
			continue
		}

		hiddenSet := make(map[string]bool)
		for otherTarget, otherPath := range knownRules {
			if otherTarget == target {
				continue
			}
			if otherPath == "" || otherPath == selfPath {
				continue
			}
			if isProtectedOrDirectoryPath(otherPath) {
				continue
			}
			hiddenSet[otherPath] = true
		}

		hiddenPaths := make([]string, 0, len(hiddenSet))
		excludeMap := make(map[string]bool, len(hiddenSet))
		for p := range hiddenSet {
			hiddenPaths = append(hiddenPaths, p)
			excludeMap[p] = true
		}
		sort.Strings(hiddenPaths)

		wsFile := filepath.Join(root, ".ai-memory", "workspaces", target+".code-workspace")
		plan.Views = append(plan.Views, TargetView{
			Target:        target,
			Supported:     true,
			SelfRulePath:  selfPath,
			WorkspacePath: wsFile,
			HiddenPaths:   hiddenPaths,
			VisiblePaths:  []string{selfPath, "MAP.md", "contexts/context.md", "TASK.md", "MEMORY.md"},
			Config: WorkspaceConfig{
				Folders: []WorkspaceFolder{
					{Path: "../.."},
				},
				Settings: WorkspaceSettings{
					FilesExclude: excludeMap,
				},
			},
		})
	}

	return plan, nil
}

// ApplyViewPlan 原子写入生成各工具独立的 .code-workspace 文件
func ApplyViewPlan(plan *ViewPlan) ([]string, error) {
	if plan == nil {
		return nil, fmt.Errorf("视图计划不能为空")
	}

	var written []string
	for _, view := range plan.Views {
		if !view.Supported || strings.TrimSpace(view.WorkspacePath) == "" {
			continue
		}
		dir := filepath.Dir(view.WorkspacePath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("创建工作区视图目录失败 [%s]: %w", dir, err)
		}
		data, err := json.MarshalIndent(view.Config, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("序列化工作区视图配置失败 [%s]: %w", view.Target, err)
		}
		data = append(data, '\n')
		if err := harness.WriteFileAtomic(view.WorkspacePath, data, 0644); err != nil {
			return nil, fmt.Errorf("写入工作区视图文件失败 [%s]: %w", view.WorkspacePath, err)
		}
		written = append(written, view.WorkspacePath)
	}
	return written, nil
}

// RestoreViews 删除对应或全部 .ai-memory/workspaces/*.code-workspace 视图文件，恢复原始展示
func RestoreViews(root string, targets []string) ([]string, error) {
	if strings.TrimSpace(root) == "" {
		root = "."
	}
	wsDir := filepath.Join(root, ".ai-memory", "workspaces")

	restoreAll := len(targets) == 0
	var normalized []string
	for _, item := range targets {
		for _, part := range strings.Split(item, ",") {
			t := NormalizeTarget(part)
			if t == "" {
				continue
			}
			if t == "all" {
				restoreAll = true
				break
			}
			if !isSafeTargetName(t) {
				return nil, fmt.Errorf("非法的目标工具名称 %q", part)
			}
			normalized = append(normalized, t)
		}
	}

	var removed []string
	if restoreAll {
		entries, err := os.ReadDir(wsDir)
		if err != nil {
			if os.IsNotExist(err) {
				return nil, nil
			}
			return nil, fmt.Errorf("读取工作区视图目录失败 [%s]: %w", wsDir, err)
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".code-workspace") {
				continue
			}
			fullPath := filepath.Join(wsDir, entry.Name())
			if err := os.Remove(fullPath); err != nil && !os.IsNotExist(err) {
				return nil, fmt.Errorf("删除工作区视图文件失败 [%s]: %w", fullPath, err)
			}
			removed = append(removed, fullPath)
		}
		sort.Strings(removed)
		return removed, nil
	}

	seen := make(map[string]bool)
	for _, t := range normalized {
		if seen[t] {
			continue
		}
		seen[t] = true
		fullPath := filepath.Join(wsDir, t+".code-workspace")
		if _, err := os.Stat(fullPath); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("检查工作区视图文件失败 [%s]: %w", fullPath, err)
		}
		if err := os.Remove(fullPath); err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("删除工作区视图文件失败 [%s]: %w", fullPath, err)
		}
		removed = append(removed, fullPath)
	}
	sort.Strings(removed)
	return removed, nil
}

// BuildOpenCommand 为支持 GUI 的工具构造直接打开 workspaceFile 的命令（绝不经过 cmd.exe /c 注入）
func BuildOpenCommand(goos string, target string, workspaceFile string) (*exec.Cmd, error) {
	t := NormalizeTarget(target)
	if !isSafeTargetName(t) {
		return nil, fmt.Errorf("非法的目标工具名称 %q", target)
	}
	if strings.TrimSpace(workspaceFile) == "" {
		return nil, fmt.Errorf("工作区视图文件路径不能为空")
	}
	if !SupportsWorkspaceView(t) {
		return nil, fmt.Errorf("[%s] 为纯 CLI / 非 VSCode 工作区工具，不支持 GUI .code-workspace 独立视图打开，请直接在终端使用对应 CLI", t)
	}

	var bin string
	switch t {
	case "cursor":
		bin = "cursor"
	case "windsurf":
		bin = "windsurf"
	case "antigravity":
		bin = "antigravity"
	default:
		return nil, fmt.Errorf("目标 [%s] 未绑定内建 GUI 启动器命令，请在客户端中通过 'Open Workspace from File...' 手动打开: %s", t, workspaceFile)
	}

	_ = goos
	return exec.Command(bin, workspaceFile), nil
}
