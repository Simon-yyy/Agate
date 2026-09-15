package git

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DefaultExcludedItems 默认需要进行隐形隔离的 AI 私有文件/目录
var DefaultExcludedItems = []string{
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

// IsGitRepo 判断当前目录是否处于 Git 仓库根目录下
func IsGitRepo() bool {
	stat, err := os.Stat(".git")
	return err == nil && stat.IsDir()
}

// ApplyPrivateExclusions 向 .git/info/exclude 注入隔离清单
func ApplyPrivateExclusions(items []string) (int, error) {
	if !IsGitRepo() {
		return 0, nil // 非 Git 仓库，跳过
	}

	if len(items) == 0 {
		items = DefaultExcludedItems
	}

	excludePath := filepath.Join(".git", "info", "exclude")
	if err := os.MkdirAll(filepath.Dir(excludePath), 0755); err != nil {
		return 0, fmt.Errorf("创建 .git/info 目录失败: %w", err)
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
