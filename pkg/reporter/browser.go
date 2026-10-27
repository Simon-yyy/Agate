package reporter

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

func buildBrowserCommand(goos, path string) (*exec.Cmd, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("浏览器打开路径不能为空")
	}

	switch goos {
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", path), nil
	case "darwin":
		return exec.Command("open", path), nil
	case "linux":
		return exec.Command("xdg-open", path), nil
	default:
		return nil, fmt.Errorf("不支持的操作系统平台: %s", goos)
	}
}

// OpenBrowser 在操作系统默认浏览器中打开指定文件路径或 URL
func OpenBrowser(path string) error {
	cmd, err := buildBrowserCommand(runtime.GOOS, path)
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动系统浏览器失败: %w", err)
	}

	go func() {
		_ = cmd.Wait()
	}()

	return nil
}
