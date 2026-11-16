package agent

import (
	"fmt"
	"sort"
	"strings"
	"time"
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

// InspectContext 兼容入口，调用全功能 InspectContextFull。
func InspectContext(getenv func(string) string, termProgram string, gitIPC string, custom map[string]Profile) DetectionContext {
	return InspectContextFull(getenv, termProgram, gitIPC, custom, nil, time.Now())
}

// InspectContextFull 综合采集环境变量、终端标识、白名单路径、受限进程链及会话上下文声明，进行全量候选收集与冲突仲裁。
func InspectContextFull(getenv func(string) string, termProgram string, gitIPC string, custom map[string]Profile, inspector ProcessInspector, now time.Time) DetectionContext {
	ctx := DetectionContext{
		EditorStatus: StatusUnknown,
		RunnerStatus: StatusUnknown,
		Transports:   DetectTransports(getenv),
	}

	if getenv == nil {
		getenv = func(string) string { return "" }
	}

	termProgram = strings.ToLower(termProgram)
	gitIPC = strings.ToLower(gitIPC)

	var customNames []string
	if len(custom) > 0 {
		customNames = make([]string, 0, len(custom))
		for name := range custom {
			customNames = append(customNames, name)
		}
		sort.Strings(customNames)
	}

	runnerCandidateMap := make(map[string][]Evidence)
	editorCandidateMap := make(map[string][]Evidence)

	// 1. 嗅探跨会话传递声明上下文 (AGATE_SESSION_CONTEXT)
	sc, sessionEvidences, sessionWarnings := InspectSessionContext(getenv, now)
	ctx.SessionContext = sc
	ctx.Warnings = append(ctx.Warnings, sessionWarnings...)
	for _, ev := range sessionEvidences {
		ctx.Evidences = append(ctx.Evidences, ev)
		if ev.TargetRole == "runner" {
			runnerCandidateMap[ev.TargetAgent] = append(runnerCandidateMap[ev.TargetAgent], ev)
		} else if ev.TargetRole == "editor" {
			editorCandidateMap[ev.TargetAgent] = append(editorCandidateMap[ev.TargetAgent], ev)
		}
	}

	// 2. 采集 Runner 候选（进程专属 env_keys 强指纹）
	for _, item := range specs {
		for _, key := range item.envKeys {
			if getenv(key) != "" {
				ev := Evidence{
					Source:      EvidenceSourceEnv,
					Key:         key,
					TargetRole:  "runner",
					TargetAgent: string(item.kind),
					Matched:     true,
					Description: fmt.Sprintf("环境变量 %s 已设置", key),
				}
				ctx.Evidences = append(ctx.Evidences, ev)
				runnerCandidateMap[string(item.kind)] = append(runnerCandidateMap[string(item.kind)], ev)
			}
		}
	}

	for _, name := range customNames {
		for _, key := range custom[name].EnvKeys {
			if key != "" && getenv(key) != "" {
				ev := Evidence{
					Source:      EvidenceSourceEnv,
					Key:         key,
					TargetRole:  "runner",
					TargetAgent: name,
					Matched:     true,
					Description: fmt.Sprintf("自定义档案环境变量 %s 已设置", key),
				}
				ctx.Evidences = append(ctx.Evidences, ev)
				runnerCandidateMap[name] = append(runnerCandidateMap[name], ev)
			}
		}
	}

	// 3. 采集 Editor 候选（宿主终端与 IPC 弱指纹）
	for _, item := range specs {
		for _, marker := range item.termPrograms {
			if strings.Contains(termProgram, marker) {
				ev := Evidence{
					Source:      EvidenceSourceTermProgram,
					Key:         "TERM_PROGRAM",
					TargetRole:  "editor",
					TargetAgent: string(item.kind),
					Matched:     true,
					Description: fmt.Sprintf("TERM_PROGRAM 包含标识 %s", marker),
				}
				ctx.Evidences = append(ctx.Evidences, ev)
				editorCandidateMap[string(item.kind)] = append(editorCandidateMap[string(item.kind)], ev)
			}
		}
		for _, marker := range item.gitIPC {
			if strings.Contains(gitIPC, marker) {
				ev := Evidence{
					Source:      EvidenceSourceIPC,
					Key:         "VSCODE_GIT_IPC_HANDLE",
					TargetRole:  "editor",
					TargetAgent: string(item.kind),
					Matched:     true,
					Description: fmt.Sprintf("Git IPC 句柄包含标识 %s", marker),
				}
				ctx.Evidences = append(ctx.Evidences, ev)
				editorCandidateMap[string(item.kind)] = append(editorCandidateMap[string(item.kind)], ev)
			}
		}
	}

	for _, name := range customNames {
		for _, marker := range custom[name].TermPrograms {
			markerLower := strings.ToLower(strings.TrimSpace(marker))
			if markerLower != "" && strings.Contains(termProgram, markerLower) {
				ev := Evidence{
					Source:      EvidenceSourceTermProgram,
					Key:         "TERM_PROGRAM",
					TargetRole:  "editor",
					TargetAgent: name,
					Matched:     true,
					Description: fmt.Sprintf("自定义档案 TERM_PROGRAM 包含标识 %s", markerLower),
				}
				ctx.Evidences = append(ctx.Evidences, ev)
				editorCandidateMap[name] = append(editorCandidateMap[name], ev)
			}
		}
	}

	// 4. 采集远程白名单路径特征（PATH, VSCODE_* 等）
	pathEvidences := InspectPathEnvironments(getenv)
	for _, ev := range pathEvidences {
		ctx.Evidences = append(ctx.Evidences, ev)
		if ev.TargetRole == "editor" {
			editorCandidateMap[ev.TargetAgent] = append(editorCandidateMap[ev.TargetAgent], ev)
		}
	}

	// 5. 采集受限祖先进程链特征 (ProcessInspector)
	procEvidences, procWarnings := InspectProcessAncestors(inspector, 0, defaultAncestorDepth())
	ctx.Warnings = append(ctx.Warnings, procWarnings...)
	for _, ev := range procEvidences {
		ctx.Evidences = append(ctx.Evidences, ev)
		if ev.TargetRole == "editor" {
			editorCandidateMap[ev.TargetAgent] = append(editorCandidateMap[ev.TargetAgent], ev)
		}
	}

	// 6. 构建 Candidates 结构化列表
	for agentName, evList := range runnerCandidateMap {
		ctx.Candidates = append(ctx.Candidates, Candidate{
			Agent:     agentName,
			Role:      "runner",
			Evidences: evList,
		})
	}
	for agentName, evList := range editorCandidateMap {
		ctx.Candidates = append(ctx.Candidates, Candidate{
			Agent:     agentName,
			Role:      "editor",
			Evidences: evList,
		})
	}
	ctx.Candidates = DeduplicateAndSortCandidates(ctx.Candidates)

	// 7. 仲裁 Runner 身份
	// 区分强指纹（env）与会话声明（session_context）
	var envRunners []string
	var sessionRunners []string
	for agent, evs := range runnerCandidateMap {
		hasEnv := false
		hasSession := false
		for _, e := range evs {
			if e.Source == EvidenceSourceEnv {
				hasEnv = true
			}
			if e.Source == EvidenceSourceSessionContext {
				hasSession = true
			}
		}
		if hasEnv {
			envRunners = append(envRunners, agent)
		} else if hasSession {
			sessionRunners = append(sessionRunners, agent)
		}
	}
	sort.Strings(envRunners)
	sort.Strings(sessionRunners)

	if len(envRunners) == 1 {
		// 现场强指纹优先
		ctx.Runner = envRunners[0]
		ctx.RunnerStatus = StatusKnown
		// 检查若会话声明了不同 runner 则告警冲突
		if len(sessionRunners) > 0 && sessionRunners[0] != envRunners[0] {
			ctx.Warnings = append(ctx.Warnings, fmt.Sprintf("会话声明的执行者 (%s) 与现场环境变量 (%s) 存在分歧，优先采纳现场变量", sessionRunners[0], envRunners[0]))
		}
	} else if len(envRunners) > 1 {
		ctx.RunnerStatus = StatusAmbiguous
		ctx.Warnings = append(ctx.Warnings, fmt.Sprintf("发现多个冲突的 Agent 进程强指纹: %v", envRunners))
	} else if len(sessionRunners) == 1 {
		// 无现场环境变量，但有合法会话声明
		ctx.Runner = sessionRunners[0]
		ctx.RunnerStatus = StatusKnown
	} else if len(sessionRunners) > 1 {
		ctx.RunnerStatus = StatusAmbiguous
		ctx.Warnings = append(ctx.Warnings, fmt.Sprintf("发现多个冲突的会话声明执行者: %v", sessionRunners))
	} else {
		ctx.RunnerStatus = StatusUnknown
	}

	// 8. 仲裁 Editor 身份 (R2-F04, R3-F02 & R4-02: 精准绑定终端体系，消除多残留 PATH 误报与跨厂商误提权)
	// 收集拥有直接进程证据 (Process) 的编辑器
	processEditorAgents := make(map[string]bool)
	for agent, evs := range editorCandidateMap {
		for _, ev := range evs {
			if ev.Source == EvidenceSourceProcess {
				processEditorAgents[agent] = true
			}
		}
	}

	// 收集仅具有全局纯 PATH 匹配（Key == "PATH"）且无活跃进程、会话、专有变量或终端的候选列表
	var pathOnlyAgents []string
	for agent, evs := range editorCandidateMap {
		hasSpecificEvidence := false
		hasPurePath := false
		for _, ev := range evs {
			if ev.Source == EvidenceSourceProcess || ev.Source == EvidenceSourceSessionContext {
				hasSpecificEvidence = true
				break
			}
			if ev.Source == EvidenceSourcePath && ev.Key != "PATH" {
				// 活跃会话专用环境变量（如 VSCODE_GIT_ASKPASS_NODE / VSCODE_IPC_HOOK_CLI）
				// 注意：若存在明确属于其他 agent 的活动进程，则该会话变量判定为残留，不算做特定有效证据
				if len(processEditorAgents) > 0 && !processEditorAgents[agent] {
					continue
				}
				hasSpecificEvidence = true
				break
			}
			if ev.Source == EvidenceSourceIPC {
				hasSpecificEvidence = true
				break
			}
			if ev.Source == EvidenceSourceTermProgram {
				hasSpecificEvidence = true
				break
			}
			if ev.Source == EvidenceSourcePath && ev.Key == "PATH" {
				hasPurePath = true
			}
		}
		if !hasSpecificEvidence && hasPurePath {
			if len(processEditorAgents) == 0 || processEditorAgents[agent] {
				pathOnlyAgents = append(pathOnlyAgents, agent)
			}
		}
	}
	sort.Strings(pathOnlyAgents)

	var level1SpecificEditors []string   // 拥有 Process、SessionContext 或直接专用终端/IPC/会话专有变量
	var level2CorrelatedEditors []string // 专属纯 PATH + 活跃终端精准印证，且无相反进程

	for agent, evs := range editorCandidateMap {
		hasProcess := false
		hasSession := false
		hasDirectTermOrIPC := false
		hasSessionPathVar := false
		hasPurePath := false

		for _, ev := range evs {
			switch ev.Source {
			case EvidenceSourceProcess:
				hasProcess = true
			case EvidenceSourceSessionContext:
				hasSession = true
			case EvidenceSourceIPC:
				hasDirectTermOrIPC = true
			case EvidenceSourceTermProgram:
				hasDirectTermOrIPC = true
			case EvidenceSourcePath:
				if ev.Key == "PATH" {
					hasPurePath = true
				} else {
					// Key != "PATH"，属于当前会话专有路径变量 (如 VSCODE_GIT_ASKPASS_NODE)
					hasSessionPathVar = true
				}
			}
		}

		if hasProcess || hasSession {
			level1SpecificEditors = append(level1SpecificEditors, agent)
		} else if hasDirectTermOrIPC || hasSessionPathVar {
			// 若存在明确属于其他 agent 的活动父进程，则会话特征或直接终端/IPC 判定为残留，予以忽略
			if len(processEditorAgents) > 0 && !processEditorAgents[agent] {
				ctx.Warnings = append(ctx.Warnings, fmt.Sprintf("检测到特定会话特征但与当前活动进程不符，忽略残留会话特征: %s", agent))
			} else {
				level1SpecificEditors = append(level1SpecificEditors, agent)
			}
		} else if hasPurePath {
			// 若存在明确属于其他 agent 的活动进程，则当前 agent 的 PATH 判定为残留，予以忽略
			if len(processEditorAgents) > 0 && !processEditorAgents[agent] {
				ctx.Warnings = append(ctx.Warnings, fmt.Sprintf("检测到安装目录白名单但与当前活动进程不符，忽略残留 PATH: %s", agent))
			} else if isPathCorroboratedForAgent(agent, termProgram, gitIPC, pathOnlyAgents, custom) {
				// 存在精准匹配的活跃终端且无相反进程证据，PATH 与终端互相印证为特定编辑器
				level2CorrelatedEditors = append(level2CorrelatedEditors, agent)
			} else {
				// 完全无精准匹配的活跃终端、IPC 或进程证据，属于孤立或未区分残留 PATH，不得升级为 known
				ctx.Warnings = append(ctx.Warnings, fmt.Sprintf("检测到安装目录白名单但无活动会话印证: %s (仅残留 PATH，维持未知状态防误认)", agent))
			}
		}
	}

	sort.Strings(level1SpecificEditors)
	sort.Strings(level2CorrelatedEditors)

	if len(level1SpecificEditors) == 1 {
		ctx.Editor = level1SpecificEditors[0]
		ctx.EditorStatus = StatusKnown
	} else if len(level1SpecificEditors) > 1 {
		ctx.EditorStatus = StatusAmbiguous
		ctx.Warnings = append(ctx.Warnings, fmt.Sprintf("发现多个冲突的特定宿主编辑器活跃特征: %v", level1SpecificEditors))
	} else if len(level2CorrelatedEditors) == 1 {
		ctx.Editor = level2CorrelatedEditors[0]
		ctx.EditorStatus = StatusKnown
	} else if len(level2CorrelatedEditors) > 1 {
		ctx.EditorStatus = StatusAmbiguous
		ctx.Warnings = append(ctx.Warnings, fmt.Sprintf("发现多个冲突的特定宿主编辑器候选: %v", level2CorrelatedEditors))
	} else {
		// 仅有孤立残留 PATH 或无活跃宿主特征时降级为未知，防盲目猜测
		ctx.EditorStatus = StatusUnknown
	}

	return ctx
}

// isPathCorroboratedForAgent 判断特定编辑器的 PATH 安装特征是否获得当前活跃会话终端的精准互证。
// (R4-02: 杜绝跨厂商无关终端误佐证，杜绝通用 vscode 终端在多残留 server PATH 冲突时误报 ambiguous)
func isPathCorroboratedForAgent(agent string, termProgram, gitIPC string, pathOnlyAgents []string, custom map[string]Profile) bool {
	term := strings.ToLower(strings.TrimSpace(termProgram))
	ipc := strings.ToLower(strings.TrimSpace(gitIPC))

	// 自定义 Profile 必须与声明的 TermPrograms 匹配
	if p, ok := custom[agent]; ok {
		for _, tp := range p.TermPrograms {
			tpLower := strings.ToLower(strings.TrimSpace(tp))
			if tpLower != "" && strings.Contains(term, tpLower) {
				return true
			}
		}
		return false
	}

	switch agent {
	case "cursor":
		// 如果终端标识明确为 vscode 体系 (如 TERM_PROGRAM=vscode 或 VSCODE_GIT_IPC_HANDLE)
		isVSCodeFamily := term == "vscode" || strings.Contains(ipc, "vscode")
		if !isVSCodeFamily {
			return false
		}
		// 若同时存在多个 vscode 派生/本体的 PATH 且无特定 IPC 区分，视为多残留冲突未决，不予单独升级
		if len(pathOnlyAgents) > 1 {
			return false
		}
		return true
	case "vscode":
		isVSCodeFamily := term == "vscode" || strings.Contains(ipc, "vscode")
		if !isVSCodeFamily {
			return false
		}
		if len(pathOnlyAgents) > 1 {
			return false
		}
		return true
	case "windsurf":
		return strings.Contains(term, "windsurf")
	case "antigravity":
		return strings.Contains(term, "antigravity")
	default:
		return false
	}
}

// DetectEnvironment 根据环境变量和终端标识检测当前 Agent。
// getenv 作为依赖注入，使调用方和测试都不需要修改进程环境。
func DetectEnvironment(getenv func(string) string, termProgram string, gitIPC string) (Kind, bool) {
	return DetectEnvironmentWithProfiles(getenv, termProgram, gitIPC, nil)
}

// DetectEnvironmentWithProfiles 根据内置指纹与自定义档案声明的 EnvKeys/TermPrograms 检测当前 Agent。
// 依据 EffectiveTarget 返回唯一且无冲突的装配目标（优先确知 Runner，次之确知 Editor）。
func DetectEnvironmentWithProfiles(getenv func(string) string, termProgram string, gitIPC string, custom map[string]Profile) (Kind, bool) {
	return DetectEnvironmentFull(getenv, termProgram, gitIPC, custom, nil)
}

// DetectEnvironmentFull 提供全量依赖注入（包含 ProcessInspector），便于单元测试精确隔离宿主进程树。
func DetectEnvironmentFull(getenv func(string) string, termProgram string, gitIPC string, custom map[string]Profile, inspector ProcessInspector) (Kind, bool) {
	ctx := InspectContextFull(getenv, termProgram, gitIPC, custom, inspector, time.Now())
	if target, ok := ctx.EffectiveTarget(); ok {
		return Kind(target), true
	}
	return "", false
}

// IsAutomationEnvironment 判断当前进程是否带有自动化 Agent/CI 指纹。
func IsAutomationEnvironment(getenv func(string) string) bool {
	return IsAutomationEnvironmentWithProfiles(getenv, nil)
}

// IsAutomationEnvironmentWithProfiles 判断当前进程是否带有内置或自定义自动化 Agent/CI 指纹。
func IsAutomationEnvironmentWithProfiles(getenv func(string) string, custom map[string]Profile) bool {
	return isAutomationEnvironmentFull(getenv, custom, nil)
}

func isAutomationEnvironmentFull(getenv func(string) string, custom map[string]Profile, inspector ProcessInspector) bool {
	if getenv == nil {
		return false
	}
	if getenv("CI") != "" || getenv("GITHUB_ACTIONS") != "" {
		return true
	}
	ctx := InspectContextFull(getenv, "", "", custom, inspector, time.Now())
	return ctx.RunnerStatus == StatusKnown || ctx.RunnerStatus == StatusAmbiguous
}
