package adapter

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDistributeRules(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "agate-adapter-test-*")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(tempDir)

	origWd, _ := os.Getwd()
	_ = os.Chdir(tempDir)
	defer os.Chdir(origWd)

	customRule := []byte("# Custom Test Rules")
	targets := []AgentTarget{TargetCursor, TargetAntigravity, TargetClaude, TargetWindsurf}

	err = DistributeRules(customRule, targets)
	if err != nil {
		t.Fatalf("分发规则失败: %v", err)
	}

	for _, target := range targets {
		destPath := TargetMapping[target]
		data, err := os.ReadFile(destPath)
		if err != nil {
			t.Errorf("目标规则文件未生成 [%s]: %v", destPath, err)
			continue
		}
		if string(data) != string(customRule) {
			t.Errorf("目标规则文件内容不一致 [%s]", destPath)
		}
	}
}

func TestTargetMappingCompleteness(t *testing.T) {
	expected := []AgentTarget{TargetAntigravity, TargetCursor, TargetClaude, TargetWindsurf}
	for _, target := range expected {
		path, ok := TargetMapping[target]
		if !ok || path == "" {
			t.Errorf("缺少目标映射: %s", target)
		}
	}
	// 验证 Antigravity 路径符合约定
	expectedGemini := filepath.Join(".gemini", "GEMINI.md")
	if TargetMapping[TargetAntigravity] != expectedGemini {
		t.Errorf("Antigravity 路径映射错误，预期 %s，实际 %s", expectedGemini, TargetMapping[TargetAntigravity])
	}
}
