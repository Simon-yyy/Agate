package workspace

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"agate/pkg/agent"
)

func TestWorkspaceViewIsolationAndNoSharedSettingsTouch(t *testing.T) {
	root := t.TempDir()

	// 准备团队共享的 .vscode/settings.json 与各工具规则文件
	vscodeDir := filepath.Join(root, ".vscode")
	if err := os.MkdirAll(vscodeDir, 0755); err != nil {
		t.Fatal(err)
	}
	sharedSettingsPath := filepath.Join(vscodeDir, "settings.json")
	originalSettings := `{"editor.tabSize": 2}`
	if err := os.WriteFile(sharedSettingsPath, []byte(originalSettings), 0644); err != nil {
		t.Fatal(err)
	}

	// 同时生成 cursor 与 antigravity 两个工具的视图
	plan, err := BuildViewPlan(root, []string{"cursor", "antigravity"}, nil)
	if err != nil {
		t.Fatalf("BuildViewPlan 失败: %v", err)
	}
	if len(plan.Views) != 2 {
		t.Fatalf("预期生成 2 个视图计划，实际: %d", len(plan.Views))
	}

	written, err := ApplyViewPlan(plan)
	if err != nil {
		t.Fatalf("ApplyViewPlan 失败: %v", err)
	}
	if len(written) != 2 {
		t.Fatalf("预期写入 2 个 .code-workspace 文件，实际: %d", len(written))
	}

	// 1. 验证团队共享 .vscode/settings.json 完全未被触碰
	afterSettings, err := os.ReadFile(sharedSettingsPath)
	if err != nil {
		t.Fatalf("读取 .vscode/settings.json 失败: %v", err)
	}
	if string(afterSettings) != originalSettings {
		t.Fatalf("严禁修改团队共享 .vscode/settings.json，实际内容变为: %s", string(afterSettings))
	}

	// 2. 验证 cursor.code-workspace 与 antigravity.code-workspace 彼此独立、互不改写
	cursorWSPath := filepath.Join(root, ".ai-memory", "workspaces", "cursor.code-workspace")
	antigravityWSPath := filepath.Join(root, ".ai-memory", "workspaces", "antigravity.code-workspace")

	cursorData, err := os.ReadFile(cursorWSPath)
	if err != nil {
		t.Fatalf("读取 cursor.code-workspace 失败: %v", err)
	}
	var cursorCfg WorkspaceConfig
	if err := json.Unmarshal(cursorData, &cursorCfg); err != nil {
		t.Fatalf("解析 cursor.code-workspace 失败: %v", err)
	}

	// Cursor 视图：自身 .cursorrules 必须可见（不在 files.exclude 中），其他规则必须隐藏
	if cursorCfg.Settings.FilesExclude[".cursorrules"] {
		t.Errorf("Cursor 视图不应隐藏自身的 .cursorrules")
	}
	for _, otherRule := range []string{"AGENTS.md", "CLAUDE.md", ".windsurfrules", ".gemini/GEMINI.md"} {
		if !cursorCfg.Settings.FilesExclude[otherRule] {
			t.Errorf("Cursor 视图应隐藏其他工具规则 %s", otherRule)
		}
	}
	// 严禁隐藏整个 .gemini 目录或双地图
	for _, mustKeep := range []string{".gemini", ".cursor", "MAP.md", "contexts/context.md", "TASK.md", "MEMORY.md"} {
		if cursorCfg.Settings.FilesExclude[mustKeep] {
			t.Errorf("Cursor 视图严禁隐藏保护路径或目录 %s", mustKeep)
		}
	}
	if len(cursorCfg.Folders) != 1 || cursorCfg.Folders[0].Path != "../.." {
		t.Errorf("Cursor 视图 folders.path 应为 ../..，实际: %+v", cursorCfg.Folders)
	}

	// Antigravity 视图：自身 .gemini/GEMINI.md 必须可见，.cursorrules 必须隐藏
	agData, err := os.ReadFile(antigravityWSPath)
	if err != nil {
		t.Fatalf("读取 antigravity.code-workspace 失败: %v", err)
	}
	var agCfg WorkspaceConfig
	if err := json.Unmarshal(agData, &agCfg); err != nil {
		t.Fatalf("解析 antigravity.code-workspace 失败: %v", err)
	}
	if agCfg.Settings.FilesExclude[".gemini/GEMINI.md"] {
		t.Errorf("Antigravity 视图不应隐藏自身的 .gemini/GEMINI.md")
	}
	if !agCfg.Settings.FilesExclude[".cursorrules"] {
		t.Errorf("Antigravity 视图应隐藏 .cursorrules")
	}
}

func TestWorkspaceIdempotentApplyAndRestoreViews(t *testing.T) {
	root := t.TempDir()

	customProfiles := map[agent.Kind]agent.Profile{
		agent.Kind("kiro"): {Path: ".kiro/rules.md"},
	}

	plan1, err := BuildViewPlan(root, []string{"cursor", "kiro"}, customProfiles)
	if err != nil {
		t.Fatalf("第一次 BuildViewPlan 失败: %v", err)
	}
	if _, err := ApplyViewPlan(plan1); err != nil {
		t.Fatalf("第一次 ApplyViewPlan 失败: %v", err)
	}

	cursorPath := filepath.Join(root, ".ai-memory", "workspaces", "cursor.code-workspace")
	firstBytes, err := os.ReadFile(cursorPath)
	if err != nil {
		t.Fatal(err)
	}

	// 再次生成应用，验证幂等性
	plan2, err := BuildViewPlan(root, []string{"cursor", "kiro"}, customProfiles)
	if err != nil {
		t.Fatalf("第二次 BuildViewPlan 失败: %v", err)
	}
	if _, err := ApplyViewPlan(plan2); err != nil {
		t.Fatalf("第二次 ApplyViewPlan 失败: %v", err)
	}
	secondBytes, err := os.ReadFile(cursorPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(firstBytes) != string(secondBytes) {
		t.Fatalf("重复执行 ApplyViewPlan 输出不一致，非幂等:\n%s\nvs\n%s", string(firstBytes), string(secondBytes))
	}

	// 验证自定义档案也被纳入其他工具的隐藏列表中
	if !strings.Contains(string(secondBytes), ".kiro/rules.md") {
		t.Errorf("Cursor 视图应隐藏自定义档案规则路径 .kiro/rules.md，实际: %s", string(secondBytes))
	}

	// 恢复单个视图 cursor
	removed, err := RestoreViews(root, []string{"cursor"})
	if err != nil {
		t.Fatalf("RestoreViews(cursor) 失败: %v", err)
	}
	if len(removed) != 1 {
		t.Fatalf("预期清理 1 个视图文件，实际: %d", len(removed))
	}
	if _, err := os.Stat(cursorPath); !os.IsNotExist(err) {
		t.Errorf("cursor.code-workspace 应已被删除")
	}

	// 恢复全部剩余视图
	removedAll, err := RestoreViews(root, nil)
	if err != nil {
		t.Fatalf("RestoreViews(all) 失败: %v", err)
	}
	if len(removedAll) != 1 {
		t.Fatalf("预期清理剩余 1 个视图文件 (kiro)，实际: %d", len(removedAll))
	}
}

func TestWorkspaceUnsupportedCLIAndUnknownTargetFallback(t *testing.T) {
	root := t.TempDir()

	if SupportsWorkspaceView("codex") {
		t.Errorf("codex 为 CLI 工具，SupportsWorkspaceView 应返回 false")
	}
	if SupportsWorkspaceView("claude") {
		t.Errorf("claude 为 CLI 工具，SupportsWorkspaceView 应返回 false")
	}
	if !SupportsWorkspaceView("gemini") || !SupportsWorkspaceView("antigravity") || !SupportsWorkspaceView("cursor") || !SupportsWorkspaceView("windsurf") {
		t.Errorf("GUI 工具应支持 WorkspaceView")
	}

	// 1. CLI 工具生成计划时应明确标记为 Supported=false，且 ApplyViewPlan 不落盘生成无用文件
	plan, err := BuildViewPlan(root, []string{"codex", "claude"}, nil)
	if err != nil {
		t.Fatalf("CLI 工具 BuildViewPlan 应安全降级而非崩溃: %v", err)
	}
	for _, v := range plan.Views {
		if v.Supported {
			t.Errorf("工具 %s 应标记为 Supported=false", v.Target)
		}
		if v.Reason == "" {
			t.Errorf("工具 %s 降级时必须提供明确 Reason", v.Target)
		}
	}
	written, err := ApplyViewPlan(plan)
	if err != nil {
		t.Fatalf("ApplyViewPlan 失败: %v", err)
	}
	if len(written) != 0 {
		t.Errorf("不支持 GUI 视图的 CLI 工具不应写入任何 .code-workspace 文件，实际写入: %v", written)
	}

	// 2. 未知工具名必须明确报错拒绝盲猜
	if _, err := BuildViewPlan(root, []string{"unknown-tool"}, nil); err == nil {
		t.Errorf("未知工具名称预期返回错误，实际成功")
	}

	// 3. BuildOpenCommand 校验：GUI 工具直接调用对应可执行文件，不经过 cmd.exe /c；CLI 工具返回降级错误
	cmd, err := BuildOpenCommand("windows", "cursor", `C:\repo\.ai-memory\workspaces\cursor.code-workspace`)
	if err != nil {
		t.Fatalf("BuildOpenCommand(cursor) 失败: %v", err)
	}
	if len(cmd.Args) != 2 || strings.Contains(strings.ToLower(cmd.Args[0]), "cmd") {
		t.Errorf("BuildOpenCommand 严禁通过 cmd.exe /c 包装，实际 Args: %v", cmd.Args)
	}

	if _, err := BuildOpenCommand("windows", "codex", `C:\repo\.ai-memory\workspaces\codex.code-workspace`); err == nil {
		t.Errorf("BuildOpenCommand(codex) 预期返回不支持 GUI 的明确降级错误")
	}
}
