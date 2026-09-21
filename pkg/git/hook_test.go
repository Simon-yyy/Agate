package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallAndUninstallHooks(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "agate-hook-test-*")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(tempDir)

	origWd, _ := os.Getwd()
	_ = os.Chdir(tempDir)
	defer os.Chdir(origWd)

	// 非 Git 目录应该报错
	if err := InstallHooks(); err == nil {
		t.Errorf("非 Git 目录预期返回错误，但得到 nil")
	}

	// 模拟 git init
	cmd := exec.Command("git", "init")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init 失败: %v, output: %s", err, string(out))
	}

	// 执行安装双重物理门禁
	if err := InstallHooks(); err != nil {
		t.Fatalf("InstallHooks 失败: %v", err)
	}

	// 检查 core.hooksPath
	cfgCmd := exec.Command("git", "config", "--local", "core.hooksPath")
	out, err := cfgCmd.Output()
	if err != nil {
		t.Fatalf("读取 core.hooksPath 失败: %v", err)
	}
	hooksPath := strings.TrimSpace(string(out))
	expectedDir := filepath.Join(".git", "custom-hooks")
	if hooksPath != expectedDir && hooksPath != filepath.ToSlash(expectedDir) {
		t.Errorf("core.hooksPath 不匹配，预期 %s，实际 %s", expectedDir, hooksPath)
	}

	// 检查两个物理脚本文件是否存在
	preCommitPath := filepath.Join(tempDir, ".git", "custom-hooks", "pre-commit")
	if _, err := os.Stat(preCommitPath); err != nil {
		t.Errorf("缺少 pre-commit 物理脚本: %v", err)
	}

	prePushPath := filepath.Join(tempDir, ".git", "custom-hooks", "pre-push")
	if _, err := os.Stat(prePushPath); err != nil {
		t.Errorf("缺少 pre-push 物理脚本: %v", err)
	}

	// 检查 pre-push 脚本内容中是否包含防越权拦截标记
	pushContent, err := os.ReadFile(prePushPath)
	if err != nil {
		t.Fatalf("读取 pre-push 脚本失败: %v", err)
	}
	contentStr := string(pushContent)
	if !strings.Contains(contentStr, `"$ALLOW_AUTOMATED_PUSH" = "1"`) {
		t.Errorf("pre-push 脚本未包含严格的正向白名单授权校验逻辑，实际内容:\n%s", contentStr)
	}
	if !strings.Contains(contentStr, "$CODEX_SESSION_ID") || !strings.Contains(contentStr, "$CODEX_THREAD_ID") {
		t.Errorf("pre-push 脚本未包含 Codex 自动化环境指纹")
	}

	// 执行卸载
	if err := UninstallHooks(); err != nil {
		t.Fatalf("UninstallHooks 失败: %v", err)
	}

	// 检查 hooksPath 已被清空
	checkCmd := exec.Command("git", "config", "--local", "core.hooksPath")
	if err := checkCmd.Run(); err == nil {
		t.Errorf("UninstallHooks 后 core.hooksPath 依然存在")
	}
}

func TestUninstallPreservesCustomUserHooks(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "agate-hook-preserve-*")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(tempDir)

	origWd, _ := os.Getwd()
	_ = os.Chdir(tempDir)
	defer os.Chdir(origWd)

	cmd := exec.Command("git", "init")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init 失败: %v, output: %s", err, string(out))
	}

	// 1. 安装 agate hooks
	if err := InstallHooks(); err != nil {
		t.Fatalf("InstallHooks 失败: %v", err)
	}

	// 2. 模拟用户在 custom-hooks 中放置了第三方的预置钩子
	customHookPath := filepath.Join(tempDir, ".git", "custom-hooks", "pre-rebase")
	userHookContent := []byte("#!/bin/sh\necho 'user custom pre-rebase'\n")
	if err := os.WriteFile(customHookPath, userHookContent, 0755); err != nil {
		t.Fatalf("写入第三方钩子失败: %v", err)
	}

	// 3. 执行卸载 agate 门禁
	if err := UninstallHooks(); err != nil {
		t.Fatalf("UninstallHooks 失败: %v", err)
	}

	// 4. 断言 agate 自身生成的钩子已移除
	preCommitPath := filepath.Join(tempDir, ".git", "custom-hooks", "pre-commit")
	if _, err := os.Stat(preCommitPath); !os.IsNotExist(err) {
		t.Errorf("卸载后 pre-commit 应当已被移除，但依然存在")
	}
	prePushPath := filepath.Join(tempDir, ".git", "custom-hooks", "pre-push")
	if _, err := os.Stat(prePushPath); !os.IsNotExist(err) {
		t.Errorf("卸载后 pre-push 应当已被移除，但依然存在")
	}

	// 5. 关键断言：用户的第三方钩子必须完好无损保留！
	savedContent, err := os.ReadFile(customHookPath)
	if err != nil {
		t.Fatalf("用户自定义钩子被误删！错误: %v", err)
	}
	if string(savedContent) != string(userHookContent) {
		t.Errorf("用户自定义钩子内容遭到篡改")
	}
}

func TestGetHookStatus(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "agate-hook-status-*")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(tempDir)

	origWd, _ := os.Getwd()
	_ = os.Chdir(tempDir)
	defer os.Chdir(origWd)

	// 非 Git 仓库状态检查
	st, err := GetHookStatus()
	if err != nil {
		t.Fatalf("非 Git 目录应当返回 nil error，但得到: %v", err)
	}
	if st.IsGitRepo {
		t.Errorf("非 Git 目录预期 IsGitRepo 为 false，但为 true")
	}

	// 初始化 Git 仓库
	cmd := exec.Command("git", "init")
	if err := cmd.Run(); err != nil {
		t.Fatalf("git init 失败: %v", err)
	}

	// 挂载前状态检查
	st, err = GetHookStatus()
	if err != nil {
		t.Fatalf("GetHookStatus 失败: %v", err)
	}
	if !st.IsGitRepo {
		t.Errorf("预期 IsGitRepo 为 true")
	}
	if st.PreCommitExists || st.PrePushExists {
		t.Errorf("未安装前不应存在门禁脚本")
	}

	// 挂载 Agate 门禁
	if err := InstallHooks(); err != nil {
		t.Fatalf("InstallHooks 失败: %v", err)
	}

	st, err = GetHookStatus()
	if err != nil {
		t.Fatalf("GetHookStatus 失败: %v", err)
	}
	if !st.PreCommitExists || !st.PreCommitAgate {
		t.Errorf("预期 pre-commit 存在且由 Agate 托管，实际: %+v", st)
	}
	if !st.PrePushExists || !st.PrePushAgate {
		t.Errorf("预期 pre-push 存在且由 Agate 托管，实际: %+v", st)
	}
}

func TestGetStagedFiles(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "agate-staged-test-*")
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
	// 配置测试用 git user 避免 commit 警告
	_ = exec.Command("git", "config", "user.name", "AgateTest").Run()
	_ = exec.Command("git", "config", "user.email", "test@agate.local").Run()

	// 暂存区为空
	files, err := GetStagedFiles()
	if err != nil {
		t.Fatalf("GetStagedFiles 失败: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("预期暂存区为空，实际: %v", files)
	}

	// 创建文件但未 git add (工作区未暂存)
	testFile := "hello.txt"
	if err := os.WriteFile(testFile, []byte("world"), 0644); err != nil {
		t.Fatalf("写入测试文件失败: %v", err)
	}

	files, err = GetStagedFiles()
	if err != nil {
		t.Fatalf("GetStagedFiles 失败: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("未 git add 前暂存区应依然为空，实际: %v", files)
	}

	// 执行 git add
	if err := exec.Command("git", "add", testFile).Run(); err != nil {
		t.Fatalf("git add 失败: %v", err)
	}

	files, err = GetStagedFiles()
	if err != nil {
		t.Fatalf("GetStagedFiles 失败: %v", err)
	}
	if len(files) != 1 || files[0] != testFile {
		t.Errorf("预期暂存区包含 [%s]，实际: %v", testFile, files)
	}

	// 测试中文与空格特殊文件名
	specialFile := "订单 详情.go"
	if err := os.WriteFile(specialFile, []byte("package main"), 0644); err != nil {
		t.Fatalf("写入特殊文件失败: %v", err)
	}
	if err := exec.Command("git", "add", specialFile).Run(); err != nil {
		t.Fatalf("git add 特殊文件失败: %v", err)
	}

	files, err = GetStagedFiles()
	if err != nil {
		t.Fatalf("GetStagedFiles 读取特殊文件名失败: %v", err)
	}
	foundSpecial := false
	for _, f := range files {
		if f == specialFile {
			foundSpecial = true
			break
		}
	}
	if !foundSpecial {
		t.Errorf("未正确解析中文与空格文件名 [%s]，实际暂存清单: %v", specialFile, files)
	}
}

func TestPrePushHookAuthorization(t *testing.T) {
	bashPath, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("当前环境未安装 Bash，跳过 Hook shell 集成测试")
	}
	probe := exec.Command(bashPath, "-c", "exit 0")
	if out, err := probe.CombinedOutput(); err != nil {
		t.Skipf("当前环境无法执行 Bash，跳过 Hook shell 集成测试: %v, output: %s", err, string(out))
	}

	tempDir, err := os.MkdirTemp("", "agate-hook-auth-*")
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

	if err := InstallHooks(); err != nil {
		t.Fatalf("InstallHooks 失败: %v", err)
	}

	prePushScript := filepath.Join(tempDir, ".git", "custom-hooks", "pre-push")

	// 辅助执行函数：在子进程中运行 pre-push（非终端环境），设置特定环境变量
	runHookWithEnv := func(envVal string, setEnv bool) (int, string) {
		cmd := exec.Command(bashPath, prePushScript)
		cmd.Dir = tempDir
		var env []string
		for _, e := range os.Environ() {
			if !strings.HasPrefix(e, "ALLOW_AUTOMATED_PUSH=") {
				env = append(env, e)
			}
		}
		if setEnv {
			env = append(env, "ALLOW_AUTOMATED_PUSH="+envVal)
		}
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		exitCode := 0
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				exitCode = exitErr.ExitCode()
			} else {
				exitCode = -1
			}
		}
		return exitCode, string(out)
	}

	// Case 1: 未设置环境变量 -> 应该被拦截 (退出码 1)
	code, out := runHookWithEnv("", false)
	if code != 1 || !strings.Contains(out, "触发 Agate pre-push 物理硬门禁拦截") {
		t.Errorf("未设置 ALLOW_AUTOMATED_PUSH 时预期被拦截，实际 code=%d, out=%s", code, out)
	}

	// Case 2: 设置 ALLOW_AUTOMATED_PUSH=0 -> 应该被拦截 (AG-006 核心验证)
	code, out = runHookWithEnv("0", true)
	if code != 1 || !strings.Contains(out, "触发 Agate pre-push 物理硬门禁拦截") {
		t.Errorf("ALLOW_AUTOMATED_PUSH=0 时预期被拦截，但实际通过！code=%d, out=%s", code, out)
	}

	// Case 3: 设置 ALLOW_AUTOMATED_PUSH=false -> 应该被拦截
	code, out = runHookWithEnv("false", true)
	if code != 1 || !strings.Contains(out, "触发 Agate pre-push 物理硬门禁拦截") {
		t.Errorf("ALLOW_AUTOMATED_PUSH=false 时预期被拦截，但实际通过！code=%d, out=%s", code, out)
	}

	// Case 4: 设置 ALLOW_AUTOMATED_PUSH=1 -> 显式授权通过硬门禁
	_, out = runHookWithEnv("1", true)
	if strings.Contains(out, "触发 Agate pre-push 物理硬门禁拦截") {
		t.Errorf("ALLOW_AUTOMATED_PUSH=1 时预期通过硬门禁，但被拦截: %s", out)
	}

	// Case 5: 设置 ALLOW_AUTOMATED_PUSH=true -> 显式授权通过硬门禁
	_, out = runHookWithEnv("true", true)
	if strings.Contains(out, "触发 Agate pre-push 物理硬门禁拦截") {
		t.Errorf("ALLOW_AUTOMATED_PUSH=true 时预期通过硬门禁，但被拦截: %s", out)
	}

	// Case 6: 存在 Agent 环境变量 (如 CURSOR_AGENT=1) 且未授权 -> 强行拦截 (RSK-001)
	cmdAgent := exec.Command(bashPath, prePushScript)
	cmdAgent.Dir = tempDir
	var agentEnv []string
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, "ALLOW_AUTOMATED_PUSH=") {
			agentEnv = append(agentEnv, e)
		}
	}
	agentEnv = append(agentEnv, "CURSOR_AGENT=1")
	cmdAgent.Env = agentEnv
	outAgent, errAgent := cmdAgent.CombinedOutput()
	if errAgent == nil || !strings.Contains(string(outAgent), "触发 Agate pre-push 物理硬门禁拦截") {
		t.Errorf("存在 CURSOR_AGENT 且未授权时预期被强行拦截，但通过: %s", string(outAgent))
	}
}

func TestHookScriptsContainAgateMarker(t *testing.T) {
	for name, script := range map[string]string{"pre-commit": preCommitScript, "pre-push": prePushScript} {
		if !strings.Contains(script, "# --- agate hook: "+name+" ---") {
			t.Errorf("%s 脚本缺少 SPEC 3.2.2 规定的标识行 `# --- agate hook: %s ---`", name, name)
		}
		if !isAgateManagedHook(script) {
			t.Errorf("%s 脚本应被识别为 agate 托管", name)
		}
	}
}

func TestIsAgateManagedHookRejectsForeignHooks(t *testing.T) {
	foreign := "#!/bin/sh\n# my own hook with the word agate mentioned in a comment\necho hi\n"
	if isAgateManagedHook(foreign) {
		t.Errorf("普通自定义钩子不应被误判为 agate 托管 (误删风险)")
	}
	legacy := "#!/bin/sh\n# agate 自动生成的 pre-commit 验证门禁\nexit 0\n"
	if !isAgateManagedHook(legacy) {
		t.Errorf("历史版本 agate 钩子应被兼容识别 (卸载兼容)")
	}
}

func TestBackupExistingHookCreatesBak(t *testing.T) {
	tmpDir := t.TempDir()
	hookPath := filepath.Join(tmpDir, "pre-commit")
	if err := os.WriteFile(hookPath, []byte("#!/bin/sh\n# user custom hook\nexit 0\n"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := backupExistingHook(hookPath); err != nil {
		t.Fatalf("备份自定义钩子失败: %v", err)
	}
	bak, err := os.ReadFile(hookPath + ".agate.bak")
	if err != nil {
		t.Fatalf("应生成 .agate.bak 备份: %v", err)
	}
	if !strings.Contains(string(bak), "user custom hook") {
		t.Errorf("备份内容与原文件不一致")
	}

	if err := os.WriteFile(hookPath, []byte(preCommitScript), 0755); err != nil {
		t.Fatal(err)
	}
	_ = os.Remove(hookPath + ".agate.bak")
	if err := backupExistingHook(hookPath); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(hookPath + ".agate.bak"); err == nil {
		t.Errorf("托管钩子重装不应再产生备份文件")
	}
}

