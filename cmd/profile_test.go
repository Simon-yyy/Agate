package cmd

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func clearAgentEnvForTest(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"CODEX_SESSION_ID", "CODEX_THREAD_ID", "CODEX_VERSION", "CODEX_CI", "CODEX_SHELL",
		"CURSOR_AGENT", "CURSOR_VERSION", "ANTIGRAVITY_AGENT", "GEMINI_AGENT", "ANTIGRAVITY_IDE",
		"CLAUDE_CODE", "CLAUDE_AGENT", "WINDSURF_AGENT", "TERM_PROGRAM", "VSCODE_GIT_IPC_HANDLE",
	} {
		t.Setenv(key, "")
	}
}

func TestProfileAddListAndAutoDetectInit(t *testing.T) {
	clearAgentEnvForTest(t)
	globalDir := t.TempDir()
	globalCfg := filepath.Join(globalDir, "config.toml")
	t.Setenv("AGATE_GLOBAL_CONFIG", globalCfg)

	repoDir := t.TempDir()
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(repoDir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(origWd)

	gitInit := exec.Command("git", "init", "--quiet")
	gitInit.Dir = repoDir
	if out, err := gitInit.CombinedOutput(); err != nil {
		t.Fatalf("git init 失败: %v %s", err, out)
	}

	// 1. 使用 agate profile add --global 注册自定义工具 trae (frontmatter-md + env_keys)
	profCmd := newProfileCommand()
	var addOut bytes.Buffer
	profCmd.SetOut(&addOut)
	profCmd.SetErr(&addOut)
	profCmd.SetArgs([]string{
		"add", "trae",
		"--path", ".trae/rules/agate.mdc",
		"--format", "frontmatter-md",
		"--env", "TRAE_AGENT,TRAE_IDE",
		"--term-program", "trae",
		"--global",
	})
	if err := profCmd.Execute(); err != nil {
		t.Fatalf("agate profile add --global 失败: %v\n%s", err, addOut.String())
	}

	// 2. 使用 agate profile add --local 注册本机工具 cline (pointer)
	profLocalCmd := newProfileCommand()
	var localOut bytes.Buffer
	profLocalCmd.SetOut(&localOut)
	profLocalCmd.SetErr(&localOut)
	profLocalCmd.SetArgs([]string{
		"add", "cline",
		"--path", ".clinerules",
		"--format", "pointer",
		"--reads", "AGENTS.md",
		"--local",
	})
	if err := profLocalCmd.Execute(); err != nil {
		t.Fatalf("agate profile add --local 失败: %v\n%s", err, localOut.String())
	}

	// 3. 验证 agate profile list 包含内置档案及 global/local 自定义档案
	listCmd := newProfileCommand()
	var listOut bytes.Buffer
	listCmd.SetOut(&listOut)
	listCmd.SetErr(&listOut)
	listCmd.SetArgs([]string{"list"})
	if err := listCmd.Execute(); err != nil {
		t.Fatalf("agate profile list 失败: %v\n%s", err, listOut.String())
	}
	listText := listOut.String()
	for _, expected := range []string{
		"[builtin] codex -> AGENTS.md",
		"[global] trae -> .trae/rules/agate.mdc (format=frontmatter-md",
		"TRAE_AGENT",
		"[local] cline -> .clinerules (format=pointer",
	} {
		if !strings.Contains(listText, expected) {
			t.Fatalf("profile list 输出缺少 %q，实际输出:\n%s", expected, listText)
		}
	}

	// 4. 在新环境中仅凭 TRAE_AGENT=1 环境变量，无需 -t 参数即可自动完成 agate init
	t.Setenv("TRAE_AGENT", "1")
	initCommand := newInitCommand()
	var initOut bytes.Buffer
	initCommand.SetOut(&initOut)
	initCommand.SetErr(&initOut)
	initCommand.SetArgs([]string{"--no-hook"})
	if err := initCommand.Execute(); err != nil {
		t.Fatalf("凭自定义 env_keys 自动 init 失败: %v\n%s", err, initOut.String())
	}
	if !strings.Contains(initOut.String(), "当前 Agent 环境") || !strings.Contains(initOut.String(), "[trae]") {
		t.Fatalf("未按自定义 Agent 环境自动选择 trae:\n%s", initOut.String())
	}

	mdcBytes, err := os.ReadFile(filepath.Join(repoDir, ".trae", "rules", "agate.mdc"))
	if err != nil {
		t.Fatalf("未生成自定义工具规则文件 .trae/rules/agate.mdc: %v", err)
	}
	if !strings.HasPrefix(string(mdcBytes), "---\ndescription: Agate Engineering Guardrails\nalwaysApply: true\n---") {
		t.Fatalf("生成的 .mdc 文件缺少预期 Frontmatter:\n%s", string(mdcBytes))
	}

	excludeBytes, err := os.ReadFile(filepath.Join(repoDir, ".git", "info", "exclude"))
	if err != nil {
		t.Fatalf("读取 .git/info/exclude 失败: %v", err)
	}
	if !strings.Contains(string(excludeBytes), ".agate/config.local.toml") {
		t.Fatalf(".git/info/exclude 未自动隔离 .agate/config.local.toml:\n%s", string(excludeBytes))
	}
}
