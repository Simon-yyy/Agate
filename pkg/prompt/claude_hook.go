package prompt

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

// ClaudeAdapter 实现 NativeAdapter 接口，处理 Claude Code 专有协议。
type ClaudeAdapter struct{}

func init() {
	RegisterNativeAdapter("claude", &ClaudeAdapter{})
}

type claudeEventBody struct {
	SessionID string
	TurnID    string
	Prompt    string
	Response  string
	ToolName  string
	ToolInput map[string]any
	Success   *bool
}

func decodeClaudeBody(payload []byte) (claudeEventBody, error) {
	var raw map[string]any
	if len(bytes.TrimSpace(payload)) == 0 {
		return claudeEventBody{}, nil
	}
	if err := json.Unmarshal(bytes.TrimSpace(payload), &raw); err != nil {
		return claudeEventBody{}, fmt.Errorf("无效载荷: %w", err)
	}
	body := claudeEventBody{
		SessionID: firstMapString(raw, "session_id", "sessionId"),
		TurnID:    firstMapString(raw, "turn_id", "turnId"),
		Prompt:    firstMapString(raw, "prompt", "user_prompt"),
		Response:  firstMapString(raw, "response", "last_assistant_message"),
		ToolName:  firstMapString(raw, "tool_name", "toolName"),
		Success:   mapBoolPtr(raw, "success"),
	}
	if input := firstMapObject(raw, "tool_input", "toolInput"); input != nil {
		body.ToolInput = input
	}
	return body, nil
}

func (a *ClaudeAdapter) Decode(eventName string, payload []byte) (NativeEvent, error) {
	body, err := decodeClaudeBody(payload)
	if err != nil {
		return NativeEvent{}, err
	}

	event := NativeEvent{
		SessionID: body.SessionID,
		TurnID:    body.TurnID,
	}

	switch eventName {
	case "UserPromptSubmit":
		event.Kind = NativeEventSubmit
		event.Text = body.Prompt
		event.TrustedUser = strings.TrimSpace(body.Prompt) != ""
		if strings.TrimSpace(event.Text) == "" {
			return event, fmt.Errorf("缺少自然语言指令")
		}

	case "PreToolUse":
		event.Kind = NativeEventBeforeTool
		// 官方工具：Read / Edit / Write
		action, targets, command := classifyToolAction(body.ToolName, body.ToolInput)
		event.Action = action
		event.Command = command
		event.Targets = targets

	case "PostToolUse":
		if isKnownReadToolName(body.ToolName) || isReadToolName(body.ToolName) || strings.EqualFold(body.ToolName, "Read") {
			event.Kind = NativeEventReadCompleted
			event.Action = NativeActionRead
			if target := extractTargetFromMap(body.ToolInput); target != "" {
				event.Targets = []string{target}
			}
			event.Success = body.Success == nil || *body.Success
		} else {
			event.Kind = NativeEventObserve
			event.Action = NativeActionOther
		}

	case "Stop":
		event.Kind = NativeEventResponseCompleted
		event.Text = body.Response

	default:
		return NativeEvent{}, fmt.Errorf("%w: 宿主 claude 不支持事件 %q", ErrUnsupportedHookTarget, eventName)
	}

	return event, nil
}

func (a *ClaudeAdapter) Encode(eventName string, decision NativeDecision) (stdout []byte, stderr string, exitCode int, err error) {
	switch eventName {
	case "PreToolUse":
		if decision.Block {
			resp := map[string]any{
				"hookSpecificOutput": map[string]any{
					"hookEventName":            "PreToolUse",
					"permissionDecision":       "deny",
					"permissionDecisionReason": decision.Reason,
				},
			}
			stdout, _ = json.Marshal(resp)
			stderr = decision.Reason
		} else {
			stdout = []byte("{}")
		}

	case "UserPromptSubmit":
		if decision.AdditionalContext != "" {
			stderr = fmt.Sprintf("已为本次提交生成上下文信封: %s", filepath.Base(decision.AdditionalContext))
		}
		if decision.Block {
			stderr = decision.Reason
		}
		stdout = []byte("{}")

	case "PostToolUse":
		resp := map[string]any{}
		if decision.AdditionalContext != "" {
			resp["additionalContext"] = fmt.Sprintf("Agate 已为本次请求生成上下文。请先读取：%s", decision.AdditionalContext)
		}
		stdout, _ = json.Marshal(resp)

	case "Stop":
		stdout = []byte("{}")

	default:
		stdout = []byte("{}")
	}

	return stdout, stderr, 0, nil
}

func extractTargetFromMap(input map[string]any) string {
	if input == nil {
		return ""
	}
	for _, k := range []string{"path", "filePath", "file_path", "target", "target_file", "file"} {
		if v, ok := input[k]; ok {
			return fmt.Sprint(v)
		}
	}
	return ""
}

func firstMapString(raw map[string]any, keys ...string) string {
	for _, key := range keys {
		if v, ok := raw[key]; ok {
			if s := strings.TrimSpace(fmt.Sprint(v)); s != "" && s != "<nil>" {
				return s
			}
		}
	}
	return ""
}

func firstMapObject(raw map[string]any, keys ...string) map[string]any {
	for _, key := range keys {
		if v, ok := raw[key]; ok {
			if m, ok := v.(map[string]any); ok {
				return m
			}
		}
	}
	return nil
}

func mapBoolPtr(raw map[string]any, key string) *bool {
	v, ok := raw[key]
	if !ok {
		return nil
	}
	switch t := v.(type) {
	case bool:
		return &t
	default:
		return nil
	}
}
