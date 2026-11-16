//go:build windows

package agent

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

type fakeProcessQuerier struct {
	parents map[int]int
	images  map[int]string
	created map[int]time.Time
	fail    map[int]error
	snapErr error
	queries int
}

func (f *fakeProcessQuerier) snapshot() (map[int]int, error) {
	if f.snapErr != nil {
		return nil, f.snapErr
	}
	copied := make(map[int]int, len(f.parents))
	for pid, ppid := range f.parents {
		copied[pid] = ppid
	}
	return copied, nil
}

func (f *fakeProcessQuerier) query(pid int) (string, time.Time, error) {
	f.queries++
	if err := f.fail[pid]; err != nil {
		return "", time.Time{}, err
	}
	return f.images[pid], f.created[pid], nil
}

func TestCollectWindowsAncestorsBoundaries(t *testing.T) {
	t.Run("快照失败不返回进程", func(t *testing.T) {
		q := &fakeProcessQuerier{snapErr: errors.New("access denied")}
		chain, err := collectWindowsAncestors(10, 8, q)
		if err == nil || len(chain) != 0 || q.queries != 0 {
			t.Fatalf("快照失败应直接返回错误，chain=%v err=%v queries=%d", chain, err, q.queries)
		}
	})

	t.Run("访问被拒绝时保留已核验节点", func(t *testing.T) {
		q := &fakeProcessQuerier{
			parents: map[int]int{10: 9, 9: 1},
			images: map[int]string{
				10: `C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe`,
			},
			created: map[int]time.Time{10: time.Unix(200, 0)},
			fail:    map[int]error{9: errors.New("access denied")},
		}
		chain, err := collectWindowsAncestors(10, 8, q)
		if err == nil || len(chain) != 1 || chain[0].PID != 10 {
			t.Fatalf("应保留已核验子进程并报告降级，chain=%v err=%v", chain, err)
		}
	})

	t.Run("疑似 PID 复用时停止且不收录该节点", func(t *testing.T) {
		q := &fakeProcessQuerier{
			parents: map[int]int{10: 9, 9: 1},
			images: map[int]string{
				10: `C:\apps\Code.exe`,
				9:  `C:\apps\Cursor.exe`,
			},
			created: map[int]time.Time{
				10: time.Unix(100, 0),
				9:  time.Unix(300, 0),
			},
		}
		chain, err := collectWindowsAncestors(10, 8, q)
		if err == nil || !strings.Contains(err.Error(), "PID 复用") {
			t.Fatalf("应报告 PID 复用，err=%v", err)
		}
		if len(chain) != 1 || chain[0].PID != 10 {
			t.Fatalf("复用节点不应进入结果，chain=%v", chain)
		}
	})

	t.Run("成环后停止", func(t *testing.T) {
		q := &fakeProcessQuerier{
			parents: map[int]int{2: 3, 3: 2},
			images: map[int]string{
				2: `C:\a\Code.exe`,
				3: `C:\b\Cursor.exe`,
			},
			created: map[int]time.Time{
				2: time.Unix(50, 0),
				3: time.Unix(40, 0),
			},
		}
		chain, err := collectWindowsAncestors(2, 8, q)
		if err == nil || !strings.Contains(err.Error(), "成环") {
			t.Fatalf("应报告成环，err=%v", err)
		}
		if len(chain) != 2 {
			t.Fatalf("成环前的已核验节点应保留，chain=%v", chain)
		}
	})

	t.Run("深度硬上限为 8", func(t *testing.T) {
		q := &fakeProcessQuerier{
			parents: map[int]int{},
			images:  map[int]string{},
			created: map[int]time.Time{},
		}
		for i := 1; i <= 12; i++ {
			q.parents[i] = i + 1
			q.images[i] = `C:\apps\helper.exe`
			q.created[i] = time.Unix(int64(100-i), 0)
		}
		q.parents[12] = 0
		chain, err := collectWindowsAncestors(1, 100, q)
		if err != nil {
			t.Fatalf("有界链路不应失败: %v", err)
		}
		if len(chain) != maxAncestorDepth {
			t.Fatalf("硬上限应为 %d，实际 %d", maxAncestorDepth, len(chain))
		}
	})

	t.Run("默认深度能看到第 8 个节点上的编辑器", func(t *testing.T) {
		q := &fakeProcessQuerier{
			parents: map[int]int{},
			images:  map[int]string{},
			created: map[int]time.Time{},
		}
		for i := 1; i <= 8; i++ {
			q.parents[i] = i + 1
			q.images[i] = `C:\Windows\System32\cmd.exe`
			q.created[i] = time.Unix(int64(100-i), 0)
		}
		q.parents[8] = 0
		q.images[8] = `D:\工具\Cursor\Cursor.exe`
		chain, err := collectWindowsAncestors(1, 0, q)
		if err != nil {
			t.Fatalf("默认深度采集失败: %v", err)
		}
		if len(chain) != 8 || !strings.EqualFold(chain[7].Name, "Cursor.exe") {
			t.Fatalf("第 8 个节点应为 Cursor.exe，chain=%v", chain)
		}
		if strings.Contains(chain[7].Path, "工具") == false {
			t.Fatalf("应保留 Unicode 路径，实际 %s", chain[7].Path)
		}
	})
}

func TestWindowsDefaultInspectorReadsCurrentProcess(t *testing.T) {
	inspector := &DefaultProcessInspector{}
	for i := 0; i < 32; i++ {
		chain, err := inspector.GetAncestors(os.Getpid(), 1)
		if len(chain) != 1 || chain[0].PID != os.Getpid() || chain[0].Path == "" {
			t.Fatalf("第 %d 次未读到当前进程路径: chain=%v err=%v", i, chain, err)
		}
		if err != nil && !strings.Contains(err.Error(), "父进程关系无法核验") && !strings.Contains(err.Error(), "元数据不可读") && !strings.Contains(err.Error(), "PID 复用") && !strings.Contains(err.Error(), "成环") {
			t.Fatalf("当前进程查询出现意外错误: %v", err)
		}
	}
}
