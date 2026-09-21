package relay

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"agate/pkg/harness"
)

// DefaultLeaseDuration 默认任务租约超时时间（2 小时防死锁）
const DefaultLeaseDuration = 2 * time.Hour

const lockTransitionTimeout = 5 * time.Second
const lockTransitionStaleAfter = 30 * time.Second

// TaskLock 任务互斥软租约锁结构
type TaskLock struct {
	TaskId     string    `json:"task_id"`
	OwnerAgent string    `json:"owner_agent"`
	AcquiredAt time.Time `json:"acquired_at"`
	ExpiresAt  time.Time `json:"expires_at"`
}

// IsExpired 检查租约是否已过期
func (l *TaskLock) IsExpired() bool {
	return time.Now().After(l.ExpiresAt)
}

// GetLockPath 获取锁文件物理路径
func GetLockPath(rootDir string) string {
	if rootDir == "" {
		rootDir = "."
	}
	return filepath.Join(rootDir, ".ai-memory", "locks", "task.lock")
}

func getLockTransitionPath(rootDir string) string {
	return GetLockPath(rootDir) + ".transition"
}

// acquireLockTransitionGuard 在检查与写入任务锁之间建立跨进程互斥，避免多个 Agent 同时读到“无锁”后都写入成功。
func acquireLockTransitionGuard(rootDir string) (func(), error) {
	guardPath := getLockTransitionPath(rootDir)
	if err := os.MkdirAll(filepath.Dir(guardPath), 0755); err != nil {
		return nil, fmt.Errorf("创建任务锁目录失败: %w", err)
	}

	deadline := time.Now().Add(lockTransitionTimeout)
	for {
		guard, err := os.OpenFile(guardPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err == nil {
			if closeErr := guard.Close(); closeErr != nil {
				_ = os.Remove(guardPath)
				return nil, fmt.Errorf("关闭任务锁过渡文件失败: %w", closeErr)
			}
			return func() { _ = os.Remove(guardPath) }, nil
		}
		if !os.IsExist(err) {
			return nil, fmt.Errorf("创建任务锁过渡文件失败: %w", err)
		}

		if info, statErr := os.Stat(guardPath); statErr == nil && time.Since(info.ModTime()) > lockTransitionStaleAfter {
			_ = os.Remove(guardPath)
			continue
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("等待任务锁过渡互斥超时，请稍后重试")
		}
		time.Sleep(time.Millisecond)
	}
}

// ReadLock 读取当前存在的锁信息
func ReadLock(rootDir string) (*TaskLock, error) {
	path := GetLockPath(rootDir)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // 锁不存在
		}
		return nil, fmt.Errorf("读取任务锁失败 [%s]: %w", path, err)
	}

	var lock TaskLock
	if err := json.Unmarshal(data, &lock); err != nil {
		// 损坏的锁视为无效
		return nil, nil
	}

	return &lock, nil
}

// AcquireLock 抢占或续约任务租约锁
func AcquireLock(rootDir string, taskId, agent string, lease time.Duration, force bool) (*TaskLock, error) {
	if lease == 0 {
		lease = DefaultLeaseDuration
	}
	releaseGuard, err := acquireLockTransitionGuard(rootDir)
	if err != nil {
		return nil, err
	}
	defer releaseGuard()

	currentLock, err := ReadLock(rootDir)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	if currentLock != nil && !currentLock.IsExpired() {
		// 锁被其他人持有且未超时
		if currentLock.OwnerAgent != agent && !force {
			return nil, fmt.Errorf("任务当前正被 [%s] 锁定施工 (租约至 %s)。请等待其交接或使用 --force 强制认领",
				currentLock.OwnerAgent, currentLock.ExpiresAt.Format("15:04:05"))
		}
	}

	newLock := &TaskLock{
		TaskId:     taskId,
		OwnerAgent: agent,
		AcquiredAt: now,
		ExpiresAt:  now.Add(lease),
	}

	data, err := json.MarshalIndent(newLock, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("序列化任务锁失败: %w", err)
	}

	lockPath := GetLockPath(rootDir)
	if err := os.MkdirAll(filepath.Dir(lockPath), 0755); err != nil {
		return nil, fmt.Errorf("创建锁目录失败: %w", err)
	}

	if err := harness.WriteFileAtomic(lockPath, data, 0644); err != nil {
		return nil, fmt.Errorf("写入任务锁失败: %w", err)
	}

	return newLock, nil
}

// ReleaseLock 释放任务租约锁
func ReleaseLock(rootDir string, agent string, force bool) error {
	lock, err := ReadLock(rootDir)
	if err != nil {
		return err
	}
	if lock == nil {
		return nil // 无锁可释
	}

	if lock.OwnerAgent != agent && !force && !lock.IsExpired() {
		return fmt.Errorf("无权释放由 [%s] 持有的任务锁 (请使用 --force 强制释放)", lock.OwnerAgent)
	}

	lockPath := GetLockPath(rootDir)
	if err := os.Remove(lockPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("删除任务锁文件失败 [%s]: %w", lockPath, err)
	}

	return nil
}
