package git

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"agate/pkg/harness"
)

const preCommitScript = `#!/usr/bin/env bash
# agate 自动生成的 pre-commit 验证门禁
set -e

if command -v agate >/dev/null 2>&1; then
    agate verify || exit 1
elif command -v adh >/dev/null 2>&1; then
    adh verify || exit 1
elif [ -f "./verify.cmd" ]; then
    cmd.exe //c "verify.cmd" || exit 1
elif [ -f "./verify.sh" ]; then
    bash "./verify.sh" || exit 1
fi

exit 0
`

const prePushScript = `#!/usr/bin/env bash
# agate 自动生成的 pre-push 验证门禁与防越权硬卡口
set -e

# 1. 物理防 AI/脚本自动化偷跑推流 (Anti-Automation Gate, Fail-Fast 毫秒短路)
# 注意：Git 调用 pre-push 时会将待推送分支列表通过 stdin (fd 0) 管道喂入，因此 fd 0 永远为管道；
# 必须检测 stdout (fd 1) 或 stderr (fd 2) 是否连接至真实终端，或检查环境变量豁免。
if [ -z "$ALLOW_AUTOMATED_PUSH" ] && [ ! -t 1 ] && [ ! -t 2 ]; then
    echo -e "\n\033[91m=================================================================\033[0m"
    echo -e "\033[91m[BLOCKED] 触发 Agate pre-push 物理硬门禁拦截！\033[0m"
    echo -e ">> 检测到当前处于非交互式终端/自动化脚本环境尝试执行 git push。"
    echo -e ">> 绝对红线：远程推送权 100% 归人类用户掌控，严禁任何 AI 偷跑推流！"
    echo -e ">> 请由人类用户在交互式终端中手动敲击 git push。"
    echo -e "\033[91m=================================================================\033[0m\n"
    exit 1
fi

# 2. 终极自检与仓库纯净度检查
if command -v agate >/dev/null 2>&1; then
    agate verify || { echo -e "\033[91m[FAIL] pre-push 阻断: 工程自检或代码洁癖未通过，禁止推流至远端!\033[0m"; exit 1; }
elif [ -f "./verify.cmd" ]; then
    cmd.exe //c "verify.cmd" || exit 1
elif [ -f "./verify.sh" ]; then
    bash "./verify.sh" || exit 1
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

	// 1. 写入 pre-commit
	preCommitPath := filepath.Join(hookDir, "pre-commit")
	if err := harness.WriteFileAtomic(preCommitPath, []byte(preCommitScript), 0755); err != nil {
		return fmt.Errorf("写入 pre-commit 脚本失败: %w", err)
	}

	// 2. 写入 pre-push
	prePushPath := filepath.Join(hookDir, "pre-push")
	if err := harness.WriteFileAtomic(prePushPath, []byte(prePushScript), 0755); err != nil {
		return fmt.Errorf("写入 pre-push 脚本失败: %w", err)
	}

	// 3. 配置本地 core.hooksPath
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

// UninstallHooks 卸载私有 Hook 配置与物理脚本
func UninstallHooks() error {
	gitCommonDir, err := GetGitCommonDir()
	if err != nil {
		return nil
	}

	cmd := exec.Command("git", "config", "--local", "--unset", "core.hooksPath")
	_ = cmd.Run()

	hookDir := filepath.Join(gitCommonDir, "custom-hooks")
	_ = os.RemoveAll(hookDir)

	return nil
}

// UninstallPreCommitHook 向后兼容别名
func UninstallPreCommitHook() error {
	return UninstallHooks()
}
