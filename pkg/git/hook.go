package git

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"agate/pkg/harness"
)

const hookScriptContent = `#!/usr/bin/env bash
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

// InstallPreCommitHook 安装基于 core.hooksPath 的本地独占 pre-commit 门禁
func InstallPreCommitHook() error {
	if !IsGitRepo() {
		return fmt.Errorf("当前目录不是 Git 仓库根目录")
	}

	hookDir := filepath.Join(".git", "custom-hooks")
	if err := os.MkdirAll(hookDir, 0755); err != nil {
		return fmt.Errorf("创建 Hook 目录失败: %w", err)
	}

	preCommitPath := filepath.Join(hookDir, "pre-commit")
	if err := harness.WriteFileAtomic(preCommitPath, []byte(hookScriptContent), 0755); err != nil {
		return fmt.Errorf("写入 pre-commit 脚本失败: %w", err)
	}

	cmd := exec.Command("git", "config", "--local", "core.hooksPath", hookDir)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("配置 git core.hooksPath 失败: %w", err)
	}

	return nil
}

// UninstallPreCommitHook 卸载私有 Hook 配置
func UninstallPreCommitHook() error {
	if !IsGitRepo() {
		return nil
	}

	cmd := exec.Command("git", "config", "--local", "--unset", "core.hooksPath")
	_ = cmd.Run()

	hookDir := filepath.Join(".git", "custom-hooks")
	_ = os.RemoveAll(hookDir)

	return nil
}
