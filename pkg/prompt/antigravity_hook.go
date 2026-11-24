package prompt

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// AntigravityAdapter 实现 NativeAdapter 接口，处理 Antigravity IDE 专有协议。
type AntigravityAdapter struct{}

func init() {
	RegisterNativeAdapter("antigravity", &AntigravityAdapter{})
}

type antigravityToolCall struct {
	Name string         `json:"name"`
	Args map[string]any `json:"args"`
}

type antigravityEventBody struct {
	ConversationID  string              `json:"conversationId"`
	InvocationID    string              `json:"invocationId"`
	InvocationNum   json.RawMessage     `json:"invocationNum"`
	LastUserInput   string              `json:"lastUserInput"`
	Prompt          string              `json:"prompt"`
	Response        string              `json:"response"`
	TranscriptPath  string              `json:"transcriptPath"`
	InitialNumSteps json.RawMessage     `json:"initialNumSteps"`
	WorkspacePaths  []string            `json:"workspacePaths"`
	ToolCall        antigravityToolCall `json:"toolCall"`
	ToolOutput      string              `json:"toolOutput"`
	Success         *bool               `json:"success"`
	Error           string              `json:"error"`
}

func (a *AntigravityAdapter) Decode(eventName string, payload []byte) (NativeEvent, error) {
	var body antigravityEventBody
	if len(bytes.TrimSpace(payload)) > 0 {
		if err := json.Unmarshal(bytes.TrimSpace(payload), &body); err != nil {
			return NativeEvent{}, fmt.Errorf("无效载荷: %w", err)
		}
	}

	event := NativeEvent{
		SessionID:    body.ConversationID,
		TurnID:       body.InvocationID,
		ClaimedRoots: body.WorkspacePaths,
	}

	switch eventName {
	case "PreInvocation":
		// 显式 prompt 是可信新提交。官方 invocationNum 从 0 起算，字段存在且为 0、
		// lastUserInput 非空时也是新一轮。大于 0 是同一请求的续跑。字段缺失不等于 0。
		prompt := strings.TrimSpace(body.Prompt)
		callNum, hasNum := antigravityInvocationNum(body.InvocationNum)
		if prompt == "" {
			if !hasNum || callNum > 0 {
				event.Kind = NativeEventObserve
				event.Text = body.LastUserInput
				event.TrustedUser = false
				return event, nil
			}
			prompt = strings.TrimSpace(body.LastUserInput)
		}
		if prompt == "" {
			event.Kind = NativeEventObserve
			event.Text = body.LastUserInput
			event.TrustedUser = false
			return event, nil
		}
		event.Kind = NativeEventSubmit
		event.Text = prompt
		event.TrustedUser = true

	case "PreToolUse":
		event.Kind = NativeEventBeforeTool
		action, targets, command := classifyToolAction(body.ToolCall.Name, body.ToolCall.Args)
		event.Action = action
		event.Command = command
		if len(targets) == 0 {
			if target := extractAntigravityTarget(body.ToolCall.Args); target != "" {
				targets = []string{target}
			}
		}
		event.Targets = targets

	case "PostToolUse":
		toolName := body.ToolCall.Name
		target := extractAntigravityTarget(body.ToolCall.Args)
		if isKnownReadToolName(toolName) || isReadToolName(toolName) {
			event.Kind = NativeEventReadCompleted
			event.Action = NativeActionRead
			if target != "" {
				event.Targets = []string{target}
			}
			event.Success = antigravityToolSucceeded(body)
		} else {
			event.Kind = NativeEventObserve
			event.Action = NativeActionOther
		}

	case "Stop":
		event.Kind = NativeEventResponseCompleted
		event.Text = body.Response

	case "PostInvocation":
		event.Kind = NativeEventResponseCompleted
		// 显式 response 优先。IDE 2.8.0 不传该字段，回复在 transcriptPath 的 PLANNER_RESPONSE 里。
		event.Text = antigravityModelReply(body)

	default:
		return NativeEvent{}, fmt.Errorf("%w: 宿主 antigravity 不支持事件 %q", ErrUnsupportedHookTarget, eventName)
	}

	return event, nil
}

func (a *AntigravityAdapter) Encode(eventName string, decision NativeDecision) (stdout []byte, stderr string, exitCode int, err error) {
	switch eventName {
	case "PreToolUse":
		if decision.Block {
			resp := map[string]any{
				"decision": "deny",
				"reason":   decision.Reason,
			}
			stdout, _ = json.Marshal(resp)
			stderr = decision.Reason
		} else {
			resp := map[string]any{
				"decision": "allow",
			}
			stdout, _ = json.Marshal(resp)
		}

	case "PreInvocation":
		// 官方 stdout 只有 injectSteps 与 terminationBehavior。省略后者表示继续调用模型。
		resp := map[string]any{}
		if decision.Block {
			resp["terminationBehavior"] = "terminate"
			resp["injectSteps"] = []any{
				map[string]any{
					"errorMessage": map[string]any{
						"userMessage": decision.Reason,
					},
				},
			}
			stderr = decision.Reason
		} else if decision.AdditionalContext != "" {
			resp["injectSteps"] = []any{
				map[string]any{
					"systemMessage": map[string]any{
						"systemMessage": fmt.Sprintf("Agate 已为本次请求生成上下文。请先读取：%s", decision.AdditionalContext),
					},
				},
			}
			stderr = fmt.Sprintf("已为本次调用生成上下文产物: %s", filepath.Base(decision.AdditionalContext))
		}
		stdout, _ = json.Marshal(resp)

	case "PostToolUse":
		stdout = []byte("{}")

	case "Stop", "PostInvocation":
		stdout = []byte("{}")

	default:
		stdout = []byte("{}")
	}

	return stdout, stderr, 0, nil
}

func antigravityToolSucceeded(body antigravityEventBody) bool {
	if strings.TrimSpace(body.Error) != "" {
		return false
	}
	if body.Success != nil {
		return *body.Success
	}
	return true
}

func antigravityModelReply(body antigravityEventBody) string {
	if strings.TrimSpace(body.Response) != "" {
		return body.Response
	}
	limit, ok := antigravityInvocationNum(body.InitialNumSteps)
	path := strings.TrimSpace(body.TranscriptPath)
	if !ok || path == "" {
		return ""
	}
	return antigravityPlannerContent(path, limit)
}

func antigravityPlannerContent(path string, maxStep int) string {
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 8*1024*1024)
	latest := ""
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var step struct {
			StepIndex int    `json:"step_index"`
			Source    string `json:"source"`
			Type      string `json:"type"`
			Content   string `json:"content"`
		}
		if json.Unmarshal(line, &step) != nil {
			continue
		}
		if step.Source != "MODEL" || step.Type != "PLANNER_RESPONSE" || step.StepIndex > maxStep {
			continue
		}
		if strings.TrimSpace(step.Content) == "" {
			continue
		}
		latest = step.Content
	}
	return latest
}

func antigravityInvocationNum(raw json.RawMessage) (int, bool) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return 0, false
	}
	var n int
	if json.Unmarshal(raw, &n) == nil {
		return n, true
	}
	var f float64
	if json.Unmarshal(raw, &f) == nil {
		return int(f), true
	}
	return 0, false
}

func extractAntigravityTarget(args map[string]any) string {
	if args == nil {
		return ""
	}
	for _, k := range []string{"Path", "path", "filePath", "file_path", "target", "target_file", "AbsolutePath", "TargetFile"} {
		if v, ok := args[k]; ok {
			return fmt.Sprint(v)
		}
	}
	return ""
}
