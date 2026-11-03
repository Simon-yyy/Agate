package agent

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	// SessionContextEnvVar 承载会话上下文传递的标准环境变量名。
	SessionContextEnvVar = "AGATE_SESSION_CONTEXT"
	// MaxSessionContextLength 会话声明最大允许字节长度（防止超大输入或缓冲区溢出）。
	MaxSessionContextLength = 512
)

// SessionContext 封装跨远程终端传递的轻量会话上下文声明。
type SessionContext struct {
	Version   string `json:"version"`              // 协议版本，当前必须为 "1"
	Editor    string `json:"editor,omitempty"`     // 声明的宿主编辑器名称
	Runner    string `json:"runner,omitempty"`     // 声明的执行者 Agent 名称
	SessionID string `json:"-"`                    // 会话唯一关联标识 (内部关联追踪，绝不序列化导出防信息泄露)
	Source    string `json:"source,omitempty"`     // 声明来源（如 "terminal", "ssh", "extension"）
	Timestamp int64  `json:"timestamp,omitempty"`  // 生成时间戳 (Unix 秒)
	ExpiresAt int64  `json:"expires_at,omitempty"` // 过期时间戳 (Unix 秒)
}

// 敏感凭据关键词黑名单（严禁透传敏感信息）。
var credentialBlacklist = []string{
	"password",
	"bearer ",
	"private_key",
	"api_key",
	"apikey",
	"auth=",
	"token=",
	"secret=",
}

// ParseSessionContext 解析并校验会话上下文声明字符串，执行四重安全防线。
func ParseSessionContext(raw string, now time.Time) (*SessionContext, []string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil, nil
	}

	var warnings []string

	// 1. 长度防线
	if len(raw) > MaxSessionContextLength {
		return nil, append(warnings, fmt.Sprintf("会话声明长度 (%d 字节) 超过最大上限 %d 字节，已作废", len(raw), MaxSessionContextLength)), errors.New("会话声明长度超限")
	}

	// 2. 凭据防火墙
	lower := strings.ToLower(raw)
	for _, badWord := range credentialBlacklist {
		if strings.Contains(lower, badWord) {
			return nil, append(warnings, fmt.Sprintf("检测到疑似凭据特征 [%s]，为保障安全已拦截并作废会话声明", badWord)), errors.New("检测到敏感凭据，声明被拦截")
		}
	}

	sc := &SessionContext{}
	pairs := strings.Split(raw, ";")
	for _, p := range pairs {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		eqIdx := strings.Index(p, "=")
		if eqIdx < 0 {
			return nil, append(warnings, fmt.Sprintf("会话声明格式非法（片段未包含键值分隔符 '='，长度: %d 字节已脱敏拦截）", len(p))), errors.New("会话声明片段格式非法")
		}
		k := strings.ToLower(strings.TrimSpace(p[:eqIdx]))
		v := strings.TrimSpace(p[eqIdx+1:])

		switch k {
		case "v":
			sc.Version = v
		case "editor":
			if !isValidAgentIdentifier(v) {
				return nil, append(warnings, fmt.Sprintf("会话声明指定编辑器标识非法 (长度: %d 字节，含路径或特殊字符已脱敏拦截)，必须为合规字母数字标识", len(v))), errors.New("会话编辑器标识非法")
			}
			sc.Editor = strings.ToLower(v)
		case "runner":
			if !isValidAgentIdentifier(v) {
				return nil, append(warnings, fmt.Sprintf("会话声明指定执行者标识非法 (长度: %d 字节，含路径或特殊字符已脱敏拦截)，必须为合规字母数字标识", len(v))), errors.New("会话执行者标识非法")
			}
			sc.Runner = strings.ToLower(v)
		case "sid", "session_id":
			if !isValidSessionID(v) {
				return nil, append(warnings, fmt.Sprintf("会话声明指定会话标识非法 (长度: %d 字节，含特殊字符已脱敏拦截)，必须为合规安全标识符", len(v))), errors.New("会话标识非法")
			}
			sc.SessionID = v
		case "src", "source":
			sc.Source = sanitizeSource(v)
		case "ts", "timestamp":
			ts, err := strconv.ParseInt(v, 10, 64)
			if err != nil || ts <= 0 {
				return nil, append(warnings, fmt.Sprintf("会话声明时间戳格式非法 (长度: %d 字节，输入内容已脱敏)，必须为有效正整数秒", len(v))), errors.New("会话时间戳非法")
			}
			sc.Timestamp = ts
		case "exp", "expires_at":
			exp, err := strconv.ParseInt(v, 10, 64)
			if err != nil || exp <= 0 {
				return nil, append(warnings, fmt.Sprintf("会话声明过期时间格式非法 (长度: %d 字节，输入内容已脱敏)，必须为有效正整数秒", len(v))), errors.New("会话过期时间非法")
			}
			sc.ExpiresAt = exp
		}
	}

	// 3. 协议版本防线
	if sc.Version != "1" {
		return nil, append(warnings, fmt.Sprintf("未知或不支持的会话声明协议版本 (长度: %d 字节，当前仅支持 v=1)", len(sc.Version))), errors.New("会话声明协议版本不支持")
	}

	// 4. 时序与时效硬约束防线 (R2-F03: 必须显式声明有效期，防陈旧/跨会话永久误信)
	if sc.ExpiresAt <= 0 {
		return nil, append(warnings, "会话声明缺失时效约束 (exp/expires_at)，为防陈旧环境滥用已拒绝作为确定身份"), errors.New("会话声明缺失时效约束")
	}

	if sc.Timestamp > 0 && sc.ExpiresAt <= sc.Timestamp {
		return nil, append(warnings, fmt.Sprintf("会话声明过期时间 (%d) 不得早于或等于生成时间 (%d)", sc.ExpiresAt, sc.Timestamp)), errors.New("会话时序逻辑非法")
	}

	if !now.IsZero() && now.Unix() > sc.ExpiresAt {
		return nil, append(warnings, fmt.Sprintf("会话声明已于 %s 过期 (当前时间: %s)", time.Unix(sc.ExpiresAt, 0).Format(time.RFC3339), now.Format(time.RFC3339))), errors.New("会话声明已过期")
	}

	return sc, warnings, nil
}

// isValidAgentIdentifier 校验 Agent/Editor 标识符是否安全合规（仅允许小写字母、数字、短横线、下划线，长度 1-64）。
// 严禁包含绝对/相对路径符号（/、\）、空格、冒号及控制字符，杜绝私有路径等敏感信息注入。(R3-S01)
func isValidAgentIdentifier(s string) bool {
	clean := strings.ToLower(strings.TrimSpace(s))
	if len(clean) == 0 || len(clean) > 64 {
		return false
	}
	for i := 0; i < len(clean); i++ {
		c := clean[i]
		if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '_' || c == '-') {
			return false
		}
	}
	return true
}

// isValidSessionID 校验会话关联标识符是否合规安全（允许字母、数字、短横线、下划线、点、冒号与路径分隔符，长度 1-128）。
func isValidSessionID(s string) bool {
	clean := strings.TrimSpace(s)
	if len(clean) == 0 || len(clean) > 128 {
		return false
	}
	for i := 0; i < len(clean); i++ {
		c := clean[i]
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') ||
			c == '_' || c == '-' || c == '.' || c == ':' || c == '/' || c == '\\') {
			return false
		}
	}
	return true
}

// sanitizeSource 对会话来源字段执行安全白名单归一化，严禁私有绝对路径等敏感信息落盘。
func sanitizeSource(raw string) string {
	s := strings.ToLower(strings.TrimSpace(raw))
	switch s {
	case "terminal", "ssh", "extension", "ide", "ci":
		return s
	case "", "unknown":
		return "unknown"
	default:
		// 任何包含路径斜杠、特殊字符的任意输入统一脱敏为 custom
		return "custom"
	}
}

// InspectSessionContext 从环境变量中嗅探会话上下文并转换为证据列表。
// (R3-F01: 严格校验声明范围与当前连接/终端环境是否契合，防止跨窗口或陈旧继承冒领)
func InspectSessionContext(getenv func(string) string, now time.Time) (*SessionContext, []Evidence, []string) {
	if getenv == nil {
		return nil, nil, nil
	}

	raw := getenv(SessionContextEnvVar)
	if raw == "" {
		return nil, nil, nil
	}

	sc, warnings, err := ParseSessionContext(raw, now)
	if err != nil || sc == nil {
		return nil, nil, warnings
	}

	// 适用范围与环境一致性检查 (R3-F01 & R4-03)
	isSSH := getenv("SSH_CONNECTION") != "" || getenv("SSH_CLIENT") != "" || getenv("SSH_TTY") != ""
	if sc.Source == "ssh" && !isSSH {
		warnings = append(warnings, "会话声明标明来源为 ssh，但当前未检测到 SSH 连接上下文，声明适用范围不匹配，已拒绝作为确定身份")
		return nil, nil, warnings
	}

	// 来源合法性校验：拒绝非受控来源（unknown/custom）的信封直接生效 (R5-02)
	if sc.Source == "unknown" || sc.Source == "custom" {
		warnings = append(warnings, "会话声明缺失合规来源特征 (source/src 必须为受控白名单 terminal/ssh/extension/ide/ci)，已拒绝作为可信身份")
		return nil, nil, warnings
	}

	// 终端会话标识一致性校验：若环境存在活跃会话 ID，且信封亦声明了 SessionID，二者发生冲突则判定为跨会话污染
	liveSID := strings.TrimSpace(getenv("AGATE_SESSION_ID"))
	if liveSID == "" {
		liveSID = strings.TrimSpace(getenv("TERM_SESSION_ID"))
	}
	if liveSID != "" && sc.SessionID != "" && liveSID != sc.SessionID {
		warnings = append(warnings, "会话声明 SessionID 与当前活动终端会话环境不一致 (检测到跨窗口/跨会话残留)，已拒绝作为确定身份")
		return nil, nil, warnings
	}

	var evidences []Evidence
	if sc.Editor != "" {
		evidences = append(evidences, Evidence{
			Source:      EvidenceSourceSessionContext,
			Key:         SessionContextEnvVar,
			TargetRole:  "editor",
			TargetAgent: sc.Editor,
			Matched:     true,
			Description: fmt.Sprintf("会话声明指定宿主编辑器: %s (来源: %s)", sc.Editor, sc.Source),
		})
	}
	if sc.Runner != "" {
		evidences = append(evidences, Evidence{
			Source:      EvidenceSourceSessionContext,
			Key:         SessionContextEnvVar,
			TargetRole:  "runner",
			TargetAgent: sc.Runner,
			Matched:     true,
			Description: fmt.Sprintf("会话声明指定执行者 Agent: %s (来源: %s)", sc.Runner, sc.Source),
		})
	}

	return sc, evidences, warnings
}
