package git

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// DefaultExcludedItems 默认需要进行隐形隔离的 AI 私有文件/目录
var DefaultExcludedItems = []string{
	".env",
	".env.*",
	".env.local",
	".gemini/",
	".cursor/",
	".agents/",
	".ai-memory/",
	".cursorrules",
	".windsurfrules",
	"CLAUDE.md",
	"TASK.md",
	"MEMORY.md",
	".ignore",
	".vscode/",
	"*.log",
	"logs/",
}

// GetGitCommonDir 解析并返回当前工程真实的 Git 存储公共目录（兼容常规仓、Git Worktree 与 Submodule）
func GetGitCommonDir(dir ...string) (string, error) {
	base := "."
	if len(dir) > 0 && dir[0] != "" {
		base = dir[0]
	}

	gitPath := filepath.Join(base, ".git")
	stat, err := os.Stat(gitPath)
	if err == nil {
		if stat.IsDir() {
			return gitPath, nil
		}
		// 若 .git 为普通文件，说明处于 git worktree 或 submodule 下
		content, readErr := os.ReadFile(gitPath)
		if readErr == nil {
			line := strings.TrimSpace(string(content))
			if strings.HasPrefix(line, "gitdir:") {
				gitDir := strings.TrimSpace(strings.TrimPrefix(line, "gitdir:"))
				if !filepath.IsAbs(gitDir) {
					gitDir = filepath.Clean(filepath.Join(base, gitDir))
				}
				// 检查 worktree 内部是否有 commondir 指向主仓
				commondirFile := filepath.Join(gitDir, "commondir")
				if cContent, cErr := os.ReadFile(commondirFile); cErr == nil {
					cDir := strings.TrimSpace(string(cContent))
					if !filepath.IsAbs(cDir) {
						cDir = filepath.Clean(filepath.Join(gitDir, cDir))
					}
					return cDir, nil
				}
				return gitDir, nil
			}
		}
	}

	// 兜底调用 git 原生命令获取公共目录
	cmd := exec.Command("git", "rev-parse", "--git-common-dir")
	cmd.Dir = base
	out, cmdErr := cmd.Output()
	if cmdErr == nil {
		res := strings.TrimSpace(string(out))
		if res != "" {
			if !filepath.IsAbs(res) {
				res = filepath.Clean(filepath.Join(base, res))
			}
			return res, nil
		}
	}

	return "", fmt.Errorf("当前目录不在任何 Git 仓库或工作树中")
}

// IsGitRepo 判断当前目录是否处于 Git 仓库或有效 Git 工作树下
func IsGitRepo() bool {
	_, err := GetGitCommonDir()
	return err == nil
}

// ApplyPrivateExclusions 向 Git 的 info/exclude 注入隔离清单（原生兼容主仓与 Worktree）
func ApplyPrivateExclusions(items []string) (int, error) {
	gitCommonDir, err := GetGitCommonDir()
	if err != nil {
		return 0, nil // 非 Git 仓库，跳过
	}

	if len(items) == 0 {
		items = DefaultExcludedItems
	}

	excludePath := filepath.Join(gitCommonDir, "info", "exclude")
	if err := os.MkdirAll(filepath.Dir(excludePath), 0755); err != nil {
		return 0, fmt.Errorf("创建 Git info 目录失败: %w", err)
	}

	hasMarker := false
	existingSet := make(map[string]bool)
	if _, err := os.Stat(excludePath); err == nil {
		f, err := os.Open(excludePath)
		if err == nil {
			scanner := bufio.NewScanner(f)
			for scanner.Scan() {
				line := strings.TrimSpace(scanner.Text())
				if strings.Contains(line, "agate private tracking start") || strings.Contains(line, "adh private tracking start") {
					hasMarker = true
				}
				if line != "" && !strings.HasPrefix(line, "#") {
					existingSet[line] = true
				}
			}
			f.Close()
		}
	}

	var toAppend []string
	for _, item := range items {
		cleanItem := strings.TrimSpace(item)
		if cleanItem != "" && !existingSet[cleanItem] {
			toAppend = append(toAppend, cleanItem)
		}
	}

	if len(toAppend) == 0 {
		return 0, nil
	}

	f, err := os.OpenFile(excludePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return 0, fmt.Errorf("打开 .git/info/exclude 失败: %w", err)
	}
	defer f.Close()

	var sb strings.Builder
	if !hasMarker {
		sb.WriteString("\n# --- agate private tracking start ---\n")
	}
	for _, item := range toAppend {
		sb.WriteString(item + "\n")
	}
	if !hasMarker {
		sb.WriteString("# --- agate private tracking end ---\n")
	}

	if _, err := f.WriteString(sb.String()); err != nil {
		return 0, fmt.Errorf("写入 .git/info/exclude 失败: %w", err)
	}

	return len(toAppend), nil
}
