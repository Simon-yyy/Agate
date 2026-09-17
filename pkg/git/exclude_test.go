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

	// 第三次测试传入 nil 时的默认清单，必须包含 .env 与 .env.local
	defaultCount, err := ApplyPrivateExclusions(nil)
	if err != nil {
		t.Fatalf("写入默认排除清单失败: %v", err)
	}
	if defaultCount == 0 {
		t.Errorf("默认排除清单写入项预期大于 0")
	}
	content, _ = os.ReadFile(excludeFile)
	if !strings.Contains(string(content), ".env") || !strings.Contains(string(content), ".env.local") {
		t.Errorf("默认排除清单中缺少 .env 或 .env.local，存在安全策略断裂风险！内容:\n%s", string(content))
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

	cleanGitCmd := func(dir string, args ...string) *exec.Cmd {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		var cleanEnv []string
		for _, e := range os.Environ() {
			if !strings.HasPrefix(e, "GIT_DIR=") &&
				!strings.HasPrefix(e, "GIT_INDEX_FILE=") &&
				!strings.HasPrefix(e, "GIT_WORK_TREE=") &&
				!strings.HasPrefix(e, "GIT_COMMON_DIR=") &&
				!strings.HasPrefix(e, "GIT_PREFIX=") {
				cleanEnv = append(cleanEnv, e)
			}
		}
		cmd.Env = cleanEnv
		return cmd
	}

	// 初始化主仓库
	cmdInit := cleanGitCmd(mainRepo, "init")
	if err := cmdInit.Run(); err != nil {
		t.Skip("本地环境未安装 git，跳过 worktree 实机测试")
	}

	cmdCfgEmail := cleanGitCmd(mainRepo, "config", "user.email", "test@test.com")
	_ = cmdCfgEmail.Run()

	cmdCfgName := cleanGitCmd(mainRepo, "config", "user.name", "test")
	_ = cmdCfgName.Run()

	dummyFile := filepath.Join(mainRepo, "init.txt")
	_ = os.WriteFile(dummyFile, []byte("init"), 0644)
	cmdAdd := cleanGitCmd(mainRepo, "add", "init.txt")
	_ = cmdAdd.Run()

	cmdCommit := cleanGitCmd(mainRepo, "commit", "-m", "init")
	_ = cmdCommit.Run()

	// 创建 git worktree
	cmdWt := cleanGitCmd(mainRepo, "worktree", "add", wtRepo, "-b", "feature")
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

func TestApplyPrivateExclusionsManagedBlockInsertionAndDedup(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "agate-git-managed-*")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(tempDir)

	origWd, _ := os.Getwd()
	_ = os.Chdir(tempDir)
	defer os.Chdir(origWd)

	gitDir := filepath.Join(tempDir, ".git")
	_ = os.MkdirAll(filepath.Join(gitDir, "info"), 0755)

	initialContent := `# user custom rule
*.local

# --- agate private tracking start ---
.gemini/
# --- agate private tracking end ---

# trailing user rule
my-notes.txt
`
	excludePath := filepath.Join(gitDir, "info", "exclude")
	if err := os.WriteFile(excludePath, []byte(initialContent), 0644); err != nil {
		t.Fatalf("写入测试初始 exclude 失败: %v", err)
	}

	// 传入包含自身同批重复的切片
	items := []string{"TASK.md", "TASK.md", ".cursorrules"}
	count, err := ApplyPrivateExclusions(items)
	if err != nil {
		t.Fatalf("ApplyPrivateExclusions 失败: %v", err)
	}
	if count != 2 {
		t.Errorf("同批次去重预期写入 2 项，实际写入: %d", count)
	}

	updated, err := os.ReadFile(excludePath)
	if err != nil {
		t.Fatalf("读取更新后的 exclude 失败: %v", err)
	}
	updatedStr := string(updated)

	// 验证同批去重有效
	if strings.Count(updatedStr, "TASK.md") != 1 {
		t.Errorf("TASK.md 出现次数不为 1，去重失效，内容:\n%s", updatedStr)
	}

	// 验证位置：TASK.md 和 .cursorrules 必须位于 end 标记之前
	startIdx := strings.Index(updatedStr, "# --- agate private tracking start ---")
	endIdx := strings.Index(updatedStr, "# --- agate private tracking end ---")
	taskIdx := strings.Index(updatedStr, "TASK.md")
	cursorIdx := strings.Index(updatedStr, ".cursorrules")
	trailingIdx := strings.Index(updatedStr, "my-notes.txt")

	if taskIdx < startIdx || taskIdx > endIdx {
		t.Errorf("TASK.md 未被正确插入至受管块内部 (start~end之间)")
	}
	if cursorIdx < startIdx || cursorIdx > endIdx {
		t.Errorf(".cursorrules 未被正确插入至受管块内部 (start~end之间)")
	}
	if trailingIdx < endIdx {
		t.Errorf("用户尾部自定义内容 my-notes.txt 遭到移位或破坏")
	}

	// 再次执行，幂等返回 0
	count2, err := ApplyPrivateExclusions(items)
	if err != nil || count2 != 0 {
		t.Errorf("幂等重新写入预期 0，实际: %d, err: %v", count2, err)
	}
}

func TestGetRepoRootSubdirectoryResolution(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "agate-git-root-*")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(tempDir)

	origWd, _ := os.Getwd()
	_ = os.Chdir(tempDir)
	defer os.Chdir(origWd)

	if err := exec.Command("git", "init").Run(); err != nil {
		t.Fatalf("git init 失败: %v", err)
	}

	// 1. 在根目录下解析
	root, err := GetRepoRoot()
	if err != nil {
		t.Fatalf("根目录下 GetRepoRoot 失败: %v", err)
	}
	realTempDir, _ := filepath.EvalSymlinks(tempDir)
	realRoot, _ := filepath.EvalSymlinks(root)
	if realRoot != realTempDir {
		t.Errorf("根目录解析不一致，预期 %s，实际 %s", realTempDir, realRoot)
	}

	// 2. 创建深度子目录并在子目录内解析 (AG-016)
	subDir := filepath.Join(tempDir, "pkg", "deep", "module")
	_ = os.MkdirAll(subDir, 0755)

	_ = os.Chdir(subDir)
	subResolved, err := GetRepoRoot()
	if err != nil {
		t.Fatalf("子目录下 GetRepoRoot 失败: %v", err)
	}
	realSubResolved, _ := filepath.EvalSymlinks(subResolved)
	if realSubResolved != realTempDir {
		t.Errorf("从子目录解析顶层根目录失败，预期 %s，实际 %s", realTempDir, realSubResolved)
	}
}
