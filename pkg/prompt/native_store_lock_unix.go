//go:build !windows

package prompt

import (
	"fmt"
	"os"
	"syscall"
)

func lockSessionFile(f *os.File) error {
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("获取跨进程会话锁失败: %w", err)
	}
	return nil
}

func unlockSessionFile(f *os.File) error {
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_UN); err != nil {
		return fmt.Errorf("释放跨进程会话锁失败: %w", err)
	}
	return nil
}
