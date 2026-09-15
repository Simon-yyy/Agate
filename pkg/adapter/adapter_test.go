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

func TestResolveTargets(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "agate-resolve-test-*")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// 1. 显式指定单目标
	res := ResolveTargets([]string{"cursor"}, tempDir)
	if len(res) != 1 || res[0] != TargetCursor {
		t.Errorf("显式指定 cursor 失败，得到: %v", res)
	}

	// 2. 显式指定 all
	resAll := ResolveTargets([]string{"all"}, tempDir)
	if len(resAll) != 4 {
		t.Errorf("指定 all 预期 4 个目标，得到: %v", resAll)
	}

	// 3. 空白目录默认推荐 2 个主流目标
	resDefault := ResolveTargets(nil, tempDir)
	if len(resDefault) != 2 {
		t.Errorf("空白目录预期默认 2 个目标，得到: %v", resDefault)
	}

	// 4. 嗅探已存在环境 (.cursorrules)
	_ = os.WriteFile(filepath.Join(tempDir, ".cursorrules"), []byte(""), 0644)
	resDetected := ResolveTargets(nil, tempDir)
	if len(resDetected) != 1 || resDetected[0] != TargetCursor {
		t.Errorf("嗅探已有 .cursorrules 失败，得到: %v", resDetected)
	}
}
