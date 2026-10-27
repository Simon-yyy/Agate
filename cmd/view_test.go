package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestViewCmdArgsAndBehavior(t *testing.T) {
	tempDir, cleanup := initTestGitRepo(t)
	defer cleanup()

	origOpen := openBrowserFunc
	var openedPath string
	openBrowserFunc = func(path string) error {
		openedPath = path
		return nil
	}
	defer func() {
		openBrowserFunc = origOpen
	}()

	// 1. 多余位置参数应当被拒绝
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"view", "extra-arg"})
	if err := rootCmd.Execute(); err == nil {
		t.Fatal("view 传入多余参数应当报错")
	}

	// 2. 无既有报告时自动生成并打开
	buf.Reset()
	openedPath = ""
	rootCmd.SetArgs([]string{"view"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("无既有报告时 view 应自动生成并成功执行，实际报错: %v", err)
	}
	if openedPath == "" {
		t.Fatal("view 应当调用 openBrowserFunc 打开生成的报告")
	}
	if _, err := os.Stat(openedPath); err != nil {
		t.Fatalf("生成的报告文件不存在 [%s]: %v", openedPath, err)
	}
	if !strings.Contains(buf.String(), "未检测到既有审查报告") {
		t.Errorf("无既有报告时应输出提示信息，实际: %s", buf.String())
	}

	// 3. 已有最新报告时直接复用最新报告
	customReport := filepath.Join(tempDir, ".ai-memory", "reviews", "review-99999999-999999.html")
	if err := os.WriteFile(customReport, []byte("<html></html>"), 0644); err != nil {
		t.Fatal(err)
	}
	buf.Reset()
	openedPath = ""
	rootCmd.SetArgs([]string{"view"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("已有报告时 view 执行失败: %v", err)
	}
	if filepath.Base(openedPath) != "review-99999999-999999.html" {
		t.Fatalf("预期打开最新报告 review-99999999-999999.html，实际打开: %s", openedPath)
	}
}
