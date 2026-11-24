package prompt

import (
	"fmt"
	"sync"
)

// NativeEventKind 表示宿主原生事件类型。
type NativeEventKind string

const (
	NativeEventSubmit            NativeEventKind = "submit"
	NativeEventReadCompleted     NativeEventKind = "read_completed"
	NativeEventResponseCompleted NativeEventKind = "response_completed"
	NativeEventBeforeTool        NativeEventKind = "before_tool"
	NativeEventObserve           NativeEventKind = "observe"
)

// NativeAction 表示模型调用的工具动作分类。
type NativeAction string

const (
	NativeActionRead  NativeAction = "read"
	NativeActionEdit  NativeAction = "edit"
	NativeActionShell NativeAction = "shell"
	NativeActionMCP   NativeAction = "mcp"
	NativeActionOther NativeAction = "other"
)

// NativeEvent 是多宿主原生事件的统一内部表示。
type NativeEvent struct {
	Kind         NativeEventKind `json:"kind"`
	SessionID    string          `json:"session_id"`
	TurnID       string          `json:"turn_id"`
	EventID      string          `json:"event_id"`
	ToolUseID    string          `json:"tool_use_id"`
	ClaimedRoots []string        `json:"claimed_roots"`
	Action       NativeAction    `json:"action"`
	Targets      []string        `json:"targets"`
	Command      string          `json:"command"`
	Text         string          `json:"text"`
	TrustedUser  bool            `json:"trusted_user"`
	Success      bool            `json:"success"`
}

// NativeDecision 是 Agate 对原生事件的统一判定结果。
type NativeDecision struct {
	Block             bool   `json:"block"`
	Reason            string `json:"reason,omitempty"`
	AdditionalContext string `json:"additional_context,omitempty"`
}

// NativeAdapter 负责各客户端私有协议与统一 NativeEvent / NativeDecision 之间的双向编解码。
type NativeAdapter interface {
	Decode(eventName string, payload []byte) (NativeEvent, error)
	Encode(eventName string, decision NativeDecision) (stdout []byte, stderr string, exitCode int, err error)
}

// NativeEventOptions 包含触发原生事件所需的宿主标识、事件名、工作目录与原始载荷。
type NativeEventOptions struct {
	Host      string
	EventName string
	RootDir   string
	WorkDir   string
	RecordDir string // 供测试隔离指定私有记录目录
	Payload   []byte
}

// NativeEventResult 是输出给调用方的最终结果。
type NativeEventResult struct {
	Stdout   []byte
	Stderr   string
	ExitCode int
	Decision NativeDecision
}

var (
	nativeAdaptersMu sync.RWMutex
	nativeAdapters   = make(map[string]NativeAdapter)
)

// RegisterNativeAdapter 注册宿主原生适配器。
func RegisterNativeAdapter(host string, adapter NativeAdapter) {
	nativeAdaptersMu.Lock()
	defer nativeAdaptersMu.Unlock()
	nativeAdapters[host] = adapter
}

// GetNativeAdapter 获取指定宿主的原生适配器。
func GetNativeAdapter(host string) (NativeAdapter, bool) {
	nativeAdaptersMu.RLock()
	defer nativeAdaptersMu.RUnlock()
	ad, ok := nativeAdapters[host]
	return ad, ok
}

// HandleNativeEvent 统一处理多宿主原生 Hook 事件。
func HandleNativeEvent(opts NativeEventOptions) (NativeEventResult, error) {
	adapter, ok := GetNativeAdapter(opts.Host)
	if !ok {
		return NativeEventResult{ExitCode: 1}, fmt.Errorf("%w: 尚未核对宿主 %q 的原生适配器", ErrUnsupportedHookTarget, opts.Host)
	}

	event, err := adapter.Decode(opts.EventName, opts.Payload)
	if err != nil {
		decision := NativeDecision{Block: true, Reason: "无效载荷: " + err.Error()}
		stdout, stderr, exitCode, encErr := adapter.Encode(opts.EventName, decision)
		if encErr != nil {
			return NativeEventResult{ExitCode: 1}, err
		}
		return NativeEventResult{Stdout: stdout, Stderr: stderr, ExitCode: exitCode, Decision: decision}, err
	}

	decision, applyErr := ApplyNativeWorkflow(opts, event)
	if applyErr != nil {
		decision.Block = true
		if decision.Reason == "" {
			decision.Reason = applyErr.Error()
		}
	}

	stdout, stderr, exitCode, err := adapter.Encode(opts.EventName, decision)
	if err != nil {
		return NativeEventResult{ExitCode: 1}, err
	}

	return NativeEventResult{
		Stdout:   stdout,
		Stderr:   stderr,
		ExitCode: exitCode,
		Decision: decision,
	}, applyErr
}
