package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func runGitTestCommand(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s 失败: %v, output: %s", strings.Join(args, " "), err, string(out))
	}
	return strings.TrimSpace(string(out))
}

func TestGetGitDiffAndMetadata(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "agate-git-diff-*")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// 初始化 git 仓库
	runGitTestCommand(t, tempDir, "init")

	// 配置 git user
	runGitTestCommand(t, tempDir, "config", "user.name", "Agate Tester")
	runGitTestCommand(t, tempDir, "config", "user.email", "tester@agate.local")

	// 1. 测试首个提交
	helloPath := filepath.Join(tempDir, "hello.txt")
	if err := os.WriteFile(helloPath, []byte("line 1\n"), 0644); err != nil {
		t.Fatalf("写入初始测试文件失败: %v", err)
	}
	runGitTestCommand(t, tempDir, "add", "hello.txt")
	// 使用 Git plumbing 创建提交，避免测试依赖 Git Bash/Hook 执行能力。
	tree := runGitTestCommand(t, tempDir, "write-tree")
	commitHash := runGitTestCommand(t, tempDir, "commit-tree", tree, "-m", "initial commit")
	runGitTestCommand(t, tempDir, "symbolic-ref", "HEAD", "refs/heads/main")
	runGitTestCommand(t, tempDir, "update-ref", "refs/heads/main", commitHash)

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
	runGitTestCommand(t, tempDir, "add", "hello.txt")
	diffStaged, err := GetGitDiff(true, tempDir)
	if err != nil {
		t.Fatalf("GetGitDiff(staged) 报错: %v", err)
	}
	if !strings.Contains(diffStaged, "+line 2") {
		t.Errorf("预期暂存区 diff 包含 '+line 2'，实际为:\n%s", diffStaged)
	}
}
