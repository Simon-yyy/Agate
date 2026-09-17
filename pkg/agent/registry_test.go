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
