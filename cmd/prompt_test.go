package cmd

import (
	"bytes"
	"strings"
	"testing"
)

func TestPromptCmd_BasicAndUsageError(t *testing.T) {
	resetPromptFlagsForTest()
	var outBuf bytes.Buffer
	var errBuf bytes.Buffer

	// 1. 空指令报错（返回退出码 2）
	exitCode := ExecuteWithArgs([]string{"prompt"}, &outBuf, &errBuf)
	if exitCode != 2 {
		t.Fatalf("空指令预期退出码 2，实际为 %d (stderr: %s)", exitCode, errBuf.String())
	}

	// 2. 正常命令行传参
	outBuf.Reset()
	errBuf.Reset()
	exitCode = ExecuteWithArgs([]string{"prompt", "帮我修改一下代码"}, &outBuf, &errBuf)
	if exitCode != 0 {
		t.Fatalf("正常 prompt 指令执行失败，退出码: %d (stderr: %s)", exitCode, errBuf.String())
	}
	outStr := outBuf.String()
	if !strings.Contains(outStr, "【宿主模型指令执行信封 (Agate Prompt Envelope)】") {
		t.Fatalf("输出未包含信封头部，实际输出:\n%s", outStr)
	}
	if !strings.Contains(outStr, "agate-intent") {
		t.Fatalf("输出未包含 agate-intent 门禁块")
	}

	// 3. --json 模式
	outBuf.Reset()
	errBuf.Reset()
	exitCode = ExecuteWithArgs([]string{"prompt", "重构 cmd/root.go", "--json"}, &outBuf, &errBuf)
	if exitCode != 0 {
		t.Fatalf("prompt --json 执行失败，退出码: %d (stderr: %s)", exitCode, errBuf.String())
	}
	jsonStr := outBuf.String()
	if !strings.Contains(jsonStr, "\"raw_instruction\": \"重构 cmd/root.go\"") {
		t.Fatalf("JSON 输出未包含原始指令字段: %s", jsonStr)
	}

	// 4. --scope 显式纠偏与 --no-dirty 模式
	outBuf.Reset()
	errBuf.Reset()
	exitCode = ExecuteWithArgs([]string{"prompt", "改下超时逻辑", "-s", "pkg/relay/lock.go", "--no-dirty", "--json"}, &outBuf, &errBuf)
	if exitCode != 0 {
		t.Fatalf("prompt -s --no-dirty 执行失败，退出码: %d (stderr: %s)", exitCode, errBuf.String())
	}
	scopeJSON := outBuf.String()
	if !strings.Contains(scopeJSON, `"anchor_confidence": "explicit"`) || !strings.Contains(scopeJSON, `"pkg/relay/lock.go"`) {
		t.Fatalf("JSON 输出未包含显式纠偏 scope 结果:\n%s", scopeJSON)
	}

	// 5. 非法 --scope 路径或空字符串返回退出码 2 (UsageError)
	outBuf.Reset()
	errBuf.Reset()
	exitCode = ExecuteWithArgs([]string{"prompt", "改下代码", "-s", "../../etc/passwd"}, &outBuf, &errBuf)
	if exitCode != 2 {
		t.Fatalf("非法 --scope 预期返回退出码 2，实际为 %d (stderr: %s)", exitCode, errBuf.String())
	}

	outBuf.Reset()
	errBuf.Reset()
	exitCode = ExecuteWithArgs([]string{"prompt", "改下代码", "-s", ""}, &outBuf, &errBuf)
	if exitCode != 2 {
		t.Fatalf("空字符串 --scope 预期返回退出码 2，实际为 %d (stderr: %s)", exitCode, errBuf.String())
	}

	outBuf.Reset()
	errBuf.Reset()
	exitCode = ExecuteWithArgs([]string{"prompt", "改下代码", "-s", "pkg"}, &outBuf, &errBuf)
	if exitCode != 2 {
		t.Fatalf("孤立容器目录 -s pkg 预期返回退出码 2，实际为 %d (stderr: %s)", exitCode, errBuf.String())
	}
}

func TestPromptCmd_HookStdinTextAndJSON(t *testing.T) {
	resetPromptFlagsForTest()
	// 1. 测试 --hook 纯文本输入
	inBuf := bytes.NewBufferString("测试管道输入纯文本需求")
	var outBuf bytes.Buffer
	var errBuf bytes.Buffer

	cmd := rootCmd
	cmd.SetIn(inBuf)
	cmd.SetOut(&outBuf)
	cmd.SetErr(&errBuf)
	cmd.SetArgs([]string{"prompt", "--hook"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("prompt --hook 纯文本执行失败: %v", err)
	}
	outStr := outBuf.String()
	if !strings.Contains(outStr, "测试管道输入纯文本需求") {
		t.Fatalf("未能正确提取管道纯文本指令，输出:\n%s", outStr)
	}
	if !strings.Contains(outStr, "<!-- agate:prompt-guard:start -->") {
		t.Fatalf("Hook 模式应包含 start 注释标签")
	}

	// 2. 测试 --hook JSON 载荷解析（模拟 IDE Hook Webhook/Stdin 协议）
	jsonPayload := `{"prompt": "从 JSON 载荷自动提取的指令", "session_id": "test-123"}`
	inBuf = bytes.NewBufferString(jsonPayload)
	outBuf.Reset()
	errBuf.Reset()

	cmd.SetIn(inBuf)
	cmd.SetOut(&outBuf)
	cmd.SetErr(&errBuf)
	cmd.SetArgs([]string{"prompt", "--hook"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("prompt --hook JSON 载荷执行失败: %v", err)
	}
	outJSONStr := outBuf.String()
	if !strings.Contains(outJSONStr, "从 JSON 载荷自动提取的指令") {
		t.Fatalf("未能从 JSON 载荷正确提取 prompt 字段，输出:\n%s", outJSONStr)
	}
	if strings.Contains(outJSONStr, "test-123") {
		t.Fatalf("安全违约：JSON 载荷中的元数据不应出现在原始指令文本中")
	}

	// 3. 测试 --hook 空 JSON 载荷返回 UsageError (退出码 2) 且不回显原始 JSON
	emptyJSON := `{"prompt": "   ", "session_id": "secret-999"}`
	inBuf = bytes.NewBufferString(emptyJSON)
	outBuf.Reset()
	errBuf.Reset()
	cmd.SetIn(inBuf)
	exitCode := ExecuteWithArgs([]string{"prompt", "--hook"}, &outBuf, &errBuf)
	if exitCode != 2 {
		t.Fatalf("空 JSON 载荷预期返回退出码 2，实际为 %d", exitCode)
	}
	if strings.Contains(outBuf.String(), "secret-999") {
		t.Fatalf("安全违约：空 JSON 载荷不应被回退当作指令输出")
	}
}

func resetPromptFlagsForTest() {
	flagPromptJSON = false
	flagPromptHook = false
	flagPromptScopes = nil
	flagPromptIgnoreDirty = false
	if promptCmd != nil && promptCmd.Flags() != nil {
		_ = promptCmd.Flags().Set("json", "false")
		_ = promptCmd.Flags().Set("hook", "false")
		_ = promptCmd.Flags().Set("no-dirty", "false")
		_ = promptCmd.Flags().Set("host", "")
		_ = promptCmd.Flags().Set("event", "")
		for _, name := range []string{"json", "hook", "scope", "no-dirty", "host", "event"} {
			if f := promptCmd.Flags().Lookup(name); f != nil {
				f.Changed = false
			}
		}
	}
}

func TestPromptCmd_HostModeRejectsJSONAndUnknownEvent(t *testing.T) {
	resetPromptFlagsForTest()
	var outBuf bytes.Buffer
	var errBuf bytes.Buffer
	rootCmd.SetIn(bytes.NewBufferString(`{"prompt":"看一下采集"}`))
	rootCmd.SetOut(&outBuf)
	rootCmd.SetErr(&errBuf)
	rootCmd.SetArgs([]string{"prompt", "--hook", "--host", "cursor", "--event", "beforeSubmitPrompt", "--json"})
	if err := rootCmd.Execute(); err == nil {
		t.Fatal("宿主原生模式与 --json 同时使用应返回用法错误")
	}

	resetPromptFlagsForTest()
	outBuf.Reset()
	errBuf.Reset()
	rootCmd.SetIn(bytes.NewBufferString(`{"prompt":"看一下采集"}`))
	rootCmd.SetOut(&outBuf)
	rootCmd.SetErr(&errBuf)
	rootCmd.SetArgs([]string{"prompt", "--hook", "--host", "cursor", "--event", "sessionStart"})
	if err := rootCmd.Execute(); err == nil {
		t.Fatal("未支持的事件应返回用法错误")
	}
	if strings.Contains(outBuf.String(), "agate-intent") {
		t.Fatal("失败的原生事件不应把执行信封写到标准输出")
	}

	// 未知事件应返回退出码 2 (UsageError)
	resetPromptFlagsForTest()
	outBuf.Reset()
	errBuf.Reset()
	rootCmd.SetIn(bytes.NewBufferString(`{"prompt":"看一下采集"}`))
	exitCode := ExecuteWithArgs([]string{"prompt", "--hook", "--host", "cursor", "--event", "sessionStart"}, &outBuf, &errBuf)
	if exitCode != 2 {
		t.Fatalf("未支持事件应返回退出码 2，实际 %d", exitCode)
	}

	// --set-diagnosis 缺少 --request-id 应返回退出码 2
	resetPromptFlagsForTest()
	outBuf.Reset()
	errBuf.Reset()
	exitCode = ExecuteWithArgs([]string{"prompt", "--set-diagnosis", "confirmed"}, &outBuf, &errBuf)
	if exitCode != 2 {
		t.Fatalf("--set-diagnosis 缺少 --request-id 应返回退出码 2，实际 %d", exitCode)
	}
}
