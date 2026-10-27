package cmd

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func initWorkspaceTestRepo(t *testing.T) (string, func()) {
	t.Helper()
	tempDir, err := os.MkdirTemp("", "agate-workspace-test-*")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}

	gitCmd := exec.Command("git", "init")
	gitCmd.Dir = tempDir
	if err := gitCmd.Run(); err != nil {
		os.RemoveAll(tempDir)
		t.Fatalf("git init 失败: %v", err)
	}

	oldWd, err := os.Getwd()
	if err != nil {
		os.RemoveAll(tempDir)
		t.Fatalf("获取当前目录失败: %v", err)
	}
	if err := os.Chdir(tempDir); err != nil {
		os.RemoveAll(tempDir)
		t.Fatalf("切换工作目录失败: %v", err)
	}

	resetFlags := func() {
		rootCmd.SetArgs(nil)
		flagWorkspaceTargets = ""
		flagWorkspaceDryRun = false
	}
	resetFlags()

	cleanup := func() {
		resetFlags()
		_ = os.Chdir(oldWd)
		_ = os.RemoveAll(tempDir)
	}
	return tempDir, cleanup
}

func TestWorkspacePlanApplyDryRunAndRestore(t *testing.T) {
	tempDir, cleanup := initWorkspaceTestRepo(t)
	defer cleanup()

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)

	// 1. 测试 workspace plan 只读预览与诚实性提示
	rootCmd.SetArgs([]string{"workspace", "plan", "-t", "cursor,codex"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("workspace plan 失败: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "cursor.code-workspace") {
		t.Errorf("plan 输出应包含 cursor.code-workspace: %s", out)
	}
	if !strings.Contains(out, "隐藏仅影响当前窗口展示，不代表停止客户端加载") {
		t.Errorf("plan 输出必须包含视图边界诚实提示: %s", out)
	}
	if !strings.Contains(out, "[codex] 降级跳过") {
		t.Errorf("plan 输出应包含 codex 降级提示: %s", out)
	}

	// 2. 测试 workspace apply --dry-run 不落盘
	buf.Reset()
	flagWorkspaceTargets = ""
	flagWorkspaceDryRun = false
	rootCmd.SetArgs([]string{"workspace", "apply", "-t", "cursor", "--dry-run"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("workspace apply --dry-run 失败: %v", err)
	}
	cursorWS := filepath.Join(tempDir, ".ai-memory", "workspaces", "cursor.code-workspace")
	if _, err := os.Stat(cursorWS); !os.IsNotExist(err) {
		t.Fatalf("--dry-run 模式下不应写入物理文件: %s", cursorWS)
	}

	// 3. 测试 workspace apply 正式写入
	buf.Reset()
	flagWorkspaceTargets = ""
	flagWorkspaceDryRun = false
	rootCmd.SetArgs([]string{"workspace", "apply", "-t", "cursor,antigravity"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("workspace apply 失败: %v", err)
	}
	if _, err := os.Stat(cursorWS); err != nil {
		t.Fatalf("apply 后预期存在 cursor.code-workspace: %v", err)
	}
	agWS := filepath.Join(tempDir, ".ai-memory", "workspaces", "antigravity.code-workspace")
	if _, err := os.Stat(agWS); err != nil {
		t.Fatalf("apply 后预期存在 antigravity.code-workspace: %v", err)
	}

	// 4. 测试 workspace restore 清理
	buf.Reset()
	flagWorkspaceTargets = ""
	rootCmd.SetArgs([]string{"workspace", "restore"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("workspace restore 失败: %v", err)
	}
	if _, err := os.Stat(cursorWS); !os.IsNotExist(err) {
		t.Errorf("restore 后 cursor.code-workspace 应已被清理")
	}
	if _, err := os.Stat(agWS); !os.IsNotExist(err) {
		t.Errorf("restore 后 antigravity.code-workspace 应已被清理")
	}
}

func TestWorkspaceUnknownSubcommandReturnsUsageErrorExitCode2(t *testing.T) {
	_, cleanup := initWorkspaceTestRepo(t)
	defer cleanup()

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"workspace", "invalid-sub"})

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("未知子命令预期返回错误，实际成功")
	}
	var usageErr *UsageError
	if !errors.As(err, &usageErr) {
		t.Fatalf("未知子命令预期返回 *UsageError (退出码 2)，实际错误类型: %T (%v)", err, err)
	}
}
