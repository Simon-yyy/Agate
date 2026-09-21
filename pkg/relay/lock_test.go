package relay

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestLockAcquireReleaseAndConflict(t *testing.T) {
	tmpDir := t.TempDir()

	// 1. Agent A 成功加锁
	lockA, err := AcquireLock(tmpDir, "TASK-1", "antigravity", time.Hour, false)
	if err != nil {
		t.Fatalf("Agent A 加锁失败: %v", err)
	}
	if lockA.OwnerAgent != "antigravity" {
		t.Errorf("持锁人错误: %s", lockA.OwnerAgent)
	}

	// 2. Agent B 尝试加锁 -> 应被拦截报错
	_, err = AcquireLock(tmpDir, "TASK-1", "cursor", time.Hour, false)
	if err == nil {
		t.Fatalf("Agent B 应被加锁拦截，但成功了")
	}

	// 3. Agent B 使用 force=true -> 强行抢占成功
	lockB, err := AcquireLock(tmpDir, "TASK-1", "cursor", time.Hour, true)
	if err != nil {
		t.Fatalf("Agent B 强制抢锁失败: %v", err)
	}
	if lockB.OwnerAgent != "cursor" {
		t.Errorf("持锁人应为 cursor，实际为: %s", lockB.OwnerAgent)
	}

	// 4. Agent A 尝试越权释放由 B 持有的锁 -> 应被拒绝
	err = ReleaseLock(tmpDir, "antigravity", false)
	if err == nil {
		t.Fatalf("Agent A 应被拒绝释放 Agent B 的锁")
	}

	// 5. Agent B 正常释放锁
	err = ReleaseLock(tmpDir, "cursor", false)
	if err != nil {
		t.Fatalf("Agent B 释放锁失败: %v", err)
	}

	// 6. 验证锁文件已清理
	currentLock, err := ReadLock(tmpDir)
	if err != nil {
		t.Fatalf("读取锁失败: %v", err)
	}
	if currentLock != nil {
		t.Errorf("释放后锁应为 nil")
	}
}

func TestExpiredLockAllowsNewClaim(t *testing.T) {
	tmpDir := t.TempDir()

	// 创建一个已过期的负时长锁
	_, err := AcquireLock(tmpDir, "TASK-EXPIRED", "old_agent", -time.Minute, false)
	if err != nil {
		t.Fatalf("创建过期锁失败: %v", err)
	}

	// 新 Agent 应该可以直接无感抢占
	newLock, err := AcquireLock(tmpDir, "TASK-EXPIRED", "new_agent", time.Hour, false)
	if err != nil {
		t.Fatalf("新 Agent 抢占过期锁失败: %v", err)
	}
	if newLock.OwnerAgent != "new_agent" {
		t.Errorf("新持锁人应为 new_agent: %s", newLock.OwnerAgent)
	}
}

func TestAcquireLockAllowsOnlyOneConcurrentOwner(t *testing.T) {
	tmpDir := t.TempDir()
	const contenders = 32

	start := make(chan struct{})
	results := make(chan error, contenders)
	var wg sync.WaitGroup
	for i := 0; i < contenders; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			_, err := AcquireLock(tmpDir, "TASK-CONCURRENT", fmt.Sprintf("agent-%d", index), time.Hour, false)
			results <- err
		}(i)
	}

	close(start)
	wg.Wait()
	close(results)

	successes := 0
	for err := range results {
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("并发认领必须仅有一个成功，实际成功数: %d", successes)
	}
}
