package reporter

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"agate/pkg/guard"
)

func TestParseDiffLines(t *testing.T) {
	rawDiff := `diff --git a/main.go b/main.go
index 123..456 100644
--- a/main.go
+++ b/main.go
@@ -1,3 +1,4 @@
 package main
-import "fmt"
+import "os"
+import "log"
`
	lines := ParseDiffLines(rawDiff)
	if len(lines) == 0 {
		t.Fatalf("预期解析出 diff 行，实际为空")
	}

	foundFile := false
	foundHunk := false
	foundAdd := false
	foundDel := false

	for _, l := range lines {
		switch l.Type {
		case "file":
			foundFile = true
		case "hunk":
			foundHunk = true
		case "add":
			foundAdd = true
		case "del":
			foundDel = true
		}
	}

	if !foundFile || !foundHunk || !foundAdd || !foundDel {
		t.Errorf("Diff 解析类型不完整: file=%v, hunk=%v, add=%v, del=%v", foundFile, foundHunk, foundAdd, foundDel)
	}
}

func TestRenderHTMLSelfContained(t *testing.T) {
	data := &ReportData{
		ProjectName:   "agate-demo",
		GeneratedAt:   "2026-09-17 12:00:00",
		GitBranch:     "main",
		GitCommit:     "a1b2c3d",
		Status:        "PASS",
		TotalDuration: "125ms",
		TaskContent:   "## 任务目标: 测试自包含导出",
		AuditResult: &guard.AuditResult{
			Violations: []guard.Violation{
				{
					Level:       "WARN",
					Category:    "资源图片过大",
					File:        "docs/arch.png",
					LineNumber:  1,
					Message:     "建议压缩图片",
					Suggestion:  "使用 tinypng 压缩",
					LineContent: "[binary data]",
				},
			},
		},
		TestOutput: "=== RUN TestDemo\n--- PASS: TestDemo (0.01s)\nPASS",
		TestPassed: true,
		GitDiffLines: []DiffLine{
			{Type: "add", Content: "+new_line()"},
		},
	}

	html, err := RenderHTML(data)
	if err != nil {
		t.Fatalf("RenderHTML 执行失败: %v", err)
	}

	// 1. 验证关键元素存在
	if !strings.Contains(html, "<!DOCTYPE html>") {
		t.Errorf("生成的 HTML 缺少 DOCTYPE 声明")
	}
	if !strings.Contains(html, "agate-demo") {
		t.Errorf("未正确渲染工程名")
	}
	if !strings.Contains(html, "PASS 准予交付") {
		t.Errorf("未正确渲染 PASS 状态徽章")
	}
	if !strings.Contains(html, "资源图片过大") {
		t.Errorf("未正确渲染 AuditResult 违规条目")
	}
	if !strings.Contains(html, "=== RUN TestDemo") {
		t.Errorf("未正确渲染控制台测试输出")
	}
	if !strings.Contains(html, "new_line()") {
		t.Errorf("未正确渲染 Diff 内容")
	}

	// 2. 验证纯离线自包含（无外部 http/https CDN 依赖）
	if strings.Contains(html, "http://") || strings.Contains(html, "https://") {
		t.Errorf("HTML 报告包含外链资源，违背零外网依赖原则")
	}
}

func TestBuildReportAndFindLatest(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "agate-report-test-*")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// 初始化 Git 仓库
	cmdInit := exec.Command("git", "init")
	cmdInit.Dir = tempDir
	if err := cmdInit.Run(); err != nil {
		t.Fatalf("git init 失败: %v", err)
	}

	// 写入模拟 TASK.md 与 context.md
	taskPath := filepath.Join(tempDir, "TASK.md")
	_ = os.WriteFile(taskPath, []byte("# Mock Task\nStatus: In Progress\n"), 0644)
	ctxDir := filepath.Join(tempDir, "contexts")
	_ = os.MkdirAll(ctxDir, 0755)
	_ = os.WriteFile(filepath.Join(ctxDir, "context.md"), []byte("# Mock Context\n"), 0644)

	// 编译输出报告
	outPath, data, err := BuildReport(ReportOptions{
		RootDir:     tempDir,
		AuditResult: &guard.AuditResult{},
		TestOutput:  "All tests pass",
		TestPassed:  true,
		Duration:    "45ms",
	})
	if err != nil {
		t.Fatalf("BuildReport 执行失败: %v", err)
	}

	if data.Status != "PASS" {
		t.Errorf("预期状态为 PASS，实际为: %s", data.Status)
	}

	// 验证文件存在且位于 .ai-memory/reviews/
	if !strings.Contains(outPath, filepath.Join(".ai-memory", "reviews")) {
		t.Errorf("输出路径未存放在 .ai-memory/reviews 目录下: %s", outPath)
	}
	if fi, err := os.Stat(outPath); err != nil || fi.Size() == 0 {
		t.Errorf("报告文件未生成或内容为空: %v", err)
	}

	// 测试 FindLatestReport
	latestPath, err := FindLatestReport(tempDir)
	if err != nil {
		t.Fatalf("FindLatestReport 报错: %v", err)
	}
	if latestPath != outPath {
		t.Errorf("FindLatestReport 预期为 %s，实际得到 %s", outPath, latestPath)
	}
}

func TestBuildReportUsesUniqueDefaultNames(t *testing.T) {
	root := t.TempDir()
	first, _, err := BuildReport(ReportOptions{RootDir: root, TestPassed: true})
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := BuildReport(ReportOptions{RootDir: root, TestPassed: true})
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatalf("连续生成报告不得复用同一文件名: %s", first)
	}
}
