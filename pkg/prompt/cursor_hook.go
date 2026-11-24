package prompt

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

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

// CursorAdapter 实现 NativeAdapter 接口，处理 Cursor 专有协议。
type CursorAdapter struct{}

func init() {
	RegisterNativeAdapter("cursor", &CursorAdapter{})
}

func (a *CursorAdapter) Decode(eventName string, payload []byte) (NativeEvent, error) {
	switch eventName {
	case "beforeSubmitPrompt", "preToolUse", "postToolUse", "afterAgentResponse":
	default:
		return NativeEvent{}, fmt.Errorf("%w: 宿主 cursor 不支持事件 %q", ErrUnsupportedHookTarget, eventName)
	}
	var body cursorEventBody
	if err := json.Unmarshal(bytes.TrimSpace(payload), &body); err != nil {
		return NativeEvent{}, fmt.Errorf("无效载荷")
	}

	event := NativeEvent{
		SessionID: body.ConversationID,
		TurnID:    firstText(body.GenerationID, body.RequestID),
		EventID:   firstText(body.RequestID, body.GenerationID),
	}

	switch eventName {
	case "beforeSubmitPrompt":
		event.Kind = NativeEventSubmit
		event.Text = firstText(body.Prompt, body.UserPrompt, body.Message, body.Instruction, body.Content, body.Text)
		event.TrustedUser = strings.TrimSpace(body.Prompt) != "" || strings.TrimSpace(body.UserPrompt) != ""
		if strings.TrimSpace(event.Text) == "" {
			return event, fmt.Errorf("缺少自然语言指令")
		}

	case "preToolUse":
		event.Kind = NativeEventBeforeTool
		action, targets, command := classifyToolAction(body.ToolName, body.ToolInput)
		event.Action = action
		event.Command = command
		if len(targets) == 0 {
			if target := extractTargetFile(body); target != "" {
				targets = []string{target}
			}
		}
		event.Targets = targets

	case "postToolUse":
		target := extractTargetFile(body)
		if isReadToolName(body.ToolName) {
			event.Kind = NativeEventReadCompleted
			event.Action = NativeActionRead
			if target != "" {
				event.Targets = []string{target}
			}
			event.Success = true
		} else {
			event.Kind = NativeEventObserve
			event.Action = NativeActionOther
		}

	case "afterAgentResponse":
		event.Kind = NativeEventResponseCompleted
		event.Text = firstText(body.Text, body.Content)
	}

	return event, nil
}

func (a *CursorAdapter) Encode(eventName string, decision NativeDecision) (stdout []byte, stderr string, exitCode int, err error) {
	switch eventName {
	case "beforeSubmitPrompt":
		if decision.Block {
			stdout, _ = json.Marshal(map[string]any{"continue": false, "user_message": decision.Reason})
			stderr = decision.Reason
		} else {
			stdout, _ = json.Marshal(map[string]any{"continue": true})
			stderr = "编译已完成。beforeSubmitPrompt 不能改写提示词，上下文尚未交付。"
			if decision.AdditionalContext != "" {
				stderr = fmt.Sprintf("编译已完成。请读取上下文产物 %s，beforeSubmitPrompt 不能改写提示词，上下文尚未交付。", filepath.Base(decision.AdditionalContext))
			}
		}
	case "preToolUse":
		if decision.Block {
			stdout, _ = json.Marshal(map[string]any{"permission": "deny", "user_message": decision.Reason})
			stderr = decision.Reason
		} else {
			stdout, _ = json.Marshal(map[string]any{"permission": "allow"})
		}
	case "postToolUse":
		resp := map[string]any{}
		if decision.AdditionalContext != "" {
			resp["additional_context"] = fmt.Sprintf("Agate 已为本次请求生成上下文。请使用 Cursor 原生 Read 读取此文件后再继续：%s", decision.AdditionalContext)
		}
		stdout, _ = json.Marshal(resp)
	case "afterAgentResponse":
		stdout = []byte("{}")
	default:
		stdout = []byte("{}")
	}
	return stdout, stderr, 0, nil
}

func isShellToolName(name string) bool {
	tool := strings.ToLower(strings.TrimSpace(name))
	return tool == "shell" || tool == "bash" || tool == "run_command" || tool == "terminal" || tool == "execute_command"
}

func isMCPToolName(name string) bool {
	return strings.Contains(strings.ToLower(strings.TrimSpace(name)), "mcp")
}

func isReadToolName(name string) bool {
	tool := strings.ToLower(strings.TrimSpace(name))
	return tool == "read" || tool == "read_file"
}

func extractShellCommand(input map[string]any) string {
	if input == nil {
		return ""
	}
	for _, k := range []string{"command", "cmd", "commandline", "script"} {
		if v, ok := input[k]; ok {
			return fmt.Sprint(v)
		}
	}
	return ""
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
	case "beforeSubmitPrompt", "preToolUse", "postToolUse", "afterAgentResponse":
	default:
		return CursorEventResult{}, fmt.Errorf("%w: 宿主 cursor 不支持事件 %q", ErrUnsupportedHookTarget, opts.Event)
	}
	var body cursorEventBody
	if err := json.Unmarshal(bytes.TrimSpace(opts.Payload), &body); err != nil {
		return denied(opts.Event, "无效载荷"), fmt.Errorf("无效载荷")
	}

	workspace, err := filepath.Abs(firstText(opts.WorkDir, opts.RootDir, "."))
	if err != nil {
		return denied(opts.Event, "无法确定工作区"), err
	}
	recordDir, dirErr := runtimeRecordDir(firstText(opts.RootDir, workspace), opts.RecordDir)
	instruction := ""
	if opts.Event == "beforeSubmitPrompt" {
		instruction = firstText(body.Prompt, body.UserPrompt, body.Message, body.Instruction, body.Content, body.Text)
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

		compiled, err := CompilePrompt(CompileOptions{RootDir: firstText(opts.RootDir, workspace), Instruction: instruction, HookMode: true})
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
				if strings.TrimSpace(body.Prompt) == "执行" && firstText(body.GenerationID, body.RequestID) != "" {
					applyCursorApproval(recordDir, body.ConversationID, workspace, &state)
				}
			}
			if saveErr := SaveRequestState(recordDir, state); saveErr != nil {
				return denied(opts.Event, "请求记录写入失败"), saveErr
			}
			if saveErr := SaveSessionActiveRequest(recordDir, body.ConversationID, workspace, requestID); saveErr != nil {
				return denied(opts.Event, "会话请求关联写入失败"), saveErr
			}
		}
		stdout, _ := json.Marshal(map[string]any{"continue": true})
		return CursorEventResult{Stdout: stdout, Stderr: stderr, State: state}, nil
	case "preToolUse":
		if strings.TrimSpace(body.ConversationID) == "" {
			return denied(opts.Event, "缺少会话标识，拒绝工具调用"), nil
		}
		state, loadErr := loadOrEmpty(recordDir, requestID, dirErr)
		if isToolActionWrite(body.ToolName, body.ToolInput) {
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
			if !state.PlanConfirmed || len(state.ConfirmedFiles) == 0 ||
				(state.ApprovedPlanID != "" && !isActiveCursorRequest(recordDir, body, workspace, requestID)) {
				return denied(opts.Event, "尚未取得当前方案及目标文件的用户确认"), nil
			}
			if !isNativeFileWrite(body.ToolName) {
				return denied(opts.Event, "当前文件确认只适用于原生文件编辑工具"), nil
			}
			targetFile := extractTargetFile(body)
			if !isFileInConfirmedScope(targetFile, state.ConfirmedFiles, workspace) {
				result := denied(opts.Event, fmt.Sprintf("目标文件 %q 超出已确认方案范围 %v，请重新出 Plan 申请授权", targetFile, state.ConfirmedFiles))
				result.State = state
				return result, nil
			}
		}
		stdout, _ := json.Marshal(map[string]any{"permission": "allow"})
		return CursorEventResult{Stdout: stdout, State: state}, nil
	case "afterAgentResponse":
		state, loadErr := loadOrEmpty(recordDir, requestID, dirErr)
		result := CursorEventResult{Stdout: []byte("{}"), State: state}
		if loadErr != nil || state.DeliveryMode != "observed_read" || state.DeliveryEvidence == "" ||
			firstText(body.GenerationID, body.RequestID) == "" || !isActiveCursorRequest(recordDir, body, workspace, requestID) {
			return result, nil
		}
		plan := parseCursorPlan(body.Text, workspace)
		if plan == nil && state.PendingPlan == nil {
			return result, nil
		}
		state.PendingPlan = plan
		if err := SaveRequestState(recordDir, state); err != nil {
			return result, err
		}
		result.State = state
		return result, nil
	case "postToolUse":
		state, loadErr := loadOrEmpty(recordDir, requestID, dirErr)
		if loadErr != nil {
			return CursorEventResult{Stderr: "没有找到本次请求记录，不能更新交付状态。"}, fmt.Errorf("没有找到本次请求记录")
		}
		response := map[string]any{}
		if isContextReadForRequest(body, state) {
			evidenceName := requestID + ".context.md"
			if state.ContextPath != "" {
				evidenceName = filepath.Base(state.ContextPath)
			}
			state.DeliveryMode = "observed_read"
			state.DeliveryEvidence = "read:" + evidenceName
			if err := SaveRequestState(recordDir, state); err != nil {
				return CursorEventResult{Stdout: []byte("{}"), State: state}, err
			}
		} else if state.ContextPath != "" && state.DeliveryEvidence == "" && isActiveCursorRequest(recordDir, body, workspace, requestID) {
			response["additional_context"] = fmt.Sprintf("Agate 已为本次请求生成上下文。请使用 Cursor 原生 Read 读取此文件后再继续：%s", state.ContextPath)
		}
		stdout, _ := json.Marshal(response)
		return CursorEventResult{Stdout: stdout, State: state}, nil
	}
	return CursorEventResult{}, ErrUnsupportedHookTarget
}

func isActiveCursorRequest(dir string, body cursorEventBody, workspace, requestID string) bool {
	active, err := LoadSessionActiveRequest(dir, body.ConversationID, workspace)
	return err == nil && active == requestID
}

func applyCursorApproval(dir, conversation, workspace string, state *RequestState) {
	sourceID, err := LoadSessionActiveRequest(dir, conversation, workspace)
	if err != nil || sourceID == state.RequestID {
		return
	}
	source, err := LoadRequestState(dir, sourceID)
	if err != nil || source.WorkspaceID != state.WorkspaceID || source.RuleVersion != state.RuleVersion ||
		source.DeliveryMode != "observed_read" || source.DeliveryEvidence == "" || source.PendingPlan == nil {
		return
	}
	plan := source.PendingPlan
	now := time.Now().UTC()
	state.Diagnosis = plan.Diagnosis
	state.DiagnosisSummary = plan.Summary
	state.PlanConfirmed = true
	state.ConfirmedFiles = plan.Files
	state.ApprovedPlanID = sourceID
	state.ApprovedPlanHash = plan.ResponseHash
	state.ApprovedAt = &now
}

func parseCursorPlan(response, workspace string) *pendingPlan {
	const opening = "\n```agate-plan\n"
	text := "\n" + strings.ReplaceAll(response, "\r\n", "\n")
	if strings.Count(text, opening) != 1 {
		return nil
	}
	_, block, _ := strings.Cut(text, opening)
	body, rest, closed := strings.Cut(block+"\n", "\n```\n")
	if !closed || !strings.Contains(rest, "执行") {
		return nil
	}
	var plan pendingPlan
	decoder := json.NewDecoder(strings.NewReader(body))
	decoder.DisallowUnknownFields()
	if !json.Valid([]byte(body)) || decoder.Decode(&plan) != nil || len(plan.Files) == 0 ||
		(plan.Diagnosis != DiagnosisConfirmed && plan.Diagnosis != DiagnosisNotApplicable) {
		return nil
	}
	if plan.Diagnosis == DiagnosisConfirmed {
		if err := ValidateDiagnosisOutput(response, plan.Diagnosis); err != nil {
			return nil
		}
		plan.Summary, _, _ = strings.Cut(response, strings.TrimPrefix(opening, "\n"))
	}
	for i, file := range plan.Files {
		clean := filepath.Clean(filepath.FromSlash(file))
		if strings.TrimSpace(file) != file || clean == "." || !filepath.IsLocal(clean) ||
			!isFileInConfirmedScope(clean, []string{clean}, workspace) {
			return nil
		}
		plan.Files[i] = filepath.ToSlash(clean)
	}
	plan.ResponseHash = hashText(response)
	return &plan
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
		return filepath.Abs(override)
	}
	common, err := git.GetGitCommonDir(root)
	if err != nil {
		return "", err
	}
	return filepath.Abs(filepath.Join(common, "agate", "requests"))
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

func extractTargetFile(body cursorEventBody) string {
	if strings.TrimSpace(body.FilePath) != "" {
		return strings.TrimSpace(body.FilePath)
	}
	if body.ToolInput != nil {
		for _, k := range []string{"file_path", "path", "target_file", "file"} {
			if v, ok := body.ToolInput[k]; ok {
				return fmt.Sprint(v)
			}
		}
	}
	return ""
}

func isFileInConfirmedScope(target string, confirmedFiles []string, workDir string) bool {
	if strings.TrimSpace(target) == "" || strings.TrimSpace(workDir) == "" {
		return false
	}
	root, err := filepath.Abs(workDir)
	if err != nil {
		return false
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(root, target)
	}
	normTarget, err := filepath.Abs(target)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(root, normTarget)
	if err != nil || rel == "." || !filepath.IsLocal(rel) {
		return false
	}
	current := root
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			break
		}
		if err != nil || info.Mode()&os.ModeSymlink != 0 {
			return false
		}
		if current == normTarget && !info.Mode().IsRegular() {
			return false
		}
	}
	for _, cf := range confirmedFiles {
		if strings.TrimSpace(cf) == "" {
			continue
		}
		if !filepath.IsAbs(cf) {
			cf = filepath.Join(root, cf)
		}
		confirmed, err := filepath.Abs(cf)
		if err == nil && (normTarget == confirmed || (runtime.GOOS == "windows" && strings.EqualFold(normTarget, confirmed))) {
			return true
		}
	}
	return false
}

func isNativeFileWrite(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "write", "strreplace", "edit", "editnotebook", "delete",
		"write_file", "edit_file", "delete_file", "write_to_file", "replace_in_file":
		return true
	default:
		return false
	}
}

func isKnownReadToolName(name string) bool {
	tool := strings.ToLower(strings.TrimSpace(name))
	switch tool {
	case "read", "read_file", "read_file_content", "view_file", "open_file":
		return true
	default:
		return false
	}
}

// classifyToolAction 只把已知文件编辑工具标成 Edit。其余工具不做策略判断。
func classifyToolAction(toolName string, input map[string]any) (NativeAction, []string, string) {
	if isNativeFileWrite(toolName) {
		targets := []string{}
		if target := extractTargetFromMap(input); target != "" {
			targets = []string{target}
		}
		return NativeActionEdit, targets, ""
	}
	if isShellToolName(toolName) {
		return NativeActionShell, nil, extractShellCommand(input)
	}
	if isMCPToolName(toolName) {
		return NativeActionMCP, nil, ""
	}
	if isKnownReadToolName(toolName) || isReadToolName(toolName) {
		targets := []string{}
		if target := extractTargetFromMap(input); target != "" {
			targets = []string{target}
		}
		return NativeActionRead, targets, ""
	}
	return NativeActionOther, nil, ""
}

// isToolActionWrite 只认结构化文件编辑。MCP、Shell 与其它工具不进入文件确认门。
func isToolActionWrite(name string, _ map[string]any) bool {
	return isNativeFileWrite(name)
}

func isWriteTool(name string) bool {
	return isToolActionWrite(name, nil)
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
	expectedNorm, err := filepath.Abs(strings.TrimSpace(state.ContextPath))
	if err != nil || strings.TrimSpace(state.ContextPath) == "" {
		return false
	}
	for _, raw := range candidates {
		if strings.TrimSpace(raw) == "" || !filepath.IsAbs(raw) {
			continue
		}
		norm, err := filepath.Abs(raw)
		if err == nil && (norm == expectedNorm || (runtime.GOOS == "windows" && strings.EqualFold(norm, expectedNorm))) {
			return true
		}
	}
	return false
}
