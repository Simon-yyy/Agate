package guard

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCleanProjectPasses(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "agate-guard-clean-*")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// 创建标准干净代码
	cleanGo := filepath.Join(tempDir, "main.go")
	_ = os.WriteFile(cleanGo, []byte("package main\n\nfunc main() {}\n"), 0644)

	res := RunPreflightAudit(tempDir)
	if res.HasErrors() {
		t.Errorf("干净工程预期无错误，但捕获到: %v", res.Violations)
	}
}

func TestCatchHardcodedPaths(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "agate-guard-path-*")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// 注入写死的路径反例
	dirtyScript := filepath.Join(tempDir, "run.cmd")
	_ = os.WriteFile(dirtyScript, []byte("@echo off\nset PATH=C:\\Users\\Admin\\bin;%PATH%\n"), 0644)

	res := RunPreflightAudit(tempDir)
	if !res.HasErrors() {
		t.Errorf("未能成功捕获硬编码个人绝对路径")
	}

	found := false
	for _, v := range res.Violations {
		if v.Category == "机器绝对路径泄露" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("未按预期分类为'机器绝对路径泄露'")
	}
}

func TestCatchForbiddenPrivateFiles(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "agate-guard-private-*")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// 注入未隔离的私有文件反例
	_ = os.WriteFile(filepath.Join(tempDir, ".env"), []byte("SECRET_KEY=123456"), 0644)
	_ = os.WriteFile(filepath.Join(tempDir, "TASK.md"), []byte("# Task list"), 0644)

	res := RunPreflightAudit(tempDir)
	if !res.HasErrors() {
		t.Errorf("未能成功捕获 .env 与 TASK.md 私有文件")
	}

	count := 0
	for _, v := range res.Violations {
		if v.Category == "私有文件泄露" {
			count++
		}
	}
	if count != 2 {
		t.Errorf("预期捕获 2 项私有文件违规，实际捕获: %d", count)
	}
}

func TestCatchVendorPollution(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "agate-guard-vendor-*")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// 模拟 vendor 中混入了上游 .github 工作流
	workflowDir := filepath.Join(tempDir, "vendor", "github.com", "upstream", ".github", "workflows")
	_ = os.MkdirAll(workflowDir, 0755)
	_ = os.WriteFile(filepath.Join(workflowDir, "ci.yml"), []byte("name: CI"), 0644)

	res := RunPreflightAudit(tempDir)
	if !res.HasErrors() {
		t.Errorf("未能成功拦截 vendor 混入上游 .github 工作流")
	}

	found := false
	for _, v := range res.Violations {
		if v.Category == "Vendor依赖污染" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("未按预期分类为'Vendor依赖污染'")
	}
}
