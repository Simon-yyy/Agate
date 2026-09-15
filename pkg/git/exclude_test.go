package git

import (
	"os"
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
