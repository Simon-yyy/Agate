package prompt

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

// WindsurfAdapter 实现 NativeAdapter 接口，处理 Windsurf Cascade 专有协议。
type WindsurfAdapter struct{}

func init() {
	RegisterNativeAdapter("windsurf", &WindsurfAdapter{})
}

type windsurfEventBody struct {
	SessionID string
	TurnID    string
	Prompt    string
	Response  string
	FilePath  string
	Command   string
	ToolInput map[string]any
	Success   *bool
}

func decodeWindsurfBody(payload []byte) (windsurfEventBody, error) {
	var raw map[string]any
	if len(bytes.TrimSpace(payload)) == 0 {
		return windsurfEventBody{}, nil
	}
	if err := json.Unmarshal(bytes.TrimSpace(payload), &raw); err != nil {
		return windsurfEventBody{}, fmt.Errorf("无效载荷: %w", err)
	}

	body := windsurfEventBody{
		SessionID: firstMapString(raw, "trajectory_id", "session_id", "sessionId"),
		TurnID:    firstMapString(raw, "turn_id", "turnId"),
		Prompt:    firstMapString(raw, "prompt", "user_prompt"),
		Response:  firstMapString(raw, "response", "cascade_response"),
		FilePath:  firstMapString(raw, "file_path", "filePath", "path"),
		Command:   firstMapString(raw, "command", "cmd"),
		Success:   mapBoolPtr(raw, "success"),
	}
	if input := firstMapObject(raw, "tool_input", "toolInput"); input != nil {
		body.ToolInput = input
	}
	// 官方嵌套 tool_info
	if info := firstMapObject(raw, "tool_info", "toolInfo"); info != nil {
		if body.SessionID == "" {
			body.SessionID = firstMapString(info, "trajectory_id", "session_id", "sessionId")
		}
		if body.Prompt == "" {
			body.Prompt = firstMapString(info, "user_prompt", "prompt")
		}
		if body.FilePath == "" {
			body.FilePath = firstMapString(info, "file_path", "filePath", "path")
		}
		if body.Command == "" {
			body.Command = firstMapString(info, "command", "cmd")
		}
		if body.Response == "" {
			body.Response = firstMapString(info, "response", "cascade_response")
		}
		if body.ToolInput == nil {
			if nested := firstMapObject(info, "tool_input", "input", "args"); nested != nil {
				body.ToolInput = nested
			}
		}
	}
	return body, nil
}

func (a *WindsurfAdapter) Decode(eventName string, payload []byte) (NativeEvent, error) {
	body, err := decodeWindsurfBody(payload)
	if err != nil {
		return NativeEvent{}, err
	}

	event := NativeEvent{
		SessionID: body.SessionID,
		TurnID:    body.TurnID,
	}

	switch eventName {
	case "pre_user_prompt":
		event.Kind = NativeEventSubmit
		event.Text = body.Prompt
		event.TrustedUser = strings.TrimSpace(body.Prompt) != ""
		if strings.TrimSpace(event.Text) == "" {
			return event, fmt.Errorf("缺少自然语言指令")
		}

	case "pre_write_code":
		event.Kind = NativeEventBeforeTool
		event.Action = NativeActionEdit
		target := firstText(body.FilePath, extractTargetFromMap(body.ToolInput))
		if target != "" {
			event.Targets = []string{target}
		}

	case "pre_run_command":
		event.Kind = NativeEventBeforeTool
		event.Action = NativeActionShell
		event.Command = firstText(body.Command, extractShellCommand(body.ToolInput))

	case "post_read_code":
		target := firstText(body.FilePath, extractTargetFromMap(body.ToolInput))
		event.Kind = NativeEventReadCompleted
		event.Action = NativeActionRead
		if target != "" {
			event.Targets = []string{target}
		}
		event.Success = body.Success == nil || *body.Success

	case "post_cascade_response":
		event.Kind = NativeEventResponseCompleted
		event.Text = body.Response

	default:
		return NativeEvent{}, fmt.Errorf("%w: 宿主 windsurf 不支持事件 %q", ErrUnsupportedHookTarget, eventName)
	}

	return event, nil
}

func (a *WindsurfAdapter) Encode(eventName string, decision NativeDecision) (stdout []byte, stderr string, exitCode int, err error) {
	switch eventName {
	case "pre_write_code", "pre_run_command":
		if decision.Block {
			// Windsurf 预写拒绝契约：stderr 给出理由并以退出码 2 退出
			stderr = decision.Reason
			return nil, stderr, 2, nil
		}
		return []byte("{}"), "", 0, nil

	case "pre_user_prompt":
		if decision.AdditionalContext != "" {
			stderr = fmt.Sprintf("已为本次提交生成上下文信封: %s", filepath.Base(decision.AdditionalContext))
		}
		if decision.Block {
			stderr = decision.Reason
		}
		return []byte("{}"), stderr, 0, nil

	case "post_read_code":
		resp := map[string]any{}
		if decision.AdditionalContext != "" {
			resp["additional_context"] = decision.AdditionalContext
		}
		stdout, _ := json.Marshal(resp)
		return stdout, "", 0, nil

	case "post_cascade_response":
		return []byte("{}"), "", 0, nil

	default:
		return []byte("{}"), "", 0, nil
	}
}
