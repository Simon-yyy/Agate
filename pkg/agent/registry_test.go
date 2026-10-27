package agent

import "testing"

func TestParseAndDetectCodex(t *testing.T) {
	if got, ok := Parse(" codex "); !ok || got != Codex {
		t.Fatalf("Parse(codex) = %q, %v", got, ok)
	}
	env := map[string]string{"CODEX_SESSION_ID": "thread-1"}
	getenv := func(key string) string { return env[key] }
	got, ok := DetectEnvironment(getenv, "", "")
	if !ok || got != Codex {
		t.Fatalf("DetectEnvironment(Codex) = %q, %v", got, ok)
	}
	if !IsAutomationEnvironment(getenv) {
		t.Fatal("Codex environment should be classified as automation")
	}
}

func TestAllHasStableOrder(t *testing.T) {
	got := All()
	if len(got) != 5 || got[0] != Codex || got[1] != Cursor {
		t.Fatalf("unexpected agent order: %v", got)
	}
}

func TestDetectEnvironmentPrefersEnvKeyOverTermProgram(t *testing.T) {
	env := map[string]string{"CLAUDE_CODE": "1"}
	getenv := func(key string) string { return env[key] }
	got, ok := DetectEnvironment(getenv, "cursor", "cursor")
	if !ok || got != Claude {
		t.Fatalf("DetectEnvironment(CLAUDE_CODE=1, TERM_PROGRAM=cursor) = %q, %v; expected %q, true", got, ok, Claude)
	}
}

func TestDetectEnvironmentWithCustomProfiles(t *testing.T) {
	custom := map[string]Profile{
		"trae": {
			Path:         ".trae/rules.md",
			Format:       "markdown",
			Confirmed:    true,
			Source:       "Trae 文档",
			Version:      "1.0",
			EnvKeys:      []string{"TRAE_AGENT", "TRAE_IDE"},
			TermPrograms: []string{"trae"},
		},
		"cline": {
			Path:         ".clinerules",
			Format:       "markdown",
			Confirmed:    true,
			Source:       "Cline 文档",
			Version:      "1.0",
			EnvKeys:      []string{"CLINE_AGENT"},
			TermPrograms: []string{"cline-terminal"},
		},
	}

	env := map[string]string{"TRAE_AGENT": "1"}
	getenv := func(key string) string { return env[key] }
	got, ok := DetectEnvironmentWithProfiles(getenv, "vscode", "", custom)
	if !ok || got != Kind("trae") {
		t.Fatalf("DetectEnvironmentWithProfiles(TRAE_AGENT=1) = %q, %v; expected trae, true", got, ok)
	}
	if !IsAutomationEnvironmentWithProfiles(getenv, custom) {
		t.Fatal("TRAE_AGENT=1 should be recognized as automation environment with custom profiles")
	}

	emptyGetenv := func(string) string { return "" }
	got, ok = DetectEnvironmentWithProfiles(emptyGetenv, "Cline-Terminal-Host", "", custom)
	if !ok || got != Kind("cline") {
		t.Fatalf("DetectEnvironmentWithProfiles(TERM_PROGRAM=Cline-Terminal-Host) = %q, %v; expected cline, true", got, ok)
	}
}
