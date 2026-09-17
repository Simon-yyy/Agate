package reporter

import (
	"fmt"
	"os/exec"
	"runtime"
)

// OpenBrowser 在操作系统默认浏览器中打开指定文件路径或 URL
func OpenBrowser(path string) error {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("cmd.exe", "/c", "start", "", path)
	case "darwin":
		cmd = exec.Command("open", path)
	default:
		cmd = exec.Command("xdg-open", path)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动系统浏览器失败: %w", err)
	}

	return nil
}
