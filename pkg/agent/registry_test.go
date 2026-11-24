package agent

import "testing"

func TestParseAndDetectCodex(t *testing.T) {
	if got, ok := Parse(" codex "); !ok || got != Codex {
		t.Fatalf("Parse(codex) = %q, %v", got, ok)
	}
	env := map[string]string{"CODEX_SESSION_ID": "thread-1"}
	getenv := func(key string) string { return env[key] }
	got, ok := DetectEnvironmentFull(getenv, "", "", nil, testMockEmpty)
	if !ok || got != Codex {
		t.Fatalf("DetectEnvironment(Codex) = %q, %v", got, ok)
	}
	if !IsAutomationEnvironment(getenv) {
		t.Fatal("Codex environment should be classified as automation")
	}
}

func TestAllHasStableOrder(t *testing.T) {
	got := All()
	if len(got) != 13 || got[0] != Codex || got[1] != Cursor {
		t.Fatalf("unexpected agent order: %v", got)
	}
}

func TestDetectEnvironmentPrefersEnvKeyOverTermProgram(t *testing.T) {
	env := map[string]string{"CLAUDE_CODE": "1"}
	getenv := func(key string) string { return env[key] }
	got, ok := DetectEnvironmentFull(getenv, "cursor", "cursor", nil, testMockEmpty)
	if !ok || got != Claude {
		t.Fatalf("DetectEnvironment(CLAUDE_CODE=1, TERM_PROGRAM=cursor) = %q, %v; expected %q, true", got, ok, Claude)
	}
}

func TestDetectEnvironmentWithCustomProfiles(t *testing.T) {
	custom := map[string]Profile{
		"my-tool": {
			Path:         ".mytool/rules.md",
			Format:       "markdown",
			Confirmed:    true,
			Source:       "MyTool 文档",
			Version:      "1.0",
			EnvKeys:      []string{"MYTOOL_AGENT", "MYTOOL_IDE"},
			TermPrograms: []string{"mytool"},
		},
		"custom-agent": {
			Path:         ".customrules",
			Format:       "markdown",
			Confirmed:    true,
			Source:       "CustomAgent 文档",
			Version:      "1.0",
			EnvKeys:      []string{"CUSTOM_AGENT"},
			TermPrograms: []string{"custom-terminal"},
		},
	}

	env := map[string]string{"MYTOOL_AGENT": "1"}
	getenv := func(key string) string { return env[key] }
	got, ok := DetectEnvironmentFull(getenv, "vscode", "", custom, testMockEmpty)
	if !ok || got != Kind("my-tool") {
		t.Fatalf("DetectEnvironmentWithProfiles(MYTOOL_AGENT=1) = %q, %v; expected my-tool, true", got, ok)
	}
	if !IsAutomationEnvironmentWithProfiles(getenv, custom) {
		t.Fatal("MYTOOL_AGENT=1 should be recognized as automation environment with custom profiles")
	}

	emptyGetenv := func(string) string { return "" }
	got, ok = DetectEnvironmentFull(emptyGetenv, "Custom-Terminal-Host", "", custom, testMockEmpty)
	if !ok || got != Kind("custom-agent") {
		t.Fatalf("DetectEnvironmentWithProfiles(TERM_PROGRAM=Custom-Terminal-Host) = %q, %v; expected custom-agent, true", got, ok)
	}
}
