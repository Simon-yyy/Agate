package cmd

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// writeVerifyFixture 按当前平台生成可执行的自检脚本，避免 Windows 测试依赖 WSL/Bash。
func writeVerifyFixture(t *testing.T, dir string, success bool) string {
	t.Helper()

	name := "verify.sh"
	content := "#!/bin/sh\nexit 0\n"
	if !success {
		content = "#!/bin/sh\necho 'unit test failed' >&2\nexit 1\n"
	}
	if runtime.GOOS == "windows" {
		name = "verify.cmd"
		content = "@echo off\r\nexit /b 0\r\n"
		if !success {
			content = "@echo off\r\necho unit test failed 1>&2\r\nexit /b 1\r\n"
		}
	}

	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0755); err != nil {
		t.Fatalf("写入测试自检脚本失败: %v", err)
	}
	return path
}

// initTestGitRepo 在临时目录初始化一个干净的 Git 仓库并切换当前工作目录
func initTestGitRepo(t *testing.T) (string, func()) {
	t.Helper()
	tempDir, err := os.MkdirTemp("", "agate-cmd-test-*")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}

	// 初始化 git 仓库
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

	// 强制重置全局命令参数与 Flag，确保测试状态彻底隔离 (AG-021)
	resetGlobalFlags := func() {
		rootCmd.SetArgs(nil)
		flagSkipGuard = false
		flagStrict = false
		flagStaged = false
		flagReport = false
		flagResumeAfterBreak = false
		flagExportOutput = ""
		flagExportOpen = false
		flagExportStaged = false
		flagTaskAgent = ""
		flagTaskForce = false
		flagTaskNextAgent = "any"
		flagTaskNote = ""
		flagTaskSkipVerify = false
		for _, name := range []string{"skip-guard", "strict", "staged", "report", "resume-after-break"} {
			if flag := verifyCmd.Flags().Lookup(name); flag != nil {
				flag.Changed = false
			}
		}
	}
	resetGlobalFlags()

	cleanup := func() {
		resetGlobalFlags()
		_ = os.Chdir(oldWd)
		_ = os.RemoveAll(tempDir)
	}

	return tempDir, cleanup
}

func TestVerifyCmdPassOnCleanRepo(t *testing.T) {
	tempDir, cleanup := initTestGitRepo(t)
	defer cleanup()

	// 写入一个合规干净的 Go 源码
	_ = os.WriteFile(filepath.Join(tempDir, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0644)

	rootCmd.SetArgs([]string{"verify"})
	err := rootCmd.Execute()
	if err != nil {
		t.Errorf("干净工程 verify 预期成功，但返回错误: %v", err)
	}
}

func TestVerifyCmdBlockOnDirtyCode(t *testing.T) {
	tempDir, cleanup := initTestGitRepo(t)
	defer cleanup()

	// 注入违规的写死绝对路径代码
	dirtyScript := filepath.Join(tempDir, "run.cmd")
	_ = os.WriteFile(dirtyScript, []byte("@echo off\nset PATH=C:\\Users\\Admin\\bin;%PATH%\n"), 0644)

	rootCmd.SetArgs([]string{"verify"})
	err := rootCmd.Execute()
	if err == nil {
		t.Errorf("存在写死路径反例时，verify 预期返回拦截错误，但返回了 nil")
	}
}

func TestIsolateCmdIntegration(t *testing.T) {
	tempDir, cleanup := initTestGitRepo(t)
	defer cleanup()

	rootCmd.SetArgs([]string{"isolate"})
	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("isolate 预期成功，但返回错误: %v", err)
	}

	// 校验 .git/info/exclude 是否存在并包含 # agate 标记
	excludePath := filepath.Join(tempDir, ".git", "info", "exclude")
	content, err := os.ReadFile(excludePath)
	if err != nil {
		t.Fatalf("无法读取 .git/info/exclude: %v", err)
	}

	strContent := string(content)
	if !strings.Contains(strContent, "agate private tracking start") {
		t.Errorf(".git/info/exclude 未包含期望的 'agate private tracking start' 标记块")
	}
	if !strings.Contains(strContent, "TASK.md") {
		t.Errorf(".git/info/exclude 未包含 TASK.md 隔离项")
	}
}

func TestVerifyCmdSkipGuardFlag(t *testing.T) {
	tempDir, cleanup := initTestGitRepo(t)
	defer cleanup()
	defer func() {
		flagSkipGuard = false
	}()

	// 注入违规的写死绝对路径代码
	dirtyScript := filepath.Join(tempDir, "run.cmd")
	_ = os.WriteFile(dirtyScript, []byte("@echo off\nset PATH=C:\\Users\\Admin\\bin;%PATH%\n"), 0644)

	// 使用 --skip-guard 应急模式，预期绕过拦截
	rootCmd.SetArgs([]string{"verify", "--skip-guard"})
	err := rootCmd.Execute()
	if err != nil {
		t.Errorf("--skip-guard 应急模式预期成功跳过前置护栏，但返回了错误: %v", err)
	}
}

func TestVerifyCmdStrictModeFailsWhenNoTests(t *testing.T) {
	tempDir, cleanup := initTestGitRepo(t)
	defer cleanup()
	defer func() {
		flagStrict = false
	}()

	// 写入合规源码，但无 verify 脚本也无测试套件
	_ = os.WriteFile(filepath.Join(tempDir, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0644)

	// 在严格模式下，预期拒绝假绿灯，返回拦截错误
	rootCmd.SetArgs([]string{"verify", "--strict"})
	err := rootCmd.Execute()
	if err == nil {
		t.Errorf("在未配置任何测试套件的工程中启用 --strict 预期报错拦截，但返回了 nil")
	}
}

func TestVerifyCmdPreservesStrictFailureReason(t *testing.T) {
	tempDir, cleanup := initTestGitRepo(t)
	defer cleanup()
	if err := os.WriteFile(filepath.Join(tempDir, "main.go"), []byte("package main\nfunc main() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"verify", "--strict"})
	if err := rootCmd.Execute(); err == nil {
		t.Fatal("严格模式无测试工程必须失败")
	}
	if !strings.Contains(buf.String(), "未检测到自检脚本或测试套件") {
		t.Fatalf("严格模式失败必须保留根因，实际输出: %s", buf.String())
	}
}

func TestVerifyCmdFailsWhenReportCannotBeWritten(t *testing.T) {
	tempDir, cleanup := initTestGitRepo(t)
	defer cleanup()
	if err := os.WriteFile(filepath.Join(tempDir, "main.go"), []byte("package main\nfunc main() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, ".ai-memory"), []byte("not-a-directory"), 0644); err != nil {
		t.Fatal(err)
	}
	rootCmd.SetArgs([]string{"verify", "--report"})
	if err := rootCmd.Execute(); err == nil || !strings.Contains(err.Error(), "生成 HTML 审查物证报告失败") {
		t.Fatalf("报告生成失败必须作为 verify 失败返回，实际: %v", err)
	}
}

func TestRunDynamicTestsRejectsNpmProjectWithoutTestScript(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tempDir, "package.json"), []byte(`{"name":"sample","scripts":{"build":"echo build"}}`), 0644); err != nil {
		t.Fatal(err)
	}

	passed, summary, err := runDynamicTests(tempDir, true)
	if err == nil || passed {
		t.Fatalf("严格模式必须拒绝无 scripts.test 的 npm 项目: passed=%t summary=%q err=%v", passed, summary, err)
	}
	if !strings.Contains(summary, "未检测到") {
		t.Fatalf("无测试脚本应明确说明未执行测试，实际: %q", summary)
	}
}

func TestInspectFrameworkTestUsesMavenTestGoal(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tempDir, "pom.xml"), []byte("<project/>"), 0644); err != nil {
		t.Fatal(err)
	}

	candidate, ok := inspectFrameworkTest(tempDir)
	if !ok {
		t.Fatal("存在 pom.xml 时应识别 Maven 测试候选项")
	}
	if candidate.command != "mvn" || strings.Join(candidate.args, " ") != "test -q" {
		t.Fatalf("Maven 必须运行真实测试 mvn test -q，实际: %s %s", candidate.command, strings.Join(candidate.args, " "))
	}
}

func TestDetectCustomScriptSkipsUnusableWindowsBash(t *testing.T) {
	if runtime.GOOS != "windows" || bashUsable() {
		t.Skip("仅验证 Windows 下不可用 Bash 的降级路径")
	}
	tempDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tempDir, "verify.sh"), []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatal(err)
	}
	script, shell, args := detectCustomScript(tempDir)
	if script != "" || shell != "" || args != nil {
		t.Fatalf("不可用 Bash 时必须回退框架探测，实际: script=%q shell=%q args=%v", script, shell, args)
	}
}

func TestRunDynamicTestsCapturesScriptOutput(t *testing.T) {
	tempDir := t.TempDir()
	name := "verify.sh"
	content := "#!/bin/sh\necho report-evidence\nexit 0\n"
	if runtime.GOOS == "windows" {
		name = "verify.cmd"
		content = "@echo off\r\necho report-evidence\r\nexit /b 0\r\n"
	}
	if err := os.WriteFile(filepath.Join(tempDir, name), []byte(content), 0755); err != nil {
		t.Fatal(err)
	}
	passed, summary, err := runDynamicTests(tempDir, false)
	if err != nil || !passed {
		t.Fatalf("自检脚本应通过: passed=%t err=%v", passed, err)
	}
	if !strings.Contains(summary, "report-evidence") {
		t.Fatalf("报告摘要必须包含真实脚本输出，实际: %q", summary)
	}
}

func TestVerifyCmdStrictFlagOverridesProjectConfig(t *testing.T) {
	tempDir, cleanup := initTestGitRepo(t)
	defer cleanup()

	if err := os.MkdirAll(filepath.Join(tempDir, ".agate"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, ".agate", "config.toml"), []byte("[guard]\nstrict = true\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}

	rootCmd.SetArgs([]string{"verify"})
	if err := rootCmd.Execute(); err == nil {
		t.Fatal("项目 strict=true 且未指定命令行参数时，应拒绝无测试工程")
	}

	rootCmd.SetArgs([]string{"verify", "--strict=false"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("显式 --strict=false 应覆盖项目配置，但返回错误: %v", err)
	}
}

func TestVerifyCmdStagedMode(t *testing.T) {
	tempDir, cleanup := initTestGitRepo(t)
	defer cleanup()
	defer func() {
		flagStaged = false
	}()

	// 1. 暂存区为空时，verify --staged 应当快速通过
	rootCmd.SetArgs([]string{"verify", "--staged"})
	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("空暂存区执行 verify --staged 预期成功，但返回错误: %v", err)
	}

	// 2. 工作区写入违规文件，但不 git add；同时 git add 一个干净文件
	cleanFile := filepath.Join(tempDir, "main.go")
	_ = os.WriteFile(cleanFile, []byte("package main\n\nfunc main() {}\n"), 0644)
	_ = exec.Command("git", "add", "main.go").Run()

	dirtyWorktree := filepath.Join(tempDir, "dirty.go")
	_ = os.WriteFile(dirtyWorktree, []byte("package main\n\nfunc Debug() {\n\tdebugger\n}\n"), 0644)

	// verify --staged 此时应当只审查 main.go，放行（因为 dirty.go 未暂存）
	rootCmd.SetArgs([]string{"verify", "--staged"})
	err = rootCmd.Execute()
	if err != nil {
		t.Errorf("暂存区干净时 verify --staged 不应受工作区未暂存代码影响，但返回错误: %v", err)
	}

	// 3. 将 dirty.go 加入暂存区，verify --staged 应当坚决拦截！
	_ = exec.Command("git", "add", "dirty.go").Run()
	rootCmd.SetArgs([]string{"verify", "--staged"})
	err = rootCmd.Execute()
	if err == nil {
		t.Errorf("暂存区存在违规代码时 verify --staged 预期拦截失败，但返回了 nil")
	}
}

func TestVerifyCmdRunsFromSubdirectory(t *testing.T) {
	tempDir, cleanup := initTestGitRepo(t)
	defer cleanup()

	// 根目录下放置当前平台可执行的自检脚本
	writeVerifyFixture(t, tempDir, true)

	// 创建深层子目录并在子目录中调用 verify (AG-016)
	subDir := filepath.Join(tempDir, "pkg", "core")
	_ = os.MkdirAll(subDir, 0755)
	_ = os.Chdir(subDir)

	rootCmd.SetArgs([]string{"verify"})
	err := rootCmd.Execute()
	if err != nil {
		t.Errorf("从子目录执行 verify 预期自动寻径到根目录并成功调度 verify.sh，但返回错误: %v", err)
	}
}

func TestVerifyTwoStrikeCircuitBreaker(t *testing.T) {
	tempDir, cleanup := initTestGitRepo(t)
	defer cleanup()

	// 注入失败的自检脚本
	verifyScript := writeVerifyFixture(t, tempDir, false)

	// Strike 1: 第一次失败
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"verify"})
	_ = rootCmd.Execute()

	if strings.Contains(buf.String(), "两振熔断警示") {
		t.Errorf("第一次失败时不应触发两振熔断警示")
	}

	// Strike 2: 第二次连续失败 -> 必须触发两振熔断警示
	buf.Reset()
	rootCmd.SetArgs([]string{"verify"})
	_ = rootCmd.Execute()

	if !strings.Contains(buf.String(), "两振熔断警示") || !strings.Contains(buf.String(), "连续失败第 2 次") {
		t.Errorf("连续第二次失败预期触发两振熔断警示，实际输出: %s", buf.String())
	}

	// Strike 3: 熔断后未经人工恢复不得再次执行脚本。
	buf.Reset()
	rootCmd.SetArgs([]string{"verify"})
	if err := rootCmd.Execute(); err == nil {
		t.Fatal("两振熔断后未显式恢复必须拒绝第三次执行")
	}

	// 修复脚本，变为通过
	if runtime.GOOS == "windows" {
		if err := os.WriteFile(verifyScript, []byte("@echo off\r\nexit /b 0\r\n"), 0755); err != nil {
			t.Fatalf("修复测试自检脚本失败: %v", err)
		}
	} else if err := os.WriteFile(verifyScript, []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatalf("修复测试自检脚本失败: %v", err)
	}
	buf.Reset()
	rootCmd.SetArgs([]string{"verify", "--resume-after-break"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("自检通过执行失败: %v", err)
	}

	// 计数文件应被自动清理重置
	streakFile := filepath.Join(tempDir, ".ai-memory", ".verify_streak")
	if _, err := os.Stat(streakFile); !os.IsNotExist(err) {
		t.Errorf("自检成功后计数文件应被清除重置")
	}
}

func TestTailLines(t *testing.T) {
	if got := tailLines("", 10); got != "" {
		t.Errorf("期望空字符串，实际得到: %q", got)
	}
	inputShort := "line1\nline2\nline3"
	if got := tailLines(inputShort, 5); got != inputShort {
		t.Errorf("短文本不应截断，期望: %q, 实际: %q", inputShort, got)
	}
	inputLong := "1\n2\n3\n4\n5"
	got := tailLines(inputLong, 2)
	if !strings.Contains(got, "前 3 行已截断") || !strings.Contains(got, "4\n5") {
		t.Errorf("长文本截断异常: %s", got)
	}
}

