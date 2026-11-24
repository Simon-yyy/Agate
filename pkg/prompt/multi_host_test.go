package prompt

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAntigravityAdapterProtocol(t *testing.T) {
	tempDir := t.TempDir()
	opts := NativeEventOptions{
		Host:      "antigravity",
		RootDir:   tempDir,
		WorkDir:   tempDir,
		RecordDir: filepath.Join(tempDir, "records"),
	}

	// 1. PreInvocation 提交应返回 injectSteps
	payloadSubmit := []byte(`{"conversationId":"ag-1","invocationId":"inv-1","prompt":"修复 Antigravity 采集"}`)
	opts.EventName = "PreInvocation"
	opts.Payload = payloadSubmit
	res, err := HandleNativeEvent(opts)
	if err != nil {
		t.Fatalf("PreInvocation 处理失败: %v", err)
	}
	if strings.Contains(string(res.Stdout), `"continue"`) {
		t.Fatalf("PreInvocation 不应返回 continue，实际: %s", res.Stdout)
	}
	firstPath := antigravityContextPath(t, res.Stdout)
	if firstPath == "" {
		t.Fatalf("显式 prompt 应通过 ephemeralMessage 交付上下文路径，实际: %s", res.Stdout)
	}

	// 官方首次调用：invocationNum=0 且 lastUserInput
	opts.Payload = []byte(`{"conversationId":"ag-official","invocationNum":0,"lastUserInput":"修复官方样本"}`)
	res, err = HandleNativeEvent(opts)
	if err != nil {
		t.Fatalf("官方首次 PreInvocation 失败: %v", err)
	}
	officialPath := antigravityContextPath(t, res.Stdout)

	// 大于 0 的同文本续跑不得重新注入
	opts.Payload = []byte(`{"conversationId":"ag-official","invocationNum":1,"lastUserInput":"修复官方样本"}`)
	res, err = HandleNativeEvent(opts)
	if err != nil {
		t.Fatalf("续跑 PreInvocation 不应报错: %v", err)
	}
	if strings.Contains(string(res.Stdout), `"injectSteps"`) {
		t.Fatalf("invocationNum>0 不应重新注入上下文: %s", res.Stdout)
	}

	// 同一会话更换用户输入，编号回到 0，应交付新的上下文
	opts.Payload = []byte(`{"conversationId":"ag-official","invocationNum":0,"lastUserInput":"更换后的需求"}`)
	res, err = HandleNativeEvent(opts)
	if err != nil {
		t.Fatalf("后续独立提交失败: %v", err)
	}
	nextPath := antigravityContextPath(t, res.Stdout)
	if nextPath == officialPath {
		t.Fatalf("更换用户输入后应生成新的上下文路径，实际仍为 %s", nextPath)
	}

	// 仅 lastUserInput、没有序号时不得当成新提交
	opts.Payload = []byte(`{"conversationId":"ag-1","invocationId":"inv-cont","lastUserInput":"继续"}`)
	res, err = HandleNativeEvent(opts)
	if err != nil {
		t.Fatalf("续跑 PreInvocation 不应报错: %v", err)
	}
	if strings.Contains(string(res.Stdout), `"injectSteps"`) {
		t.Fatalf("续跑不应重新注入上下文: %s", res.Stdout)
	}

	// 2. 文件写入工具不得当成读取；write_to_file 在确认前必须拒绝
	payloadTool := []byte(`{"conversationId":"ag-1","invocationId":"inv-1","toolCall":{"name":"write_to_file","args":{"path":"src/foo.go"}}}`)
	opts.EventName = "PreToolUse"
	opts.Payload = payloadTool
	res, err = HandleNativeEvent(opts)
	if err != nil {
		t.Fatalf("PreToolUse 处理失败: %v", err)
	}
	var denyResp map[string]any
	if err := json.Unmarshal(res.Stdout, &denyResp); err != nil {
		t.Fatalf("解析 Antigravity 拒绝响应失败: %v, raw=%s", err, res.Stdout)
	}
	if denyResp["decision"] != "deny" {
		t.Fatalf("未交付上下文前 decision 必须为 deny，实际: %v", denyResp["decision"])
	}
	if !strings.Contains(fmt.Sprint(denyResp["reason"]), "上下文尚未交付") {
		t.Fatalf("拒绝理由应为上下文尚未交付，实际: %v", denyResp["reason"])
	}

	// 精确命中上下文的 view_file 登记读取回执后，确认前写入仍拒绝，但不再报尚未交付
	opts.EventName = "PostToolUse"
	opts.Payload = []byte(fmt.Sprintf(`{"conversationId":"ag-official","toolCall":{"name":"view_file","args":{"AbsolutePath":%q}}}`, nextPath))
	res, err = HandleNativeEvent(opts)
	if err != nil {
		t.Fatalf("view_file 回执失败: %v", err)
	}
	if string(res.Stdout) != "{}" {
		t.Fatalf("PostToolUse 应返回空对象，实际: %s", res.Stdout)
	}
	opts.EventName = "PreToolUse"
	opts.Payload = []byte(`{"conversationId":"ag-official","toolCall":{"name":"write_to_file","args":{"TargetFile":"src/foo.go"}}}`)
	res, err = HandleNativeEvent(opts)
	if err != nil {
		t.Fatalf("读取回执后的写入检查失败: %v", err)
	}
	if err := json.Unmarshal(res.Stdout, &denyResp); err != nil {
		t.Fatalf("解析读取后的拒绝响应失败: %v, raw=%s", err, res.Stdout)
	}
	reason := fmt.Sprint(denyResp["reason"])
	if denyResp["decision"] != "deny" || strings.Contains(reason, "上下文尚未交付") || !strings.Contains(reason, "未诊断") {
		t.Fatalf("读取回执后、确认前应因未诊断拒绝，实际: %v", denyResp)
	}

	// IDE 的 read_file 使用 Path。空 error 应把该上下文文件记为已读。
	opts.EventName = "PreInvocation"
	opts.Payload = []byte(`{"conversationId":"ag-path","invocationNum":0,"lastUserInput":"读取 Path 键"}`)
	res, err = HandleNativeEvent(opts)
	if err != nil {
		t.Fatalf("Path 键提交失败: %v", err)
	}
	pathKeyFile := antigravityContextPath(t, res.Stdout)
	opts.EventName = "PostToolUse"
	opts.Payload = []byte(fmt.Sprintf(`{"conversationId":"ag-path","toolCall":{"name":"read_file","args":{"Path":%q}},"error":""}`, pathKeyFile))
	res, err = HandleNativeEvent(opts)
	if err != nil {
		t.Fatalf("read_file Path 回执失败: %v", err)
	}
	opts.EventName = "PreToolUse"
	opts.Payload = []byte(`{"conversationId":"ag-path","toolCall":{"name":"write_to_file","args":{"TargetFile":"src/foo.go"}}}`)
	res, err = HandleNativeEvent(opts)
	if err != nil {
		t.Fatalf("Path 回执后的写入检查失败: %v", err)
	}
	if err := json.Unmarshal(res.Stdout, &denyResp); err != nil {
		t.Fatalf("解析 Path 回执后的拒绝响应失败: %v", err)
	}
	pathReason := fmt.Sprint(denyResp["reason"])
	if denyResp["decision"] != "deny" || strings.Contains(pathReason, "上下文尚未交付") || !strings.Contains(pathReason, "未诊断") {
		t.Fatalf("Path 键读取后应记为已读并因未诊断拒绝写入，实际: %v", denyResp)
	}

	// 空 response 时从记录文件取模型回复。步骤号未到的围栏不登记，用户原文里的围栏也不算模型方案。
	transcriptPath := writeAntigravityTranscript(t, tempDir)
	opts.EventName = "PreInvocation"
	opts.Payload = []byte("{\"conversationId\":\"ag-transcript\",\"invocationNum\":0,\"lastUserInput\":\"请看\\n```agate-plan\\n{\\\"files\\\":[\\\"evil.txt\\\"],\\\"diagnosis\\\":\\\"not_applicable\\\"}\\n```\\n执行\"}")
	res, err = HandleNativeEvent(opts)
	if err != nil {
		t.Fatalf("记录文件提交失败: %v", err)
	}
	transcriptContext := antigravityContextPath(t, res.Stdout)
	opts.EventName = "PostToolUse"
	opts.Payload = []byte(fmt.Sprintf(`{"conversationId":"ag-transcript","toolCall":{"name":"read_file","args":{"Path":%q}},"error":""}`, transcriptContext))
	if _, err = HandleNativeEvent(opts); err != nil {
		t.Fatalf("记录文件读取回执失败: %v", err)
	}
	opts.EventName = "PostInvocation"
	opts.Payload = []byte(fmt.Sprintf(`{"conversationId":"ag-transcript","initialNumSteps":42,"transcriptPath":%q}`, transcriptPath))
	if _, err = HandleNativeEvent(opts); err != nil {
		t.Fatalf("过早的 PostInvocation 不应失败: %v", err)
	}
	earlySnap := loadAntigravitySnapshot(t, opts, "ag-transcript")
	if earlySnap.PlanCandidate != nil || earlySnap.Diagnosis != DiagnosisUndiagnosed {
		t.Fatalf("步骤号未到时不应登记方案，实际: %+v", earlySnap.PlanCandidate)
	}
	opts.Payload = []byte(fmt.Sprintf(`{"conversationId":"ag-transcript","initialNumSteps":45,"transcriptPath":%q}`, transcriptPath))
	if _, err = HandleNativeEvent(opts); err != nil {
		t.Fatalf("到达步骤号后的 PostInvocation 失败: %v", err)
	}
	lateSnap := loadAntigravitySnapshot(t, opts, "ag-transcript")
	if lateSnap.PlanConfirmed || lateSnap.PlanCandidate == nil || len(lateSnap.PlanCandidate.Files) != 1 || lateSnap.PlanCandidate.Files[0] != "sample.txt" {
		t.Fatalf("应登记未确认的 sample.txt 方案，实际: %+v confirmed=%v", lateSnap.PlanCandidate, lateSnap.PlanConfirmed)
	}
	explicitPayload, err := json.Marshal(map[string]any{
		"conversationId":  "ag-transcript",
		"response":        "显式回复。\n```agate-plan\n{\"files\":[\"explicit.txt\"],\"diagnosis\":\"not_applicable\"}\n```\n执行",
		"initialNumSteps": 45,
		"transcriptPath":  transcriptPath,
	})
	if err != nil {
		t.Fatalf("构造显式回复失败: %v", err)
	}
	opts.Payload = explicitPayload
	if _, err = HandleNativeEvent(opts); err != nil {
		t.Fatalf("显式 response 的 PostInvocation 失败: %v", err)
	}
	explicitSnap := loadAntigravitySnapshot(t, opts, "ag-transcript")
	if explicitSnap.PlanCandidate == nil || len(explicitSnap.PlanCandidate.Files) != 1 || explicitSnap.PlanCandidate.Files[0] != "explicit.txt" {
		t.Fatalf("显式 response 应优先于记录文件，实际: %+v", explicitSnap.PlanCandidate)
	}

	// 无 invocationId 时确认后应能写入已确认范围，新提交后旧批准失效
	opts.EventName = "PreInvocation"
	opts.Payload = []byte(`{"conversationId":"ag-empty-turn","invocationNum":0,"lastUserInput":"新建 sample.txt"}`)
	res, err = HandleNativeEvent(opts)
	if err != nil {
		t.Fatalf("无轮次会话提交失败: %v", err)
	}
	emptyCtx := antigravityContextPath(t, res.Stdout)
	opts.EventName = "PostToolUse"
	opts.Payload = []byte(fmt.Sprintf(`{"conversationId":"ag-empty-turn","toolCall":{"name":"read_file","args":{"Path":%q}},"error":""}`, emptyCtx))
	if _, err = HandleNativeEvent(opts); err != nil {
		t.Fatalf("无轮次会话读取回执失败: %v", err)
	}
	opts.EventName = "PostInvocation"
	opts.Payload = []byte("{\"conversationId\":\"ag-empty-turn\",\"response\":\"方案。\\n```agate-plan\\n{\\\"files\\\":[\\\"sample.txt\\\"],\\\"diagnosis\\\":\\\"not_applicable\\\"}\\n```\\n请回复执行\"}")
	if _, err = HandleNativeEvent(opts); err != nil {
		t.Fatalf("无轮次会话登记方案失败: %v", err)
	}
	opts.EventName = "PreInvocation"
	opts.Payload = []byte(`{"conversationId":"ag-empty-turn","invocationNum":0,"lastUserInput":"执行"}`)
	res, err = HandleNativeEvent(opts)
	if err != nil {
		t.Fatalf("无轮次确认失败: %v", err)
	}
	confirmCtx := antigravityContextPath(t, res.Stdout)
	confirmSnap := loadAntigravitySnapshot(t, opts, "ag-empty-turn")
	if !confirmSnap.PlanConfirmed || confirmSnap.ConfirmedTurnID == "" || len(confirmSnap.ConfirmedFiles) != 1 {
		t.Fatalf("无轮次确认应绑定 ActiveTurnID，实际: %+v", confirmSnap)
	}
	opts.EventName = "PostToolUse"
	opts.Payload = []byte(fmt.Sprintf(`{"conversationId":"ag-empty-turn","toolCall":{"name":"read_file","args":{"Path":%q}},"error":""}`, confirmCtx))
	if _, err = HandleNativeEvent(opts); err != nil {
		t.Fatalf("确认轮读取回执失败: %v", err)
	}
	opts.EventName = "PreToolUse"
	opts.Payload = []byte(`{"conversationId":"ag-empty-turn","toolCall":{"name":"write_file","args":{"Path":"sample.txt","Content":"hello"}}}`)
	res, err = HandleNativeEvent(opts)
	if err != nil {
		t.Fatalf("无轮次确认后写入检查失败: %v", err)
	}
	if err := json.Unmarshal(res.Stdout, &denyResp); err != nil {
		t.Fatalf("解析无轮次写入响应失败: %v", err)
	}
	if denyResp["decision"] != "allow" {
		t.Fatalf("无轮次确认后范围内写入应放行，实际: %v", denyResp)
	}
	opts.EventName = "PreInvocation"
	opts.Payload = []byte(`{"conversationId":"ag-empty-turn","invocationNum":0,"lastUserInput":"另起指令"}`)
	if _, err = HandleNativeEvent(opts); err != nil {
		t.Fatalf("新提交失败: %v", err)
	}
	opts.EventName = "PreToolUse"
	opts.Payload = []byte(`{"conversationId":"ag-empty-turn","toolCall":{"name":"write_file","args":{"Path":"sample.txt","Content":"hello"}}}`)
	res, err = HandleNativeEvent(opts)
	if err != nil {
		t.Fatalf("新提交后写入检查失败: %v", err)
	}
	if err := json.Unmarshal(res.Stdout, &denyResp); err != nil {
		t.Fatalf("解析新提交后写入响应失败: %v", err)
	}
	if denyResp["decision"] != "deny" {
		t.Fatalf("新提交后不得复用旧批准，实际: %v", denyResp)
	}

	// 官方 error 字段表示失败，不得登记已读
	opts.EventName = "PreInvocation"
	opts.Payload = []byte(`{"conversationId":"ag-fail","invocationNum":0,"lastUserInput":"失败读取"}`)
	res, err = HandleNativeEvent(opts)
	if err != nil {
		t.Fatalf("失败读取前的提交失败: %v", err)
	}
	failPath := antigravityContextPath(t, res.Stdout)
	opts.EventName = "PostToolUse"
	opts.Payload = []byte(fmt.Sprintf(`{"conversationId":"ag-fail","toolCall":{"name":"view_file","args":{"AbsolutePath":%q}},"error":"read failed"}`, failPath))
	if _, err = HandleNativeEvent(opts); err != nil {
		t.Fatalf("失败读取回执不应中断: %v", err)
	}
	opts.EventName = "PreToolUse"
	opts.Payload = []byte(`{"conversationId":"ag-fail","toolCall":{"name":"write_to_file","args":{"TargetFile":"src/foo.go"}}}`)
	res, err = HandleNativeEvent(opts)
	if err != nil {
		t.Fatalf("失败读取后的写入检查失败: %v", err)
	}
	if err := json.Unmarshal(res.Stdout, &denyResp); err != nil {
		t.Fatalf("解析失败读取后的拒绝响应失败: %v", err)
	}
	if !strings.Contains(fmt.Sprint(denyResp["reason"]), "上下文尚未交付") {
		t.Fatalf("失败读取后应仍未交付，实际: %v", denyResp)
	}

	// 错路径和旧轮都不登记已读
	opts.EventName = "PostToolUse"
	opts.Payload = []byte(`{"conversationId":"ag-fail","toolCall":{"name":"view_file","args":{"AbsolutePath":"D:/other/file.go"}}}`)
	if _, err = HandleNativeEvent(opts); err != nil {
		t.Fatalf("错路径读取不应中断: %v", err)
	}
	opts.Payload = []byte(fmt.Sprintf(`{"conversationId":"ag-fail","invocationId":"old-turn","toolCall":{"name":"view_file","args":{"AbsolutePath":%q}}}`, failPath))
	if _, err = HandleNativeEvent(opts); err != nil {
		t.Fatalf("旧轮读取不应中断: %v", err)
	}
	opts.EventName = "PreToolUse"
	opts.Payload = []byte(`{"conversationId":"ag-fail","toolCall":{"name":"write_to_file","args":{"TargetFile":"src/foo.go"}}}`)
	res, err = HandleNativeEvent(opts)
	if err != nil {
		t.Fatalf("错路径或旧轮后的写入检查失败: %v", err)
	}
	if err := json.Unmarshal(res.Stdout, &denyResp); err != nil {
		t.Fatalf("解析错路径后的拒绝响应失败: %v", err)
	}
	if !strings.Contains(fmt.Sprint(denyResp["reason"]), "上下文尚未交付") {
		t.Fatalf("错路径和旧轮后应仍未交付，实际: %v", denyResp)
	}

	// 工作区不匹配时按官方字段终止模型调用
	opts.EventName = "PreInvocation"
	opts.Payload = []byte(fmt.Sprintf(`{"conversationId":"ag-root","invocationNum":0,"lastUserInput":"冲突","workspacePaths":[%q]}`, filepath.Join(tempDir, "other")))
	res, err = HandleNativeEvent(opts)
	if err == nil {
		t.Fatal("工作区冲突应返回错误")
	}
	if !strings.Contains(string(res.Stdout), `"terminationBehavior":"terminate"`) || !strings.Contains(string(res.Stdout), `"userMessage"`) {
		t.Fatalf("拒绝响应应包含终止字段和用户可见错误，实际: %s", res.Stdout)
	}
}

func antigravityContextPath(t *testing.T, stdout []byte) string {
	t.Helper()
	var resp map[string]any
	if err := json.Unmarshal(stdout, &resp); err != nil {
		t.Fatalf("解析 PreInvocation 响应失败: %v raw=%s", err, stdout)
	}
	if _, ok := resp["continue"]; ok {
		t.Fatalf("响应不应包含 continue: %s", stdout)
	}
	if _, ok := resp["contextPath"]; ok {
		t.Fatalf("响应不应包含 contextPath: %s", stdout)
	}
	steps, ok := resp["injectSteps"].([]any)
	if !ok || len(steps) != 1 {
		t.Fatalf("应返回一条 injectSteps，实际: %s", stdout)
	}
	step, ok := steps[0].(map[string]any)
	if !ok {
		t.Fatalf("injectSteps 条目格式错误: %s", stdout)
	}
	if _, hasType := step["type"]; hasType {
		t.Fatalf("injectSteps 不应使用 type/path 步骤: %s", stdout)
	}
	sys, _ := step["systemMessage"].(map[string]any)
	msg, _ := sys["systemMessage"].(string)
	const marker = "请先读取："
	idx := strings.LastIndex(msg, marker)
	if idx < 0 {
		t.Fatalf("systemMessage 应包含上下文路径，实际: %s", stdout)
	}
	return strings.TrimSpace(msg[idx+len(marker):])
}

func writeAntigravityTranscript(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "transcript_full.jsonl")
	lines := []map[string]any{
		{
			"step_index": 41,
			"source":     "USER_EXPLICIT",
			"type":       "USER_INPUT",
			"content":    "```agate-plan\n{\"files\":[\"evil.txt\"],\"diagnosis\":\"not_applicable\"}\n```\n执行",
		},
		{
			"step_index": 40,
			"source":     "MODEL",
			"type":       "PLANNER_RESPONSE",
			"content":    "还没有围栏",
		},
		{
			"step_index": 45,
			"source":     "MODEL",
			"type":       "PLANNER_RESPONSE",
			"content":    "方案如下。\n```agate-plan\n{\"files\":[\"sample.txt\"],\"diagnosis\":\"not_applicable\"}\n```\n请回复执行",
		},
		{
			"step_index": 50,
			"source":     "MODEL",
			"type":       "PLANNER_RESPONSE",
			"content":    "```agate-plan\n{\"files\":[\"outside.txt\"],\"diagnosis\":\"not_applicable\"}\n```\n执行",
		},
	}
	var buf strings.Builder
	for _, line := range lines {
		raw, err := json.Marshal(line)
		if err != nil {
			t.Fatalf("序列化记录失败: %v", err)
		}
		buf.Write(raw)
		buf.WriteByte('\n')
	}
	if err := os.WriteFile(path, []byte(buf.String()), 0o644); err != nil {
		t.Fatalf("写入记录文件失败: %v", err)
	}
	return path
}

func loadAntigravitySnapshot(t *testing.T, opts NativeEventOptions, sessionID string) NativeSnapshot {
	t.Helper()
	workspace, err := filepath.Abs(opts.WorkDir)
	if err != nil {
		t.Fatalf("解析工作区失败: %v", err)
	}
	storeDir, err := ResolveNativeStoreDir(opts.RootDir, opts.Host, workspace, opts.RecordDir)
	if err != nil {
		t.Fatalf("解析会话目录失败: %v", err)
	}
	snap, err := NewNativeStore(storeDir).LoadSnapshot(sessionID, opts.Host, workspace)
	if err != nil {
		t.Fatalf("读取快照失败: %v", err)
	}
	return snap
}

func TestClaudeAdapterProtocol(t *testing.T) {
	tempDir := t.TempDir()
	opts := NativeEventOptions{
		Host:      "claude",
		RootDir:   tempDir,
		WorkDir:   tempDir,
		RecordDir: filepath.Join(tempDir, "records"),
	}

	// 1. 官方下划线字段 UserPromptSubmit
	opts.EventName = "UserPromptSubmit"
	opts.Payload = []byte(`{"session_id":"cl-1","turn_id":"t-1","prompt":"分析性能瓶颈"}`)
	_, err := HandleNativeEvent(opts)
	if err != nil {
		t.Fatalf("UserPromptSubmit 失败: %v", err)
	}

	// 2. 官方 tool_name/tool_input + Edit 在确认前拒绝
	opts.EventName = "PreToolUse"
	opts.Payload = []byte(`{"session_id":"cl-1","turn_id":"t-1","tool_name":"Edit","tool_input":{"file_path":"main.go"}}`)
	res, err := HandleNativeEvent(opts)
	if err != nil {
		t.Fatalf("PreToolUse 失败: %v", err)
	}
	var claudeResp struct {
		HookSpecificOutput struct {
			HookEventName            string `json:"hookEventName"`
			PermissionDecision       string `json:"permissionDecision"`
			PermissionDecisionReason string `json:"permissionDecisionReason"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal(res.Stdout, &claudeResp); err != nil {
		t.Fatalf("解析 Claude 响应失败: %v, raw=%s", err, res.Stdout)
	}
	if claudeResp.HookSpecificOutput.PermissionDecision != "deny" {
		t.Fatalf("Claude 拒绝决策应为 deny，实际: %s", claudeResp.HookSpecificOutput.PermissionDecision)
	}
	if claudeResp.HookSpecificOutput.HookEventName != "PreToolUse" {
		t.Fatalf("HookEventName 应为 PreToolUse，实际: %s", claudeResp.HookSpecificOutput.HookEventName)
	}
}

func TestCodexAdapterPatchExtraction(t *testing.T) {
	patchContent := `diff --git a/pkg/a.go b/pkg/a.go
--- a/pkg/a.go
+++ b/pkg/a.go
@@ -1 +1 @@
-old
+new
diff --git a/pkg/b.go b/pkg/b.go
--- a/pkg/b.go
+++ b/pkg/b.go
@@ -1 +1 @@
-old
+new
`
	targets := extractTargetsFromPatch(patchContent)
	if len(targets) != 2 {
		t.Fatalf("应提取 2 个目标文件，实际: %v", targets)
	}
	if targets[0] != "pkg/a.go" || targets[1] != "pkg/b.go" {
		t.Fatalf("提取的目标文件不匹配: %v", targets)
	}

	applyTargets := extractTargetsFromApplyPatch(map[string]any{
		"command": "*** Add File: pkg/c.go\n+hello\n*** Update File: pkg/a.go\n*** Delete File: pkg/old.go\n*** Move File: pkg/x.go -> pkg/y.go\n",
	})
	want := map[string]bool{"pkg/c.go": true, "pkg/a.go": true, "pkg/old.go": true, "pkg/x.go": true, "pkg/y.go": true}
	for _, tpath := range applyTargets {
		delete(want, tpath)
	}
	if len(want) != 0 {
		t.Fatalf("apply_patch command 目标核验不完整，缺少: %v 实际: %v", want, applyTargets)
	}
}

func TestCodexStopUsesLastAssistantMessage(t *testing.T) {
	ad := &CodexAdapter{}
	payload := []byte(`{"session_id":"cx-1","turn_id":"t-1","last_assistant_message":"方案完成，请确认执行"}`)
	event, err := ad.Decode("Stop", payload)
	if err != nil {
		t.Fatalf("Stop 解码失败: %v", err)
	}
	if event.Kind != NativeEventResponseCompleted {
		t.Fatalf("Stop 应为 ResponseCompleted")
	}
	if !strings.Contains(event.Text, "方案完成") {
		t.Fatalf("应读取 last_assistant_message，实际: %q", event.Text)
	}
}

func TestCodexUserPromptSubmitUsesHookSpecificOutput(t *testing.T) {
	ad := &CodexAdapter{}
	stdout, _, _, err := ad.Encode("UserPromptSubmit", NativeDecision{AdditionalContext: filepath.Join(t.TempDir(), "ctx.md")})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(stdout), `"hookSpecificOutput"`) || !strings.Contains(string(stdout), `"additionalContext"`) {
		t.Fatalf("UserPromptSubmit 应输出 hookSpecificOutput.additionalContext，实际: %s", stdout)
	}
	if strings.Contains(string(stdout), `"additional_context"`) {
		t.Fatalf("不应再使用顶层 additional_context: %s", stdout)
	}
}

func TestWindsurfAdapterExitCode2OnDenial(t *testing.T) {
	tempDir := t.TempDir()
	opts := NativeEventOptions{
		Host:      "windsurf",
		RootDir:   tempDir,
		WorkDir:   tempDir,
		RecordDir: filepath.Join(tempDir, "records"),
	}

	// 1. 官方嵌套提交使用 trajectory_id，不得因缺少 session_id 返回退出码 1
	opts.EventName = "pre_user_prompt"
	opts.Payload = []byte(`{"trajectory_id":"traj-1","tool_info":{"user_prompt":"优化内存占用"}}`)
	res, err := HandleNativeEvent(opts)
	if err != nil || res.ExitCode != 0 {
		t.Fatalf("嵌套提交应成功: err=%v exit=%d stderr=%s", err, res.ExitCode, res.Stderr)
	}

	// 2. 预写拦截 -> 契约要求 exitCode 必须为 2！
	opts.EventName = "pre_write_code"
	opts.Payload = []byte(`{"trajectory_id":"traj-1","tool_info":{"file_path":"server.go"}}`)
	res, err = HandleNativeEvent(opts)
	if err != nil {
		t.Fatalf("pre_write_code 错误: %v", err)
	}
	if res.ExitCode != 2 {
		t.Fatalf("Windsurf 预写拒绝时退出码必须为 2，实际为: %d", res.ExitCode)
	}
	if !strings.Contains(res.Stderr, "上下文尚未交付") {
		t.Fatalf("Windsurf 拒绝理由应输出在 stderr，实际: %s", res.Stderr)
	}
}
