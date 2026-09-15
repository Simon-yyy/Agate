package harness

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
)

// CopyFile 执行跨平台物理文件拷贝
func CopyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("打开源文件失败 [%s]: %w", src, err)
	}
	defer srcFile.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return fmt.Errorf("创建目标目录失败: %w", err)
	}

	dstFile, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("创建目标文件失败 [%s]: %w", dst, err)
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return fmt.Errorf("拷贝文件内容失败: %w", err)
	}
	return nil
}

// WriteFileAtomic 原子性或直接覆写文件
func WriteFileAtomic(dst string, content []byte, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return fmt.Errorf("创建目录失败 [%s]: %w", filepath.Dir(dst), err)
	}
	return os.WriteFile(dst, content, perm)
}

// LinkOrCopy 优先尝试符号链接，若在 Windows 或权限受限环境失败，自动平滑降级为物理拷贝
func LinkOrCopy(src, dst string) (bool, error) {
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return false, fmt.Errorf("创建目标目录失败: %w", err)
	}

	// 移除可能已存在的目标文件
	_ = os.Remove(dst)

	// 非 Windows 系统或允许软链时尝试 Symlink
	if runtime.GOOS != "windows" {
		if err := os.Symlink(src, dst); err == nil {
			return true, nil // true 表示创建了软链
		}
	}

	// 降级为物理拷贝
	if err := CopyFile(src, dst); err != nil {
		return false, err
	}
	return false, nil // false 表示降级为拷贝
}
