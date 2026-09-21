package config

import (
	"os"
	"path/filepath"
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
