package adapter

import (
	"os"
	"path/filepath"
	"strings"
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
	res, err := ResolveTargets([]string{"cursor"}, tempDir)
	if err != nil || len(res) != 1 || res[0] != TargetCursor {
		t.Errorf("显式指定 cursor 失败，得到: %v, err: %v", res, err)
	}

	// 2. 显式指定 all
	resAll, err := ResolveTargets([]string{"all"}, tempDir)
	if err != nil || len(resAll) != 5 || resAll[4] != TargetCodex {
		t.Errorf("指定 all 预期包含 5 个目标及 codex，得到: %v, err: %v", resAll, err)
	}

	// 3. 空白目录默认推荐 2 个主流目标
	resDefault, err := ResolveTargets(nil, tempDir)
	if err != nil || len(resDefault) != 2 {
		t.Errorf("空白目录预期默认 2 个目标，得到: %v, err: %v", resDefault, err)
	}

	// 4. 嗅探已存在环境 (.cursorrules)
	_ = os.WriteFile(filepath.Join(tempDir, ".cursorrules"), []byte(""), 0644)
	resDetected, err := ResolveTargets(nil, tempDir)
	if err != nil || len(resDetected) != 1 || resDetected[0] != TargetCursor {
		t.Errorf("嗅探已有 .cursorrules 失败，得到: %v, err: %v", resDetected, err)
	}

	// 5. 关键断言：未知目标必须显式报错拦截，拒绝静默吞没 (AG-013)
	_, err = ResolveTargets([]string{"cursr"}, tempDir)
	if err == nil {
		t.Errorf("输入未知目标 'cursr' 预期返回错误，但返回了 nil")
	}

	// 6. 关键断言：重复指定目标自动去重
	resDeduplicated, err := ResolveTargets([]string{"cursor", "cursor", "antigravity"}, tempDir)
	if err != nil {
		t.Fatalf("去重测试失败: %v", err)
	}
	if len(resDeduplicated) != 2 {
		t.Errorf("重复输入预期去重为 2 个目标，实际: %d (%v)", len(resDeduplicated), resDeduplicated)
	}

	resCodex, err := ResolveTargets([]string{"codex"}, tempDir)
	if err != nil || len(resCodex) != 1 || resCodex[0] != TargetCodex {
		t.Errorf("显式指定 codex 失败，得到: %v, err: %v", resCodex, err)
	}
}

func TestDistributeCodexRulesPreservesAgentsMap(t *testing.T) {
	tempDir := t.TempDir()
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(tempDir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(origWd)

	original := "# Project Map\n\n## Modules\n- cmd/\n"
	if err := os.WriteFile("AGENTS.md", []byte(original), 0644); err != nil {
		t.Fatal(err)
	}
	rules := []byte("- Run `agate verify` after changes.")
	if err := DistributeRules(rules, []AgentTarget{TargetCodex}); err != nil {
		t.Fatalf("Codex 规则分发失败: %v", err)
	}
	first, err := os.ReadFile("AGENTS.md")
	if err != nil {
		t.Fatal(err)
	}
	if string(first) == original || !containsAll(string(first), []string{"# Project Map", codexRulesStart, string(rules), codexRulesEnd}) {
		t.Fatalf("Codex 分发未保留地图或缺少受管区块: %s", first)
	}
	if err := DistributeRules(rules, []AgentTarget{TargetCodex}); err != nil {
		t.Fatalf("Codex 规则重复分发失败: %v", err)
	}
	second, _ := os.ReadFile("AGENTS.md")
	if string(first) != string(second) {
		t.Fatalf("重复分发不幂等")
	}
}

func containsAll(value string, parts []string) bool {
	for _, part := range parts {
		if !strings.Contains(value, part) {
			return false
		}
	}
	return true
}

func TestDistributeRulesBackupProtection(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "agate-backup-test-*")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(tempDir)

	origWd, _ := os.Getwd()
	_ = os.Chdir(tempDir)
	defer os.Chdir(origWd)

	// 预先写入用户手工维护的规则
	originalUserRules := []byte("# User Custom Rules - DO NOT OVERWRITE")
	targetFile := ".cursorrules"
	if err := os.WriteFile(targetFile, originalUserRules, 0644); err != nil {
		t.Fatalf("预写用户规约失败: %v", err)
	}

	// 执行分发新的规约
	newRules := []byte("# Agate Managed Skill Rules")
	if err := DistributeRules(newRules, []AgentTarget{TargetCursor}); err != nil {
		t.Fatalf("DistributeRules 执行失败: %v", err)
	}

	// 1. 验证目标文件已更新为新规则
	newData, err := os.ReadFile(targetFile)
	if err != nil || string(newData) != string(newRules) {
		t.Errorf("目标文件未正确更新为新规则")
	}

	// 2. 验证备份文件 .cursorrules.agate.bak 存在且完整保留了用户原始规则
	backupPath := targetFile + ".agate.bak"
	backupData, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatalf("预期存在备份文件 %s，但读取失败: %v", backupPath, err)
	}
	if string(backupData) != string(originalUserRules) {
		t.Errorf("备份文件内容不匹配，预期: %s, 实际: %s", string(originalUserRules), string(backupData))
	}
}

func TestLoadGlobalRules(t *testing.T) {
	tempHome, err := os.MkdirTemp("", "agate-home-*")
	if err != nil {
		t.Fatalf("创建临时 Home 目录失败: %v", err)
	}
	defer os.RemoveAll(tempHome)

	// 1. 未配置任何全局文件时返回 nil, "", nil
	content, path, err := loadGlobalRulesFrom(tempHome)
	if err != nil {
		t.Fatalf("未配置全局规约预期返回 nil err，但得到: %v", err)
	}
	if content != nil || path != "" {
		t.Errorf("未配置全局文件预期返回空，实际: path=%s, content=%v", path, content)
	}

	// 2. 模拟在 ~/.agate/rules.md 放置全局文件
	agateDir := filepath.Join(tempHome, ".agate")
	_ = os.MkdirAll(agateDir, 0755)
	rulesFile := filepath.Join(agateDir, "rules.md")
	testRuleContent := "# My Custom Global Rules"
	_ = os.WriteFile(rulesFile, []byte(testRuleContent), 0644)

	content, path, err = loadGlobalRulesFrom(tempHome)
	if err != nil {
		t.Fatalf("读取全局规约失败: %v", err)
	}
	if path != rulesFile {
		t.Errorf("来源路径预期为 %s，实际为 %s", rulesFile, path)
	}
	if string(content) != testRuleContent {
		t.Errorf("规约内容预期 %s，实际 %s", testRuleContent, string(content))
	}
}
