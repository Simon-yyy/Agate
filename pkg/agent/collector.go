package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
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

// DefaultProcessInspector 系统默认进程探测器。
type DefaultProcessInspector struct{}

// GetAncestors 沿当前进程向上采集有限祖先进程链（最大深度受限，不读取完整命令行）。
func (d *DefaultProcessInspector) GetAncestors(pid int, maxDepth int) ([]ProcessInfo, error) {
	if maxDepth <= 0 {
		maxDepth = 4
	}
	if maxDepth > 8 {
		maxDepth = 8 // 硬上限防御
	}

	if runtime.GOOS != "linux" {
		// 非 Linux 平台安全降级，不请求提权，不执行外部 shell
		return nil, nil
	}

	var chain []ProcessInfo
	currPID := pid
	if currPID <= 0 {
		currPID = os.Getpid()
	}

	for depth := 0; depth < maxDepth; depth++ {
		if currPID <= 1 {
			break
		}

		info, err := readLinuxProcInfo(currPID)
		if err != nil {
			// 进程已退出或容器限制不可读时安全截断返回
			break
		}

		chain = append(chain, info)
		if info.PPID <= 1 || info.PPID == currPID {
			break
		}
		currPID = info.PPID
	}

	return chain, nil
}

// readLinuxProcInfo 读取指定 PID 的父进程及可执行文件路径（仅通过 /proc 内部接口）。
func readLinuxProcInfo(pid int) (ProcessInfo, error) {
	statPath := fmt.Sprintf("/proc/%d/stat", pid)
	data, err := os.ReadFile(statPath)
	if err != nil {
		return ProcessInfo{}, err
	}

	// /proc/[pid]/stat 格式形如: pid (comm) state ppid ...
	statStr := string(data)
	closeParen := strings.LastIndex(statStr, ")")
	if closeParen < 0 || closeParen+2 >= len(statStr) {
		return ProcessInfo{}, fmt.Errorf("无法解析 /proc/%d/stat", pid)
	}

	comm := ""
	openParen := strings.Index(statStr, "(")
	if openParen >= 0 && openParen < closeParen {
		comm = statStr[openParen+1 : closeParen]
	}

	rest := strings.Fields(statStr[closeParen+2:])
	if len(rest) < 2 {
		return ProcessInfo{}, fmt.Errorf("/proc/%d/stat 字段不完整", pid)
	}

	ppid, err := strconv.Atoi(rest[1])
	if err != nil {
		ppid = 0
	}

	exePath := ""
	if target, err := os.Readlink(fmt.Sprintf("/proc/%d/exe", pid)); err == nil {
		exePath = target
	}

	name := comm
	if exePath != "" {
		name = filepath.Base(exePath)
	}

	return ProcessInfo{
		PID:  pid,
		PPID: ppid,
		Path: exePath,
		Name: name,
	}, nil
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

// InspectProcessAncestors 使用指定的 ProcessInspector 扫描祖先进程并匹配白名单特征。
func InspectProcessAncestors(inspector ProcessInspector, pid int, maxDepth int) ([]Evidence, []string) {
	if inspector == nil {
		inspector = &DefaultProcessInspector{}
	}

	var evidences []Evidence
	var warnings []string

	ancestors, err := inspector.GetAncestors(pid, maxDepth)
	if err != nil {
		warnings = append(warnings, fmt.Sprintf("祖先进程探测降级: %v", err))
		return evidences, warnings
	}

	for _, p := range ancestors {
		if p.Path == "" {
			continue
		}
		if rule, ok := MatchPathWhitelist(p.Path); ok {
			evidences = append(evidences, Evidence{
				Source:      EvidenceSourceProcess,
				Key:         fmt.Sprintf("pid_%d", p.PID),
				TargetRole:  rule.Role,
				TargetAgent: rule.Agent,
				Matched:     true,
				Description: fmt.Sprintf("祖先进程 [%d:%s] %s", p.PID, p.Name, rule.Description),
			})
		}
	}

	return evidences, warnings
}
