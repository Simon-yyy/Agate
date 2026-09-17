package cmd

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestHookStatusCmd(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "agate-hook-cmd-test-*")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(tempDir)

	origWd, _ := os.Getwd()
	_ = os.Chdir(tempDir)
	defer os.Chdir(origWd)

	// 1. 非 Git 仓库测试
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"hook", "status"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("非 Git 仓库执行 hook status 应当正常退出: %v", err)
	}

	// 2. 初始化 Git 仓库
	cmd := exec.Command("git", "init")
	if err := cmd.Run(); err != nil {
		t.Fatalf("git init 失败: %v", err)
	}

	// 3. 检查未安装状态
	buf.Reset()
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("未安装执行 hook status 失败: %v", err)
	}
	outStr := buf.String()
	if !strings.Contains(outStr, "Git 本地物理门禁状态") {
		t.Errorf("输出未包含状态标题: %s", outStr)
	}

	// 4. 执行 hook install
	rootCmd.SetArgs([]string{"hook", "install"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("hook install 失败: %v", err)
	}

	// 5. 再次检查 status，预期 pre-commit 与 pre-push 均为已挂载
	buf.Reset()
	rootCmd.SetArgs([]string{"hook", "status"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("安装后 hook status 失败: %v", err)
	}
	outStr = buf.String()
	if !strings.Contains(outStr, "已挂载 (Agate 托管)") {
		t.Errorf("状态中缺少已挂载提示: %s", outStr)
	}

	// 6. 执行 hook uninstall
	rootCmd.SetArgs([]string{"hook", "uninstall"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("hook uninstall 失败: %v", err)
	}
}
