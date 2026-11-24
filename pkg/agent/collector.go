package agent

import (
	"fmt"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// ProcessInfo 封装单个进程的基础元数据（严格不包含完整命令行）。
type ProcessInfo struct {
	PID  int    `json:"pid"`
	PPID int    `json:"ppid"`
	Path string `json:"path"`
	Name string `json:"name"`
}

// ProcessInspector 抽象祖先进程检查器接口，方便在单测中依赖注入。
type ProcessInspector interface {
	GetAncestors(pid int, maxDepth int) ([]ProcessInfo, error)
}

// DefaultProcessInspector 系统默认进程探测器。具体采集按平台编译。
type DefaultProcessInspector struct{}

const maxAncestorDepth = 8

// defaultAncestorDepth 按平台选择默认采集深度。Windows 覆盖更深的本地宿主链，Linux 保持原有 4 层。
func defaultAncestorDepth() int {
	if runtime.GOOS == "windows" {
		return maxAncestorDepth
	}
	return 4
}

// normalizeAncestorDepth 把非正深度换成平台默认值，并执行硬上限。
func normalizeAncestorDepth(maxDepth int) int {
	if maxDepth <= 0 {
		maxDepth = defaultAncestorDepth()
	}
	if maxDepth > maxAncestorDepth {
		maxDepth = maxAncestorDepth
	}
	return maxDepth
}

// matchLocalEditorProcess 按已确认的本地可执行文件名识别 Editor。
// 只接受当前祖先进程上的完整路径，目录名或近似文件名不算命中。
func matchLocalEditorProcess(info ProcessInfo) (string, bool) {
	return matchLocalEditorProcessWithProfiles(info, nil)
}

func matchLocalEditorProcessWithProfiles(info ProcessInfo, custom map[string]Profile) (string, bool) {
	if strings.TrimSpace(info.Path) == "" {
		return "", false
	}
	normalized := strings.ReplaceAll(info.Path, "\\", "/")
	base := strings.ToLower(path.Base(normalized))
	switch base {
	case "cursor.exe":
		return "cursor", true
	case "code.exe", "code - insiders.exe":
		return "vscode", true
	case "antigravity ide.exe", "antigravity.exe":
		return "antigravity", true
	case "windsurf.exe":
		return "windsurf", true
	case "trae.exe":
		return "trae", true
	case "kiro.exe":
		return "kiro", true
	}
	if len(custom) > 0 {
		names := make([]string, 0, len(custom))
		for name := range custom {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			p := custom[name]
			for _, pn := range p.ProcessNames {
				if strings.ToLower(strings.TrimSpace(pn)) == base {
					return name, true
				}
			}
		}
	}
	return "", false
}

// PathWhitelistRule 远程白名单路径特征规则。
type PathWhitelistRule struct {
	Agent       string // "cursor" 或 "vscode"
	Role        string // "editor"
	Pattern     string // 归一化后必须匹配的子路径片段
	Description string // 说明摘要
}

var defaultPathRules = []PathWhitelistRule{
	{
		Agent:       "cursor",
		Role:        "editor",
		Pattern:     "/.cursor-server/",
		Description: "命中 Cursor 远程服务端白名单目录片段 (/.cursor-server/)",
	},
	{
		Agent:       "vscode",
		Role:        "editor",
		Pattern:     "/.vscode-server/",
		Description: "命中 VS Code 远程服务端对照目录片段 (/.vscode-server/)",
	},
}

// normalizePathForMatch 跨平台受限归一化路径，统统转换为正斜杠并小写，不依赖宿主操作系统。
func normalizePathForMatch(rawPath string) string {
	if rawPath == "" {
		return ""
	}
	s := strings.ReplaceAll(rawPath, "\\", "/")
	return strings.ToLower(s)
}

// MatchPathWhitelist 匹配路径是否命中远程服务端的白名单特征。
func MatchPathWhitelist(rawPath string) (rule PathWhitelistRule, matched bool) {
	if rawPath == "" {
		return PathWhitelistRule{}, false
	}
	normalized := normalizePathForMatch(rawPath)
	for _, r := range defaultPathRules {
		if strings.Contains(normalized, r.Pattern) {
			return r, true
		}
	}
	return PathWhitelistRule{}, false
}

// splitPathList 跨平台拆分 PATH 环境变量中的各个独立路径条目。
func splitPathList(pathVal string) []string {
	if pathVal == "" {
		return nil
	}
	// 兼容分号 (Windows) 与冒号 (Unix/Linux)
	var rawItems []string
	if strings.Contains(pathVal, ";") {
		rawItems = strings.Split(pathVal, ";")
	} else {
		rawItems = strings.Split(pathVal, ":")
	}
	var res []string
	for _, item := range rawItems {
		trimmed := strings.TrimSpace(item)
		if trimmed != "" {
			res = append(res, trimmed)
		}
	}
	return res
}

// InspectPathEnvironments 检查关键环境变量中的白名单路径特征，逐项收集全部候选。
func InspectPathEnvironments(getenv func(string) string) []Evidence {
	if getenv == nil {
		return nil
	}

	var evidences []Evidence
	seenRules := make(map[string]bool)

	// 1. 拆分 PATH 逐项扫描，杜绝首个命中短路
	if pathVal := getenv("PATH"); pathVal != "" {
		items := splitPathList(pathVal)
		for _, item := range items {
			if rule, ok := MatchPathWhitelist(item); ok {
				ruleKey := "PATH:" + rule.Agent
				if !seenRules[ruleKey] {
					seenRules[ruleKey] = true
					evidences = append(evidences, Evidence{
						Source:      EvidenceSourcePath,
						Key:         "PATH",
						TargetRole:  rule.Role,
						TargetAgent: rule.Agent,
						Matched:     true,
						Description: fmt.Sprintf("PATH 环境变量条目 %s", rule.Description),
					})
				}
			}
		}
	}

	// 2. 检查特定环境变量
	otherKeys := []string{
		"VSCODE_GIT_ASKPASS_NODE",
		"VSCODE_IPC_HOOK_CLI",
		"VSCODE_GIT_IPC_HANDLE",
		"APPLICATION_INSIGHTS_NO_DIAGNOSTIC_CHANNEL",
	}

	for _, key := range otherKeys {
		val := getenv(key)
		if val == "" {
			continue
		}
		if rule, ok := MatchPathWhitelist(val); ok {
			ruleKey := key + ":" + rule.Agent
			if !seenRules[ruleKey] {
				seenRules[ruleKey] = true
				evidences = append(evidences, Evidence{
					Source:      EvidenceSourcePath,
					Key:         key,
					TargetRole:  rule.Role,
					TargetAgent: rule.Agent,
					Matched:     true,
					Description: fmt.Sprintf("环境变量 %s %s", key, rule.Description),
				})
			}
		}
	}

	return evidences
}

// InspectProcessAncestors 使用指定的 ProcessInspector 扫描祖先进程并匹配白名单特征（兼容旧签名）。
func InspectProcessAncestors(inspector ProcessInspector, pid int, maxDepth int) ([]Evidence, []string) {
	return InspectProcessAncestorsWithProfiles(inspector, pid, maxDepth, nil)
}

// InspectProcessAncestorsWithProfiles 使用指定的 ProcessInspector 扫描祖先进程，并支持通过自定义 profiles 匹配进程。
func InspectProcessAncestorsWithProfiles(inspector ProcessInspector, pid int, maxDepth int, custom map[string]Profile) ([]Evidence, []string) {
	if inspector == nil {
		inspector = &DefaultProcessInspector{}
	}

	var evidences []Evidence
	var warnings []string

	ancestors, err := inspector.GetAncestors(pid, maxDepth)
	if err != nil {
		warnings = append(warnings, fmt.Sprintf("祖先进程探测降级: %v", err))
	}

	for level, p := range ancestors {
		ev, ok := evidenceFromAncestor(p, level)
		if ok {
			evidences = append(evidences, ev)
		}
		if strings.TrimSpace(p.Path) == "" {
			continue
		}
		base := path.Base(strings.ReplaceAll(p.Path, "\\", "/"))
		for name, profile := range custom {
			for _, processName := range profile.ProcessNames {
				if strings.EqualFold(strings.TrimSpace(processName), base) && (!ok || ev.TargetAgent != name) {
					evidences = append(evidences, Evidence{Source: EvidenceSourceProcess, Key: fmt.Sprintf("pid_%d", p.PID), TargetRole: "editor", TargetAgent: name, Matched: true, Executable: p.Path, Description: fmt.Sprintf("祖先进程 [%d:%s] 第 %d 级匹配自定义档案", p.PID, base, level)})
					break
				}
			}
		}
	}

	return evidences, warnings
}

// evidenceFromAncestor 把已核验路径转成 Editor 证据。
func evidenceFromAncestor(p ProcessInfo, level int) (Evidence, bool) {
	return evidenceFromAncestorWithProfiles(p, level, nil)
}

// evidenceFromAncestorWithProfiles 支持传入 profiles 把已核验路径转成 Editor 证据。
func evidenceFromAncestorWithProfiles(p ProcessInfo, level int, custom map[string]Profile) (Evidence, bool) {
	if strings.TrimSpace(p.Path) == "" {
		return Evidence{}, false
	}
	if agentName, ok := matchLocalEditorProcessWithProfiles(p, custom); ok {
		return Evidence{
			Source:      EvidenceSourceProcess,
			Key:         fmt.Sprintf("pid_%d", p.PID),
			TargetRole:  "editor",
			TargetAgent: agentName,
			Matched:     true,
			Executable:  p.Path,
			Description: fmt.Sprintf("祖先进程 [%d:%s] 第 %d 级精确匹配本地可执行文件", p.PID, filepath.Base(strings.ReplaceAll(p.Path, "\\", "/")), level),
		}, true
	}
	rule, ok := MatchPathWhitelist(p.Path)
	if !ok {
		return Evidence{}, false
	}
	name := p.Name
	if name == "" {
		name = filepath.Base(strings.ReplaceAll(p.Path, "\\", "/"))
	}
	return Evidence{
		Source:      EvidenceSourceProcess,
		Key:         fmt.Sprintf("pid_%d", p.PID),
		TargetRole:  rule.Role,
		TargetAgent: rule.Agent,
		Matched:     true,
		Executable:  p.Path,
		Description: fmt.Sprintf("祖先进程 [%d:%s] 第 %d 级%s", p.PID, name, level, rule.Description),
	}, true
}
