package cmd

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

	rootCmd.SetArgs([]string{"init", "-t", "cursor", "--no-hook"})
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

func TestInitDryRunAndErrorsDoNotCreateFiles(t *testing.T) {
	root := t.TempDir()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(old)
	for _, args := range [][]string{{"-t", "cursor", "--dry-run"}, {"-t", "typo"}} {
		cmd := newInitCommand()
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs(args)
		err := cmd.Execute()
		if args[1] == "cursor" && (err != nil || !strings.Contains(out.String(), "[create] cursor")) {
			t.Fatalf("预览失败: %v %s", err, out.String())
		}
		if args[1] == "typo" && err == nil {
			t.Fatal("错误目标未拒绝")
		}
		entries, err := os.ReadDir(root)
		if err != nil || len(entries) != 0 {
			t.Fatalf("只读预览或错误输入产生了副作用: %v %v", entries, err)
		}
	}
}

func TestInitCustomProfileUsesOnlySelectedTarget(t *testing.T) {
	root := t.TempDir()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(old)
	if err := os.Mkdir(".agate", 0755); err != nil {
		t.Fatal(err)
	}
	content := `[agent]
targets = ["codex", "cursor"]
[agent.profiles.newtool]
path = ".newtool/rules.md"
format = "markdown"
confirmed = true
source = "用户确认的工具手册"
version = "1.0"
`
	if err := os.WriteFile(filepath.Join(".agate", "config.toml"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("CLAUDE.md", []byte("用户原有 Claude 规则"), 0644); err != nil {
		t.Fatal(err)
	}
	cmd := newInitCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"-t", "newtool", "--no-hook"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("声明工具初始化失败: %v %s", err, out.String())
	}
	data, err := os.ReadFile(filepath.Join(".newtool", "rules.md"))
	if err != nil || !strings.Contains(string(data), "agate:newtool-rules:start") {
		t.Fatalf("新工具规约未受管: %v", err)
	}
	claude, _ := os.ReadFile("CLAUDE.md")
	if string(claude) != "用户原有 Claude 规则" {
		t.Fatal("显式目标修改了其他入口")
	}
	if _, err := os.Stat("AGENTS.md"); !os.IsNotExist(err) {
		t.Fatal("显式目标未覆盖项目默认集合")
	}
	if !strings.Contains(out.String(), "展示建议") || strings.Contains(out.String(), "注入生效") {
		t.Fatal("缺少展示建议或仍然错误宣称加载生效")
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

func TestInitSubdirNormalizationAndDynamicPrivateExclude(t *testing.T) {
	root := t.TempDir()
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(origWd)

	gitInit := exec.Command("git", "init", "--quiet")
	gitInit.Dir = root
	if out, err := gitInit.CombinedOutput(); err != nil {
		t.Fatalf("git init 失败: %v %s", err, out)
	}

	if err := os.MkdirAll(filepath.Join(root, ".agate"), 0755); err != nil {
		t.Fatal(err)
	}
	cfgContent := `[agent]
targets = ["newtool", "codex"]
[agent.profiles.newtool]
path = ".newtool/rules.md"
format = "markdown"
confirmed = true
source = "用户确认的工具手册"
version = "1.0"
`
	if err := os.WriteFile(filepath.Join(root, ".agate", "config.toml"), []byte(cfgContent), 0644); err != nil {
		t.Fatal(err)
	}

	subDir := filepath.Join(root, "cmd", "subpkg")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(subDir); err != nil {
		t.Fatal(err)
	}

	cmd := newInitCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--no-hook"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("子目录执行 init 失败: %v\n%s", err, out.String())
	}

	for _, rel := range []string{".newtool/rules.md", "AGENTS.md", "MAP.md", "contexts/context.md", "TASK.md", "MEMORY.md", ".ignore"} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
			t.Errorf("仓库根目录未生成预期资产 %s: %v", rel, err)
		}
		if _, err := os.Stat(filepath.Join(subDir, filepath.FromSlash(rel))); !os.IsNotExist(err) {
			t.Errorf("子目录中错误生成了资产 %s", rel)
		}
	}

	excludeData, err := os.ReadFile(filepath.Join(root, ".git", "info", "exclude"))
	if err != nil {
		t.Fatalf("读取 .git/info/exclude 失败: %v", err)
	}
	excludeText := string(excludeData)
	if !strings.Contains(excludeText, ".newtool/rules.md") {
		t.Errorf(".git/info/exclude 未包含自定义规则路径 .newtool/rules.md:\n%s", excludeText)
	}
	if !strings.Contains(excludeText, "AGENTS.md") {
		t.Errorf(".git/info/exclude 未包含未跟踪的 AGENTS.md:\n%s", excludeText)
	}
}
