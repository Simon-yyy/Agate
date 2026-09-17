package git

import (
	"bytes"
	"os/exec"
	"strings"
)

// GetGitDiff 获取当前仓库的 Git Diff 文本（支持 staged 模式）
func GetGitDiff(staged bool, dir ...string) (string, error) {
	root, err := GetRepoRoot(dir...)
	if err != nil {
		return "", err
	}

	args := []string{"diff"}
	if staged {
		args = append(args, "--cached")
	}

	cmd := exec.Command("git", args...)
	cmd.Dir = root
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return "", err
	}

	return stdout.String(), nil
}

// GetGitMetadata 获取当前 Git 分支名与最近一次 Commit 的短 Hash
func GetGitMetadata(dir ...string) (branch string, commit string, err error) {
	root, err := GetRepoRoot(dir...)
	if err != nil {
		return "", "", err
	}

	// 1. 获取分支名
	cmdBranch := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmdBranch.Dir = root
	var outBranch bytes.Buffer
	cmdBranch.Stdout = &outBranch
	if err := cmdBranch.Run(); err == nil {
		branch = strings.TrimSpace(outBranch.String())
	}

	// 2. 获取 Commit 短 Hash
	cmdCommit := exec.Command("git", "rev-parse", "--short", "HEAD")
	cmdCommit.Dir = root
	var outCommit bytes.Buffer
	cmdCommit.Stdout = &outCommit
	if err := cmdCommit.Run(); err == nil {
		commit = strings.TrimSpace(outCommit.String())
	}

	return branch, commit, nil
}
