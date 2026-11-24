package prompt

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

// CodexAdapter 实现 NativeAdapter 接口，处理 Codex 专有协议。
type CodexAdapter struct{}

func init() {
	RegisterNativeAdapter("codex", &CodexAdapter{})
}

type codexEventBody struct {
	SessionID string
	TurnID    string
	Prompt    string
	Response  string
	ToolName  string
	ToolInput map[string]any
	Success   *bool
}

func decodeCodexBody(payload []byte) (codexEventBody, error) {
	var raw map[string]any
	if len(bytes.TrimSpace(payload)) == 0 {
		return codexEventBody{}, nil
	}
	if err := json.Unmarshal(bytes.TrimSpace(payload), &raw); err != nil {
		return codexEventBody{}, fmt.Errorf("无效载荷: %w", err)
	}
	body := codexEventBody{
		SessionID: firstMapString(raw, "session_id", "sessionId"),
		TurnID:    firstMapString(raw, "turn_id", "turnId"),
		Prompt:    firstMapString(raw, "prompt", "user_prompt"),
		Response:  firstMapString(raw, "last_assistant_message", "response"),
		ToolName:  firstMapString(raw, "tool_name", "toolName"),
		Success:   mapBoolPtr(raw, "success"),
	}
	if input := firstMapObject(raw, "tool_input", "toolInput"); input != nil {
		body.ToolInput = input
	}
	return body, nil
}

func (a *CodexAdapter) Decode(eventName string, payload []byte) (NativeEvent, error) {
	body, err := decodeCodexBody(payload)
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
		toolName := strings.ToLower(strings.TrimSpace(body.ToolName))
		if toolName == "apply_patch" {
			event.Action = NativeActionEdit
			event.Targets = extractTargetsFromApplyPatch(body.ToolInput)
		} else {
			action, targets, command := classifyToolAction(body.ToolName, body.ToolInput)
			event.Action = action
			event.Command = command
			event.Targets = targets
		}

	case "PostToolUse":
		if isKnownReadToolName(body.ToolName) || isReadToolName(body.ToolName) {
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
		return NativeEvent{}, fmt.Errorf("%w: 宿主 codex 不支持事件 %q", ErrUnsupportedHookTarget, eventName)
	}

	return event, nil
}

func (a *CodexAdapter) Encode(eventName string, decision NativeDecision) (stdout []byte, stderr string, exitCode int, err error) {
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
		resp := map[string]any{}
		if decision.AdditionalContext != "" {
			resp["hookSpecificOutput"] = map[string]any{
				"hookEventName":     "UserPromptSubmit",
				"additionalContext": decision.AdditionalContext,
			}
			stderr = fmt.Sprintf("已为本次提交生成上下文信封: %s", filepath.Base(decision.AdditionalContext))
		}
		if decision.Block {
			stderr = decision.Reason
		}
		stdout, _ = json.Marshal(resp)

	case "PostToolUse":
		resp := map[string]any{}
		if decision.AdditionalContext != "" {
			resp["additional_context"] = fmt.Sprintf("Agate 已为本次请求生成上下文。请先读取：%s", decision.AdditionalContext)
		}
		stdout, _ = json.Marshal(resp)

	case "Stop":
		stdout = []byte("{}")

	default:
		stdout = []byte("{}")
	}

	return stdout, stderr, 0, nil
}

// extractTargetsFromApplyPatch 按 tool_input.command 与 patch/diff 核验新增、修改、移动、删除和多目标。
func extractTargetsFromApplyPatch(input map[string]any) []string {
	if input == nil {
		return nil
	}
	var chunks []string
	for _, k := range []string{"command", "patch", "diff", "content"} {
		if v, ok := input[k]; ok {
			chunks = append(chunks, fmt.Sprint(v))
		}
	}
	seen := make(map[string]bool)
	var targets []string
	for _, chunk := range chunks {
		for _, t := range extractTargetsFromPatch(chunk) {
			if !seen[t] {
				seen[t] = true
				targets = append(targets, t)
			}
		}
		// *** Add/Update/Delete/Move File 语法
		scanner := bufio.NewScanner(strings.NewReader(chunk))
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			var candidate string
			switch {
			case strings.HasPrefix(line, "*** Add File:"):
				candidate = strings.TrimSpace(strings.TrimPrefix(line, "*** Add File:"))
			case strings.HasPrefix(line, "*** Update File:"):
				candidate = strings.TrimSpace(strings.TrimPrefix(line, "*** Update File:"))
			case strings.HasPrefix(line, "*** Delete File:"):
				candidate = strings.TrimSpace(strings.TrimPrefix(line, "*** Delete File:"))
			case strings.HasPrefix(line, "*** Move File:"):
				rest := strings.TrimSpace(strings.TrimPrefix(line, "*** Move File:"))
				parts := strings.Split(rest, " -> ")
				if len(parts) == 2 {
					for _, p := range parts {
						p = strings.TrimSpace(p)
						if p != "" && !seen[p] {
							seen[p] = true
							targets = append(targets, filepath.ToSlash(filepath.Clean(p)))
						}
					}
					continue
				}
				candidate = rest
			}
			candidate = strings.TrimSpace(candidate)
			if candidate != "" && !seen[candidate] {
				seen[candidate] = true
				targets = append(targets, filepath.ToSlash(filepath.Clean(candidate)))
			}
		}
	}
	return targets
}

// extractTargetsFromPatch 从 git diff / patch 文本中提取涉及的所有目标文件路径。
func extractTargetsFromPatch(patchText string) []string {
	var targets []string
	seen := make(map[string]bool)
	scanner := bufio.NewScanner(strings.NewReader(patchText))
	for scanner.Scan() {
		line := scanner.Text()
		var candidate string
		if strings.HasPrefix(line, "+++ b/") {
			candidate = strings.TrimPrefix(line, "+++ b/")
		} else if strings.HasPrefix(line, "--- a/") {
			candidate = strings.TrimPrefix(line, "--- a/")
		} else if strings.HasPrefix(line, "diff --git a/") {
			parts := strings.Split(line, " b/")
			if len(parts) == 2 {
				candidate = parts[1]
			}
		}
		candidate = strings.TrimSpace(candidate)
		if candidate != "" && candidate != "/dev/null" && !seen[candidate] {
			seen[candidate] = true
			targets = append(targets, filepath.ToSlash(filepath.Clean(candidate)))
		}
	}
	return targets
}
