package agent

import "testing"

func TestProfilePathsArePortableAndConfined(t *testing.T) {
	for _, path := range []string{"../rules.md", "a/../rules.md", "C:/rules.md", "C:\\rules.md", "/rules.md", "//server/rules.md", ".git/config", "nested/.git/config", ".agate/config.toml", "MAP.md", "contexts/context.md", ".env.test", "a/NUL.md", "a/COM1", "a/rules.md.", "a//rules.md", "a/./rules.md", "a/rules\x1b.md"} {
		if _, err := NormalizeRulePath(path); err == nil {
			t.Errorf("危险或不可移植路径被接受: %q", path)
		}
	}
	if _, err := NormalizeRulePath(".newtool/rules.md"); err != nil {
		t.Fatal(err)
	}
}

func TestCustomProfileConfirmationAndOwnership(t *testing.T) {
	valid := Profile{Path: ".tool/rules.md", Format: "markdown", Confirmed: true, Source: "工具文档", Version: "1.0"}
	for _, name := range []string{"codex", "gemini", "all", "BadName", "bad.name"} {
		if _, err := Profiles(map[string]Profile{name: valid}); err == nil {
			t.Errorf("非法或内置名称被覆写: %s", name)
		}
	}
	invalid := valid
	invalid.Path = "agents.MD"
	if _, err := Profiles(map[string]Profile{"tool": invalid}); err == nil {
		t.Fatal("跨平台同路径碰撞未被阻断")
	}
	invalid = valid
	invalid.Confirmed = false
	if _, err := Profiles(map[string]Profile{"tool": invalid}); err == nil {
		t.Fatal("未确认档案仍然可写入")
	}
	invalid = valid
	invalid.Format = "shell"
	if _, err := Profiles(map[string]Profile{"tool": invalid}); err == nil {
		t.Fatal("不支持格式被接受")
	}
	for _, path := range []string{"cmd/entry.go", ".github/workflows/check.yml", "AGENTS.md/rules.md"} {
		invalid = valid
		invalid.Path = path
		if _, err := Profiles(map[string]Profile{"tool": invalid}); err == nil {
			t.Errorf("可执行配置或路径层级碰撞被接受: %s", path)
		}
	}
}

func TestCustomProfileFormatsMdcAndFingerprints(t *testing.T) {
	mdc := Profile{
		Path:         ".cursor/rules/agate.mdc",
		Format:       "frontmatter-md",
		Confirmed:    true,
		Source:       "Cursor Rules 手册",
		Version:      "1.0",
		EnvKeys:      []string{"TRAE_AGENT"},
		TermPrograms: []string{"trae"},
		Frontmatter:  "description: Agate Rules\nalwaysApply: true",
	}
	pointer := Profile{
		Path:      ".cline/rules.md",
		Format:    "pointer",
		Confirmed: true,
		Source:    "Cline 手册",
		Version:   "1.0",
	}
	got, err := Profiles(map[string]Profile{"trae": mdc, "cline": pointer})
	if err != nil {
		t.Fatalf("合法的 frontmatter-md 与 pointer 档案被拒绝: %v", err)
	}
	if got["trae"].Format != "frontmatter-md" || got["cline"].Format != "pointer" {
		t.Fatalf("档案格式未保留: %+v", got)
	}

	for _, badEnv := range [][]string{{""}, {"TRAE AGENT"}, {"TRAE\x1b"}} {
		bad := mdc
		bad.EnvKeys = badEnv
		if _, err := Profiles(map[string]Profile{"trae": bad}); err == nil {
			t.Fatalf("非法 env_keys 未被阻断: %v", badEnv)
		}
	}
	for _, badTerm := range [][]string{{""}, {"trae ide"}, {"trae\n"}} {
		bad := mdc
		bad.TermPrograms = badTerm
		if _, err := Profiles(map[string]Profile{"trae": bad}); err == nil {
			t.Fatalf("非法 term_programs 未被阻断: %v", badTerm)
		}
	}
	for _, badFM := range []string{"description: ok\n---\ninjected: true", "bad\x00char"} {
		bad := mdc
		bad.Frontmatter = badFM
		if _, err := Profiles(map[string]Profile{"trae": bad}); err == nil {
			t.Fatalf("非法 frontmatter 未被阻断: %q", badFM)
		}
	}
}
