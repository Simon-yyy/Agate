package prompt

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestNativeApprovalIsBoundToCurrentTurn(t *testing.T) {
	tempDir := t.TempDir()
	recordDir := filepath.Join(tempDir, "records")
	opts := NativeEventOptions{
		Host:      "cursor",
		EventName: "test",
		RootDir:   tempDir,
		WorkDir:   tempDir,
		RecordDir: recordDir,
	}

	sessionID := "sess-1001"

	submitEvent := NativeEvent{
		Kind:        NativeEventSubmit,
		SessionID:   sessionID,
		TurnID:      "turn-1",
		EventID:     "evt-1",
		Text:        "修复 demo bug",
		TrustedUser: true,
	}
	dec, err := ApplyNativeWorkflow(opts, submitEvent)
	if err != nil {
		t.Fatalf("提交任务失败: %v", err)
	}
	if dec.Block {
		t.Fatalf("提交任务不应被阻断: %s", dec.Reason)
	}
	contextPath := dec.AdditionalContext
	if contextPath == "" {
		t.Fatalf("提交任务应返回编译上下文路径")
	}

	writeBeforeRead := NativeEvent{
		Kind:      NativeEventBeforeTool,
		SessionID: sessionID,
		TurnID:    "turn-1",
		Action:    NativeActionEdit,
		Targets:   []string{"pkg/demo.go"},
	}
	dec, _ = ApplyNativeWorkflow(opts, writeBeforeRead)
	if !dec.Block || dec.Reason != "上下文尚未交付，不能执行写入" {
		t.Fatalf("未读上下文前应阻断写入，实际: block=%v, reason=%s", dec.Block, dec.Reason)
	}

	irrelevantRead := NativeEvent{
		Kind:      NativeEventReadCompleted,
		SessionID: sessionID,
		TurnID:    "turn-1",
		Action:    NativeActionRead,
		Targets:   []string{filepath.Join(tempDir, "other.md")},
		Success:   true,
	}
	ApplyNativeWorkflow(opts, irrelevantRead)
	dec, _ = ApplyNativeWorkflow(opts, writeBeforeRead)
	if !dec.Block || dec.Reason != "上下文尚未交付，不能执行写入" {
		t.Fatalf("读取无关文件后仍应阻断写入，实际: block=%v, reason=%s", dec.Block, dec.Reason)
	}

	correctRead := NativeEvent{
		Kind:      NativeEventReadCompleted,
		SessionID: sessionID,
		TurnID:    "turn-1",
		Action:    NativeActionRead,
		Targets:   []string{contextPath},
		Success:   true,
	}
	readDec, _ := ApplyNativeWorkflow(opts, correctRead)
	if readDec.AdditionalContext == "" {
		t.Fatalf("命中本轮上下文的读回执应交付路径")
	}

	dec, _ = ApplyNativeWorkflow(opts, writeBeforeRead)
	if !dec.Block || dec.Reason != "诊断状态为未诊断，不能仅凭解码标题、explicit 或 Plan 进入修复方案" {
		t.Fatalf("未诊断前应阻断写入，实际: block=%v, reason=%s", dec.Block, dec.Reason)
	}

	modelResponse := "经过排查，发现根因在 demo.go。\n\n" +
		"现象与预期：空指针异常\n" +
		"问题位置和证据：demo.go 第 10 行未判空\n" +
		"因果解释：直接解引用导致 panic\n" +
		"不确定项：无\n" +
		"方案依据：增加判空守卫\n\n" +
		"```agate-plan\n" +
		"{\n" +
		"  \"diagnosis\": \"confirmed\",\n" +
		"  \"files\": [\"pkg/demo.go\"]\n" +
		"}\n" +
		"```\n" +
		"👉 请确认方案：确认无误请回复‘执行’"

	respEvent := NativeEvent{
		Kind:      NativeEventResponseCompleted,
		SessionID: sessionID,
		TurnID:    "turn-1",
		Text:      modelResponse,
	}
	ApplyNativeWorkflow(opts, respEvent)

	dec, _ = ApplyNativeWorkflow(opts, writeBeforeRead)
	if !dec.Block || dec.Reason != "方案尚未获得用户显式确认（缺少‘执行’授权），不能执行修改" {
		t.Fatalf("未确认前应阻断写入，实际: block=%v, reason=%s", dec.Block, dec.Reason)
	}

	fakeConfirm := NativeEvent{
		Kind:        NativeEventSubmit,
		SessionID:   sessionID,
		TurnID:      "turn-2-fake",
		EventID:     "evt-fake",
		Text:        "执行",
		TrustedUser: false,
	}
	fakeDec, _ := ApplyNativeWorkflow(opts, fakeConfirm)
	if fakeDec.AdditionalContext != "" {
		ApplyNativeWorkflow(opts, NativeEvent{
			Kind:      NativeEventReadCompleted,
			SessionID: sessionID,
			TurnID:    "turn-2-fake",
			Targets:   []string{fakeDec.AdditionalContext},
			Success:   true,
		})
	}
	dec, _ = ApplyNativeWorkflow(opts, NativeEvent{
		Kind: NativeEventBeforeTool, SessionID: sessionID, TurnID: "turn-2-fake",
		Action: NativeActionEdit, Targets: []string{"pkg/demo.go"},
	})
	if !dec.Block {
		t.Fatalf("非 TrustedUser 提交执行不应授权，实际未阻断")
	}

	// 伪造确认后需重新走完整确认链
	reSubmit, err := ApplyNativeWorkflow(opts, NativeEvent{
		Kind: NativeEventSubmit, SessionID: sessionID, TurnID: "turn-1b", EventID: "evt-1b",
		Text: "修复 demo bug", TrustedUser: true,
	})
	if err != nil || reSubmit.Block {
		t.Fatalf("重新提交失败: %v %+v", err, reSubmit)
	}
	ApplyNativeWorkflow(opts, NativeEvent{
		Kind: NativeEventReadCompleted, SessionID: sessionID, TurnID: "turn-1b",
		Targets: []string{reSubmit.AdditionalContext}, Success: true,
	})
	ApplyNativeWorkflow(opts, NativeEvent{
		Kind: NativeEventResponseCompleted, SessionID: sessionID, TurnID: "turn-1b", Text: modelResponse,
	})

	realConfirm := NativeEvent{
		Kind:        NativeEventSubmit,
		SessionID:   sessionID,
		TurnID:      "turn-2",
		EventID:     "evt-2",
		Text:        "执行",
		TrustedUser: true,
	}
	dec, err = ApplyNativeWorkflow(opts, realConfirm)
	if err != nil || dec.Block {
		t.Fatalf("用户确认执行失败: %v, dec=%+v", err, dec)
	}
	confirmContextPath := dec.AdditionalContext
	ApplyNativeWorkflow(opts, NativeEvent{
		Kind:      NativeEventReadCompleted,
		SessionID: sessionID,
		TurnID:    "turn-2",
		Targets:   []string{confirmContextPath},
		Success:   true,
	})

	outsideWrite := NativeEvent{
		Kind:      NativeEventBeforeTool,
		SessionID: sessionID,
		TurnID:    "turn-2",
		Action:    NativeActionEdit,
		Targets:   []string{"pkg/outside.go"},
	}
	dec, _ = ApplyNativeWorkflow(opts, outsideWrite)
	if !dec.Block {
		t.Fatalf("越界写入应当被阻断，实际未阻断: %+v", dec)
	}

	inScopeWrite := NativeEvent{
		Kind:      NativeEventBeforeTool,
		SessionID: sessionID,
		TurnID:    "turn-2",
		Action:    NativeActionEdit,
		Targets:   []string{"pkg/demo.go"},
	}
	dec, _ = ApplyNativeWorkflow(opts, inScopeWrite)
	if dec.Block {
		t.Fatalf("授权范围内的文件应当放行，实际被阻断: %s", dec.Reason)
	}

	// 旧轮写入即使文件相同也必须拒绝
	staleTurnWrite := NativeEvent{
		Kind: NativeEventBeforeTool, SessionID: sessionID, TurnID: "turn-1b",
		Action: NativeActionEdit, Targets: []string{"pkg/demo.go"},
	}
	dec, _ = ApplyNativeWorkflow(opts, staleTurnWrite)
	if !dec.Block || !strings.Contains(dec.Reason, "轮次") {
		t.Fatalf("旧轮写入应因轮次不一致被阻断，实际: %+v", dec)
	}

	shellWrite := NativeEvent{
		Kind:      NativeEventBeforeTool,
		SessionID: sessionID,
		TurnID:    "turn-2",
		Action:    NativeActionShell,
		Command:   "echo 'APPROVED' > pkg/demo.go",
	}
	dec, _ = ApplyNativeWorkflow(opts, shellWrite)
	if dec.Block {
		t.Fatalf("确认后的 Shell 应放行，实际: %s", dec.Reason)
	}
	mcpCall := NativeEvent{
		Kind:      NativeEventBeforeTool,
		SessionID: sessionID,
		TurnID:    "turn-2",
		Action:    NativeActionMCP,
	}
	dec, _ = ApplyNativeWorkflow(opts, mcpCall)
	if dec.Block {
		t.Fatalf("有会话的 MCP 应放行，实际: %s", dec.Reason)
	}

	nextTurn := NativeEvent{
		Kind:        NativeEventSubmit,
		SessionID:   sessionID,
		TurnID:      "turn-3",
		EventID:     "evt-3",
		Text:        "查看新的文件",
		TrustedUser: true,
	}
	ApplyNativeWorkflow(opts, nextTurn)

	dec, _ = ApplyNativeWorkflow(opts, inScopeWrite)
	if !dec.Block {
		t.Fatalf("新一轮提交后旧授权必须失效，实际未阻断: %+v", dec)
	}
}

func TestNativeEmptyTurnIDConfirmAllowsInScopeWrite(t *testing.T) {
	tempDir := t.TempDir()
	opts := NativeEventOptions{
		Host:      "antigravity",
		RootDir:   tempDir,
		WorkDir:   tempDir,
		RecordDir: filepath.Join(tempDir, "records"),
	}
	sessionID := "sess-empty-turn"

	// 模拟 Antigravity：提交与工具事件都不带 TurnID/EventID
	submit := NativeEvent{Kind: NativeEventSubmit, SessionID: sessionID, Text: "新建 sample.txt", TrustedUser: true}
	dec, err := ApplyNativeWorkflow(opts, submit)
	if err != nil || dec.Block || dec.AdditionalContext == "" {
		t.Fatalf("无轮次提交失败: err=%v dec=%+v", err, dec)
	}
	ApplyNativeWorkflow(opts, NativeEvent{Kind: NativeEventReadCompleted, SessionID: sessionID, Targets: []string{dec.AdditionalContext}, Success: true})
	planText := "方案如下。\n```agate-plan\n{\"files\":[\"sample.txt\"],\"diagnosis\":\"not_applicable\"}\n```\n请回复执行"
	ApplyNativeWorkflow(opts, NativeEvent{Kind: NativeEventResponseCompleted, SessionID: sessionID, Text: planText})

	confirm := NativeEvent{Kind: NativeEventSubmit, SessionID: sessionID, Text: "执行", TrustedUser: true}
	confirmDec, err := ApplyNativeWorkflow(opts, confirm)
	if err != nil || confirmDec.Block || confirmDec.AdditionalContext == "" {
		t.Fatalf("无轮次确认失败: err=%v dec=%+v", err, confirmDec)
	}
	ApplyNativeWorkflow(opts, NativeEvent{Kind: NativeEventReadCompleted, SessionID: sessionID, Targets: []string{confirmDec.AdditionalContext}, Success: true})

	allowed, _ := ApplyNativeWorkflow(opts, NativeEvent{
		Kind: NativeEventBeforeTool, SessionID: sessionID, Action: NativeActionEdit, Targets: []string{"sample.txt"},
	})
	if allowed.Block {
		t.Fatalf("无轮次确认后范围内写入应放行: %s", allowed.Reason)
	}

	outside, _ := ApplyNativeWorkflow(opts, NativeEvent{
		Kind: NativeEventBeforeTool, SessionID: sessionID, Action: NativeActionEdit, Targets: []string{"outside.txt"},
	})
	if !outside.Block || !strings.Contains(outside.Reason, "超出已确认方案范围") {
		t.Fatalf("无轮次确认后越权写入仍应拒绝: %+v", outside)
	}

	next := NativeEvent{Kind: NativeEventSubmit, SessionID: sessionID, Text: "另起一条指令", TrustedUser: true}
	nextDec, err := ApplyNativeWorkflow(opts, next)
	if err != nil || nextDec.Block {
		t.Fatalf("新提交失败: err=%v dec=%+v", err, nextDec)
	}
	ApplyNativeWorkflow(opts, NativeEvent{Kind: NativeEventReadCompleted, SessionID: sessionID, Targets: []string{nextDec.AdditionalContext}, Success: true})
	stale, _ := ApplyNativeWorkflow(opts, NativeEvent{
		Kind: NativeEventBeforeTool, SessionID: sessionID, Action: NativeActionEdit, Targets: []string{"sample.txt"},
	})
	if !stale.Block {
		t.Fatalf("新提交后不得复用旧批准")
	}
}

func TestNativeOrphanExecuteClearsStaleApproval(t *testing.T) {
	tempDir := t.TempDir()
	opts := NativeEventOptions{
		Host:      "cursor",
		RootDir:   tempDir,
		WorkDir:   tempDir,
		RecordDir: filepath.Join(tempDir, "records"),
	}
	sessionID := "sess-orphan-exec"

	submit := NativeEvent{Kind: NativeEventSubmit, SessionID: sessionID, TurnID: "t1", EventID: "e1", Text: "修复 sample.txt", TrustedUser: true}
	dec, err := ApplyNativeWorkflow(opts, submit)
	if err != nil || dec.Block || dec.AdditionalContext == "" {
		t.Fatalf("提交失败: err=%v dec=%+v", err, dec)
	}
	ApplyNativeWorkflow(opts, NativeEvent{Kind: NativeEventReadCompleted, SessionID: sessionID, TurnID: "t1", Targets: []string{dec.AdditionalContext}, Success: true})
	planText := "现象与预期：x\n问题位置和证据：sample.txt\n因果解释：y\n不确定项：无\n方案依据：z\n\n```agate-plan\n{\"diagnosis\":\"confirmed\",\"files\":[\"sample.txt\"]}\n```\n👉 请确认方案：确认无误请回复‘执行’"
	ApplyNativeWorkflow(opts, NativeEvent{Kind: NativeEventResponseCompleted, SessionID: sessionID, TurnID: "t1", Text: planText})

	confirm := NativeEvent{Kind: NativeEventSubmit, SessionID: sessionID, TurnID: "t2", EventID: "e2", Text: "执行", TrustedUser: true}
	confirmDec, err := ApplyNativeWorkflow(opts, confirm)
	if err != nil || confirmDec.Block {
		t.Fatalf("首次确认失败: err=%v dec=%+v", err, confirmDec)
	}
	ApplyNativeWorkflow(opts, NativeEvent{Kind: NativeEventReadCompleted, SessionID: sessionID, TurnID: "t2", Targets: []string{confirmDec.AdditionalContext}, Success: true})
	allowed, _ := ApplyNativeWorkflow(opts, NativeEvent{Kind: NativeEventBeforeTool, SessionID: sessionID, TurnID: "t2", Action: NativeActionEdit, Targets: []string{"sample.txt"}})
	if allowed.Block {
		t.Fatalf("确认轮范围内写入应放行: %s", allowed.Reason)
	}

	orphan := NativeEvent{Kind: NativeEventSubmit, SessionID: sessionID, TurnID: "t3", EventID: "e3", Text: "执行", TrustedUser: true}
	orphanDec, err := ApplyNativeWorkflow(opts, orphan)
	if err != nil || orphanDec.Block {
		t.Fatalf("孤立执行仍应完成编译轮次: err=%v dec=%+v", err, orphanDec)
	}
	ApplyNativeWorkflow(opts, NativeEvent{Kind: NativeEventReadCompleted, SessionID: sessionID, TurnID: "t3", Targets: []string{orphanDec.AdditionalContext}, Success: true})

	staleWrite := NativeEvent{Kind: NativeEventBeforeTool, SessionID: sessionID, TurnID: "t2", Action: NativeActionEdit, Targets: []string{"sample.txt"}}
	staleDec, _ := ApplyNativeWorkflow(opts, staleWrite)
	if !staleDec.Block {
		t.Fatalf("旧确认轮写入在孤立执行后必须阻断")
	}
	newWrite := NativeEvent{Kind: NativeEventBeforeTool, SessionID: sessionID, TurnID: "t3", Action: NativeActionEdit, Targets: []string{"sample.txt"}}
	newDec, _ := ApplyNativeWorkflow(opts, newWrite)
	if !newDec.Block {
		t.Fatalf("无新方案的确认轮也不能放行写入")
	}
}

func TestNativeRejectsMissingSessionAndRootConflict(t *testing.T) {
	tempDir := t.TempDir()
	opts := NativeEventOptions{Host: "cursor", RootDir: tempDir, WorkDir: tempDir, RecordDir: filepath.Join(tempDir, "records")}
	dec, err := ApplyNativeWorkflow(opts, NativeEvent{Kind: NativeEventBeforeTool, Action: NativeActionEdit, Targets: []string{"a.go"}})
	if err == nil || !dec.Block || !strings.Contains(dec.Reason, "拒绝工具调用") {
		t.Fatalf("缺会话标识应拒绝工具调用: err=%v dec=%+v", err, dec)
	}
	dec, err = ApplyNativeWorkflow(opts, NativeEvent{Kind: NativeEventBeforeTool, Action: NativeActionShell, Command: "git status"})
	if err == nil || !dec.Block || !strings.Contains(dec.Reason, "拒绝工具调用") {
		t.Fatalf("缺会话标识应拒绝 Shell: err=%v dec=%+v", err, dec)
	}
	dec, err = ApplyNativeWorkflow(opts, NativeEvent{Kind: NativeEventBeforeTool, Action: NativeActionRead, Targets: []string{"README.md"}})
	if err == nil || !dec.Block || !strings.Contains(dec.Reason, "拒绝工具调用") {
		t.Fatalf("缺会话标识应拒绝 Read: err=%v dec=%+v", err, dec)
	}
	dec, err = ApplyNativeWorkflow(opts, NativeEvent{
		Kind: NativeEventSubmit, SessionID: "s1", TurnID: "t1", Text: "x", TrustedUser: true,
		ClaimedRoots: []string{filepath.Join(tempDir, "other-root")},
	})
	if err == nil || !dec.Block || !strings.Contains(dec.Reason, "工作区") {
		t.Fatalf("根目录冲突应拒绝: err=%v dec=%+v", err, dec)
	}
}

func TestNativeCrossProcessLockSerializesWriters(t *testing.T) {
	tempDir := t.TempDir()
	opts := NativeEventOptions{Host: "cursor", RootDir: tempDir, WorkDir: tempDir, RecordDir: filepath.Join(tempDir, "records")}
	sessionID := "sess-lock"
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := ApplyNativeWorkflow(opts, NativeEvent{
				Kind: NativeEventSubmit, SessionID: sessionID, TurnID: "t-lock", EventID: "e-lock",
				Text: "并行提交", TrustedUser: true,
			})
			errs <- err
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("跨进程锁下并行提交不应失败: %v", err)
		}
	}
}

func TestNativeShellFollowsFileConfirmation(t *testing.T) {
	tempDir := t.TempDir()
	opts := NativeEventOptions{
		Host:      "cursor",
		RootDir:   tempDir,
		WorkDir:   tempDir,
		RecordDir: filepath.Join(tempDir, "records"),
	}
	sessionID := "sess-shell"
	plan := "现象与预期：需要改 demo.go\n问题位置和证据：demo.go\n因果解释：缺判断\n不确定项：无\n方案依据：补判断\n\n" +
		"```agate-plan\n{\"diagnosis\":\"not_applicable\",\"files\":[\"pkg/demo.go\"]}\n```\n请回复执行"

	early, _ := ApplyNativeWorkflow(opts, NativeEvent{
		Kind: NativeEventBeforeTool, SessionID: sessionID, TurnID: "turn-1",
		Action: NativeActionShell, Command: "git status",
	})
	if early.Block {
		t.Fatalf("有会话的 Shell 应在诊断检查之前放行，实际: %+v", early)
	}
	earlyOther, _ := ApplyNativeWorkflow(opts, NativeEvent{
		Kind: NativeEventBeforeTool, SessionID: sessionID, TurnID: "turn-1",
		Action: NativeActionOther,
	})
	if earlyOther.Block {
		t.Fatalf("有会话的 Other（如 Task/Grep）应在诊断检查之前放行，实际: %+v", earlyOther)
	}
	noSessionOther, _ := ApplyNativeWorkflow(opts, NativeEvent{
		Kind: NativeEventBeforeTool, Action: NativeActionOther,
	})
	if !noSessionOther.Block {
		t.Fatalf("没有会话标识的 Other 应拒绝，实际: %+v", noSessionOther)
	}

	submit, err := ApplyNativeWorkflow(opts, NativeEvent{
		Kind: NativeEventSubmit, SessionID: sessionID, TurnID: "turn-1", EventID: "evt-1",
		Text: "改 demo.go", TrustedUser: true,
	})
	if err != nil || submit.Block || submit.AdditionalContext == "" {
		t.Fatalf("提交失败: err=%v dec=%+v", err, submit)
	}
	ApplyNativeWorkflow(opts, NativeEvent{
		Kind: NativeEventReadCompleted, SessionID: sessionID, TurnID: "turn-1",
		Action: NativeActionRead, Targets: []string{submit.AdditionalContext}, Success: true,
	})
	ApplyNativeWorkflow(opts, NativeEvent{
		Kind: NativeEventResponseCompleted, SessionID: sessionID, TurnID: "turn-1", Text: plan,
	})
	beforeConfirm, _ := ApplyNativeWorkflow(opts, NativeEvent{
		Kind: NativeEventBeforeTool, SessionID: sessionID, TurnID: "turn-1",
		Action: NativeActionShell, Command: "agate verify",
	})
	if beforeConfirm.Block {
		t.Fatalf("未执行前 Shell 应放行，实际: %+v", beforeConfirm)
	}
	editBeforeConfirm, _ := ApplyNativeWorkflow(opts, NativeEvent{
		Kind: NativeEventBeforeTool, SessionID: sessionID, TurnID: "turn-1",
		Action: NativeActionEdit, Targets: []string{"pkg/demo.go"},
	})
	if !editBeforeConfirm.Block || !strings.Contains(editBeforeConfirm.Reason, "执行") {
		t.Fatalf("未执行前文件编辑应拒绝，实际: %+v", editBeforeConfirm)
	}

	confirm, err := ApplyNativeWorkflow(opts, NativeEvent{
		Kind: NativeEventSubmit, SessionID: sessionID, TurnID: "turn-2", EventID: "evt-2",
		Text: "执行", TrustedUser: true,
	})
	if err != nil || confirm.Block || confirm.AdditionalContext == "" {
		t.Fatalf("确认失败: err=%v dec=%+v", err, confirm)
	}
	ApplyNativeWorkflow(opts, NativeEvent{
		Kind: NativeEventReadCompleted, SessionID: sessionID, TurnID: "turn-2",
		Action: NativeActionRead, Targets: []string{confirm.AdditionalContext}, Success: true,
	})

	for _, cmd := range []string{"git status --short", "agate verify", "go test ./...", "git commit -m \"接通 <hook>\""} {
		dec, _ := ApplyNativeWorkflow(opts, NativeEvent{
			Kind: NativeEventBeforeTool, SessionID: sessionID, TurnID: "turn-2",
			Action: NativeActionShell, Command: cmd,
		})
		if dec.Block {
			t.Errorf("确认后命令 %q 应放行，实际: %s", cmd, dec.Reason)
		}
	}
}

func TestIsTargetInConfirmedScope(t *testing.T) {
	ws := t.TempDir()
	srcDir := filepath.Join(ws, "src")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	mainGo := filepath.Join(srcDir, "main.go")
	if err := os.WriteFile(mainGo, []byte("package main"), 0o644); err != nil {
		t.Fatalf("创建文件失败: %v", err)
	}

	confirmed := []string{"src/main.go", "config.toml"}

	if !IsTargetInConfirmedScope("src/main.go", confirmed, ws) {
		t.Errorf("相对路径应在范围内")
	}
	if !IsTargetInConfirmedScope(mainGo, confirmed, ws) {
		t.Errorf("绝对路径应在范围内")
	}
	if IsTargetInConfirmedScope("src/other.go", confirmed, ws) {
		t.Errorf("未授权文件不应在范围内")
	}
}

func TestNativeOrdinaryReadDeliversPathWithoutReceipt(t *testing.T) {
	tempDir := t.TempDir()
	opts := NativeEventOptions{Host: "cursor", RootDir: tempDir, WorkDir: tempDir, RecordDir: filepath.Join(tempDir, "records")}
	sessionID := "sess-ordinary"
	submit, err := ApplyNativeWorkflow(opts, NativeEvent{
		Kind: NativeEventSubmit, SessionID: sessionID, TurnID: "t1", EventID: "e1",
		Text: "修复 sample.txt", TrustedUser: true,
	})
	if err != nil || submit.Block || submit.AdditionalContext == "" {
		t.Fatalf("提交失败: err=%v dec=%+v", err, submit)
	}

	ordinary, _ := ApplyNativeWorkflow(opts, NativeEvent{
		Kind: NativeEventReadCompleted, SessionID: sessionID, TurnID: "t1",
		Action: NativeActionRead, Targets: []string{filepath.Join(tempDir, "MAP.md")}, Success: true,
	})
	if ordinary.AdditionalContext != submit.AdditionalContext {
		t.Fatalf("待交付时普通读取应返回本轮路径，实际: %+v", ordinary)
	}
	write, _ := ApplyNativeWorkflow(opts, NativeEvent{
		Kind: NativeEventBeforeTool, SessionID: sessionID, TurnID: "t1",
		Action: NativeActionEdit, Targets: []string{"sample.txt"},
	})
	if !write.Block || !strings.Contains(write.Reason, "上下文尚未交付") {
		t.Fatalf("普通读取不得登记回执，写入仍应阻断: %+v", write)
	}

	failed, _ := ApplyNativeWorkflow(opts, NativeEvent{
		Kind: NativeEventReadCompleted, SessionID: sessionID, TurnID: "t1",
		Action: NativeActionRead, Targets: []string{submit.AdditionalContext}, Success: false,
	})
	if failed.AdditionalContext != "" {
		t.Fatalf("失败读取不得交付路径: %+v", failed)
	}

	otherTurn, _ := ApplyNativeWorkflow(opts, NativeEvent{
		Kind: NativeEventReadCompleted, SessionID: sessionID, TurnID: "old",
		Action: NativeActionRead, Targets: []string{filepath.Join(tempDir, "other.md")}, Success: true,
	})
	if otherTurn.AdditionalContext != "" {
		t.Fatalf("其他轮读取不得交付路径: %+v", otherTurn)
	}
}

func TestNativeLateReplyMissingTurnAndCrossSession(t *testing.T) {
	tempDir := t.TempDir()
	opts := NativeEventOptions{Host: "cursor", RootDir: tempDir, WorkDir: tempDir, RecordDir: filepath.Join(tempDir, "records")}
	planText := "现象与预期：x\n问题位置和证据：sample.txt\n因果解释：y\n不确定项：无\n方案依据：z\n\n```agate-plan\n{\"diagnosis\":\"confirmed\",\"files\":[\"sample.txt\"]}\n```\n👉 请确认方案：确认无误请回复‘执行’"

	first, err := ApplyNativeWorkflow(opts, NativeEvent{
		Kind: NativeEventSubmit, SessionID: "sess-a", TurnID: "t1", EventID: "e1",
		Text: "修复 sample.txt", TrustedUser: true,
	})
	if err != nil || first.Block {
		t.Fatalf("首次提交失败: %v %+v", err, first)
	}
	ApplyNativeWorkflow(opts, NativeEvent{
		Kind: NativeEventReadCompleted, SessionID: "sess-a", TurnID: "t1",
		Action: NativeActionRead, Targets: []string{first.AdditionalContext}, Success: true,
	})
	second, err := ApplyNativeWorkflow(opts, NativeEvent{
		Kind: NativeEventSubmit, SessionID: "sess-a", TurnID: "t2", EventID: "e2",
		Text: "换一个任务", TrustedUser: true,
	})
	if err != nil || second.Block {
		t.Fatalf("第二轮提交失败: %v %+v", err, second)
	}
	ApplyNativeWorkflow(opts, NativeEvent{
		Kind: NativeEventResponseCompleted, SessionID: "sess-a", TurnID: "t1", Text: planText,
	})
	lateWrite, _ := ApplyNativeWorkflow(opts, NativeEvent{
		Kind: NativeEventBeforeTool, SessionID: "sess-a", TurnID: "t2",
		Action: NativeActionEdit, Targets: []string{"sample.txt"},
	})
	if !lateWrite.Block {
		t.Fatalf("迟到回复不得登记方案并放行写入: %+v", lateWrite)
	}

	ApplyNativeWorkflow(opts, NativeEvent{
		Kind: NativeEventReadCompleted, SessionID: "sess-a", TurnID: "t2",
		Action: NativeActionRead, Targets: []string{second.AdditionalContext}, Success: true,
	})
	ApplyNativeWorkflow(opts, NativeEvent{
		Kind: NativeEventResponseCompleted, SessionID: "sess-a", TurnID: "t2", Text: planText,
	})
	confirm := NativeEvent{Kind: NativeEventSubmit, SessionID: "sess-a", TurnID: "t3", EventID: "e3", Text: "执行", TrustedUser: true}
	confirmed, err := ApplyNativeWorkflow(opts, confirm)
	if err != nil || confirmed.Block {
		t.Fatalf("确认失败: %v %+v", err, confirmed)
	}
	again, err := ApplyNativeWorkflow(opts, confirm)
	if err != nil || again.Block || again.AdditionalContext == "" {
		t.Fatalf("同一条确认应幂等: err=%v dec=%+v", err, again)
	}
	ApplyNativeWorkflow(opts, NativeEvent{
		Kind: NativeEventReadCompleted, SessionID: "sess-a", TurnID: "t3",
		Action: NativeActionRead, Targets: []string{confirmed.AdditionalContext}, Success: true,
	})
	missingTurn, _ := ApplyNativeWorkflow(opts, NativeEvent{
		Kind: NativeEventBeforeTool, SessionID: "sess-a", Action: NativeActionEdit, Targets: []string{"sample.txt"},
	})
	if missingTurn.Block {
		t.Fatalf("未带轮次的写入应回退到当前活跃轮，实际: %+v", missingTurn)
	}

	otherSession, _ := ApplyNativeWorkflow(opts, NativeEvent{
		Kind: NativeEventBeforeTool, SessionID: "sess-b", TurnID: "t3",
		Action: NativeActionEdit, Targets: []string{"sample.txt"},
	})
	if !otherSession.Block {
		t.Fatalf("跨会话不得复用批准: %+v", otherSession)
	}
}

func TestNativeTwoProcessesSerializeSnapshot(t *testing.T) {
	if os.Getenv("AGATE_LOCK_CHILD") == "1" {
		if err := nativeLockChild(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(0)
	}
	dir := t.TempDir()
	var wg sync.WaitGroup
	errCh := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cmd := exec.Command(os.Args[0], "-test.run=^TestNativeTwoProcessesSerializeSnapshot$", "-test.count=1")
			cmd.Env = append(os.Environ(), "AGATE_LOCK_CHILD=1", "AGATE_LOCK_DIR="+dir)
			out, err := cmd.CombinedOutput()
			if err != nil {
				errCh <- fmt.Errorf("%v: %s", err, out)
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "counter"))
	if err != nil || string(raw) != "2" {
		t.Fatalf("两个进程应串行累加到 2，实际 %q err=%v", raw, err)
	}
}

func nativeLockChild() error {
	dir := os.Getenv("AGATE_LOCK_DIR")
	store := NewNativeStore(dir)
	return store.WithLock("sess-proc", func() error {
		path := filepath.Join(dir, "counter")
		raw, _ := os.ReadFile(path)
		n := 0
		fmt.Sscanf(string(raw), "%d", &n)
		time.Sleep(200 * time.Millisecond)
		return os.WriteFile(path, []byte(fmt.Sprintf("%d", n+1)), 0o600)
	})
}
