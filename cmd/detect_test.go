package cmd

import (
	"agate/pkg/agent"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type stubInspector struct {
	chain []agent.ProcessInfo
	calls int
}

func (s *stubInspector) GetAncestors(pid int, maxDepth int) ([]agent.ProcessInfo, error) {
	if s == nil {
		return nil, nil
	}
	s.calls++
	return s.chain, nil
}

func TestDetectCmdStandardAndJSON(t *testing.T) {
	clearAgentEnvForTest(t)
	t.Setenv("CODEX_SESSION_ID", "secret-session-id-999")
	t.Setenv("TERM_PROGRAM", "cursor")

	// 1. 测试标准与 --explain 输出
	cmd := newDetectCommand(&stubInspector{})
	var outBuf bytes.Buffer
	cmd.SetOut(&outBuf)
	cmd.SetErr(&outBuf)
	cmd.SetArgs([]string{"--explain"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("detect --explain 执行失败: %v", err)
	}

	outStr := outBuf.String()
	if !strings.Contains(outStr, "执行者 (Runner):") || !strings.Contains(outStr, "codex") {
		t.Fatalf("detect 输出应包含执行者 codex，实际输出:\n%s", outStr)
	}
	if !strings.Contains(outStr, "宿主编辑器 (Editor):") || !strings.Contains(outStr, "cursor") {
		t.Fatalf("detect 输出应包含宿主编辑器 cursor，实际输出:\n%s", outStr)
	}
	// 验证脱敏：绝不回显具体 session id 等敏感值
	if strings.Contains(outStr, "secret-session-id-999") {
		t.Fatalf("安全红线违规：detect 输出中回显了敏感变量取值 secret-session-id-999！")
	}

	// 2. 测试 --json 输出
	var jsonBuf bytes.Buffer
	cmd.SetOut(&jsonBuf)
	cmd.SetErr(&jsonBuf)
	cmd.SetArgs([]string{"--json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("detect --json 执行失败: %v", err)
	}

	var jsonResult detectJSONOutput
	if err := json.Unmarshal(jsonBuf.Bytes(), &jsonResult); err != nil {
		t.Fatalf("解析 detect --json 失败: %v\nJSON 内容:\n%s", err, jsonBuf.String())
	}

	if jsonResult.Version != "1.0" {
		t.Fatalf("JSON 版本预期 1.0，实际: %s", jsonResult.Version)
	}
	if jsonResult.Runner != "codex" || jsonResult.RunnerStatus != "known" {
		t.Fatalf("JSON Runner 预期 codex (known)，实际: %s (%s)", jsonResult.Runner, jsonResult.RunnerStatus)
	}
	if jsonResult.Editor != "cursor" || jsonResult.EditorStatus != "known" {
		t.Fatalf("JSON Editor 预期 cursor (known)，实际: %s (%s)", jsonResult.Editor, jsonResult.EditorStatus)
	}
	if jsonResult.EffectiveTarget != "codex" || jsonResult.HasConflict {
		t.Fatalf("JSON EffectiveTarget 预期 codex (无冲突)，实际: %s (conflict=%t)", jsonResult.EffectiveTarget, jsonResult.HasConflict)
	}
}

func TestDetectCmdConflictOutput(t *testing.T) {
	clearAgentEnvForTest(t)
	t.Setenv("CODEX_SESSION_ID", "1")
	t.Setenv("CLAUDE_CODE", "1")

	cmd := newDetectCommand(&stubInspector{})
	var jsonBuf bytes.Buffer
	cmd.SetOut(&jsonBuf)
	cmd.SetErr(&jsonBuf)
	cmd.SetArgs([]string{"--json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("detect --json 失败: %v", err)
	}

	var jsonResult detectJSONOutput
	if err := json.Unmarshal(jsonBuf.Bytes(), &jsonResult); err != nil {
		t.Fatalf("反序列化 JSON 失败: %v", err)
	}

	if !jsonResult.HasConflict || jsonResult.RunnerStatus != "ambiguous" {
		t.Fatalf("存在多个强指纹时应标记为 ambiguous 并冲突，实际: %s (conflict=%t)", jsonResult.RunnerStatus, jsonResult.HasConflict)
	}
}

func TestDetectCmdSessionContextPassing(t *testing.T) {
	clearAgentEnvForTest(t)
	// 传入包含私有路径的 sid，以及有效的未来 exp
	exp := time.Now().Add(time.Hour).Unix()
	t.Setenv("AGATE_SESSION_CONTEXT", fmt.Sprintf("v=1;editor=cursor;runner=codex;sid=/home/user/myproject/session-123;src=terminal;exp=%d", exp))

	cmd := newDetectCommand(&stubInspector{})
	var jsonBuf bytes.Buffer
	cmd.SetOut(&jsonBuf)
	cmd.SetErr(&jsonBuf)
	cmd.SetArgs([]string{"--json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("detect --json 失败: %v", err)
	}

	rawJSON := jsonBuf.String()
	// RD-S01 严格脱敏断言：绝不允许泄露原始私有路径或 session_id
	if strings.Contains(rawJSON, "/home/user/myproject/session-123") {
		t.Fatalf("安全红线拦截：detect --json 中回显了敏感私有路径会话值！")
	}

	var jsonResult detectJSONOutput
	if err := json.Unmarshal(jsonBuf.Bytes(), &jsonResult); err != nil {
		t.Fatalf("反序列化 JSON 失败: %v", err)
	}

	if jsonResult.Runner != "codex" || jsonResult.Editor != "cursor" {
		t.Fatalf("通过会话声明透传预期 runner=codex, editor=cursor，实际: runner=%s, editor=%s", jsonResult.Runner, jsonResult.Editor)
	}
	if jsonResult.SessionContext == nil || jsonResult.SessionContext.Version != "1" {
		t.Fatalf("脱敏后的 SessionContext 结构体预期回显版本 1，实际: %+v", jsonResult.SessionContext)
	}
}

func TestDetectCmdConfigErrorExposed(t *testing.T) {
	clearAgentEnvForTest(t)
	dir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir, ".agate"), 0755)
	// 写入损坏的非法语法配置文件
	_ = os.WriteFile(filepath.Join(dir, ".agate", "config.toml"), []byte("[broken toml\nkey = = value\n"), 0644)

	orig, _ := os.Getwd()
	_ = os.Chdir(dir)
	defer func() { _ = os.Chdir(orig) }()

	cmd := newDetectCommand(&stubInspector{})
	var outBuf bytes.Buffer
	cmd.SetOut(&outBuf)
	cmd.SetErr(&outBuf)
	cmd.SetArgs([]string{})

	err := cmd.Execute()
	// RD-S03: 配置损坏时必须报错，绝不吞掉错误假装成功
	if err == nil {
		t.Fatalf("配置文件损坏时 detect 预期返回错误，实际返回成功")
	}

	// R3-S01: 错误输出与 JSON 模式下绝不回显配置文件私有路径
	var jsonBuf bytes.Buffer
	cmdJSON := newDetectCommand(&stubInspector{})
	cmdJSON.SetOut(&jsonBuf)
	cmdJSON.SetErr(&jsonBuf)
	cmdJSON.SetArgs([]string{"--json"})
	_ = cmdJSON.Execute()

	jsonStr := jsonBuf.String()
	if strings.Contains(jsonStr, dir) {
		t.Fatalf("detect --json 中泄漏了配置文件的私有目录绝对路径: %s", dir)
	}
	var res map[string]interface{}
	if err := json.Unmarshal(jsonBuf.Bytes(), &res); err == nil {
		if cfgErr, ok := res["config_error"].(string); ok {
			if strings.Contains(cfgErr, dir) || strings.Contains(cfgErr, "key = = value") {
				t.Fatalf("config_error 字段泄漏了私有路径或原始错误内容: %s", cfgErr)
			}
		}
	}
}

func TestDetectCmdKnownRunnerWinsOverEditorConflict(t *testing.T) {
	// R5-01: 当 Runner 为 known 且 Editor 为 ambiguous 冲突时，detect 必须稳定输出 effective_target=codex
	clearAgentEnvForTest(t)
	t.Setenv("CODEX_SESSION_ID", "1")

	dir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir, ".agate"), 0755)
	cfgContent := `[agent]
[agent.profiles.p1]
path = ".p1/rules.md"
format = "markdown"
confirmed = true
source = "test"
version = "1.0"
term_programs = ["term_conflict"]

[agent.profiles.p2]
path = ".p2/rules.md"
format = "markdown"
confirmed = true
source = "test"
version = "1.0"
term_programs = ["term_conflict"]
`
	_ = os.WriteFile(filepath.Join(dir, ".agate", "config.toml"), []byte(cfgContent), 0644)
	t.Setenv("TERM_PROGRAM", "term_conflict")

	orig, _ := os.Getwd()
	_ = os.Chdir(dir)
	defer func() { _ = os.Chdir(orig) }()

	// 1. JSON 模式
	var jsonBuf bytes.Buffer
	cmd := newDetectCommand(&stubInspector{})
	cmd.SetOut(&jsonBuf)
	cmd.SetErr(&jsonBuf)
	cmd.SetArgs([]string{"--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("detect --json 失败: %v", err)
	}

	var jsonResult detectJSONOutput
	if err := json.Unmarshal(jsonBuf.Bytes(), &jsonResult); err != nil {
		t.Fatalf("解析 JSON 失败: %v", err)
	}
	if jsonResult.EffectiveTarget != "codex" {
		t.Fatalf("EffectiveTarget 预期为 codex，实际: %s", jsonResult.EffectiveTarget)
	}
	if !jsonResult.HasConflict || jsonResult.EditorStatus != "ambiguous" {
		t.Fatalf("Editor 应标记为 ambiguous 冲突，实际: %s (conflict=%t)", jsonResult.EditorStatus, jsonResult.HasConflict)
	}

	// 2. 文本模式
	clearAgentEnvForTest(t)
	t.Setenv("CODEX_SESSION_ID", "1")
	t.Setenv("TERM_PROGRAM", "term_conflict")
	var textBuf bytes.Buffer
	cmdText := newDetectCommand(&stubInspector{})
	cmdText.SetOut(&textBuf)
	cmdText.SetErr(&textBuf)
	cmdText.SetArgs([]string{})
	if err := cmdText.Execute(); err != nil {
		t.Fatalf("detect 文本模式失败: %v", err)
	}
	textOut := textBuf.String()
	if !strings.Contains(textOut, "首选装配目标:") || !strings.Contains(textOut, "codex") {
		t.Fatalf("文本模式首选装配目标应包含 codex，实际输出:\n%s", textOut)
	}
}

func TestDetectExplainAndJSONHideProcessInstallPath(t *testing.T) {
	clearAgentEnvForTest(t)
	secretPath := `C:\Users\secret\AppData\Local\Programs\cursor\Cursor.exe`
	inspector := &stubInspector{chain: []agent.ProcessInfo{{
		PID: 15, PPID: 1, Path: secretPath, Name: "Cursor.exe",
	}}}
	cmd := newDetectCommand(inspector)
	var explainBuf bytes.Buffer
	cmd.SetOut(&explainBuf)
	cmd.SetErr(&explainBuf)
	cmd.SetArgs([]string{"--explain"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("detect --explain 失败: %v", err)
	}
	explainOut := explainBuf.String()
	if !strings.Contains(explainOut, "process") || !strings.Contains(explainOut, "pid_15") || !strings.Contains(explainOut, "Cursor.exe") {
		t.Fatalf("explain 应展示进程名、PID 和匹配原因:\n%s", explainOut)
	}
	if strings.Contains(explainOut, `C:\Users\secret`) {
		t.Fatalf("explain 泄漏了安装路径:\n%s", explainOut)
	}

	jsonCmd := newDetectCommand(inspector)
	var jsonBuf bytes.Buffer
	jsonCmd.SetOut(&jsonBuf)
	jsonCmd.SetErr(&jsonBuf)
	jsonCmd.SetArgs([]string{"--json"})
	if err := jsonCmd.Execute(); err != nil {
		t.Fatalf("detect --json 失败: %v", err)
	}
	if strings.Contains(jsonBuf.String(), `C:\Users\secret`) {
		t.Fatalf("json 泄漏了安装路径:\n%s", jsonBuf.String())
	}
	var parsed detectJSONOutput
	if err := json.Unmarshal(jsonBuf.Bytes(), &parsed); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, ev := range parsed.Evidences {
		if ev.Source == "process" && ev.TargetRole == "editor" && ev.TargetAgent == "cursor" {
			found = true
		}
	}
	if !found || parsed.Editor != "cursor" {
		t.Fatalf("json 应记录 cursor 进程证据，editor=%s evidences=%+v", parsed.Editor, parsed.Evidences)
	}
	if len(parsed.Warnings) == 0 && inspector.calls != 2 {
		t.Fatalf("两次命令应各自采集，实际调用 %d", inspector.calls)
	}
}
