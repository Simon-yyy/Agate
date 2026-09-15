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
	if !strings.Contains(contentStr, "ALLOW_AUTOMATED_PUSH") {
		t.Errorf("pre-push 脚本未包含防自动化偷跑核心拦截逻辑，实际内容:\n%s", contentStr)
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
