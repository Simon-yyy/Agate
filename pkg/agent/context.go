package agent

import (
	"sort"
	"strings"
)

// 证据来源类型常量
const (
	EvidenceSourceEnv            = "env"
	EvidenceSourceTermProgram    = "term_program"
	EvidenceSourceIPC            = "ipc"
	EvidenceSourcePath           = "path"
	EvidenceSourceProcess        = "process"
	EvidenceSourceSessionContext = "session_context"
)

// IdentityStatus 标识身份判定的置信状态。
type IdentityStatus string

const (
	StatusKnown     IdentityStatus = "known"     // 具备唯一且无冲突的确凿证据
	StatusUnknown   IdentityStatus = "unknown"   // 缺乏可用证据
	StatusAmbiguous IdentityStatus = "ambiguous" // 存在相互冲突的同级候选
)

// Transport 标识可观察到的运行与连接环境类型。
type Transport string

const (
	TransportLocal     Transport = "local"
	TransportSSH       Transport = "ssh"
	TransportContainer Transport = "container"
	TransportWSL       Transport = "wsl"
)

// Evidence 记录单条探测证据，确保不暴露敏感值。
type Evidence struct {
	Source      string `json:"source"`                // 来源类型，如 "env", "term_program", "ipc", "path", "process", "session_context"
	Key         string `json:"key"`                   // 变量名或探测键名
	TargetRole  string `json:"target_role"`           // 适用身份: "runner" 或 "editor"
	TargetAgent string `json:"target_agent"`          // 匹配到的目标工具名称
	Matched     bool   `json:"matched"`               // 是否成功命中
	Description string `json:"description,omitempty"` // 不含敏感值的说明摘要
}

// Candidate 记录某个候选工具及其支持证据列表。
type Candidate struct {
	Agent     string     `json:"agent"`
	Role      string     `json:"role"` // "runner" 或 "editor"
	Evidences []Evidence `json:"evidences"`
}

// DetectionContext 统一汇总当前环境的宿主编辑器、实际 Agent、传输环境与判定证据。
type DetectionContext struct {
	Editor         string          `json:"editor"`                    // 宿主编辑器（如 "cursor", "vscode"），无证据时为 ""
	EditorStatus   IdentityStatus  `json:"editor_status"`             // 宿主判定状态
	Runner         string          `json:"runner"`                    // 实际执行 Agent（如 "codex", "claude"），无证据时为 ""
	RunnerStatus   IdentityStatus  `json:"runner_status"`             // 执行者判定状态
	Transports     []Transport     `json:"transports"`                // 可观察传输环境列表
	Evidences      []Evidence      `json:"evidences"`                 // 全量命中的脱敏证据列表
	Candidates     []Candidate     `json:"candidates"`                // 收集到的候选集合
	SessionContext *SessionContext `json:"session_context,omitempty"` // 跨会话传递的声明上下文
	Warnings       []string        `json:"warnings,omitempty"`        // 异常或告警提示
}

// EffectiveTarget 返回最适合规则装配的首选目标。
// 优先级：唯一且无冲突的 Runner > 唯一且无冲突的 Editor > 未知 ""。
func (ctx *DetectionContext) EffectiveTarget() (string, bool) {
	if ctx.RunnerStatus == StatusKnown && ctx.Runner != "" {
		return ctx.Runner, true
	}
	if ctx.RunnerStatus == StatusUnknown && ctx.EditorStatus == StatusKnown && ctx.Editor != "" {
		return ctx.Editor, true
	}
	return "", false
}

// HasConflict 判断是否存在执行者或宿主层级的候选冲突。
// 准确反映当前检测上下文中是否存在歧义/冲突（用于 detect 诊断输出与冲突裁决）。
func (ctx *DetectionContext) HasConflict() bool {
	return ctx.RunnerStatus == StatusAmbiguous || ctx.EditorStatus == StatusAmbiguous
}

// DetectTransports 从环境变量中嗅探可观察的连接传输标签。
func DetectTransports(getenv func(string) string) []Transport {
	var transports []Transport
	seen := make(map[Transport]bool)

	add := func(t Transport) {
		if !seen[t] {
			seen[t] = true
			transports = append(transports, t)
		}
	}

	// 检查 SSH 相关环境变量
	if getenv("SSH_CONNECTION") != "" || getenv("SSH_CLIENT") != "" || getenv("SSH_TTY") != "" {
		add(TransportSSH)
	}

	// 检查容器环境标识
	if getenv("CONTAINER") != "" || getenv("KUBERNETES_SERVICE_HOST") != "" || getenv("DOCKER_CONTAINER") != "" {
		add(TransportContainer)
	}

	// 检查 WSL 环境标识
	if getenv("WSL_DISTRO_NAME") != "" || getenv("WSL_INTEROP") != "" {
		add(TransportWSL)
	}

	if len(transports) == 0 {
		add(TransportLocal)
	}
	return transports
}

// DeduplicateAndSortCandidates 对候选工具列表按 Agent 字母序做确定性排序。
func DeduplicateAndSortCandidates(candidates []Candidate) []Candidate {
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Role != candidates[j].Role {
			return candidates[i].Role < candidates[j].Role
		}
		return strings.ToLower(candidates[i].Agent) < strings.ToLower(candidates[j].Agent)
	})
	return candidates
}
