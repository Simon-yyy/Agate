package git

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"agate/pkg/harness"
)

const preCommitScript = `#!/usr/bin/env bash
# agate 自动生成的 pre-commit 验证门禁 (针对 Git 暂存区 Index 精准审计)
set -e

if command -v agate >/dev/null 2>&1; then
    agate verify --staged || exit 1
elif [ -f "./verify.cmd" ]; then
    cmd.exe //c "verify.cmd" || exit 1
elif [ -f "./verify.sh" ]; then
    bash "./verify.sh" || exit 1
else
    echo -e "\n\033[91m[FAIL] pre-commit 门禁拦截: 未找到 agate 或工程自检脚本 (verify.sh/cmd)！\033[0m"
    echo -e ">> 请确保系统 PATH 包含 agate，或在项目根目录下提供自检脚本。"
    echo -e ">> 如需临时应急跳过，可使用 git commit --no-verify\n"
    exit 1
fi

exit 0
`

const prePushScript = `#!/usr/bin/env bash
# agate 自动生成的 pre-push 验证门禁与防越权硬卡口
set -e

# 1. 物理防 AI/脚本自动化偷跑推流 (Anti-Automation Gate, Fail-Fast 毫秒短路)
# 注意：Git 调用 pre-push 时会将待推送分支列表通过 stdin (fd 0) 管道喂入，因此 fd 0 永远为管道；
# 必须检测 stdout (fd 1) 或 stderr (fd 2) 是否连接至真实终端，或检查环境变量显式授权 (仅认 1 或 true)。
is_authorized=0
if [ "$ALLOW_AUTOMATED_PUSH" = "1" ] || [ "$ALLOW_AUTOMATED_PUSH" = "true" ]; then
    is_authorized=1
fi

# 增加 Agent/CI 自动化环境指纹检测 (防 PTY 伪终端伪造 TTY 绕过 RSK-001)
is_agent_env=0
if [ -n "$CI" ] || [ -n "$GITHUB_ACTIONS" ] || [ -n "$CURSOR_AGENT" ] || [ -n "$ANTIGRAVITY_AGENT" ] || [ -n "$GEMINI_AGENT" ] || [ -n "$CLAUDE_AGENT" ] || [ -n "$WINDSURF_AGENT" ] || [ -n "$AI_AGENT" ]; then
    is_agent_env=1
fi

if [ "$is_authorized" -eq 0 ]; then
    if [ "$is_agent_env" -eq 1 ] || { [ ! -t 1 ] && [ ! -t 2 ]; }; then
        echo -e "\n\033[91m=================================================================\033[0m"
        echo -e "\033[91m[BLOCKED] 触发 Agate pre-push 物理硬门禁拦截！\033[0m"
        echo -e ">> 检测到当前处于非交互终端或智能体自动化环境尝试执行 git push。"
        echo -e ">> 安全保护：未获得人类用户显式授权，严禁任何 AI/脚本擅自偷跑推流！"
        echo -e ">> 授权通道：唯有在用户明确指令推送时，声明 ALLOW_AUTOMATED_PUSH=1 方可执行；"
        echo -e ">> 常规场景：请由人类用户在交互式终端中手动敲击 git push。"
        echo -e "\033[91m=================================================================\033[0m\n"
        exit 1
    fi
fi

# 2. 终极自检与仓库纯净度检查
if command -v agate >/dev/null 2>&1; then
    agate verify || { echo -e "\033[91m[FAIL] pre-push 阻断: 工程自检或代码洁癖未通过，禁止推流至远端!\033[0m"; exit 1; }
elif [ -f "./verify.cmd" ]; then
    cmd.exe //c "verify.cmd" || exit 1
elif [ -f "./verify.sh" ]; then
    bash "./verify.sh" || exit 1
else
    echo -e "\n\033[91m[FAIL] pre-push 阻断: 未找到 agate 或工程自检脚本 (verify.sh/cmd)，禁止推流至远端！\033[0m"
    echo -e ">> 请确保系统 PATH 包含 agate，或在项目根目录下提供自检脚本。"
    exit 1
fi

exit 0
`

// InstallHooks 安装基于 core.hooksPath 的本地 pre-commit 与 pre-push 双重物理硬门禁
func InstallHooks() error {
	gitCommonDir, err := GetGitCommonDir()
	if err != nil {
		return fmt.Errorf("当前目录不是 Git 仓库或工作树根目录: %w", err)
	}

	hookDir := filepath.Join(gitCommonDir, "custom-hooks")
	if err := os.MkdirAll(hookDir, 0755); err != nil {
		return fmt.Errorf("创建 Hook 目录失败: %w", err)
	}

	// 1. 记录可能已存在的旧 core.hooksPath 配置，避免卸载时丢失原先环境配置
	origHooksPathFile := filepath.Join(hookDir, ".agate_orig_hooks_path")
	if _, err := os.Stat(origHooksPathFile); os.IsNotExist(err) {
		getCmd := exec.Command("git", "config", "--local", "core.hooksPath")
		if out, err := getCmd.Output(); err == nil {
			oldPath := strings.TrimSpace(string(out))
			if oldPath != "" && oldPath != hookDir {
				_ = harness.WriteFileAtomic(origHooksPathFile, []byte(oldPath), 0644)
			}
		}
	}

	// 2. 写入 pre-commit
	preCommitPath := filepath.Join(hookDir, "pre-commit")
	if err := harness.WriteFileAtomic(preCommitPath, []byte(preCommitScript), 0755); err != nil {
		return fmt.Errorf("写入 pre-commit 脚本失败: %w", err)
	}

	// 3. 写入 pre-push
	prePushPath := filepath.Join(hookDir, "pre-push")
	if err := harness.WriteFileAtomic(prePushPath, []byte(prePushScript), 0755); err != nil {
		return fmt.Errorf("写入 pre-push 脚本失败: %w", err)
	}

	// 4. 配置本地 core.hooksPath
	cmd := exec.Command("git", "config", "--local", "core.hooksPath", hookDir)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("配置 git core.hooksPath 失败: %w", err)
	}

	return nil
}

// InstallPreCommitHook 向后兼容别名
func InstallPreCommitHook() error {
	return InstallHooks()
}

// UninstallHooks 卸载私有 Hook 配置与物理脚本，绝不破坏用户其它自定义钩子
func UninstallHooks() error {
	gitCommonDir, err := GetGitCommonDir()
	if err != nil {
		return nil
	}

	hookDir := filepath.Join(gitCommonDir, "custom-hooks")
	origHooksPathFile := filepath.Join(hookDir, ".agate_orig_hooks_path")

	// 1. 恢复或清理 core.hooksPath
	if origContent, err := os.ReadFile(origHooksPathFile); err == nil {
		oldPath := strings.TrimSpace(string(origContent))
		if oldPath != "" {
			_ = exec.Command("git", "config", "--local", "core.hooksPath", oldPath).Run()
		} else {
			_ = exec.Command("git", "config", "--local", "--unset", "core.hooksPath").Run()
		}
		_ = os.Remove(origHooksPathFile)
	} else {
		_ = exec.Command("git", "config", "--local", "--unset", "core.hooksPath").Run()
	}

	// 2. 精准删除 agate 生成的文件，保护用户其它自定义钩子
	_ = os.Remove(filepath.Join(hookDir, "pre-commit"))
	_ = os.Remove(filepath.Join(hookDir, "pre-push"))
	_ = os.Remove(origHooksPathFile)

	// 3. 仅当目录为空时，才安全移除 custom-hooks 目录
	entries, err := os.ReadDir(hookDir)
	if err == nil && len(entries) == 0 {
		_ = os.Remove(hookDir)
	}

	return nil
}

// UninstallPreCommitHook 向后兼容别名
func UninstallPreCommitHook() error {
	return UninstallHooks()
}

// HookStatusInfo 描述本地 Git 门禁配置与物理钩子状态
type HookStatusInfo struct {
	IsGitRepo       bool   `json:"is_git_repo"`
	HooksPath       string `json:"hooks_path"`
	PreCommitExists bool   `json:"pre_commit_exists"`
	PreCommitAgate  bool   `json:"pre_commit_agate"`
	PrePushExists   bool   `json:"pre_push_exists"`
	PrePushAgate    bool   `json:"pre_push_agate"`
}

// GetHookStatus 查询并返回当前 Git 仓库的门禁挂载状态
func GetHookStatus(dir ...string) (HookStatusInfo, error) {
	var status HookStatusInfo

	workDir := "."
	if len(dir) > 0 && dir[0] != "" {
		workDir = dir[0]
	}

	cmd := exec.Command("git", "rev-parse", "--is-inside-work-tree")
	cmd.Dir = workDir
	if err := cmd.Run(); err != nil {
		status.IsGitRepo = false
		return status, nil
	}
	status.IsGitRepo = true

	// 查询 core.hooksPath
	cfgCmd := exec.Command("git", "config", "--local", "core.hooksPath")
	cfgCmd.Dir = workDir
	if out, err := cfgCmd.Output(); err == nil {
		status.HooksPath = strings.TrimSpace(string(out))
	}

	gitCommonDir, err := GetGitCommonDir(workDir)
	if err != nil {
		return status, err
	}

	effectiveHookDir := status.HooksPath
	if effectiveHookDir == "" {
		effectiveHookDir = filepath.Join(gitCommonDir, "hooks")
	} else if !filepath.IsAbs(effectiveHookDir) {
		effectiveHookDir = filepath.Join(workDir, effectiveHookDir)
	}

	// 检查 pre-commit
	preCommitPath := filepath.Join(effectiveHookDir, "pre-commit")
	if content, err := os.ReadFile(preCommitPath); err == nil {
		status.PreCommitExists = true
		status.PreCommitAgate = strings.Contains(string(content), "agate")
	}

	// 检查 pre-push
	prePushPath := filepath.Join(effectiveHookDir, "pre-push")
	if content, err := os.ReadFile(prePushPath); err == nil {
		status.PrePushExists = true
		status.PrePushAgate = strings.Contains(string(content), "agate") || strings.Contains(string(content), "ALLOW_AUTOMATED_PUSH")
	}

	return status, nil
}

// GetStagedFiles 返回当前 Git 暂存区 (Index) 中变动的文件列表 (过滤已删除文件)
func GetStagedFiles(dir ...string) ([]string, error) {
	workDir := "."
	if len(dir) > 0 && dir[0] != "" {
		workDir = dir[0]
	}

	cmd := exec.Command("git", "-c", "core.quotepath=false", "diff", "--cached", "--name-only", "--diff-filter=ACMR")
	cmd.Dir = workDir
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("读取 Git 暂存区失败: %w", err)
	}

	var files []string
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for _, l := range lines {
		trimmed := strings.TrimSpace(l)
		if trimmed != "" {
			files = append(files, filepath.ToSlash(trimmed))
		}
	}
	return files, nil
}
