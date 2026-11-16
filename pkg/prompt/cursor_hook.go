package prompt

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"agate/pkg/git"
)

// ErrUnsupportedHookTarget 表示 --host 或 --event 不在已核对协议支持范围内（应映射为用法错误）。
var ErrUnsupportedHookTarget = errors.New("不支持的宿主或事件")

// CursorEventOptions 是一次宿主原生事件的输入。
type CursorEventOptions struct {
	Host    string
	Event   string
	Payload []byte
	RootDir string
	WorkDir string
	// RecordDir 供测试指定私有记录目录。为空时使用 Git 公共目录下的 agate/requests。
	RecordDir string
}

// CursorEventResult 把协议响应和诊断信息分开。Stdout 只放该事件允许的 JSON。
type CursorEventResult struct {
	Stdout []byte
	Stderr string
	State  RequestState
}

type cursorEventBody struct {
	Prompt         string         `json:"prompt"`
	UserPrompt     string         `json:"user_prompt"`
	Message        string         `json:"message"`
	Instruction    string         `json:"instruction"`
	Content        string         `json:"content"`
	Text           string         `json:"text"`
	ConversationID string         `json:"conversation_id"`
	GenerationID   string         `json:"generation_id"`
	RequestID      string         `json:"request_id"`
	ToolName       string         `json:"tool_name"`
	ToolInput      map[string]any `json:"tool_input"`
	FilePath       string         `json:"file_path"`
}

// HandleCursorEvent 处理已核对的 Cursor 事件。其他宿主不在这里猜测协议。
func HandleCursorEvent(opts CursorEventOptions) (CursorEventResult, error) {
	if opts.Host != "cursor" {
		return CursorEventResult{}, fmt.Errorf("%w: 尚未核对宿主 %q 的原生事件协议", ErrUnsupportedHookTarget, opts.Host)
	}
	switch opts.Event {
	case "beforeSubmitPrompt", "preToolUse", "postToolUse":
	default:
		return CursorEventResult{}, fmt.Errorf("%w: 宿主 cursor 不支持事件 %q", ErrUnsupportedHookTarget, opts.Event)
	}
	var body cursorEventBody
	if err := json.Unmarshal(bytes.TrimSpace(opts.Payload), &body); err != nil {
		return denied(opts.Event, "无效载荷"), fmt.Errorf("无效载荷")
	}

	recordDir, dirErr := runtimeRecordDir(opts.RootDir, opts.RecordDir)
	instruction := firstText(body.Prompt, body.UserPrompt, body.Message, body.Instruction, body.Content, body.Text)
	workspace := opts.WorkDir
	if workspace == "" {
		workspace = opts.RootDir
	}
	requestID := resolveEventRequestID(recordDir, body, instruction, workspace, dirErr)

	switch opts.Event {
	case "beforeSubmitPrompt":
		if strings.TrimSpace(instruction) == "" {
			return denied(opts.Event, "缺少自然语言指令"), fmt.Errorf("缺少自然语言指令")
		}
		if dirErr == nil {
			if existing, loadErr := LoadRequestState(recordDir, requestID); loadErr == nil {
				if ReuseAllowed(existing, instruction, workspace) == nil {
					if existing.ContextPath != "" {
						if _, statErr := os.Stat(existing.ContextPath); statErr == nil {
							_ = SaveSessionActiveRequest(recordDir, body.ConversationID, workspace, requestID)
							stdout, _ := json.Marshal(map[string]any{"continue": true})
							stderr := "已复用同请求编译记录。"
							if existing.DeliveryEvidence == "" {
								stderr = fmt.Sprintf("编译已完成。请读取上下文产物 %s，beforeSubmitPrompt 不能改写提示词，上下文尚未交付。", filepath.Base(existing.ContextPath))
							}
							return CursorEventResult{Stdout: stdout, Stderr: stderr, State: existing}, nil
						}
					}
				}
			}
		}

		compiled, err := CompilePrompt(CompileOptions{RootDir: opts.RootDir, Instruction: instruction, HookMode: true})
		state := RequestState{
			RequestID:       requestID,
			InstructionHash: hashText(instruction),
			WorkspaceID:     hashText(workspace),
			RuleVersion:     RuleVersion,
			Diagnosis:       DiagnosisUndiagnosed,
			DeliveryMode:    "pending_read",
		}
		if err != nil || compiled == nil || strings.TrimSpace(compiled.CompiledPrompt) == "" {
			if err == nil {
				err = fmt.Errorf("编译失败")
			}
			state.CompileStatus = "failed"
			state.Failure = "编译失败"
			return denied(opts.Event, "编译失败"), err
		}
		state.CompileStatus = "compiled"
		stderr := "编译已完成。beforeSubmitPrompt 不能改写提示词，上下文尚未交付。"
		if dirErr != nil {
			state.Failure = "交付记录写入失败"
			stderr += "交付记录写入失败。"
		} else {
			contextPath, artifactErr := SaveContextArtifact(recordDir, requestID, compiled.CompiledPrompt)
			if artifactErr != nil {
				state.Failure = "上下文产物写入失败"
				stderr += "上下文产物写入失败。"
			} else {
				state.ContextPath = contextPath
				stderr = fmt.Sprintf("编译已完成。请读取上下文产物 %s，beforeSubmitPrompt 不能改写提示词，上下文尚未交付。", filepath.Base(contextPath))
			}
			if saveErr := SaveRequestState(recordDir, state); saveErr != nil {
				state.Failure = "交付记录写入失败"
				stderr += "交付记录写入失败。"
			}
			_ = SaveSessionActiveRequest(recordDir, body.ConversationID, workspace, requestID)
		}
		stdout, _ := json.Marshal(map[string]any{"continue": true})
		return CursorEventResult{Stdout: stdout, Stderr: stderr, State: state}, nil
	case "preToolUse":
		state, loadErr := loadOrEmpty(recordDir, requestID, dirErr)
		if isWriteTool(body.ToolName) {
			if loadErr != nil || state.DeliveryEvidence == "" {
				result := denied(opts.Event, "上下文尚未交付，不能执行写入")
				result.State = state
				if result.State.RequestID == "" {
					result.State.RequestID = requestID
				}
				return result, nil
			}
			if err := RepairPlanAllowed(state); err != nil {
				result := denied(opts.Event, err.Error())
				result.State = state
				return result, nil
			}
		}
		stdout, _ := json.Marshal(map[string]any{"permission": "allow"})
		return CursorEventResult{Stdout: stdout, State: state}, nil
	default:
		state, loadErr := loadOrEmpty(recordDir, requestID, dirErr)
		if loadErr != nil {
			return CursorEventResult{Stderr: "没有找到本次请求记录，不能更新交付状态。"}, fmt.Errorf("没有找到本次请求记录")
		}
		if isContextReadForRequest(body, state) {
			evidenceName := requestID + ".context.md"
			if state.ContextPath != "" {
				evidenceName = filepath.Base(state.ContextPath)
			}
			state.DeliveryMode = "observed_read"
			state.DeliveryEvidence = "read:" + evidenceName
			if dirErr == nil {
				_ = SaveRequestState(recordDir, state)
			}
		}
		stdout, _ := json.Marshal(map[string]any{})
		return CursorEventResult{Stdout: stdout, State: state}, nil
	}
}

func resolveEventRequestID(recordDir string, body cursorEventBody, instruction, workspace string, dirErr error) string {
	explicitKey := firstText(body.GenerationID, body.RequestID, instruction)
	if explicitKey != "" {
		return RecordID(body.ConversationID, explicitKey, workspace)
	}
	if dirErr == nil && strings.TrimSpace(body.ConversationID) != "" {
		if activeID, err := LoadSessionActiveRequest(recordDir, body.ConversationID, workspace); err == nil {
			return activeID
		}
	}
	return RecordID(body.ConversationID, "", workspace)
}

func runtimeRecordDir(root, override string) (string, error) {
	if strings.TrimSpace(override) != "" {
		return override, nil
	}
	common, err := git.GetGitCommonDir(root)
	if err != nil {
		return "", err
	}
	return filepath.Join(common, "agate", "requests"), nil
}

func loadOrEmpty(dir, requestID string, dirErr error) (RequestState, error) {
	if dirErr != nil {
		return RequestState{}, dirErr
	}
	return LoadRequestState(dir, requestID)
}

func denied(event, message string) CursorEventResult {
	var payload map[string]any
	if event == "preToolUse" {
		payload = map[string]any{"permission": "deny", "user_message": message}
	} else if event == "beforeSubmitPrompt" {
		payload = map[string]any{"continue": false, "user_message": message}
	} else {
		payload = map[string]any{}
	}
	stdout, _ := json.Marshal(payload)
	return CursorEventResult{Stdout: stdout, Stderr: message}
}

func firstText(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func isWriteTool(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "write", "strreplace", "edit", "editnotebook", "delete", "shell", "bash":
		return true
	default:
		lower := strings.ToLower(name)
		return strings.Contains(lower, "mcp")
	}
}

func isContextReadForRequest(body cursorEventBody, state RequestState) bool {
	if !strings.EqualFold(body.ToolName, "read") && !strings.EqualFold(body.ToolName, "read_file") {
		return false
	}
	var candidates []string
	if strings.TrimSpace(body.FilePath) != "" {
		candidates = append(candidates, body.FilePath)
	}
	for _, key := range []string{"file_path", "path", "target_file"} {
		if val, ok := body.ToolInput[key]; ok {
			candidates = append(candidates, fmt.Sprint(val))
		}
	}
	expectedName := state.RequestID + ".context.md"
	if state.ContextPath != "" {
		expectedName = filepath.Base(state.ContextPath)
	}
	expectedNorm := filepath.ToSlash(strings.ToLower(strings.TrimSpace(state.ContextPath)))
	for _, raw := range candidates {
		norm := filepath.ToSlash(strings.ToLower(strings.TrimSpace(raw)))
		if norm == "" {
			continue
		}
		if expectedNorm != "" && norm == expectedNorm {
			return true
		}
		if strings.HasSuffix(norm, "/"+strings.ToLower(expectedName)) || norm == strings.ToLower(expectedName) {
			return true
		}
	}
	return false
}
