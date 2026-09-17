package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGetGitDiffAndMetadata(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "agate-git-diff-*")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// 初始化 git 仓库
	cmdInit := exec.Command("git", "init")
	cmdInit.Dir = tempDir
	if err := cmdInit.Run(); err != nil {
		t.Fatalf("git init 失败: %v", err)
	}

	// 配置 git user
	_ = exec.Command("git", "-C", tempDir, "config", "user.name", "Agate Tester").Run()
	_ = exec.Command("git", "-C", tempDir, "config", "user.email", "tester@agate.local").Run()

	// 1. 测试首个提交
	helloPath := filepath.Join(tempDir, "hello.txt")
	_ = os.WriteFile(helloPath, []byte("line 1\n"), 0644)
	_ = exec.Command("git", "-C", tempDir, "add", "hello.txt").Run()
	_ = exec.Command("git", "-C", tempDir, "commit", "-m", "initial commit").Run()

	// 验证 metadata
	branch, commit, err := GetGitMetadata(tempDir)
	if err != nil {
		t.Fatalf("GetGitMetadata 报错: %v", err)
	}
	if branch == "" {
		t.Errorf("预期获取到有效分支名，实际为空")
	}
	if commit == "" {
		t.Errorf("预期获取到 commit hash，实际为空")
	}

	// 2. 修改文件但未暂存 (工作区 diff)
	_ = os.WriteFile(helloPath, []byte("line 1\nline 2\n"), 0644)
	diffUnstaged, err := GetGitDiff(false, tempDir)
	if err != nil {
		t.Fatalf("GetGitDiff(unstaged) 报错: %v", err)
	}
	if !strings.Contains(diffUnstaged, "+line 2") {
		t.Errorf("预期 diff 包含 '+line 2'，实际为:\n%s", diffUnstaged)
	}

	// 3. 暂存改动 (staged diff)
	_ = exec.Command("git", "-C", tempDir, "add", "hello.txt").Run()
	diffStaged, err := GetGitDiff(true, tempDir)
	if err != nil {
		t.Fatalf("GetGitDiff(staged) 报错: %v", err)
	}
	if !strings.Contains(diffStaged, "+line 2") {
		t.Errorf("预期暂存区 diff 包含 '+line 2'，实际为:\n%s", diffStaged)
	}
}
