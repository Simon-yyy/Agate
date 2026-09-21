package relay

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"agate/pkg/git"
)

func initGitRepo(t *testing.T, dir string) {
	t.Helper()
	cmd := exec.Command("git", "init")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatalf("git init failed: %v", err)
	}
	_ = exec.Command("git", "-C", dir, "config", "user.name", "Test").Run()
	_ = exec.Command("git", "-C", dir, "config", "user.email", "test@test.com").Run()
	_, _ = git.ApplyPrivateExclusions(git.DefaultExcludedItems, dir)
}

func TestRelayFullLifecycle(t *testing.T) {
	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)

	// 初始文件配置
	_ = os.WriteFile(filepath.Join(tmpDir, "AGENTS.md"), []byte("# AGENTS"), 0644)
	_ = os.MkdirAll(filepath.Join(tmpDir, "contexts"), 0755)
	_ = os.WriteFile(filepath.Join(tmpDir, "contexts", "context.md"), []byte("# CONTEXT"), 0644)

	// 1. Agent A (Antigravity) 认领任务
	m1, lock1, err := ClaimTask(tmpDir, "antigravity", false)
	if err != nil {
		t.Fatalf("ClaimTask 失败: %v", err)
	}
	if m1.Status != StatusClaimed || m1.CurrentAgent != "antigravity" {
		t.Errorf("认领后状态异常: status=%s, agent=%s", m1.Status, m1.CurrentAgent)
	}
	if lock1.OwnerAgent != "antigravity" {
		t.Errorf("持锁人异常: %s", lock1.OwnerAgent)
	}

	// 2. 状态查询验证
	statusRes, err := GetTaskStatus(tmpDir)
	if err != nil {
		t.Fatalf("GetTaskStatus 失败: %v", err)
	}
	if statusRes.Lock == nil || statusRes.Lock.OwnerAgent != "antigravity" {
		t.Errorf("状态查询中锁信息缺失")
	}

	// 3. Agent A 执行交接 (Handover)
	m2, reportPath, err := HandoverTask(HandoverOptions{
		RootDir:      tmpDir,
		CurrentAgent: "antigravity",
		NextAgent:    "cursor",
		Note:         "阶段 1 鉴权通过，请接棒排查单测",
		SkipVerify:   false,
	})
	if err != nil {
		t.Fatalf("HandoverTask 失败: %v", err)
	}
	if m2.Status != StatusHandoverReady {
		t.Errorf("交接后状态应为 HANDOVER_READY，实际为: %s", m2.Status)
	}
	if m2.NextAgent != "cursor" {
		t.Errorf("指定接棒者应为 cursor，实际为: %s", m2.NextAgent)
	}
	if reportPath == "" {
		t.Errorf("交接物证报告路径不应为空")
	}
	if !strings.Contains(m2.Body, "Handover Timeline") || !strings.Contains(m2.Body, "cursor") {
		t.Errorf("交接后正文未正确累积交接流转日志: %s", m2.Body)
	}

	// 4. Agent B (Cursor) 接棒唤醒 (Resume)
	resumeRes, err := ResumeTask(tmpDir, "cursor")
	if err != nil {
		t.Fatalf("ResumeTask 失败: %v", err)
	}
	if resumeRes.Manifest.Status != StatusInProgress {
		t.Errorf("接棒后状态应为 IN_PROGRESS，实际为: %s", resumeRes.Manifest.Status)
	}
	if resumeRes.Manifest.CurrentAgent != "cursor" {
		t.Errorf("接棒后当前执行者应为 cursor，实际为: %s", resumeRes.Manifest.CurrentAgent)
	}
	if resumeRes.Lock.OwnerAgent != "cursor" {
		t.Errorf("新锁持有人应为 cursor")
	}
}

func TestDetectCurrentAgentPrecision(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "agate-detect-test-*")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// 重置环境干扰
	t.Setenv("ANTIGRAVITY_AGENT", "")
	t.Setenv("GEMINI_AGENT", "")
	t.Setenv("ANTIGRAVITY_IDE", "")
	t.Setenv("CURSOR_AGENT", "")
	t.Setenv("CURSOR_VERSION", "")
	t.Setenv("CLAUDE_CODE", "")
	t.Setenv("CLAUDE_AGENT", "")
	t.Setenv("WINDSURF_AGENT", "")
	t.Setenv("CODEX_SESSION_ID", "")
	t.Setenv("CODEX_THREAD_ID", "")
	t.Setenv("CODEX_VERSION", "")
	t.Setenv("CODEX_CI", "")
	t.Setenv("CODEX_SHELL", "")
	t.Setenv("TERM_PROGRAM", "")
	t.Setenv("VSCODE_GIT_IPC_HANDLE", "")

	// 1. 独占 .cursorrules 目录
	_ = os.WriteFile(filepath.Join(tempDir, ".cursorrules"), []byte("cursor"), 0644)
	if a := DetectCurrentAgent(tempDir); a != "cursor" {
		t.Errorf("独占 .cursorrules 预期推断为 cursor，实际为: %s", a)
	}

	// 2. 环境变量优先判定
	t.Setenv("TERM_PROGRAM", "cursor")
	if a := DetectCurrentAgent(tempDir); a != "cursor" {
		t.Errorf("TERM_PROGRAM=cursor 预期推断为 cursor，实际为: %s", a)
	}

	t.Setenv("TERM_PROGRAM", "")
	t.Setenv("CLAUDE_CODE", "1")
	if a := DetectCurrentAgent(tempDir); a != "claude" {
		t.Errorf("CLAUDE_CODE=1 预期推断为 claude，实际为: %s", a)
	}

	// 3. 显式 Antigravity 环境变量
	t.Setenv("CLAUDE_CODE", "")
	t.Setenv("ANTIGRAVITY_AGENT", "1")
	if a := DetectCurrentAgent(tempDir); a != "antigravity" {
		t.Errorf("ANTIGRAVITY_AGENT=1 预期推断为 antigravity，实际为: %s", a)
	}

	t.Setenv("ANTIGRAVITY_AGENT", "")
	t.Setenv("CODEX_SESSION_ID", "session-1")
	if a := DetectCurrentAgent(tempDir); a != "codex" {
		t.Errorf("CODEX_SESSION_ID 存在时预期推断为 codex，实际为: %s", a)
	}
}

func TestDoneTaskAndQuartetClean(t *testing.T) {
	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)

	// 1. 测试 Windows \r\n 换行下的 Frontmatter 解析 (FLAW-2 验证)
	crlfContent := "---\r\ntask_id: TASK-CRLF\r\ntitle: CRLF测试\r\nstatus: IN_PROGRESS\r\ncurrent_agent: tester\r\nnext_agent: any\r\nlast_verified_at: none\r\nreceipt_html: none\r\nupdated_at: 2026-09-17 12:00:00\r\n---\r\n\r\n## 接力四要素\r\n1. **已完成事项**: 完成 CRLF 格式\r\n2. **在途断点**: 无\r\n3. **接棒建议与下一步**: 测试通过\r\n4. **暗坑警示**: 谨防换行污染\r\n\r\n## 📜 跨 Agent 交接流转日志 (Handover Timeline)\r\n| 2026-09-17 | tester | next | doc.html | note |\r\n"
	m, err := ParseTaskBoard(crlfContent)
	if err != nil {
		t.Fatalf("ParseTaskBoard 解析 CRLF 内容失败: %v", err)
	}
	if m.TaskId != "TASK-CRLF" || m.Status != StatusInProgress {
		t.Errorf("CRLF 解析字段不符合预期: %+v", m)
	}

	// 2. 测试 ExtractHandoverQuartet 截断 (FLAW-1 验证: Traps 绝不能包含 Timeline)
	q := ExtractHandoverQuartet(m.Body)
	if strings.Contains(q.Traps, "Handover Timeline") || strings.Contains(q.Traps, "doc.html") {
		t.Errorf("Traps 字段被后续的 Handover Timeline 污染！实际 Traps 内容: %s", q.Traps)
	}
	if !strings.Contains(q.Traps, "谨防换行污染") {
		t.Errorf("Traps 未正确提取核心暗坑: %s", q.Traps)
	}

	// 3. 测试 DoneTask 任务归档与彻底释放锁 (GAP-1 验证)
	taskPath := filepath.Join(tmpDir, "TASK.md")
	if err := SaveTaskBoard(taskPath, m); err != nil {
		t.Fatalf("保存任务看板失败: %v", err)
	}
	// 先认领以加锁
	_, _, _ = ClaimTask(tmpDir, "tester", false)

	doneManifest, outPath, err := DoneTask(HandoverOptions{
		RootDir:      tmpDir,
		CurrentAgent: "tester",
		Note:         "功能全量落地，单测通过",
		SkipVerify:   true,
	})
	if err != nil {
		t.Fatalf("DoneTask 执行失败: %v", err)
	}
	if doneManifest.Status != StatusDone {
		t.Errorf("DoneTask 后状态应为 DONE，实际为: %s", doneManifest.Status)
	}
	if doneManifest.NextAgent != "none" {
		t.Errorf("完成任务后指定接棒者应为 none，实际为: %s", doneManifest.NextAgent)
	}
	if outPath == "" {
		t.Errorf("DoneTask 应当生成终态审查物证报告")
	}

	// 检查锁已彻底释放
	lock, _ := ReadLock(tmpDir)
	if lock != nil {
		t.Errorf("完成任务后互斥锁应被彻底释放，实际仍存在锁")
	}

	// 检查 status 中包含 RecentTimeline (GAP-2 验证)
	statusRes, err := GetTaskStatus(tmpDir)
	if err != nil {
		t.Fatalf("GetTaskStatus 失败: %v", err)
	}
	if len(statusRes.RecentTimeline) == 0 {
		t.Errorf("GetTaskStatus 应包含 RecentTimeline")
	}
}

func TestOnlyLockOwnerCanHandoverOrCompleteTask(t *testing.T) {
	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)
	if _, _, err := ClaimTask(tmpDir, "owner", false); err != nil {
		t.Fatalf("准备持锁任务失败: %v", err)
	}

	if _, _, err := HandoverTask(HandoverOptions{RootDir: tmpDir, CurrentAgent: "intruder", SkipVerify: true}); err == nil {
		t.Fatal("非持锁 Agent 执行 handover 必须被拒绝")
	}
	manifest, err := LoadTaskBoard(filepath.Join(tmpDir, "TASK.md"))
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Status != StatusClaimed {
		t.Fatalf("越权 handover 后任务状态不得变化，实际: %s", manifest.Status)
	}

	if _, _, err := DoneTask(HandoverOptions{RootDir: tmpDir, CurrentAgent: "intruder", SkipVerify: true}); err == nil {
		t.Fatal("非持锁 Agent 执行 done 必须被拒绝")
	}
	manifest, err = LoadTaskBoard(filepath.Join(tmpDir, "TASK.md"))
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Status != StatusClaimed {
		t.Fatalf("越权 done 后任务状态不得变化，实际: %s", manifest.Status)
	}
}

func TestClaimTaskRejectsDoneTask(t *testing.T) {
	tmpDir := t.TempDir()
	if err := SaveTaskBoard(filepath.Join(tmpDir, "TASK.md"), &TaskManifest{TaskId: "TASK-DONE", Title: "已完成任务", Status: StatusDone, CurrentAgent: "owner", NextAgent: "none", ReceiptHTML: "none", LastVerifiedAt: "none"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ClaimTask(tmpDir, "new-agent", false); err == nil {
		t.Fatal("DONE 状态任务不得被重新认领")
	}
}
