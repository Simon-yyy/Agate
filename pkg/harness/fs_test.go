package harness

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCopyFile(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "agate-fs-test-*")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(tempDir)

	src := filepath.Join(tempDir, "source.txt")
	dst := filepath.Join(tempDir, "subdir", "dest.txt")
	testData := []byte("Hello, AI Dev Harness!")

	if err := os.WriteFile(src, testData, 0644); err != nil {
		t.Fatalf("创建源文件失败: %v", err)
	}

	if err := CopyFile(src, dst); err != nil {
		t.Fatalf("CopyFile 执行失败: %v", err)
	}

	readData, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("读取目标文件失败: %v", err)
	}

	if string(readData) != string(testData) {
		t.Errorf("数据不一致，预期: %s, 实际: %s", string(testData), string(readData))
	}
}

func TestWriteFileAtomic(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "agate-atomic-test-*")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(tempDir)

	target := filepath.Join(tempDir, "deep", "nested", "file.txt")
	testData := []byte("Atomic Write Data")

	// 首次写入
	if err := WriteFileAtomic(target, testData, 0644); err != nil {
		t.Fatalf("WriteFileAtomic 首次执行失败: %v", err)
	}

	readData, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("读取文件失败: %v", err)
	}
	if string(readData) != string(testData) {
		t.Errorf("写入数据不一致")
	}

	// 覆盖写入测试原子替换
	updatedData := []byte("Updated Atomic Content")
	if err := WriteFileAtomic(target, updatedData, 0644); err != nil {
		t.Fatalf("WriteFileAtomic 覆盖写入失败: %v", err)
	}
	readUpdated, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("读取覆盖后的文件失败: %v", err)
	}
	if string(readUpdated) != string(updatedData) {
		t.Errorf("覆盖写入数据不一致，预期: %s, 实际: %s", string(updatedData), string(readUpdated))
	}
}

func TestLinkOrCopy(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "agate-link-test-*")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(tempDir)

	src := filepath.Join(tempDir, "rule.md")
	dst := filepath.Join(tempDir, "linked-rule.md")
	testContent := []byte("# Rule Single Source of Truth")

	_ = os.WriteFile(src, testContent, 0644)

	// 执行 LinkOrCopy (在 Windows 下将安全降级或软链)
	_, err = LinkOrCopy(src, dst)
	if err != nil {
		t.Fatalf("LinkOrCopy 失败: %v", err)
	}

	readData, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("读取链接/拷贝目标失败: %v", err)
	}

	if string(readData) != string(testContent) {
		t.Errorf("内容不符合预期")
	}
}
