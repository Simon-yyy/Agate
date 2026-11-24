//go:build windows

package prompt

import (
	"os"
	"path/filepath"
	"testing"

	"agate/pkg/harness"
)

func TestWriteFileAtomicKeepsOldFileWhenLocked(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "state.json")
	old := []byte(`{"compile_status":"old"}`)
	if err := os.WriteFile(dst, old, 0o600); err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(dst, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := lockSessionFile(file); err != nil {
		t.Fatal(err)
	}
	locked := true
	defer func() {
		if locked {
			_ = unlockSessionFile(file)
		}
	}()

	err = harness.WriteFileAtomic(dst, []byte(`{"compile_status":"new"}`), 0o600)
	if err == nil {
		t.Fatal("目标被独占锁定时应拒绝替换")
	}
	if err := unlockSessionFile(file); err != nil {
		t.Fatal(err)
	}
	locked = false
	got, readErr := os.ReadFile(dst)
	if readErr != nil {
		t.Fatalf("替换失败后旧记录应仍可读: %v", readErr)
	}
	if string(got) != string(old) {
		t.Fatalf("替换失败后应保留旧内容，实际: %s", got)
	}
}
