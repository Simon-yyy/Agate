package prompt

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCursorHookCompilesWithoutRewritingPrompt(t *testing.T) {
	dir := t.TempDir()
	payload := []byte(`{"conversation_id":"conv/1","generation_id":"gen/1","prompt":"为什么 Windows 认不出本地编辑器"}`)
	result, err := HandleCursorEvent(CursorEventOptions{
		Host: "cursor", Event: "beforeSubmitPrompt", Payload: payload, RecordDir: dir, WorkDir: dir,
	})
	if err != nil {
		t.Fatal(err)
	}
	var response map[string]any
	if err := json.Unmarshal(result.Stdout, &response); err != nil {
		t.Fatal(err)
	}
	if response["continue"] != true {
		t.Fatalf("beforeSubmitPrompt 应允许提交，实际 %s", result.Stdout)
	}
	if _, ok := response["prompt"]; ok {
		t.Fatal("不能在 beforeSubmitPrompt 中改写提示词")
	}
	if strings.Contains(string(result.Stdout), "agate-intent") {
		t.Fatal("标准输出不应把执行信封当成已经交付")
	}
	if !strings.Contains(result.Stderr, "尚未交付") {
		t.Fatalf("应说明上下文尚未交付: %s", result.Stderr)
	}
	if result.State.CompileStatus != "compiled" || result.State.Diagnosis != DiagnosisUndiagnosed {
		t.Fatalf("编译完成不等于诊断完成: %+v", result.State)
	}
	if result.State.ContextPath == "" {
		t.Fatal("beforeSubmitPrompt 必须将编译后的上下文产物落盘")
	}
	artifactData, readErr := os.ReadFile(result.State.ContextPath)
	if readErr != nil || !strings.Contains(string(artifactData), "agate-intent") {
		t.Fatalf("上下文产物应包含编译信封，err=%v", readErr)
	}

	again, err := HandleCursorEvent(CursorEventOptions{
		Host: "cursor", Event: "beforeSubmitPrompt", Payload: payload, RecordDir: dir, WorkDir: dir,
	})
	if err != nil || again.State.RequestID != result.State.RequestID {
		t.Fatalf("重复事件应幂等，err=%v id=%s/%s", err, again.State.RequestID, result.State.RequestID)
	}
}

func TestCursorHookBlocksWriteUntilDiagnosisAndDelivery(t *testing.T) {
	dir := t.TempDir()
	base := `{"conversation_id":"conv","generation_id":"gen","prompt":"修复祖先进程采集"}`
	submitRes, err := HandleCursorEvent(CursorEventOptions{
		Host: "cursor", Event: "beforeSubmitPrompt", Payload: []byte(base), RecordDir: dir, WorkDir: dir,
	})
	if err != nil {
		t.Fatal(err)
	}

	// 文件编辑在未交付时被拦截。MCP 不因工具名进入这道门。
	write := []byte(`{"conversation_id":"conv","generation_id":"gen","tool_name":"Write"}`)
	blocked, err := HandleCursorEvent(CursorEventOptions{
		Host: "cursor", Event: "preToolUse", Payload: write, RecordDir: dir, WorkDir: dir,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(blocked.Stdout), `"permission":"deny"`) {
		t.Fatalf("未交付且未诊断时 Write 应被拦住: %s", blocked.Stdout)
	}
	mcp := []byte(`{"conversation_id":"conv","generation_id":"gen","tool_name":"mcp_filesystem_write"}`)
	mcpRes, err := HandleCursorEvent(CursorEventOptions{
		Host: "cursor", Event: "preToolUse", Payload: mcp, RecordDir: dir, WorkDir: dir,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(mcpRes.Stdout), `"permission":"allow"`) {
		t.Fatalf("MCP 不应因工具名被文件门拦住: %s", mcpRes.Stdout)
	}

	// 读取无关文件不能误标记交付
	unrelatedRead := []byte(`{"conversation_id":"conv","generation_id":"gen","tool_name":"Read","file_path":"README.md"}`)
	if _, err := HandleCursorEvent(CursorEventOptions{
		Host: "cursor", Event: "postToolUse", Payload: unrelatedRead, RecordDir: dir, WorkDir: dir,
	}); err != nil {
		t.Fatal(err)
	}
	stillUnDelivered, err := HandleCursorEvent(CursorEventOptions{
		Host: "cursor", Event: "preToolUse", Payload: []byte(`{"conversation_id":"conv","generation_id":"gen","tool_name":"Write"}`), RecordDir: dir, WorkDir: dir,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(stillUnDelivered.Stdout), "上下文尚未交付") {
		t.Fatalf("读取无关文件不应标记交付，实际: %s", stillUnDelivered.Stdout)
	}

	// 读取本次请求的上下文产物（不带 generation_id，测试会话活跃指针关联）
	readPayload, err := json.Marshal(map[string]any{"conversation_id": "conv", "tool_name": "Read", "file_path": submitRes.State.ContextPath})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := HandleCursorEvent(CursorEventOptions{
		Host: "cursor", Event: "postToolUse", Payload: readPayload, RecordDir: dir, WorkDir: dir,
	}); err != nil {
		t.Fatal(err)
	}

	// 重复的 beforeSubmitPrompt 不应抹除已交付状态
	repeatRes, err := HandleCursorEvent(CursorEventOptions{
		Host: "cursor", Event: "beforeSubmitPrompt", Payload: []byte(base), RecordDir: dir, WorkDir: dir,
	})
	if err != nil || repeatRes.State.DeliveryEvidence == "" {
		t.Fatalf("重复事件不应抹除已交付证据: err=%v state=%+v", err, repeatRes.State)
	}

	write = []byte(`{"conversation_id":"conv","tool_name":"Write"}`)
	stillBlocked, err := HandleCursorEvent(CursorEventOptions{
		Host: "cursor", Event: "preToolUse", Payload: write, RecordDir: dir, WorkDir: dir,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(stillBlocked.Stdout), "未诊断") && !strings.Contains(stillBlocked.Stderr, "未诊断") {
		t.Fatalf("只完成交付不能跳过诊断: stdout=%s stderr=%s", stillBlocked.Stdout, stillBlocked.Stderr)
	}

	evidence := "现象：Windows 宿主未识别。位置与证据：pkg/agent/collector.go 仅实现 Linux。原因：未调用 Windows 快照 API 导致。不确定项：已确认。依据：新增单测验证。"
	if _, err := UpdateDiagnosisState(dir, stillBlocked.State.RequestID, DiagnosisConfirmed, evidence); err != nil {
		t.Fatal(err)
	}
	allowed, err := HandleCursorEvent(CursorEventOptions{
		Host: "cursor", Event: "preToolUse", Payload: write, RecordDir: dir, WorkDir: dir,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(allowed.Stdout), `"permission":"deny"`) {
		t.Fatalf("诊断和交付完成但用户尚未确认时应阻断: %s", allowed.Stdout)
	}
}

func TestContextReadRejectsMatchingBasenameInOtherDirectory(t *testing.T) {
	state := RequestState{RequestID: "request", ContextPath: filepath.Join(t.TempDir(), "request.context.md")}
	body := cursorEventBody{ToolName: "Read", FilePath: filepath.Join(t.TempDir(), "request.context.md")}
	if isContextReadForRequest(body, state) {
		t.Fatal("其他目录的同名文件不能作为读取回执")
	}
}

func TestCursorApprovalFollowsCurrentDeliveredPlan(t *testing.T) {
	workspace := t.TempDir()
	records := t.TempDir()
	call := func(event, conversation, generation string, fields map[string]any) CursorEventResult {
		t.Helper()
		fields["conversation_id"] = conversation
		fields["generation_id"] = generation
		payload, err := json.Marshal(fields)
		if err != nil {
			t.Fatal(err)
		}
		result, err := HandleCursorEvent(CursorEventOptions{
			Host: "cursor", Event: event, Payload: payload, RecordDir: records, WorkDir: workspace,
		})
		if err != nil {
			t.Fatalf("%s: %v", event, err)
		}
		return result
	}
	read := func(generation, path string) CursorEventResult {
		return call("postToolUse", "conv", generation, map[string]any{
			"tool_name": "Read", "tool_input": map[string]any{"path": path},
		})
	}
	assertPermission := func(conversation, generation, tool, path, want string) {
		t.Helper()
		result := call("preToolUse", conversation, generation, map[string]any{
			"tool_name": tool, "tool_input": map[string]any{"path": path},
		})
		var response struct{ Permission string }
		if err := json.Unmarshal(result.Stdout, &response); err != nil || response.Permission != want {
			t.Fatalf("%s %s: 期望 %s，得到 %s", tool, path, want, result.Stdout)
		}
	}
	proposal := call("beforeSubmitPrompt", "conv", "one", map[string]any{"prompt": "给 sample.txt 增加一行"})
	plan := "拟修改 sample.txt，追加一行。\n\n```agate-plan\n{\"files\":[\"sample.txt\"],\"diagnosis\":\"not_applicable\"}\n```\n\n请回复‘执行’。"
	response := call("afterAgentResponse", "conv", "one", map[string]any{"text": plan})
	if response.State.PendingPlan != nil {
		t.Fatal("未交付本次上下文时不能登记方案")
	}
	assertPermission("conv", "one", "Read", "sample.txt", "allow")
	if result := read("one", filepath.Join(workspace, "sample.txt")); !strings.Contains(string(result.Stdout), "additional_context") {
		t.Fatal("普通读取后应提供当前请求的绝对路径")
	}
	read("one", proposal.State.ContextPath)
	hypothesis := strings.Replace(plan, "not_applicable", "hypothesis", 1)
	if result := call("afterAgentResponse", "conv", "one", map[string]any{"text": hypothesis}); result.State.PendingPlan != nil {
		t.Fatal("待验证假设不能登记成待确认修复方案")
	}
	call("afterAgentResponse", "conv", "one", map[string]any{"text": plan})
	assertPermission("conv", "one", "Write", "sample.txt", "deny")
	other := call("beforeSubmitPrompt", "other", "two", map[string]any{"prompt": "执行"})
	if other.State.PlanConfirmed {
		t.Fatal("不应跨会话绑定方案")
	}
	originalWorkspace := workspace
	workspace = t.TempDir()
	other = call("beforeSubmitPrompt", "conv", "two", map[string]any{"prompt": "执行"})
	if other.State.PlanConfirmed {
		t.Fatal("不应跨工作区绑定方案")
	}
	workspace = originalWorkspace
	confirmation := call("beforeSubmitPrompt", "conv", "two", map[string]any{"prompt": "执行"})
	if !confirmation.State.PlanConfirmed || confirmation.State.ApprovedPlanID != proposal.State.RequestID {
		t.Fatalf("确认未绑定到当前方案: %+v", confirmation.State)
	}
	read("two", proposal.State.ContextPath)
	assertPermission("conv", "two", "Write", "sample.txt", "deny")
	read("two", confirmation.State.ContextPath)
	assertPermission("conv", "two", "Write", "sample.txt", "allow")
	assertPermission("conv", "two", "Write", "other.txt", "deny")
	assertPermission("conv", "two", "Shell", "sample.txt", "allow")
	call("beforeSubmitPrompt", "conv", "three", map[string]any{"prompt": "改成 other.txt"})
	call("beforeSubmitPrompt", "conv", "two", map[string]any{"prompt": "执行"})
	assertPermission("conv", "two", "Write", "sample.txt", "deny")
	call("afterAgentResponse", "conv", "one", map[string]any{"text": plan})
	replay := call("beforeSubmitPrompt", "conv", "four", map[string]any{"prompt": "执行"})
	if replay.State.PlanConfirmed {
		t.Fatal("方案调整后不能重放旧授权")
	}
}

func TestCursorHookRejectsUnknownHostAndInvalidPayload(t *testing.T) {
	if _, err := HandleCursorEvent(CursorEventOptions{Host: "windsurf", Event: "beforeSubmitPrompt", Payload: []byte(`{}`)}); err == nil {
		t.Fatal("未核对宿主不应静默成功")
	}
	result, err := HandleCursorEvent(CursorEventOptions{Host: "cursor", Event: "beforeSubmitPrompt", Payload: []byte(`not-json`), RecordDir: t.TempDir()})
	if err == nil || !strings.Contains(string(result.Stdout), `"continue":false`) {
		t.Fatalf("无效载荷应明确失败，err=%v stdout=%s", err, result.Stdout)
	}
}

func TestCursorPreToolUseRejectsMissingConversation(t *testing.T) {
	dir := t.TempDir()
	for _, toolName := range []string{"Write", "Shell", "Read", "mcp_filesystem_write"} {
		payload, err := json.Marshal(map[string]any{
			"tool_name":  toolName,
			"tool_input": map[string]any{"command": "git status"},
		})
		if err != nil {
			t.Fatal(err)
		}
		res, err := HandleCursorEvent(CursorEventOptions{
			Host: "cursor", Event: "preToolUse", Payload: payload, RecordDir: dir, WorkDir: dir,
		})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(res.Stdout), `"permission":"deny"`) || !strings.Contains(string(res.Stdout), "拒绝工具调用") {
			t.Fatalf("没有会话标识时 %s 应拒绝，实际: %s", toolName, res.Stdout)
		}
	}
}

func TestCursorPreToolUse_ShellRequiresConfirmation(t *testing.T) {
	dir := t.TempDir()
	for _, command := range []string{"git status", "echo foo > bar.txt", "agate verify"} {
		payload, err := json.Marshal(map[string]any{
			"conversation_id": "conv-shell",
			"generation_id":   "gen-shell",
			"tool_name":       "Shell",
			"tool_input":      map[string]any{"command": command},
		})
		if err != nil {
			t.Fatal(err)
		}
		res, err := HandleCursorEvent(CursorEventOptions{
			Host: "cursor", Event: "preToolUse", Payload: payload, RecordDir: dir, WorkDir: dir,
		})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(res.Stdout), `"permission":"allow"`) {
			t.Fatalf("Shell %q 应在诊断检查之前放行，实际: %s", command, res.Stdout)
		}
	}
}

func TestCursorPreToolUse_ConfirmedFileScope(t *testing.T) {
	dir := t.TempDir()
	convID := "conv-scope"
	genID := "gen-scope"
	reqID := RecordID(convID, genID, dir)
	// 保存已确认方案，范围限定为 pkg/agent/profile.go
	state := RequestState{
		RequestID:        reqID,
		InstructionHash:  hashText("修改 profile"),
		WorkspaceID:      hashText(dir),
		RuleVersion:      RuleVersion,
		CompileStatus:    "compiled",
		DeliveryMode:     "observed_read",
		DeliveryEvidence: "read:context.md",
		Diagnosis:        DiagnosisConfirmed,
		PlanConfirmed:    true,
		ConfirmedFiles:   []string{"pkg/agent/profile.go"},
	}
	if err := SaveRequestState(dir, state); err != nil {
		t.Fatal(err)
	}

	// 1. 修改在范围内的文件 -> 放行
	inScopePayload := []byte(`{"conversation_id":"` + convID + `","generation_id":"` + genID + `","tool_name":"Edit","file_path":"pkg/agent/profile.go"}`)
	inRes, err := HandleCursorEvent(CursorEventOptions{
		Host: "cursor", Event: "preToolUse", Payload: inScopePayload, RecordDir: dir, WorkDir: dir,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(inRes.Stdout), `"permission":"allow"`) {
		t.Fatalf("范围内文件应当被放行，实际: %s", inRes.Stdout)
	}

	// 2. 修改超出范围的文件 -> 阻断
	outScopePayload := []byte(`{"conversation_id":"` + convID + `","generation_id":"` + genID + `","tool_name":"Edit","file_path":"cmd/init.go"}`)
	outRes, err := HandleCursorEvent(CursorEventOptions{
		Host: "cursor", Event: "preToolUse", Payload: outScopePayload, RecordDir: dir, WorkDir: dir,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(outRes.Stdout), `"permission":"deny"`) || !strings.Contains(string(outRes.Stdout), "超出已确认方案范围") {
		t.Fatalf("超范围文件应当被拦截，实际: %s", outRes.Stdout)
	}
}
