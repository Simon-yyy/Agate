package harness

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScaffoldIdempotentGeneration(t *testing.T) {
	tempDir := t.TempDir()
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(tempDir); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.Chdir(origWd)
	}()

	steps := []struct {
		name string
		path string
		fn   func() (bool, error)
	}{
		{name: ".ignore", path: ".ignore", fn: EnsureIgnore},
		{name: "MAP.md", path: "MAP.md", fn: func() (bool, error) { return EnsureMap("demo-project") }},
		{name: "contexts/context.md", path: filepath.Join("contexts", "context.md"), fn: EnsureContext},
		{name: "TASK.md", path: "TASK.md", fn: EnsureTaskBoard},
		{name: "MEMORY.md", path: "MEMORY.md", fn: EnsureMemory},
	}

	for _, step := range steps {
		created, err := step.fn()
		if err != nil {
			t.Fatalf("首次生成 %s 失败: %v", step.name, err)
		}
		if !created {
			t.Fatalf("首次生成 %s 预期 created=true，实际为 false", step.name)
		}

		// 写入自定义标记内容后再次调用，验证幂等且不覆盖
		customMarker := "CUSTOM_CONTENT_" + step.name
		if err := os.WriteFile(step.path, []byte(customMarker), 0644); err != nil {
			t.Fatalf("写入自定义内容到 %s 失败: %v", step.name, err)
		}

		createdAgain, err := step.fn()
		if err != nil {
			t.Fatalf("二次调用 %s 失败: %v", step.name, err)
		}
		if createdAgain {
			t.Fatalf("二次调用 %s 预期 created=false，实际为 true", step.name)
		}

		got, err := os.ReadFile(step.path)
		if err != nil {
			t.Fatalf("读取 %s 失败: %v", step.name, err)
		}
		if !strings.Contains(string(got), customMarker) {
			t.Fatalf("二次调用 %s 不应覆盖已有内容，实际内容: %q", step.name, string(got))
		}
	}
}
