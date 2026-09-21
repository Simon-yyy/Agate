package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestInitCmdSuccess(t *testing.T) {
	// 测试默认目标策略时隔离宿主 Codex 环境指纹，避免环境相关结果污染断言。
	for _, key := range []string{
		"CODEX_SESSION_ID", "CODEX_THREAD_ID", "CODEX_VERSION", "CODEX_CI", "CODEX_SHELL",
		"CURSOR_AGENT", "CURSOR_VERSION", "ANTIGRAVITY_AGENT", "GEMINI_AGENT", "ANTIGRAVITY_IDE",
		"CLAUDE_CODE", "CLAUDE_AGENT", "WINDSURF_AGENT", "TERM_PROGRAM", "VSCODE_GIT_IPC_HANDLE",
	} {
		t.Setenv(key, "")
	}
	tempDir, err := os.MkdirTemp("", "agate-init-test-*")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(tempDir)

	origWd, _ := os.Getwd()
	_ = os.Chdir(tempDir)
	defer os.Chdir(origWd)

	cmd := exec.Command("git", "init")
	if err := cmd.Run(); err != nil {
		t.Fatalf("git init 失败: %v", err)
	}

	rootCmd.SetArgs([]string{"init"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("正常 init 执行失败: %v", err)
	}

	// 验证必要文件生成
	requiredFiles := []string{
		".ignore",
		"MAP.md",
		filepath.Join("contexts", "context.md"),
		"TASK.md",
		"MEMORY.md",
		".cursorrules",
	}
	for _, f := range requiredFiles {
		if _, err := os.Stat(filepath.Join(tempDir, f)); err != nil {
			t.Errorf("缺少初始化生成的文件: %s", f)
		}
	}
}

func TestInitCmdRejectsUnknownTargets(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "agate-init-err-*")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(tempDir)

	origWd, _ := os.Getwd()
	_ = os.Chdir(tempDir)
	defer os.Chdir(origWd)

	// 输入打错的目标，预期阻断报错拒绝假成功 (AG-013)
	rootCmd.SetArgs([]string{"init", "--targets", "cursr"})
	err = rootCmd.Execute()
	if err == nil {
		t.Errorf("输入未知目标 'cursr' 预期 init 报错，但返回了 nil")
	}
}
