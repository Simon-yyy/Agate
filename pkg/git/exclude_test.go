package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplyPrivateExclusions(t *testing.T) {
	// 创建临时测试目录模拟 git 根目录
	tempDir, err := os.MkdirTemp("", "agate-git-test-*")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(tempDir)

	origWd, _ := os.Getwd()
	_ = os.Chdir(tempDir)
	defer os.Chdir(origWd)

	// 未创建 .git 目录时，应安全跳过
	count, err := ApplyPrivateExclusions(nil)
	if err != nil {
		t.Errorf("非 git 仓库应安全返回，但得到错误: %v", err)
	}
	if count != 0 {
		t.Errorf("非 git 仓库预期添加 0 项，实际: %d", count)
	}

	// 模拟创建 .git 目录
	gitDir := filepath.Join(tempDir, ".git")
	_ = os.MkdirAll(gitDir, 0755)

	// 第一次应用排除
	testItems := []string{".gemini/", ".cursorrules", "TASK.md"}
	count, err = ApplyPrivateExclusions(testItems)
	if err != nil {
		t.Fatalf("第一次写入排除失败: %v", err)
	}
	if count != len(testItems) {
		t.Errorf("预期写入 %d 项，实际写入: %d", len(testItems), count)
	}

	// 第二次应用完全相同的项，验证去重能力
	count, err = ApplyPrivateExclusions(testItems)
	if err != nil {
		t.Fatalf("第二次写入排除失败: %v", err)
	}
	if count != 0 {
		t.Errorf("去重失败，重复写入了 %d 项", count)
	}

	// 验证文件内容格式
	excludeFile := filepath.Join(gitDir, "info", "exclude")
	content, err := os.ReadFile(excludeFile)
	if err != nil {
		t.Fatalf("读取 exclude 文件失败: %v", err)
	}

	for _, item := range testItems {
		if !strings.Contains(string(content), item) {
			t.Errorf("exclude 文件中缺少预期条目: %s", item)
		}
	}
}

func TestGitWorktreeCompatibility(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "agate-wt-test-*")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(tempDir)

	origWd, _ := os.Getwd()
	defer os.Chdir(origWd)

	mainRepo := filepath.Join(tempDir, "main")
	wtRepo := filepath.Join(tempDir, "wt")
	_ = os.MkdirAll(mainRepo, 0755)

	// 初始化主仓库
	cmdInit := exec.Command("git", "init")
	cmdInit.Dir = mainRepo
	if err := cmdInit.Run(); err != nil {
		t.Skip("本地环境未安装 git，跳过 worktree 实机测试")
	}

	cmdCfgEmail := exec.Command("git", "config", "user.email", "test@test.com")
	cmdCfgEmail.Dir = mainRepo
	_ = cmdCfgEmail.Run()

	cmdCfgName := exec.Command("git", "config", "user.name", "test")
	cmdCfgName.Dir = mainRepo
	_ = cmdCfgName.Run()

	dummyFile := filepath.Join(mainRepo, "init.txt")
	_ = os.WriteFile(dummyFile, []byte("init"), 0644)
	cmdAdd := exec.Command("git", "add", "init.txt")
	cmdAdd.Dir = mainRepo
	_ = cmdAdd.Run()

	cmdCommit := exec.Command("git", "commit", "-m", "init")
	cmdCommit.Dir = mainRepo
	_ = cmdCommit.Run()

	// 创建 git worktree
	cmdWt := exec.Command("git", "worktree", "add", wtRepo, "-b", "feature")
	cmdWt.Dir = mainRepo
	if out, err := cmdWt.CombinedOutput(); err != nil {
		t.Fatalf("创建 git worktree 失败: %v, 输出: %s", err, string(out))
	}

	// 切换到 worktree 目录
	_ = os.Chdir(wtRepo)

	// 1. 验证 IsGitRepo() 识别 worktree
	if !IsGitRepo() {
		t.Errorf("未能成功识别 git worktree 为 Git 仓库")
	}

	// 2. 验证 GetGitCommonDir() 解析到了主仓目录
	commonDir, err := GetGitCommonDir()
	if err != nil {
		t.Errorf("解析 GitCommonDir 失败: %v", err)
	}
	if !strings.Contains(filepath.ToSlash(commonDir), "/main/.git") && filepath.ToSlash(commonDir) != ".git" {
		t.Errorf("GitCommonDir 预期指向主仓 .git，实际: %s", commonDir)
	}

	// 3. 验证 ApplyPrivateExclusions 在 worktree 下能正常写入
	testItems := []string{".gemini/", "TASK.md"}
	count, err := ApplyPrivateExclusions(testItems)
	if err != nil {
		t.Fatalf("在 worktree 下写入排除失败: %v", err)
	}
	if count != len(testItems) {
		t.Errorf("预期写入 %d 项，实际: %d", len(testItems), count)
	}

	// 4. 验证 InstallHooks 在 worktree 下能成功安装
	if err := InstallHooks(); err != nil {
		t.Fatalf("在 worktree 下安装物理门禁失败: %v", err)
	}
}
