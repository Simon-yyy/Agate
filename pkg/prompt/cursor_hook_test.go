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

	// 校验 Write、Shell、MCP 等全部写路径均被拦截
	for _, toolName := range []string{"Write", "Shell", "mcp_filesystem_write"} {
		write := []byte(`{"conversation_id":"conv","generation_id":"gen","tool_name":"` + toolName + `"}`)
		blocked, err := HandleCursorEvent(CursorEventOptions{
			Host: "cursor", Event: "preToolUse", Payload: write, RecordDir: dir, WorkDir: dir,
		})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(blocked.Stdout), `"permission":"deny"`) {
			t.Fatalf("未交付且未诊断时 %s 应被拦住: %s", toolName, blocked.Stdout)
		}
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
	artifactName := filepath.Base(submitRes.State.ContextPath)
	read := []byte(`{"conversation_id":"conv","tool_name":"Read","file_path":".git/agate/requests/` + artifactName + `"}`)
	if _, err := HandleCursorEvent(CursorEventOptions{
		Host: "cursor", Event: "postToolUse", Payload: read, RecordDir: dir, WorkDir: dir,
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

	write := []byte(`{"conversation_id":"conv","tool_name":"Write"}`)
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
	if !strings.Contains(string(allowed.Stdout), `"permission":"allow"`) {
		t.Fatalf("诊断和交付都完成后应放行: %s", allowed.Stdout)
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
