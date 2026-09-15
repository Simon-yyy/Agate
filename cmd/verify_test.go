package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

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

	cleanup := func() {
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

