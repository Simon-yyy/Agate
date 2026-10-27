package adapter

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"agate/pkg/agent"
)

func applyTestPlan(t *testing.T, root string, rules string, targets ...AgentTarget) *Plan {
	t.Helper()
	plan, err := BuildPlan(root, []byte(rules), targets, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := plan.Apply(); err != nil {
		t.Fatal(err)
	}
	return plan
}

func TestPlanCoexistencePreservesOtherTargets(t *testing.T) {
	root := t.TempDir()
	applyTestPlan(t, root, "规则版本 A", TargetCodex, TargetCursor, TargetClaude)
	codexBefore, _ := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	claudeBefore, _ := os.ReadFile(filepath.Join(root, "CLAUDE.md"))
	plan := applyTestPlan(t, root, "规则版本 B", TargetCursor)
	for path, before := range map[string][]byte{"AGENTS.md": codexBefore, "CLAUDE.md": claudeBefore} {
		after, err := os.ReadFile(filepath.Join(root, path))
		if err != nil || !bytes.Equal(before, after) {
			t.Fatalf("更新 Cursor 影响了 %s", path)
		}
	}
	diagnostics := strings.Join(plan.Diagnostics(), "\n")
	if !strings.Contains(diagnostics, "可能同时读取") || !strings.Contains(diagnostics, "不代表全部入口同版") {
		t.Fatalf("重叠与版本诊断缺失: %s", diagnostics)
	}
	second, err := BuildPlan(root, []byte("规则版本 B"), []AgentTarget{TargetCursor}, nil, false)
	if err != nil || second.Entries()[0].Action != "skip" {
		t.Fatalf("重复初始化不幂等: %v", err)
	}
	if err := second.Apply(); err != nil {
		t.Fatal(err)
	}
}

func TestPlanRejectsStaleSnapshotBeforeAnyWrites(t *testing.T) {
	root := t.TempDir()
	applyTestPlan(t, root, "版本 A", TargetCursor, TargetClaude)
	plan, err := BuildPlan(root, []byte("版本 B"), []AgentTarget{TargetCursor, TargetClaude}, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	cursorBefore, _ := os.ReadFile(filepath.Join(root, ".cursorrules"))
	if err := os.WriteFile(filepath.Join(root, "CLAUDE.md"), []byte("用户期间修改"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := plan.Apply(); err == nil {
		t.Fatal("旧计划覆盖了用户期间修改")
	}
	cursorAfter, _ := os.ReadFile(filepath.Join(root, ".cursorrules"))
	if !bytes.Equal(cursorBefore, cursorAfter) {
		t.Fatal("过期检查之前发生了部分写入")
	}
}

func TestPlanMalformedMarkersBlockAllTargets(t *testing.T) {
	for _, broken := range []string{codexRulesStart, codexRulesEnd + "\n" + codexRulesStart, codexRulesStart + "\n" + codexRulesStart + "\n" + codexRulesEnd} {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte(broken), 0644); err != nil {
			t.Fatal(err)
		}
		plan, err := BuildPlan(root, []byte("规则"), []AgentTarget{TargetCursor, TargetCodex}, nil, false)
		if err != nil || !plan.HasConflicts() || plan.Apply() == nil {
			t.Fatalf("标记错误没有阻断: %v", err)
		}
		if _, err := os.Stat(filepath.Join(root, ".cursorrules")); !os.IsNotExist(err) {
			t.Fatal("冲突计划仍然创建了其他目标")
		}
	}
}

func allowGitCleanup(t *testing.T, root string) {
	t.Helper()
	t.Cleanup(func() {
		_ = filepath.Walk(filepath.Join(root, ".git"), func(path string, info os.FileInfo, err error) error {
			if err == nil && info != nil && !info.IsDir() {
				_ = os.Chmod(path, 0666)
			}
			return nil
		})
	})
}

func TestPlanTrackedFileRequiresExplicitAllowance(t *testing.T) {
	root := t.TempDir()
	allowGitCleanup(t, root)
	for _, args := range [][]string{{"init", "--quiet"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("测试仓库初始化失败: %v %s", err, out)
		}
	}
	path := filepath.Join(root, "AGENTS.md")
	if err := os.WriteFile(path, []byte("团队维护的规约\n"), 0644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "add", "AGENTS.md")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("测试索引准备失败: %v %s", err, out)
	}
	plan, err := BuildPlan(root, []byte("新规则"), []AgentTarget{TargetCodex}, nil, false)
	if err != nil || !plan.HasConflicts() || plan.Apply() == nil {
		t.Fatalf("已跟踪文件未得到保护: %v", err)
	}
	allowed, err := BuildPlan(root, []byte("新规则"), []AgentTarget{TargetCodex}, nil, true)
	if err != nil || allowed.Apply() != nil {
		t.Fatalf("明确授权不能更新: %v", err)
	}
	data, _ := os.ReadFile(path)
	if !strings.HasPrefix(string(data), "团队维护的规约\n") {
		t.Fatal("修改了受管区块外内容")
	}
}

func TestPlanRejectsSymlinkTargets(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	outPath := filepath.Join(outside, "rules.md")
	if err := os.WriteFile(outPath, []byte("外部正文"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outPath, filepath.Join(root, "AGENTS.md")); err != nil {
		t.Skipf("当前系统未授予符号链接权限: %v", err)
	}
	plan, err := BuildPlan(root, []byte("新规则"), []AgentTarget{TargetCodex}, nil, false)
	if err != nil || !plan.HasConflicts() || plan.Apply() == nil {
		t.Fatalf("符号链接目标未被阻断: %v", err)
	}
	data, _ := os.ReadFile(outPath)
	if string(data) != "外部正文" {
		t.Fatal("修改了项目外文件")
	}
}

func TestCustomProfilePlanAndResolution(t *testing.T) {
	root := t.TempDir()
	custom := map[string]agent.Profile{"newtool": {Path: ".newtool/rules.md", Format: "markdown", Confirmed: true, Source: "用户确认的工具手册", Version: "本地测试版本"}}
	resolved, err := ResolveTargetsWithProfiles([]string{"newtool"}, root, TargetCodex, true, custom)
	if err != nil || len(resolved) != 1 || resolved[0] != "newtool" {
		t.Fatalf("声明式工具无法选择: %v %v", resolved, err)
	}
	plan, err := BuildPlan(root, []byte("新工具规约"), resolved, custom, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, ".newtool")); !os.IsNotExist(err) {
		t.Fatal("只读计划创建了目录")
	}
	if !strings.Contains(strings.Join(plan.Diagnostics(), "\n"), "加载及会话刷新未验证") {
		t.Fatal("把路径确认当作加载证明")
	}
	if err := plan.Apply(); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveTargetsWithProfiles([]string{"all", "typo"}, root, "", false, custom); err == nil {
		t.Fatal("all 静默吞掉了错误目标")
	}
}

func TestManagedRulesPreserveCRLFAndUserSuffix(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "AGENTS.md")
	original := "用户前言\r\n" + codexRulesStart + "\r\n旧正文\r\n" + codexRulesEnd + "\r\n用户后记\r\n"
	if err := os.WriteFile(path, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}
	applyTestPlan(t, root, "新版\n规则", TargetCodex)
	data, _ := os.ReadFile(path)
	if !strings.HasPrefix(string(data), "用户前言\r\n") || !strings.HasSuffix(string(data), "\r\n用户后记\r\n") || strings.Contains(strings.ReplaceAll(string(data), "\r\n", ""), "\n") {
		t.Fatal("受管更新没有保留 CRLF 或用户内容")
	}
}

func TestConcurrentPlansDoNotOverwriteEachOther(t *testing.T) {
	root := t.TempDir()
	first, err := BuildPlan(root, []byte("版本 A"), []AgentTarget{TargetCodex}, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	second, err := BuildPlan(root, []byte("版本 B"), []AgentTarget{TargetCodex}, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	errors := make(chan error, 2)
	var group sync.WaitGroup
	for _, plan := range []*Plan{first, second} {
		group.Add(1)
		go func(plan *Plan) {
			defer group.Done()
			<-start
			errors <- plan.Apply()
		}(plan)
	}
	close(start)
	group.Wait()
	close(errors)
	successes := 0
	for err := range errors {
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("两个初始状态相同的竞争计划预期只成功一个，实际 %d", successes)
	}
}

func TestPlanRechecksNewGitRepository(t *testing.T) {
	root := t.TempDir()
	allowGitCleanup(t, root)
	applyTestPlan(t, root, "版本 A", TargetCodex)
	plan, err := BuildPlan(root, []byte("版本 B"), []AgentTarget{TargetCodex}, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"init", "--quiet"}, {"add", "AGENTS.md"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("准备新测试仓库失败: %v %s", err, out)
		}
	}
	if err := plan.Apply(); err == nil {
		t.Fatal("旧计划未复核新仓库中已跟踪的入口")
	}
}

func TestLegacyMatchingContentIsNotAssumedOwned(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".cursorrules")
	original := "与规约源相同的用户正文\n"
	if err := os.WriteFile(path, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}
	plan := applyTestPlan(t, root, original, TargetCursor)
	updated, _ := os.ReadFile(path)
	if !strings.HasPrefix(string(updated), original) || !strings.Contains(plan.Entries()[0].Note, "保留原正文") {
		t.Fatal("仅凭正文与源相同就接管了无标记用户内容")
	}
}

func TestPlanTrackedFileCaseInsensitiveSameFile(t *testing.T) {
	root := t.TempDir()
	allowGitCleanup(t, root)
	cmdInit := exec.Command("git", "init", "--quiet")
	cmdInit.Dir = root
	if out, err := cmdInit.CombinedOutput(); err != nil {
		t.Fatalf("测试仓库初始化失败: %v %s", err, out)
	}

	// 使用小写 agents.md 写入并加入 Git 索引
	lowerPath := filepath.Join(root, "agents.md")
	if err := os.WriteFile(lowerPath, []byte("团队维护的规约\n"), 0644); err != nil {
		t.Fatal(err)
	}
	cmdAdd := exec.Command("git", "add", "agents.md")
	cmdAdd.Dir = root
	if out, err := cmdAdd.CombinedOutput(); err != nil {
		t.Fatalf("测试索引准备失败: %v %s", err, out)
	}

	infoLower, err1 := os.Stat(lowerPath)
	infoUpper, err2 := os.Stat(filepath.Join(root, "AGENTS.md"))
	if err1 != nil || err2 != nil || !os.SameFile(infoLower, infoUpper) {
		t.Skip("当前文件系统区分大小写，跳过大小写不敏感同文件跟踪测试")
	}

	plan, err := BuildPlan(root, []byte("新规则"), []AgentTarget{TargetCodex}, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.HasConflicts() || plan.Apply() == nil {
		t.Fatalf("大小写不敏感文件系统下已跟踪的 agents.md 未能阻断 AGENTS.md 覆盖")
	}
}

func TestPlanApplyAllSkipDoesNotCreateLocksDir(t *testing.T) {
	root := t.TempDir()
	applyTestPlan(t, root, "幂等规则正文", TargetCursor)
	_ = os.RemoveAll(filepath.Join(root, ".ai-memory"))

	skipPlan, err := BuildPlan(root, []byte("幂等规则正文"), []AgentTarget{TargetCursor}, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(skipPlan.Entries()) != 1 || skipPlan.Entries()[0].Action != "skip" {
		t.Fatalf("预期计划动作为 skip，实际: %+v", skipPlan.Entries())
	}
	if err := skipPlan.Apply(); err != nil {
		t.Fatalf("全 skip 计划 Apply 失败: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".ai-memory", "locks")); !os.IsNotExist(err) {
		t.Fatalf("全 skip 计划不应在磁盘上创建 .ai-memory/locks 目录")
	}
}

func TestPlanRejectsForeignManagedMarkers(t *testing.T) {
	root := t.TempDir()
	foreign := "<!-- agate:cursor-rules:start -->\nCursor 规则\n<!-- agate:cursor-rules:end -->\n"
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte(foreign), 0644); err != nil {
		t.Fatal(err)
	}

	plan, err := BuildPlan(root, []byte("Codex 规则"), []AgentTarget{TargetCodex}, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.HasConflicts() || plan.Entries()[0].Action != "conflict" || !strings.Contains(plan.Entries()[0].Note, "包含其他工具的受管区块标记，拒绝混入") {
		t.Fatalf("预期拦截跨工具受管标记混入，实际计划: %+v", plan.Entries())
	}
	if err := plan.Apply(); err == nil {
		t.Fatal("包含跨工具受管标记的冲突计划不应允许 Apply")
	}
}

func TestPlanReverseOverlapReadingDiagnostics(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".cursorrules"), []byte("现有 Cursor 规则\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "CLAUDE.md"), []byte("现有 Claude 规则\n"), 0644); err != nil {
		t.Fatal(err)
	}

	plan, err := BuildPlan(root, []byte("Codex 规约"), []AgentTarget{TargetCodex}, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	diagText := strings.Join(plan.Diagnostics(), "\n")
	expectedCursor := "项目中已存在 cursor (.cursorrules)，该工具可能同时读取 AGENTS.md；请核验实际版本、配置和规则冲突"
	expectedClaude := "项目中已存在 claude (CLAUDE.md)，该工具可能同时读取 AGENTS.md；请核验实际版本、配置和规则冲突"
	if !strings.Contains(diagText, expectedCursor) {
		t.Fatalf("缺失反向重叠读取诊断 (%s)，实际诊断:\n%s", expectedCursor, diagText)
	}
	if !strings.Contains(diagText, expectedClaude) {
		t.Fatalf("缺失反向重叠读取诊断 (%s)，实际诊断:\n%s", expectedClaude, diagText)
	}
}

func TestFrontmatterMarkdownAndPointerFormats(t *testing.T) {
	root := t.TempDir()
	custom := map[string]agent.Profile{
		"trae": {
			Path:      ".cursor/rules/agate.mdc",
			Format:    "frontmatter-md",
			Confirmed: true,
			Source:    "Cursor MDC 手册",
			Version:   "1.0",
		},
		"cline": {
			Path:      ".clinerules",
			Format:    "pointer",
			Confirmed: true,
			Source:    "Cline 手册",
			Version:   "1.0",
			Reads:     []string{".clinerules", "AGENTS.md"},
		},
	}

	rulesV1 := "完整长篇规约正文 V1：严禁全仓盲扫，遵守两阶段门禁。"
	plan, err := BuildPlan(root, []byte(rulesV1), []AgentTarget{TargetCodex, "trae", "cline"}, custom, false)
	if err != nil {
		t.Fatalf("构建计划失败: %v", err)
	}
	if err := plan.Apply(); err != nil {
		t.Fatalf("应用计划失败: %v", err)
	}

	mdcPath := filepath.Join(root, ".cursor", "rules", "agate.mdc")
	mdcData, err := os.ReadFile(mdcPath)
	if err != nil {
		t.Fatalf("读取 .mdc 文件失败: %v", err)
	}
	mdcText := string(mdcData)
	if !strings.HasPrefix(mdcText, "---\ndescription: Agate Engineering Guardrails\nalwaysApply: true\n---\n\n<!-- agate:trae-rules:start -->") {
		t.Fatalf("frontmatter-md 创建格式不符合预期:\n%s", mdcText)
	}

	// 模拟用户修改顶部 YAML Frontmatter 并在下方追加自定义内容，随后用 V2 规约更新
	userEdited := strings.Replace(mdcText, "alwaysApply: true", "alwaysApply: false\nglobs: \"**/*.go\"", 1) + "\n用户尾部补充说明\n"
	if err := os.WriteFile(mdcPath, []byte(userEdited), 0644); err != nil {
		t.Fatal(err)
	}

	rulesV2 := "完整长篇规约正文 V2：新增开放式工具扩展。"
	planV2, err := BuildPlan(root, []byte(rulesV2), []AgentTarget{TargetCodex, "trae", "cline"}, custom, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := planV2.Apply(); err != nil {
		t.Fatal(err)
	}

	mdcUpdated, _ := os.ReadFile(mdcPath)
	mdcUpdatedText := string(mdcUpdated)
	if !strings.HasPrefix(mdcUpdatedText, "---\ndescription: Agate Engineering Guardrails\nalwaysApply: false\nglobs: \"**/*.go\"\n---\n\n") {
		t.Fatalf("更新受管区块未保留第 1 行用户 Frontmatter:\n%s", mdcUpdatedText)
	}
	if !strings.Contains(mdcUpdatedText, rulesV2) || !strings.HasSuffix(mdcUpdatedText, "\n用户尾部补充说明\n") {
		t.Fatalf("更新受管区块未包含新规约或丢失用户尾部内容:\n%s", mdcUpdatedText)
	}

	// 验证 pointer 模式写入精炼引导块，且 inspect Digest 与完整规约一致
	clineData, err := os.ReadFile(filepath.Join(root, ".clinerules"))
	if err != nil {
		t.Fatal(err)
	}
	clineText := string(clineData)
	if strings.Contains(clineText, rulesV2) {
		t.Fatalf("pointer 模式不应复制完整规约正文:\n%s", clineText)
	}
	for _, keyword := range []string{"MAP.md", "contexts/context.md", "AGENTS.md", "agate verify"} {
		if !strings.Contains(clineText, keyword) {
			t.Fatalf("pointer 模式缺少核心引导词 %q:\n%s", keyword, clineText)
		}
	}

	planCheck, err := BuildPlan(root, []byte(rulesV2), []AgentTarget{TargetCodex, "trae", "cline"}, custom, false)
	if err != nil {
		t.Fatal(err)
	}
	expectedDigest := ruleDigest([]byte(rulesV2))
	for _, item := range planCheck.Inventory() {
		if item.Digest != expectedDigest {
			t.Fatalf("入口 %s 的盘点 Digest (%s) 与完整规约摘要 (%s) 不一致", item.Path, item.Digest, expectedDigest)
		}
	}
	for _, entry := range planCheck.Entries() {
		if entry.Action != "skip" {
			t.Fatalf("幂等检查预期 skip，但 %s 为 %s", entry.Path, entry.Action)
		}
	}
}
