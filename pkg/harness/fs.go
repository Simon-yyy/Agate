package harness

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// CopyFile 执行跨平台物理文件拷贝（采用原子写入防止中途崩溃损坏）
func CopyFile(src, dst string) error {
	srcContent, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("打开源文件失败 [%s]: %w", src, err)
	}
	srcInfo, err := os.Stat(src)
	perm := os.FileMode(0644)
	if err == nil {
		perm = srcInfo.Mode().Perm()
	}
	return WriteFileAtomic(dst, srcContent, perm)
}

// WriteFileAtomic 原子性安全写入文件（同卷临时文件写入、fsync 强制刷盘、os.Rename 原子替换）
func WriteFileAtomic(dst string, content []byte, perm os.FileMode) error {
	dir := filepath.Dir(dst)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("创建目标目录失败 [%s]: %w", dir, err)
	}

	// 1. 在目标所在同一目录下创建临时文件，保证与目标处于同一文件系统卷，从而支持原子重命名
	tmpFile, err := os.CreateTemp(dir, ".tmp-atomic-*")
	if err != nil {
		return fmt.Errorf("创建临时文件失败 [%s]: %w", dir, err)
	}
	tmpName := tmpFile.Name()
	cleanTmp := true
	defer func() {
		if cleanTmp {
			_ = os.Remove(tmpName)
		}
	}()

	// 2. 写入数据并强制刷入物理磁盘
	if _, err := tmpFile.Write(content); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("写入临时文件失败 [%s]: %w", tmpName, err)
	}
	if err := tmpFile.Sync(); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("刷新文件缓存至物理介质失败 [%s]: %w", tmpName, err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("关闭临时文件失败 [%s]: %w", tmpName, err)
	}

	// 3. 赋予目标权限
	if err := os.Chmod(tmpName, perm); err != nil {
		return fmt.Errorf("设置临时文件权限失败 [%s]: %w", tmpName, err)
	}

	// 4. 原子重命名替换目标文件
	if err := os.Rename(tmpName, dst); err != nil {
		return fmt.Errorf("原子重命名替换目标文件失败 [%s -> %s]: %w", tmpName, dst, err)
	}

	cleanTmp = false
	return nil
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
