package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExportCmdBasic(t *testing.T) {
	_, cleanup := initTestGitRepo(t)
	defer cleanup()

	// 1. 执行 agate export
	rootCmd.SetArgs([]string{"export"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("agate export 执行失败: %v", err)
	}

	// 2. 检查 .ai-memory/reviews/ 下是否生成了 html 文件
	reviewsDir := filepath.Join(".ai-memory", "reviews")
	entries, err := os.ReadDir(reviewsDir)
	if err != nil || len(entries) == 0 {
		t.Fatalf("预期在 %s 下生成审查报告，但未找到文件", reviewsDir)
	}

	foundHTML := false
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "review-") && strings.HasSuffix(e.Name(), ".html") {
			foundHTML = true
			content, err := os.ReadFile(filepath.Join(reviewsDir, e.Name()))
			if err != nil || !strings.Contains(string(content), "<!DOCTYPE html>") {
				t.Errorf("审查报告内容异常或缺少 DOCTYPE")
			}
			break
		}
	}

	if !foundHTML {
		t.Errorf("未在 %s 找到 review-*.html 报告文件", reviewsDir)
	}
}

func TestExportCmdCustomOutput(t *testing.T) {
	_, cleanup := initTestGitRepo(t)
	defer cleanup()

	customOut := filepath.Join("reports", "my-audit.html")
	rootCmd.SetArgs([]string{"export", "-o", customOut})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("agate export -o %s 执行失败: %v", customOut, err)
	}

	fi, err := os.Stat(customOut)
	if err != nil || fi.Size() == 0 {
		t.Errorf("自定义路径审查报告未生成或为空: %v", err)
	}
}

func TestVerifyCmdWithReportFlag(t *testing.T) {
	_, cleanup := initTestGitRepo(t)
	defer cleanup()

	// 创建一个当前平台可执行的最简自检脚本
	writeVerifyFixture(t, ".", true)

	// 执行 agate verify --report
	rootCmd.SetArgs([]string{"verify", "--report"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("agate verify --report 执行失败: %v", err)
	}

	// 验证生成了 html
	reviewsDir := filepath.Join(".ai-memory", "reviews")
	entries, err := os.ReadDir(reviewsDir)
	if err != nil || len(entries) == 0 {
		t.Fatalf("agate verify --report 预期生成报告文件，但未找到")
	}
}
