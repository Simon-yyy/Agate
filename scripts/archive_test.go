package scripts

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestArchiveStopsWhenGoBuildFails(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("仅验证 Windows PowerShell 归档脚本")
	}

	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	fakeGo := filepath.Join(t.TempDir(), "failing-go.cmd")
	if err := os.WriteFile(fakeGo, []byte("@echo off\r\nexit /b 1\r\n"), 0755); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command("powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", "scripts/archive.ps1")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GO_BIN="+fakeGo)
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("Go 构建失败时归档脚本必须返回非零退出码，输出: %s", output)
	}
	if strings.Contains(string(output), "归档完成") {
		t.Fatalf("Go 构建失败时不得报告归档完成，输出: %s", output)
	}
}
