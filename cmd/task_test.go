package cmd

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"agate/pkg/git"
)

func initTaskTestRepo(t *testing.T) (string, func()) {
	t.Helper()
	tempDir, err := os.MkdirTemp("", "agate-task-test-*")
	if err != nil {
		t.Fatalf("创建临时测试目录失败: %v", err)
	}

	gitCmd := exec.Command("git", "init")
	gitCmd.Dir = tempDir
	if err := gitCmd.Run(); err != nil {
		os.RemoveAll(tempDir)
		t.Fatalf("git init 失败: %v", err)
	}

	_ = exec.Command("git", "-C", tempDir, "config", "user.name", "Test").Run()
	_ = exec.Command("git", "-C", tempDir, "config", "user.email", "test@test.com").Run()

	// 补齐架构双地图与隔离
	_ = os.WriteFile(filepath.Join(tempDir, "AGENTS.md"), []byte("# AGENTS"), 0644)
	_ = os.MkdirAll(filepath.Join(tempDir, "contexts"), 0755)
	_ = os.WriteFile(filepath.Join(tempDir, "contexts", "context.md"), []byte("# CONTEXT"), 0644)
	_, _ = git.ApplyPrivateExclusions(git.DefaultExcludedItems, tempDir)

	oldWd, err := os.Getwd()
	if err != nil {
		os.RemoveAll(tempDir)
		t.Fatalf("获取当前目录失败: %v", err)
	}

	if err := os.Chdir(tempDir); err != nil {
		os.RemoveAll(tempDir)
		t.Fatalf("切换工作目录失败: %v", err)
	}

	resetTaskFlags := func() {
		rootCmd.SetArgs(nil)
		flagTaskAgent = ""
		flagTaskForce = false
		flagTaskNextAgent = "any"
		flagTaskNote = ""
		flagTaskSkipVerify = false
	}
	resetTaskFlags()

	cleanup := func() {
		resetTaskFlags()
		_ = os.Chdir(oldWd)
		_ = os.RemoveAll(tempDir)
	}

	return tempDir, cleanup
}

func TestTaskCmdStatusEmptyAndClaim(t *testing.T) {
	_, cleanup := initTaskTestRepo(t)
	defer cleanup()

	// 1. 测试认领任务
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"task", "claim", "--agent=cursor"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("task claim 执行失败: %v, 输出: %s", err, buf.String())
	}
	if !strings.Contains(buf.String(), "任务认领成功") || !strings.Contains(buf.String(), "cursor") {
		t.Errorf("认领输出不符合预期: %s", buf.String())
	}

	// 2. 测试状态查询
	buf.Reset()
	rootCmd.SetArgs([]string{"task", "status"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("task status 执行失败: %v, 输出: %s", err, buf.String())
	}
	if !strings.Contains(buf.String(), "施工锁定中") || !strings.Contains(buf.String(), "cursor") {
		t.Errorf("状态看板输出未包含持锁人: %s", buf.String())
	}
}

func TestTaskCmdHandoverAndResume(t *testing.T) {
	_, cleanup := initTaskTestRepo(t)
	defer cleanup()

	// 1. 认领并初始化
	rootCmd.SetArgs([]string{"task", "claim", "--agent=antigravity"})
	_ = rootCmd.Execute()

	// 2. 执行交接
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"task", "handover", "--agent=antigravity", "--to=cursor", "--note=阶段1已完成"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("task handover 执行失败: %v, 输出: %s", err, buf.String())
	}
	if !strings.Contains(buf.String(), "HANDOVER_READY") || !strings.Contains(buf.String(), "cursor") {
		t.Errorf("交接输出未包含就绪状态: %s", buf.String())
	}

	// 3. 新 Agent 接棒唤醒
	buf.Reset()
	rootCmd.SetArgs([]string{"task", "resume", "--agent=cursor"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("task resume 执行失败: %v, 输出: %s", err, buf.String())
	}
	if !strings.Contains(buf.String(), "成功接棒任务") || !strings.Contains(buf.String(), "cursor") {
		t.Errorf("接棒输出不符合预期: %s", buf.String())
	}
}

func TestTaskCmdConflictProtection(t *testing.T) {
	_, cleanup := initTaskTestRepo(t)
	defer cleanup()

	// 1. Agent A 认领
	rootCmd.SetArgs([]string{"task", "claim", "--agent=antigravity"})
	_ = rootCmd.Execute()

	// 2. Agent B 在未交接且无 force 时尝试认领 -> 应当失败
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"task", "claim", "--agent=cursor"})

	err := rootCmd.Execute()
	if err == nil {
		t.Fatalf("Agent B 在冲突时认领应当失败，但成功了")
	}
	if !strings.Contains(err.Error(), "antigravity") {
		t.Errorf("错误信息应当提示当前持有人: %v", err)
	}

	// 3. Agent B 使用 --force 强制认领 -> 应当成功
	buf.Reset()
	rootCmd.SetArgs([]string{"task", "claim", "--agent=cursor", "--force"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("带 --force 强制认领应当成功，得到错误: %v", err)
	}
}

func TestTaskCmdHandoverDynamicTestVerification(t *testing.T) {
	tempDir, cleanup := initTaskTestRepo(t)
	defer cleanup()

	// 1. Agent A 认领任务
	rootCmd.SetArgs([]string{"task", "claim", "--agent=antigravity"})
	_ = rootCmd.Execute()

	// 2. 写入一个失败的当前平台自检脚本（模拟单测红灯）
	writeVerifyFixture(t, tempDir, false)

	// 3. 执行交接 -> 预期被动态测试门禁拦截拒绝
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"task", "handover", "--agent=antigravity", "--to=cursor"})

	err := rootCmd.Execute()
	if err == nil {
		t.Fatalf("自检脚本失败时，task handover 必须被拦截拒绝，但成功了")
	}
	if !strings.Contains(err.Error(), "动态自检未通过") && !strings.Contains(err.Error(), "自检未通过") {
		t.Errorf("错误提示应包含动态自检失败信息，实际为: %v", err)
	}

	// 检查 TASK.md 状态未被流转为 HANDOVER_READY
	taskContent, _ := os.ReadFile(filepath.Join(tempDir, "TASK.md"))
	if strings.Contains(string(taskContent), "HANDOVER_READY") {
		t.Errorf("自检失败时 TASK.md 不应流转为 HANDOVER_READY: %s", string(taskContent))
	}

	// 4. 携带 --skip-verify 应急交接 -> 预期允许放行
	buf.Reset()
	rootCmd.SetArgs([]string{"task", "handover", "--agent=antigravity", "--to=cursor", "--skip-verify"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("带 --skip-verify 时应跳过自检完成交接，但报错: %v", err)
	}
	if !strings.Contains(buf.String(), "HANDOVER_READY") {
		t.Errorf("跳过自检交接后应输出 HANDOVER_READY: %s", buf.String())
	}
}

func TestTaskCmdDoneWorkflow(t *testing.T) {
	tempDir, cleanup := initTaskTestRepo(t)
	defer cleanup()

	// 1. 认领任务
	rootCmd.SetArgs([]string{"task", "claim", "--agent=cursor"})
	_ = rootCmd.Execute()

	// 2. 模拟单测红灯，task done 应该被阻断
	writeVerifyFixture(t, tempDir, false)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"task", "done", "--agent=cursor"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatalf("单测红灯时 task done 预期被拦截，但成功了")
	}

	// 3. 修复单测，再次调用 task done
	writeVerifyFixture(t, tempDir, true)
	buf.Reset()
	rootCmd.SetArgs([]string{"task", "done", "--agent=cursor", "--note=全量完工验收"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("task done 执行失败: %v, 输出: %s", err, buf.String())
	}
	if !strings.Contains(buf.String(), "DONE") || !strings.Contains(buf.String(), "任务已终态交付") {
		t.Errorf("task done 输出不符合预期: %s", buf.String())
	}

	// 4. 检查 TASK.md 状态为 DONE
	taskContent, _ := os.ReadFile(filepath.Join(tempDir, "TASK.md"))
	if !strings.Contains(string(taskContent), "status: DONE") {
		t.Errorf("TASK.md 状态应为 DONE: %s", string(taskContent))
	}

	// 5. 检查 task status 输出包含 DONE 与最近交接记录
	buf.Reset()
	rootCmd.SetArgs([]string{"task", "status"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("task status 执行失败: %v", err)
	}
	if !strings.Contains(buf.String(), "DONE") || !strings.Contains(buf.String(), "最近交接记录") {
		t.Errorf("task status 输出应包含 DONE 与交接记录: %s", buf.String())
	}
}
