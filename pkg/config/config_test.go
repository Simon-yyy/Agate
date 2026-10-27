package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadProjectConfig(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".agate")
	if err := os.Mkdir(path, 0755); err != nil {
		t.Fatal(err)
	}
	content := "[agent]\ntargets = [\"codex\", \"cursor\"] # 目标工具\n[guard]\nstrict = true # 严格模式\ntodo = \"warning\" # 占位\nabsolute_path = \"error\" # 占位\nlarge_file_mb = 20 # 占位\n[hooks]\ninstall = false # 禁用\n"
	if err := os.WriteFile(filepath.Join(path, "config.toml"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, source, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if source == "" || !cfg.Loaded || len(cfg.Agent.Targets) != 2 || !cfg.Guard.Strict || cfg.Guard.Todo != "warning" || cfg.Guard.AbsolutePath != "error" || cfg.Guard.LargeFileMB != 20 || len(cfg.Warnings) != 3 || cfg.Hooks.Install {
		t.Fatalf("配置解析结果异常: %#v, source=%q", cfg, source)
	}
}

func TestStripInlineCommentPreservesHashInString(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".agate")
	if err := os.Mkdir(path, 0755); err != nil {
		t.Fatal(err)
	}
	content := "[guard]\ntodo = \"warning#keep\" # trailing comment\n"
	if err := os.WriteFile(filepath.Join(path, "config.toml"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, _, err := Load(root)
	if err != nil || cfg.Guard.Todo != "warning#keep" {
		t.Fatalf("字符串中的井号不应被截断: %#v, err=%v", cfg, err)
	}
}

func TestLoadMissingConfigUsesDefaults(t *testing.T) {
	cfg, source, err := Load(t.TempDir())
	if err != nil || source != "" || cfg.Loaded || !cfg.Hooks.Install {
		t.Fatalf("默认配置异常: %#v, source=%q, err=%v", cfg, source, err)
	}
}

func TestLoadDeclarativeToolProfile(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".agate"), 0755); err != nil {
		t.Fatal(err)
	}
	content := `[agent]
targets = ["newtool"]
[agent.profiles.newtool] # 声明经确认的路径
path = ".newtool/rules.md"
format = "markdown"
confirmed = true
source = "工具手册, 本地核对"
version = "1.0"
reads = ["AGENTS.md", ".newtool/rules.md",]
`
	if err := os.WriteFile(filepath.Join(root, FileName), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, _, err := Load(root)
	if err != nil || cfg.Agent.Profiles["newtool"].Path != ".newtool/rules.md" || len(cfg.Agent.Profiles["newtool"].Reads) != 2 {
		t.Fatalf("档案配置加载失败: %#v %v", cfg, err)
	}
	for _, bad := range []string{
		strings.Replace(content, "confirmed = true", "confirmed = false", 1),
		strings.Replace(content, ".newtool/rules.md", "../rules.md", 1),
		strings.Replace(content, "format = \"markdown\"", "format = \"markdown\"\nformat = \"markdown\"", 1),
	} {
		if err := os.WriteFile(filepath.Join(root, FileName), []byte(bad), 0644); err != nil {
			t.Fatal(err)
		}
		if _, _, err := Load(root); err == nil {
			t.Fatal("非法、未确认或重复配置未被阻断")
		}
	}
}

func TestProfileStringArraysPreserveCommasAndEscapes(t *testing.T) {
	values, err := parseArray(`["a,b", "quote\"inside",]`)
	if err != nil || len(values) != 2 || values[0] != "a,b" || values[1] != "quote\"inside" {
		t.Fatalf("字符串数组解析错误: %v %v", values, err)
	}
}

func TestThreeLayerMergeAndMonotonicSecurityTightening(t *testing.T) {
	homeDir := t.TempDir()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(homeDir, ".agate"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".agate"), 0755); err != nil {
		t.Fatal(err)
	}

	globalContent := `[agent]
targets = ["trae"]
[agent.profiles.trae]
path = ".trae/rules.md"
format = "frontmatter-md"
confirmed = true
source = "个人全局工具库"
version = "1.0"
env_keys = ["TRAE_AGENT"]
term_programs = ["trae"]
frontmatter = "description: Trae Guardrails\nalwaysApply: true"
`
	if err := os.WriteFile(filepath.Join(homeDir, ".agate", "config.toml"), []byte(globalContent), 0644); err != nil {
		t.Fatal(err)
	}

	projectContent := `[agent]
targets = ["codex"]
[guard]
strict = true
[hooks]
install = true
`
	if err := os.WriteFile(filepath.Join(root, ".agate", "config.toml"), []byte(projectContent), 0644); err != nil {
		t.Fatal(err)
	}

	localContent := `[agent]
targets = ["trae", "cline"]
[agent.profiles.cline]
path = ".clinerules"
format = "pointer"
confirmed = true
source = "本机追加工具"
version = "1.0"
env_keys = ["CLINE_AGENT"]
[guard]
strict = false
[hooks]
install = false
`
	if err := os.WriteFile(filepath.Join(root, ".agate", "config.local.toml"), []byte(localContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, source, err := LoadMerged(root, homeDir)
	if err != nil {
		t.Fatalf("三层配置合并失败: %v", err)
	}
	if !cfg.Loaded || source == "" {
		t.Fatalf("预期配置已加载，实际: %+v, source=%q", cfg, source)
	}
	if len(cfg.Agent.Targets) != 2 || cfg.Agent.Targets[0] != "trae" || cfg.Agent.Targets[1] != "cline" {
		t.Fatalf("config.local.toml 应能覆盖本机默认 targets，实际: %v", cfg.Agent.Targets)
	}
	traeProfile, ok := cfg.Agent.Profiles["trae"]
	if !ok || traeProfile.Scope != "global" || traeProfile.Format != "frontmatter-md" || len(traeProfile.EnvKeys) != 1 || traeProfile.EnvKeys[0] != "TRAE_AGENT" || len(traeProfile.TermPrograms) != 1 || traeProfile.TermPrograms[0] != "trae" || !strings.Contains(traeProfile.Frontmatter, "alwaysApply: true") {
		t.Fatalf("全局档案 trae 字段或 Scope 异常: %+v", traeProfile)
	}
	clineProfile, ok := cfg.Agent.Profiles["cline"]
	if !ok || clineProfile.Scope != "local" || clineProfile.Format != "pointer" {
		t.Fatalf("本机档案 cline 字段或 Scope 异常: %+v", clineProfile)
	}
	if !cfg.Guard.Strict || !cfg.Hooks.Install {
		t.Fatalf("安全门禁只升不降失效: Strict=%v, Hooks.Install=%v", cfg.Guard.Strict, cfg.Hooks.Install)
	}
	warningsText := strings.Join(cfg.Warnings, "\n")
	if !strings.Contains(warningsText, "guard.strict") || !strings.Contains(warningsText, "hooks.install") {
		t.Fatalf("缺少 config.local.toml 降级拦截告警: %s", warningsText)
	}
}
