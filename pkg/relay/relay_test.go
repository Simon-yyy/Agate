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
		TestPassed:   true,
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

func TestHandoverAndDoneBlockWhenTestFailedWithEmptyOutput(t *testing.T) {
	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)
	_ = os.WriteFile(filepath.Join(tmpDir, "AGENTS.md"), []byte("# AGENTS"), 0644)
	_ = os.MkdirAll(filepath.Join(tmpDir, "contexts"), 0755)
	_ = os.WriteFile(filepath.Join(tmpDir, "contexts", "context.md"), []byte("# CONTEXT"), 0644)

	if _, _, err := ClaimTask(tmpDir, "antigravity", false); err != nil {
		t.Fatalf("ClaimTask 失败: %v", err)
	}

	if _, _, err := HandoverTask(HandoverOptions{
		RootDir:      tmpDir,
		CurrentAgent: "antigravity",
		NextAgent:    "cursor",
		SkipVerify:   false,
		TestPassed:   false,
		TestOutput:   "",
	}); err == nil {
		t.Fatal("TestPassed=false 且 TestOutput 为空时 HandoverTask 必须无条件拦截")
	}

	if _, _, err := DoneTask(HandoverOptions{
		RootDir:      tmpDir,
		CurrentAgent: "antigravity",
		SkipVerify:   false,
		TestPassed:   false,
		TestOutput:   "",
	}); err == nil {
		t.Fatal("TestPassed=false 且 TestOutput 为空时 DoneTask 必须无条件拦截")
	}
}

func TestResumeTaskEnforcesNextAgent(t *testing.T) {
	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)
	_ = os.WriteFile(filepath.Join(tmpDir, "AGENTS.md"), []byte("# AGENTS"), 0644)
	_ = os.MkdirAll(filepath.Join(tmpDir, "contexts"), 0755)
	_ = os.WriteFile(filepath.Join(tmpDir, "contexts", "context.md"), []byte("# CONTEXT"), 0644)

	if _, _, err := ClaimTask(tmpDir, "antigravity", false); err != nil {
		t.Fatalf("ClaimTask 失败: %v", err)
	}
	if _, _, err := HandoverTask(HandoverOptions{
		RootDir:      tmpDir,
		CurrentAgent: "antigravity",
		NextAgent:    "cursor",
		SkipVerify:   true,
	}); err != nil {
		t.Fatalf("HandoverTask 失败: %v", err)
	}

	// 非指定 Agent (claude) 尝试接棒应被拒绝
	if _, err := ResumeTask(tmpDir, "claude"); err == nil {
		t.Fatal("非指定 NextAgent 越权执行 ResumeTask 必须被拒绝")
	}

	// 指定 Agent (cursor) 正常接棒应成功
	res, err := ResumeTask(tmpDir, "cursor")
	if err != nil {
		t.Fatalf("指定 NextAgent 执行 ResumeTask 应成功，实际报错: %v", err)
	}
	if res.Manifest.CurrentAgent != "cursor" || res.Manifest.Status != StatusInProgress {
		t.Fatalf("接棒后状态异常: agent=%s status=%s", res.Manifest.CurrentAgent, res.Manifest.Status)
	}
}

func TestAppendNoteWithoutNextActionSuffixAndPipeEscaping(t *testing.T) {
	m := &TaskManifest{
		Body: "## 接力四要素\n1. **已完成事项**: 初始完成\n2. **在途断点**: 无\n3. **接棒建议与下一步**: 继续推进\n4. **暗坑警示**: 无\n",
	}
	appendNoteToBody(m, "新增不带英文后缀标题的批注")
	q := ExtractHandoverQuartet(m.Body)
	if !strings.Contains(q.NextAction, "新增不带英文后缀标题的批注") {
		t.Fatalf("不含 (Next Action) 的标题未能正确追加批注，实际 NextAction: %s", q.NextAction)
	}

	appendHandoverTimeline(m, "antigravity", "cursor", "none", "包含|竖线|的备注")
	if !strings.Contains(m.Body, `包含\|竖线\|的备注`) {
		t.Fatalf("交接时间线未转义竖线字符: %s", m.Body)
	}
}

func TestRelaySameAgentDifferentSessionAndSnapshotExpirationWarning(t *testing.T) {
	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)
	_ = os.WriteFile(filepath.Join(tmpDir, "MAP.md"), []byte("# MAP"), 0644)
	_ = os.MkdirAll(filepath.Join(tmpDir, "contexts"), 0755)
	_ = os.WriteFile(filepath.Join(tmpDir, "contexts", "context.md"), []byte("# CONTEXT"), 0644)
	appFile := filepath.Join(tmpDir, "main.go")
	_ = os.WriteFile(appFile, []byte("package main\nfunc main() {}\n"), 0644)

	t.Setenv("AGATE_SESSION_ID", "sess-1")
	if _, _, err := ClaimTask(tmpDir, "codex", false); err != nil {
		t.Fatalf("会话 1 认领失败: %v", err)
	}

	// 同一工具 codex 的另一会话 sess-2 尝试交接或归档应被拒绝
	t.Setenv("AGATE_SESSION_ID", "sess-2")
	if _, _, err := HandoverTask(HandoverOptions{
		RootDir:      tmpDir,
		CurrentAgent: "codex",
		NextAgent:    "cursor",
		SkipVerify:   true,
	}); err == nil || !strings.Contains(err.Error(), "另一会话") {
		t.Fatalf("预期拦截同一工具另一会话的越权交接，实际: %v", err)
	}

	// 切回会话 1 正常交接
	t.Setenv("AGATE_SESSION_ID", "sess-1")
	manifest, reportPath, err := HandoverTask(HandoverOptions{
		RootDir:      tmpDir,
		CurrentAgent: "codex",
		NextAgent:    "cursor",
		SkipVerify:   false,
		TestPassed:   true,
	})
	if err != nil {
		t.Fatalf("会话 1 交接失败: %v", err)
	}
	if manifest.SnapshotDigest == "" || manifest.SnapshotDigest == "none" {
		t.Fatalf("交接后预期写入有效的 SnapshotDigest，实际: %q", manifest.SnapshotDigest)
	}

	// 交接后修改工作区代码并删除报告文件，验证 ResumeTask 产生双重过期与缺失告警
	_ = os.WriteFile(appFile, []byte("package main\nfunc main() { println(1) }\n"), 0644)
	_ = os.Remove(reportPath)

	t.Setenv("AGATE_SESSION_ID", "sess-cursor")
	res, err := ResumeTask(tmpDir, "cursor")
	if err != nil {
		t.Fatalf("ResumeTask 失败: %v", err)
	}
	joinedWarnings := strings.Join(res.Warnings, "\n")
	if !strings.Contains(joinedWarnings, "交接审查物证文件缺失: ") {
		t.Errorf("未产生交接物证文件缺失告警，实际告警: %v", res.Warnings)
	}
	if !strings.Contains(joinedWarnings, "工作区代码在交接后已发生变化，历史交接物证已过期，请重新运行 agate verify") {
		t.Errorf("未产生交接后代码变更快照过期告警，实际告警: %v", res.Warnings)
	}
}

func TestDetectCurrentAgentCodexAndConfigFallback(t *testing.T) {
	for _, key := range []string{
		"ANTIGRAVITY_AGENT", "GEMINI_AGENT", "ANTIGRAVITY_IDE",
		"CURSOR_AGENT", "CURSOR_VERSION", "CLAUDE_CODE", "CLAUDE_AGENT",
		"WINDSURF_AGENT", "CODEX_SESSION_ID", "CODEX_THREAD_ID",
		"CODEX_VERSION", "CODEX_CI", "CODEX_SHELL", "TERM_PROGRAM", "VSCODE_GIT_IPC_HANDLE",
	} {
		t.Setenv(key, "")
	}

	root := t.TempDir()
	// 1. 仅存在受管 AGENTS.md 时识别为 codex
	_ = os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("<!-- agate:codex-rules:start -->\n<!-- agate:codex-rules:end -->\n"), 0644)
	if got := DetectCurrentAgent(root); got != "codex" {
		t.Fatalf("仅存在受管 AGENTS.md 时预期识别为 codex，实际: %s", got)
	}

	// 2. 多入口并存时回退读取 .agate/config.toml 唯一 target
	_ = os.WriteFile(filepath.Join(root, ".cursorrules"), []byte("cursor"), 0644)
	_ = os.MkdirAll(filepath.Join(root, ".agate"), 0755)
	_ = os.WriteFile(filepath.Join(root, ".agate", "config.toml"), []byte("[agent]\ntargets = [\"cursor\"]\n"), 0644)
	if got := DetectCurrentAgent(root); got != "cursor" {
		t.Fatalf("多入口并存且配置唯一 target=cursor 时预期返回 cursor，实际: %s", got)
	}

	// 3. 配置也为多目标时，回退读取 TASK.md 的 CurrentAgent
	_ = os.WriteFile(filepath.Join(root, ".agate", "config.toml"), []byte("[agent]\ntargets = [\"codex\", \"cursor\"]\n"), 0644)
	_ = SaveTaskBoard(filepath.Join(root, "TASK.md"), &TaskManifest{
		TaskId:       "TASK-1",
		Title:        "测试",
		Status:       StatusInProgress,
		CurrentAgent: "claude",
		NextAgent:    "any",
	})
	if got := DetectCurrentAgent(root); got != "claude" {
		t.Fatalf("多目标无法唯一确定时预期回退 TASK.md CurrentAgent (claude)，实际: %s", got)
	}
}

func TestMultiRoleRelayDevTestOpsAndSessionRoleCardIsolation(t *testing.T) {
	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)

	agentsMDPath := filepath.Join(tmpDir, "AGENTS.md")
	originalAgentsContent := "# AGENTS\nShared rules"
	_ = os.WriteFile(agentsMDPath, []byte(originalAgentsContent), 0644)
	_ = os.MkdirAll(filepath.Join(tmpDir, "contexts"), 0755)
	_ = os.WriteFile(filepath.Join(tmpDir, "contexts", "context.md"), []byte("# CONTEXT"), 0644)
	appFile := filepath.Join(tmpDir, "service.go")
	_ = os.WriteFile(appFile, []byte("package main\n"), 0644)

	// 0. 非法角色名必须拦截
	if _, _, err := ClaimTaskWithRole(tmpDir, "cursor", "hacker", false); err == nil {
		t.Fatal("非法角色名预期报错，实际成功")
	}

	// 1. 开发角色 (dev) 认领任务
	m1, lock1, err := ClaimTaskWithRole(tmpDir, "cursor", RoleDev, false)
	if err != nil {
		t.Fatalf("dev 认领失败: %v", err)
	}
	if m1.CurrentRole != RoleDev || lock1.Role != RoleDev {
		t.Fatalf("认领后角色应为 dev，实际 manifest=%s, lock=%s", m1.CurrentRole, lock1.Role)
	}

	// 验证会话角色卡片生成在 .ai-memory/sessions/active-role.md，且绝不污染 AGENTS.md
	roleCardPath := GetActiveRoleCardPath(tmpDir)
	cardBytes, err := os.ReadFile(roleCardPath)
	if err != nil {
		t.Fatalf("读取 active-role.md 失败: %v", err)
	}
	if !strings.Contains(string(cardBytes), "角色职责不自动授予 Git 提交、推送、发布或生产部署权限，外溢动作仍须人类显式指令授权") {
		t.Errorf("角色卡片缺少安全边界声明: %s", string(cardBytes))
	}
	agentsBytes, _ := os.ReadFile(agentsMDPath)
	if string(agentsBytes) != originalAgentsContent {
		t.Fatalf("角色切换严禁修改共享 AGENTS.md，实际内容变为: %s", string(agentsBytes))
	}

	// 2. 开发角色 (dev) 交接给测试角色 (test)
	m2, _, err := HandoverTask(HandoverOptions{
		RootDir:      tmpDir,
		CurrentAgent: "cursor",
		NextAgent:    "antigravity",
		CurrentRole:  RoleDev,
		NextRole:     RoleTest,
		Note:         "开发完毕，请测试角色验收",
		SkipVerify:   false,
		TestPassed:   true,
	})
	if err != nil {
		t.Fatalf("dev -> test 交接失败: %v", err)
	}
	if m2.CurrentRole != RoleDev || m2.NextRole != RoleTest {
		t.Fatalf("交接后角色异常: current=%s, next=%s", m2.CurrentRole, m2.NextRole)
	}

	// 3. 运维角色 (ops) 试图越权抢接指定给 test 的任务 -> 必须被拦截
	if _, err := ResumeTaskWithRole(tmpDir, "antigravity", RoleOps); err == nil {
		t.Fatal("NextRole=test 时，传入 role=ops 越权接棒必须被拒绝")
	}

	// 4. 篡改代码模拟交接后快照过期，测试角色 (test) 不传 role 自动继承 NextRole=test 并收到角色复验门禁告警
	_ = os.WriteFile(appFile, []byte("package main\n// modified after handover\n"), 0644)
	resTest, err := ResumeTaskWithRole(tmpDir, "antigravity", "")
	if err != nil {
		t.Fatalf("test 角色自动继承接棒失败: %v", err)
	}
	if resTest.Manifest.CurrentRole != RoleTest {
		t.Fatalf("未传 role 时预期自动继承 NextRole=test，实际: %s", resTest.Manifest.CurrentRole)
	}
	joinedWarnings := strings.Join(resTest.Warnings, "\n")
	if !strings.Contains(joinedWarnings, "[角色复验门禁:test]") {
		t.Errorf("test 角色接棒遇到快照过期时应产生角色复验门禁告警，实际: %v", resTest.Warnings)
	}

	// 5. 测试角色 (test) 完成验收后交接给运维角色 (ops)
	m3, _, err := HandoverTask(HandoverOptions{
		RootDir:      tmpDir,
		CurrentAgent: "antigravity",
		NextAgent:    "codex",
		CurrentRole:  RoleTest,
		NextRole:     RoleOps,
		Note:         "测试验收通过，移交运维归档",
		SkipVerify:   false,
		TestPassed:   true,
	})
	if err != nil {
		t.Fatalf("test -> ops 交接失败: %v", err)
	}
	if m3.NextRole != RoleOps {
		t.Fatalf("预期 NextRole=ops，实际: %s", m3.NextRole)
	}

	// 6. 运维角色 (ops) 接棒并执行终态归档 DoneTask
	resOps, err := ResumeTaskWithRole(tmpDir, "codex", RoleOps)
	if err != nil {
		t.Fatalf("ops 接棒失败: %v", err)
	}
	if resOps.Manifest.CurrentRole != RoleOps {
		t.Fatalf("预期 CurrentRole=ops，实际: %s", resOps.Manifest.CurrentRole)
	}

	doneManifest, _, err := DoneTask(HandoverOptions{
		RootDir:      tmpDir,
		CurrentAgent: "codex",
		CurrentRole:  RoleOps,
		Note:         "运维终检通过并归档",
		SkipVerify:   false,
		TestPassed:   true,
	})
	if err != nil {
		t.Fatalf("ops DoneTask 失败: %v", err)
	}
	if doneManifest.Status != StatusDone || doneManifest.CurrentRole != RoleOps || doneManifest.NextRole != "none" {
		t.Fatalf("ops 归档后状态异常: status=%s, currentRole=%s, nextRole=%s",
			doneManifest.Status, doneManifest.CurrentRole, doneManifest.NextRole)
	}
	if !strings.Contains(doneManifest.Body, "snapshot_digest="+doneManifest.SnapshotDigest) {
		t.Errorf("ops 终态交付时间线必须严格绑定最新 snapshot_digest，实际正文:\n%s", doneManifest.Body)
	}
}

