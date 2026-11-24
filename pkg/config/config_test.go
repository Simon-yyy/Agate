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
targets = ["tool-a"]
[agent.profiles.tool-a]
path = ".tool-a/rules.md"
format = "frontmatter-md"
confirmed = true
source = "个人全局工具库"
version = "1.0"
env_keys = ["TOOL_A_AGENT"]
term_programs = ["tool-a"]
frontmatter = "description: Tool A Guardrails\nalwaysApply: true"
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
targets = ["tool-a", "tool-b"]
[agent.profiles.tool-b]
path = ".tool-b-rules"
format = "pointer"
confirmed = true
source = "本机追加工具"
version = "1.0"
env_keys = ["TOOL_B_AGENT"]
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
	if len(cfg.Agent.Targets) != 2 || cfg.Agent.Targets[0] != "tool-a" || cfg.Agent.Targets[1] != "tool-b" {
		t.Fatalf("config.local.toml 应能覆盖本机默认 targets，实际: %v", cfg.Agent.Targets)
	}
	traeProfile, ok := cfg.Agent.Profiles["tool-a"]
	if !ok || traeProfile.Scope != "global" || traeProfile.Format != "frontmatter-md" || len(traeProfile.EnvKeys) != 1 || traeProfile.EnvKeys[0] != "TOOL_A_AGENT" || len(traeProfile.TermPrograms) != 1 || traeProfile.TermPrograms[0] != "tool-a" || !strings.Contains(traeProfile.Frontmatter, "alwaysApply: true") {
		t.Fatalf("全局档案 tool-a 字段或 Scope 异常: %+v", traeProfile)
	}
	clineProfile, ok := cfg.Agent.Profiles["tool-b"]
	if !ok || clineProfile.Scope != "local" || clineProfile.Format != "pointer" {
		t.Fatalf("本机档案 tool-b 字段或 Scope 异常: %+v", clineProfile)
	}
	if !cfg.Guard.Strict || !cfg.Hooks.Install {
		t.Fatalf("安全门禁只升不降失效: Strict=%v, Hooks.Install=%v", cfg.Guard.Strict, cfg.Hooks.Install)
	}
	warningsText := strings.Join(cfg.Warnings, "\n")
	if !strings.Contains(warningsText, "guard.strict") || !strings.Contains(warningsText, "hooks.install") {
		t.Fatalf("缺少 config.local.toml 降级拦截告警: %s", warningsText)
	}
}

func TestLoadMergedWhitespaceGlobalConfigIsolation(t *testing.T) {
	// R2-S02: 纯空白 AGATE_GLOBAL_CONFIG 变量必须具有与空值一致的安全隔离语义
	homeDir := t.TempDir()
	homeAgate := filepath.Join(homeDir, ".agate")
	if err := os.MkdirAll(homeAgate, 0755); err != nil {
		t.Fatal(err)
	}

	// 家目录配置包含安全门禁与 hook 调整，试图将 hooks.install 置为 false，strict 置为 true
	homeConfig := `[agent]
targets = ["cursor"]
[guard]
strict = true
[hooks]
install = false
`
	homeConfigPath := filepath.Join(homeAgate, "config.toml")
	if err := os.WriteFile(homeConfigPath, []byte(homeConfig), 0644); err != nil {
		t.Fatal(err)
	}

	root := t.TempDir()
	// 项目未显式配置 guard 和 hooks，默认 Guard.Strict=false, Hooks.Install=true

	whitespaceCases := []string{
		"",
		" ",
		"   ",
		"\t",
		"  \t \n ",
	}

	for _, ws := range whitespaceCases {
		t.Run("ws_"+strings.TrimSpace(ws), func(t *testing.T) {
			t.Setenv("AGATE_GLOBAL_CONFIG", ws)
			cfg, _, err := LoadMerged(root, homeDir)
			if err != nil {
				t.Fatalf("LoadMerged 失败: %v", err)
			}
			// 家目录的安全策略不得隐式污染项目默认门禁
			if !cfg.Hooks.Install {
				t.Fatalf("纯空白 AGATE_GLOBAL_CONFIG (%q) 导致家目录 hooks.install=false 绕过隔离生效！", ws)
			}
			if cfg.Guard.Strict {
				t.Fatalf("纯空白 AGATE_GLOBAL_CONFIG (%q) 导致家目录 guard.strict=true 绕过隔离生效！", ws)
			}
		})
	}

	// 对照组：显式指定 AGATE_GLOBAL_CONFIG 时，才允许全局安全策略生效
	t.Run("explicit_global_config", func(t *testing.T) {
		t.Setenv("AGATE_GLOBAL_CONFIG", homeConfigPath)
		cfg, _, err := LoadMerged(root, homeDir)
		if err != nil {
			t.Fatalf("LoadMerged 失败: %v", err)
		}
		if cfg.Hooks.Install {
			t.Fatalf("显式指定 AGATE_GLOBAL_CONFIG 时 hooks.install 应按全局配置生效为 false")
		}
		if !cfg.Guard.Strict {
			t.Fatalf("显式指定 AGATE_GLOBAL_CONFIG 时 guard.strict 应按全局配置生效为 true")
		}
	})
}

func TestValidateGuardPolicyDoesNotLeakSensitiveValue(t *testing.T) {
	// R4-04: 错误信息绝不得回显输入原值，杜绝敏感配置泄露
	err := validateGuardPolicy("todo", "secret_token_123456")
	if err == nil {
		t.Fatal("非法策略值应返回错误")
	}
	if strings.Contains(err.Error(), "secret_token_123456") {
		t.Fatalf("安全红线拦截：validateGuardPolicy 错误信息泄漏了输入原值: %s", err.Error())
	}
	if !strings.Contains(err.Error(), "guard.todo") || !strings.Contains(err.Error(), "仅支持 warning/warn") {
		t.Fatalf("错误信息应包含字段名与合法枚举提示，实际: %s", err.Error())
	}
}
