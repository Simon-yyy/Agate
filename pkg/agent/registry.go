package agent

import (
	"sort"
	"strings"
)

// Kind 标识当前协同工具类型。
type Kind string

const (
	Codex       Kind = "codex"
	Cursor      Kind = "cursor"
	Antigravity Kind = "antigravity"
	Claude      Kind = "claude"
	Windsurf    Kind = "windsurf"
)

type spec struct {
	kind         Kind
	aliases      []string
	envKeys      []string
	termPrograms []string
	gitIPC       []string
}

var specs = []spec{
	{kind: Codex, aliases: []string{"codex"}, envKeys: []string{"CODEX_SESSION_ID", "CODEX_THREAD_ID", "CODEX_VERSION", "CODEX_CI", "CODEX_SHELL"}, termPrograms: []string{"codex"}},
	{kind: Cursor, aliases: []string{"cursor"}, envKeys: []string{"CURSOR_AGENT", "CURSOR_VERSION"}, termPrograms: []string{"cursor"}, gitIPC: []string{"cursor"}},
	{kind: Antigravity, aliases: []string{"antigravity", "gemini"}, envKeys: []string{"ANTIGRAVITY_AGENT", "GEMINI_AGENT", "ANTIGRAVITY_IDE"}, termPrograms: []string{"antigravity"}},
	{kind: Claude, aliases: []string{"claude"}, envKeys: []string{"CLAUDE_CODE", "CLAUDE_AGENT"}, termPrograms: []string{"claude"}},
	{kind: Windsurf, aliases: []string{"windsurf"}, envKeys: []string{"WINDSURF_AGENT"}, termPrograms: []string{"windsurf"}},
}

// Parse 将用户输入解析为标准 Agent 类型。
func Parse(name string) (Kind, bool) {
	clean := strings.ToLower(strings.TrimSpace(name))
	for _, item := range specs {
		for _, alias := range item.aliases {
			if clean == alias {
				return item.kind, true
			}
		}
	}
	return "", false
}

// All 返回稳定顺序的全部 Agent 类型。
func All() []Kind {
	result := make([]Kind, 0, len(specs))
	for _, item := range specs {
		result = append(result, item.kind)
	}
	return result
}

// DetectEnvironment 根据环境变量和终端标识检测当前 Agent。
// getenv 作为依赖注入，使调用方和测试都不需要修改进程环境。
func DetectEnvironment(getenv func(string) string, termProgram string, gitIPC string) (Kind, bool) {
	return DetectEnvironmentWithProfiles(getenv, termProgram, gitIPC, nil)
}

// DetectEnvironmentWithProfiles 根据内置指纹与自定义档案声明的 EnvKeys/TermPrograms 检测当前 Agent。
func DetectEnvironmentWithProfiles(getenv func(string) string, termProgram string, gitIPC string, custom map[string]Profile) (Kind, bool) {
	var customNames []string
	if len(custom) > 0 {
		customNames = make([]string, 0, len(custom))
		for name := range custom {
			customNames = append(customNames, name)
		}
		sort.Strings(customNames)
	}

	// 第一遍：先扫描所有内置 specs 的 envKeys，再扫描自定义档案的 EnvKeys（进程级强指纹）
	for _, item := range specs {
		for _, key := range item.envKeys {
			if getenv(key) != "" {
				return item.kind, true
			}
		}
	}
	for _, name := range customNames {
		for _, key := range custom[name].EnvKeys {
			if key != "" && getenv(key) != "" {
				return Kind(name), true
			}
		}
	}

	// 第二遍：再扫描内置 specs 的 termPrograms 与 gitIPC，以及自定义档案的 TermPrograms（宿主终端弱指纹）
	termProgram = strings.ToLower(termProgram)
	gitIPC = strings.ToLower(gitIPC)
	for _, item := range specs {
		for _, marker := range item.termPrograms {
			if strings.Contains(termProgram, marker) {
				return item.kind, true
			}
		}
		for _, marker := range item.gitIPC {
			if strings.Contains(gitIPC, marker) {
				return item.kind, true
			}
		}
	}
	for _, name := range customNames {
		for _, marker := range custom[name].TermPrograms {
			markerLower := strings.ToLower(strings.TrimSpace(marker))
			if markerLower != "" && strings.Contains(termProgram, markerLower) {
				return Kind(name), true
			}
		}
	}
	return "", false
}

// IsAutomationEnvironment 判断当前进程是否带有自动化 Agent/CI 指纹。
func IsAutomationEnvironment(getenv func(string) string) bool {
	return IsAutomationEnvironmentWithProfiles(getenv, nil)
}

// IsAutomationEnvironmentWithProfiles 判断当前进程是否带有内置或自定义自动化 Agent/CI 指纹。
func IsAutomationEnvironmentWithProfiles(getenv func(string) string, custom map[string]Profile) bool {
	if getenv("CI") != "" || getenv("GITHUB_ACTIONS") != "" {
		return true
	}
	for _, item := range specs {
		for _, key := range item.envKeys {
			if getenv(key) != "" {
				return true
			}
		}
	}
	for _, profile := range custom {
		for _, key := range profile.EnvKeys {
			if key != "" && getenv(key) != "" {
				return true
			}
		}
	}
	return false
}
