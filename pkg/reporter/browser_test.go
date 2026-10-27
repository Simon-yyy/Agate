package reporter

import (
	"strings"
	"testing"
)

func TestBuildBrowserCommandPlatformsAndSafety(t *testing.T) {
	maliciousPath := `C:\temp\review&calc.exe.html`

	t.Run("windows avoids cmd.exe shell injection", func(t *testing.T) {
		cmd, err := buildBrowserCommand("windows", maliciousPath)
		if err != nil {
			t.Fatalf("buildBrowserCommand(windows) 失败: %v", err)
		}
		if strings.Contains(strings.ToLower(cmd.Path), "cmd") {
			t.Fatalf("Windows 浏览器启动严禁经过 cmd.exe Shell 解释器: %v", cmd.Args)
		}
		if len(cmd.Args) != 3 || cmd.Args[0] != "rundll32" || cmd.Args[1] != "url.dll,FileProtocolHandler" || cmd.Args[2] != maliciousPath {
			t.Fatalf("Windows 命令参数不符合预期: %v", cmd.Args)
		}
	})

	t.Run("darwin uses open directly", func(t *testing.T) {
		cmd, err := buildBrowserCommand("darwin", maliciousPath)
		if err != nil {
			t.Fatalf("buildBrowserCommand(darwin) 失败: %v", err)
		}
		if len(cmd.Args) != 2 || cmd.Args[0] != "open" || cmd.Args[1] != maliciousPath {
			t.Fatalf("darwin 命令参数不符合预期: %v", cmd.Args)
		}
	})

	t.Run("linux uses xdg-open directly", func(t *testing.T) {
		cmd, err := buildBrowserCommand("linux", maliciousPath)
		if err != nil {
			t.Fatalf("buildBrowserCommand(linux) 失败: %v", err)
		}
		if len(cmd.Args) != 2 || cmd.Args[0] != "xdg-open" || cmd.Args[1] != maliciousPath {
			t.Fatalf("linux 命令参数不符合预期: %v", cmd.Args)
		}
	})

	t.Run("unsupported platform returns error", func(t *testing.T) {
		if _, err := buildBrowserCommand("unsupported", maliciousPath); err == nil {
			t.Fatal("不支持的平台预期返回错误")
		}
	})

	t.Run("empty path returns error", func(t *testing.T) {
		if _, err := buildBrowserCommand("windows", "   "); err == nil {
			t.Fatal("空路径预期返回错误")
		}
	})
}
