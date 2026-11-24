package adapter

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"agate/pkg/agent"
)

func TestPlanNativeHooksModes(t *testing.T) {
	tempDir := t.TempDir()
	rtSupported := agent.NativeRuntime{
		Host:         agent.Cursor,
		ClientVer:    "1.0.0",
		SupportsHook: true,
		WorkDir:      tempDir,
	}

	rtUnread := agent.NativeRuntime{
		Host:         agent.Cursor,
		ClientVer:    "",
		SupportsHook: true,
		WorkDir:      tempDir,
	}
	skipped, err := PlanNativeHooks(tempDir, agent.Cursor, rtUnread, agent.NativeModeAuto)
	if err != nil || skipped == nil || !skipped.Skipped || !strings.Contains(skipped.Reason, "尚未验收") {
		t.Fatalf("没有读到版本时 auto 模式应跳过，实际: err=%v, plan=%+v", err, skipped)
	}
	plan, err := PlanNativeHooks(tempDir, agent.Cursor, rtSupported, agent.NativeModeAuto)
	if err != nil || plan == nil || plan.Skipped {
		t.Fatalf("已验收版本在 auto 模式应生成计划，实际: err=%v, plan=%+v", err, plan)
	}
	if !strings.HasSuffix(filepath.ToSlash(plan.Path), ".cursor/hooks.json") {
		t.Fatalf("Cursor 路径不匹配: %s", plan.Path)
	}

	planOff, err := PlanNativeHooks(tempDir, agent.Cursor, rtSupported, agent.NativeModeOff)
	if err != nil || planOff == nil || !planOff.Skipped {
		t.Fatalf("off 模式应跳过，实际: err=%v, plan=%+v", err, planOff)
	}

	rtUnsupported := agent.NativeRuntime{
		Host:         agent.Kind("unknown-agent"),
		SupportsHook: false,
		WorkDir:      tempDir,
	}
	_, err = PlanNativeHooks(tempDir, agent.Kind("unknown-agent"), rtUnsupported, agent.NativeModeRequire)
	if err == nil {
		t.Fatalf("不支持的宿主在 require 模式必须报错")
	}

	planAutoSkip, err := PlanNativeHooks(tempDir, agent.Kind("unknown-agent"), rtUnsupported, agent.NativeModeAuto)
	if err != nil || planAutoSkip == nil || !planAutoSkip.Skipped {
		t.Fatalf("不支持的宿主在 auto 模式应平滑跳过，实际: err=%v, plan=%+v", err, planAutoSkip)
	}
}

func TestPlanNativeHooksRequireChecksHostAndVersion(t *testing.T) {
	tempDir := t.TempDir()
	_, err := PlanNativeHooks(tempDir, agent.Cursor, agent.NativeRuntime{
		Host: agent.Claude, ClientVer: "1.0", SupportsHook: true,
	}, agent.NativeModeRequire)
	if err == nil {
		t.Fatalf("require 下目标与宿主不一致必须失败")
	}
	_, err = PlanNativeHooks(tempDir, agent.Cursor, agent.NativeRuntime{
		Host: agent.Cursor, ClientVer: "", SupportsHook: true,
	}, agent.NativeModeRequire)
	if err == nil {
		t.Fatalf("require 下缺少客户端版本必须失败")
	}
	_, err = PlanNativeHooks(tempDir, agent.OpenCode, agent.NativeRuntime{
		Host: agent.OpenCode, ClientVer: "1.0", SupportsHook: false,
	}, agent.NativeModeRequire)
	if err == nil {
		t.Fatalf("OpenCode 无已核对协议时 require 不得写 Hook")
	}
	readPlan, err := PlanNativeHooks(tempDir, agent.Cursor, agent.NativeRuntime{
		Host: agent.Cursor, ClientVer: "9.9.9", SupportsHook: true,
	}, agent.NativeModeRequire)
	if err != nil || readPlan == nil || readPlan.Skipped {
		t.Fatalf("Cursor 读到版本后 require 应生成计划，实际: err=%v plan=%+v", err, readPlan)
	}
	_, err = PlanNativeHooks(tempDir, agent.Claude, agent.NativeRuntime{
		Host: agent.Claude, ClientVer: "9.9.9", SupportsHook: true,
	}, agent.NativeModeRequire)
	if err == nil || !strings.Contains(err.Error(), "尚未验收") {
		t.Fatalf("未登记宿主版本在 require 模式必须失败，实际: %v", err)
	}
}

func TestPlanNativeHooksMergesExistingWithoutClobber(t *testing.T) {
	tempDir := t.TempDir()
	hookDir := filepath.Join(tempDir, ".cursor")
	if err := os.MkdirAll(hookDir, 0o755); err != nil {
		t.Fatal(err)
	}
	existing := `{
  "version": 1,
  "hooks": {
    "preToolUse": [
      {"command": "agate prompt --hook --host cursor --event preToolUse", "failClosed": true},
      {"command": "user-custom-hook.ps1"}
    ],
    "sessionStart": [
      {"command": "echo hello"}
    ]
  },
  "customField": true
}`
	path := filepath.Join(hookDir, "hooks.json")
	if err := os.WriteFile(path, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	restore := agent.TestingAcceptNativeHookVersion(agent.Cursor, "1.2.3")
	defer restore()
	rt := agent.NativeRuntime{Host: agent.Cursor, ClientVer: "1.2.3", SupportsHook: true, WorkDir: tempDir}
	plan, err := PlanNativeHooks(tempDir, agent.Cursor, rt, agent.NativeModeAuto)
	if err != nil || plan == nil || plan.Skipped || plan.Conflict {
		t.Fatalf("合并计划失败: err=%v plan=%+v", err, plan)
	}
	if err := ExecuteNativeHookPlan(plan); err != nil {
		t.Fatalf("落盘失败: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatal(err)
	}
	if root["customField"] != true {
		t.Fatalf("应保留未知字段 customField")
	}
	hooks := root["hooks"].(map[string]any)
	if _, ok := hooks["sessionStart"]; !ok {
		t.Fatalf("应保留用户事件 sessionStart")
	}
	pre := hooks["preToolUse"].([]any)
	if len(pre) < 2 {
		t.Fatalf("应保留用户条目并更新 Agate 条目, got=%v", pre)
	}
	foundFailClosed := false
	foundUser := false
	for _, item := range pre {
		entry := item.(map[string]any)
		cmd, _ := entry["command"].(string)
		if strings.Contains(cmd, "agate prompt --hook") {
			if fc, ok := entry["failClosed"].(bool); ok && fc {
				foundFailClosed = true
			}
		}
		if cmd == "user-custom-hook.ps1" {
			foundUser = true
		}
	}
	if !foundFailClosed || !foundUser {
		t.Fatalf("应保留 failClosed 与用户条目: failClosed=%v user=%v body=%s", foundFailClosed, foundUser, data)
	}
}

func TestPlanNativeHooksAllSupportedHosts(t *testing.T) {
	tempDir := t.TempDir()
	hosts := []struct {
		target   agent.Kind
		relPath  string
		expected string
	}{
		{agent.Cursor, ".cursor/hooks.json", "beforeSubmitPrompt"},
		{agent.Antigravity, ".agents/hooks.json", "PreInvocation"},
		{agent.Claude, ".claude/settings.json", "UserPromptSubmit"},
		{agent.Codex, ".codex/hooks.json", "UserPromptSubmit"},
		{agent.Windsurf, ".windsurf/hooks.json", "pre_user_prompt"},
	}

	for _, tc := range hosts {
		restore := agent.TestingAcceptNativeHookVersion(tc.target, "1.0")
		defer restore()
		rt := agent.NativeRuntime{Host: tc.target, ClientVer: "1.0", SupportsHook: true, WorkDir: tempDir}
		plan, err := PlanNativeHooks(tempDir, tc.target, rt, agent.NativeModeAuto)
		if err != nil || plan == nil || plan.Skipped {
			t.Fatalf("生成 %s 计划失败: err=%v, plan=%+v", tc.target, err, plan)
		}
		if !strings.HasSuffix(filepath.ToSlash(plan.Path), tc.relPath) {
			t.Errorf("宿主 %s 路径预期 %s，实际: %s", tc.target, tc.relPath, plan.Path)
		}
		if !strings.Contains(string(plan.Content), tc.expected) {
			t.Errorf("宿主 %s 内容应包含 %s，实际:\n%s", tc.target, tc.expected, plan.Content)
		}
		if err := ExecuteNativeHookPlan(plan); err != nil {
			t.Fatalf("执行 %s 计划落盘失败: %v", tc.target, err)
		}
	}
}

func TestPlanNativeHooksEightTargetsStayBlocked(t *testing.T) {
	tempDir := t.TempDir()
	blocked := []agent.Kind{
		agent.Copilot, agent.Trae, agent.OpenCode, agent.QwenCode,
		agent.Cline, agent.Kiro, agent.AntigravityCLI, agent.GeminiCLI,
	}
	for _, kind := range blocked {
		rt := agent.NativeRuntime{Host: kind, ClientVer: "9.9.9", SupportsHook: false, WorkDir: tempDir}
		plan, err := PlanNativeHooks(tempDir, kind, rt, agent.NativeModeAuto)
		if err != nil || plan == nil || !plan.Skipped {
			t.Fatalf("%s 应保持阻塞跳过: err=%v plan=%+v", kind, err, plan)
		}
		if !strings.Contains(plan.Reason, "协议未核对") {
			t.Fatalf("%s 阻塞原因应说明协议未核对: %s", kind, plan.Reason)
		}
	}
}

func TestRollbackNativeHooksKeepsUserEntries(t *testing.T) {
	tempDir := t.TempDir()
	hookDir := filepath.Join(tempDir, ".cursor")
	_ = os.MkdirAll(hookDir, 0o755)
	path := filepath.Join(hookDir, "hooks.json")
	body := `{
  "version": 1,
  "hooks": {
    "preToolUse": [
      {"command": "agate prompt --hook --host cursor --event preToolUse", "failClosed": true, "agateManaged": true},
      {"command": "echo user mentions agate prompt --hook"},
      {"command": "keep-me.ps1"}
    ]
  }
}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := RollbackNativeHooks(tempDir, agent.Cursor); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), `"agateManaged": true`) {
		t.Fatalf("回退后不应残留受管条目: %s", data)
	}
	if !strings.Contains(string(data), "echo user mentions agate prompt --hook") {
		t.Fatalf("回退后应保留仅提及命令文本的用户条目: %s", data)
	}
	if !strings.Contains(string(data), "keep-me.ps1") {
		t.Fatalf("回退后应保留用户条目: %s", data)
	}
}

func TestAntigravityHookPlanUsesOfficialShape(t *testing.T) {
	tempDir := t.TempDir()
	hookDir := filepath.Join(tempDir, ".agents")
	if err := os.MkdirAll(hookDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(hookDir, "hooks.json")
	existing := `{
  "agate": {
    "PreInvocation": [{"command": "echo user pre"}],
    "PreToolUse": [{"matcher": "view_file", "hooks": [{"command": "echo user tool"}]}]
  }
}`
	if err := os.WriteFile(path, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}
	restore := agent.TestingAcceptNativeHookVersion(agent.Antigravity, "1.0")
	defer restore()
	rt := agent.NativeRuntime{Host: agent.Antigravity, ClientVer: "1.0", SupportsHook: true, WorkDir: tempDir}
	plan, err := PlanNativeHooks(tempDir, agent.Antigravity, rt, agent.NativeModeAuto)
	if err != nil || plan == nil || plan.Skipped {
		t.Fatalf("生成 Antigravity 计划失败: err=%v plan=%+v", err, plan)
	}
	var root map[string]any
	if err := json.Unmarshal(plan.Content, &root); err != nil {
		t.Fatal(err)
	}
	group, _ := root["agate"].(map[string]any)
	if group == nil {
		t.Fatalf("应使用顶层 agate 分组: %s", plan.Content)
	}
	pre, _ := group["PreInvocation"].([]any)
	if len(pre) < 2 {
		t.Fatalf("应保留用户命令并加入受管命令: %s", plan.Content)
	}
	tools, _ := group["PreToolUse"].([]any)
	foundMatcher := false
	foundUser := false
	for _, item := range tools {
		bucket, _ := item.(map[string]any)
		if bucket["matcher"] == "*" {
			foundMatcher = true
		}
		if bucket["matcher"] == "view_file" {
			foundUser = true
		}
	}
	if !foundMatcher || !foundUser {
		t.Fatalf("工具事件应保留用户 matcher 并添加全量 matcher: %s", plan.Content)
	}
	if err := ExecuteNativeHookPlan(plan); err != nil {
		t.Fatal(err)
	}
	st := statusFor(t, tempDir, agent.Antigravity)
	if !st.HasAgate || st.Blocked {
		t.Fatalf("官方结构应识别为已托管: %+v", st)
	}
	if err := RollbackNativeHooks(tempDir, agent.Antigravity); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	text := string(data)
	if strings.Contains(text, `"agateManaged": true`) {
		t.Fatalf("回退后不应残留受管条目: %s", text)
	}
	if !strings.Contains(text, "echo user pre") || !strings.Contains(text, "echo user tool") {
		t.Fatalf("回退后应保留用户命令: %s", text)
	}
}

func TestExecuteNativeHookPlanRefusesStaleFile(t *testing.T) {
	tempDir := t.TempDir()
	hookDir := filepath.Join(tempDir, ".cursor")
	if err := os.MkdirAll(hookDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(hookDir, "hooks.json")
	original := []byte("{\n  \"hooks\": {}\n}\n")
	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatal(err)
	}
	restore := agent.TestingAcceptNativeHookVersion(agent.Cursor, "1.0")
	defer restore()
	rt := agent.NativeRuntime{Host: agent.Cursor, ClientVer: "1.0", SupportsHook: true, WorkDir: tempDir}
	plan, err := PlanNativeHooks(tempDir, agent.Cursor, rt, agent.NativeModeAuto)
	if err != nil || plan == nil || plan.Skipped {
		t.Fatalf("生成计划失败: err=%v plan=%+v", err, plan)
	}
	changed := []byte("{\n  \"hooks\": {},\n  \"user\": true\n}\n")
	if err := os.WriteFile(path, changed, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ExecuteNativeHookPlan(plan); err == nil || !strings.Contains(err.Error(), "已变化") {
		t.Fatalf("计划生成后改过的配置应拒绝写入: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(changed) {
		t.Fatalf("拒绝写入后应保留用户改动: %s", got)
	}
}

func TestNativeHookStatusParsesCompactAndIndentedJSON(t *testing.T) {
	tempDir := t.TempDir()
	hookDir := filepath.Join(tempDir, ".cursor")
	if err := os.MkdirAll(hookDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(hookDir, "hooks.json")
	managed := `{"version":1,"hooks":{"preToolUse":[{"command":"agate prompt --hook --host cursor --event preToolUse","agateManaged":true,"failClosed":true}]}}`
	if err := os.WriteFile(path, []byte(managed), 0o644); err != nil {
		t.Fatal(err)
	}
	compact := statusFor(t, tempDir, agent.Cursor)
	if !compact.HasAgate || compact.Blocked {
		t.Fatalf("紧凑 JSON 应识别为已托管: %+v", compact)
	}

	indented := "{\n  \"version\": 1,\n  \"hooks\": {\n    \"preToolUse\": [\n      {\"command\": \"agate prompt --hook --host cursor --event preToolUse\", \"agateManaged\": true}\n    ]\n  }\n}\n"
	if err := os.WriteFile(path, []byte(indented), 0o644); err != nil {
		t.Fatal(err)
	}
	pretty := statusFor(t, tempDir, agent.Cursor)
	if !pretty.HasAgate || pretty.Blocked {
		t.Fatalf("缩进 JSON 应识别为已托管: %+v", pretty)
	}

	mention := `{"note":"agateManaged: true","hooks":{"preToolUse":[{"command":"echo user mentions agate prompt --hook"}]}}`
	if err := os.WriteFile(path, []byte(mention), 0o644); err != nil {
		t.Fatal(err)
	}
	userOnly := statusFor(t, tempDir, agent.Cursor)
	if userOnly.HasAgate {
		t.Fatalf("用户字段中的同名文字不应算已托管: %+v", userOnly)
	}

	if err := os.WriteFile(path, []byte(`{"hooks":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	empty := statusFor(t, tempDir, agent.Cursor)
	if empty.HasAgate || empty.Blocked {
		t.Fatalf("空配置不应算已托管: %+v", empty)
	}

	if err := os.WriteFile(path, []byte(`{"hooks":`), 0o644); err != nil {
		t.Fatal(err)
	}
	bad := statusFor(t, tempDir, agent.Cursor)
	if bad.HasAgate || !bad.Blocked || !strings.Contains(bad.Reason, "非法 JSON") {
		t.Fatalf("非法 JSON 应记为无效且不算已托管: %+v", bad)
	}
}

func TestDivergedHookCommandUsesAbsolutePathWhenUntracked(t *testing.T) {
	orig := resolveHookInvocation
	t.Cleanup(func() { resolveHookInvocation = orig })
	resolveHookInvocation = func() hookInvocation {
		return hookInvocation{
			Prefix:   `C:\candidate\agate.exe`,
			Diverged: true,
			Note:     "PATH 上的 agate 指向另一份程序，隔离配置使用当前程序的绝对路径",
		}
	}
	restore := agent.TestingAcceptNativeHookVersion(agent.Cursor, "1.2.3")
	defer restore()
	dir := t.TempDir()
	rt := agent.NativeRuntime{Host: agent.Cursor, ClientVer: "1.2.3", SupportsHook: true, WorkDir: dir}
	plan, err := PlanNativeHooks(dir, agent.Cursor, rt, agent.NativeModeAuto)
	if err != nil || plan == nil || plan.Skipped || plan.Conflict {
		t.Fatalf("未跟踪配置应使用绝对路径: err=%v plan=%+v", err, plan)
	}
	if !strings.Contains(string(plan.Content), `C:\\candidate\\agate.exe prompt --hook --host cursor --event preToolUse`) {
		t.Fatalf("命令应指向当前程序: %s", plan.Content)
	}
	if !strings.Contains(plan.Reason, "另一份程序") {
		t.Fatalf("预览应说明 PATH 指向另一份程序: %s", plan.Reason)
	}
}

func TestDivergedHookCommandSkipsTrackedFile(t *testing.T) {
	orig := resolveHookInvocation
	t.Cleanup(func() { resolveHookInvocation = orig })
	resolveHookInvocation = func() hookInvocation {
		return hookInvocation{Prefix: `C:\candidate\agate.exe`, Diverged: true}
	}
	restore := agent.TestingAcceptNativeHookVersion(agent.Cursor, "1.2.3")
	defer restore()
	dir := t.TempDir()
	gitCmd(t, dir, "init")
	hookDir := filepath.Join(dir, ".cursor")
	if err := os.MkdirAll(hookDir, 0o755); err != nil {
		t.Fatal(err)
	}
	hookPath := filepath.Join(hookDir, "hooks.json")
	original := []byte(`{"version":1,"hooks":{}}`)
	if err := os.WriteFile(hookPath, original, 0o644); err != nil {
		t.Fatal(err)
	}
	gitCmd(t, dir, "add", "--", ".cursor/hooks.json")

	rt := agent.NativeRuntime{Host: agent.Cursor, ClientVer: "1.2.3", SupportsHook: true, WorkDir: dir}
	plan, err := PlanNativeHooksWithOptions(dir, agent.Cursor, rt, agent.NativeModeAuto, true)
	if err != nil || plan == nil || !plan.Skipped || !plan.Tracked {
		t.Fatalf("已跟踪配置应拒绝绝对路径: err=%v plan=%+v", err, plan)
	}
	if len(plan.Content) != 0 {
		t.Fatalf("拒绝时不应生成绝对路径内容: %s", plan.Content)
	}
	if !strings.Contains(plan.Reason, "拒绝把本机绝对路径写入已跟踪的 Hook 配置") {
		t.Fatalf("原因应说明拒绝写入共享配置: %s", plan.Reason)
	}
	got, err := os.ReadFile(hookPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(original) {
		t.Fatalf("已跟踪文件不应被改写: %s", got)
	}
}

func gitCmd(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

func statusFor(t *testing.T, root string, host agent.Kind) NativeHookStatus {
	t.Helper()
	list, err := ListNativeHookStatus(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, st := range list {
		if st.Host == host {
			return st
		}
	}
	t.Fatalf("未找到宿主 %s 的状态", host)
	return NativeHookStatus{}
}
