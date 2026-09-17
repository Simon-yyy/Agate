package relay

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"agate/pkg/harness"
)

// TaskStatus 任务生命周期状态
type TaskStatus string

const (
	StatusTodo          TaskStatus = "TODO"
	StatusClaimed       TaskStatus = "CLAIMED"
	StatusInProgress    TaskStatus = "IN_PROGRESS"
	StatusBlocked       TaskStatus = "BLOCKED"
	StatusHandoverReady TaskStatus = "HANDOVER_READY"
	StatusDone          TaskStatus = "DONE"
)

// TaskManifest 结构化任务接力单元数据模型
type TaskManifest struct {
	TaskId         string     `json:"task_id"`
	Title          string     `json:"title"`
	Status         TaskStatus `json:"status"`
	CurrentAgent   string     `json:"current_agent"`
	NextAgent      string     `json:"next_agent"`
	LastVerifiedAt string     `json:"last_verified_at"`
	ReceiptHTML    string     `json:"receipt_html"`
	UpdatedAt      string     `json:"updated_at"`
	Body           string     `json:"-"` // Markdown 正文内容
}

// HandoverQuartet 接力四要素内容
type HandoverQuartet struct {
	Done       string
	InProgress string
	NextAction string
	Traps      string
}

// ParseTaskBoard 从文本内容中解析 YAML Frontmatter 与 Markdown 正文
func ParseTaskBoard(content string) (*TaskManifest, error) {
	manifest := &TaskManifest{
		TaskId:         "TASK-INIT",
		Title:          "初始协同任务",
		Status:         StatusTodo,
		CurrentAgent:   "none",
		NextAgent:      "any",
		LastVerifiedAt: "none",
		ReceiptHTML:    "none",
		UpdatedAt:      time.Now().Format("2006-01-02 15:04:05"),
	}

	content = strings.ReplaceAll(content, "\r\n", "\n")
	trimmed := strings.TrimSpace(content)
	if !strings.HasPrefix(trimmed, "---") {
		manifest.Body = content
		return manifest, nil
	}

	// 截取 frontmatter
	afterFirst := strings.TrimPrefix(trimmed, "---")
	secondIndex := strings.Index(afterFirst, "\n---")
	if secondIndex == -1 {
		// 没有合法的闭合 ---
		manifest.Body = content
		return manifest, nil
	}

	yamlBlock := afterFirst[:secondIndex]
	bodyBlock := strings.TrimSpace(afterFirst[secondIndex+4:])
	manifest.Body = bodyBlock

	scanner := bufio.NewScanner(strings.NewReader(yamlBlock))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		// 去除引号
		val = strings.Trim(val, `"'`)

		switch key {
		case "task_id":
			manifest.TaskId = val
		case "title":
			manifest.Title = val
		case "status":
			manifest.Status = TaskStatus(val)
		case "current_agent":
			manifest.CurrentAgent = val
		case "next_agent":
			manifest.NextAgent = val
		case "last_verified_at":
			manifest.LastVerifiedAt = val
		case "receipt_html":
			manifest.ReceiptHTML = val
		case "updated_at":
			manifest.UpdatedAt = val
		}
	}

	return manifest, nil
}

// FormatTaskBoard 将 TaskManifest 序列化为带 YAML Frontmatter 的完整 Markdown 文本
func FormatTaskBoard(m *TaskManifest) string {
	var sb strings.Builder

	sb.WriteString("---\n")
	sb.WriteString(fmt.Sprintf("task_id: %s\n", m.TaskId))
	sb.WriteString(fmt.Sprintf("title: %s\n", m.Title))
	sb.WriteString(fmt.Sprintf("status: %s\n", m.Status))
	sb.WriteString(fmt.Sprintf("current_agent: %s\n", m.CurrentAgent))
	sb.WriteString(fmt.Sprintf("next_agent: %s\n", m.NextAgent))
	sb.WriteString(fmt.Sprintf("last_verified_at: %s\n", m.LastVerifiedAt))
	sb.WriteString(fmt.Sprintf("receipt_html: %s\n", m.ReceiptHTML))
	sb.WriteString(fmt.Sprintf("updated_at: %s\n", m.UpdatedAt))
	sb.WriteString("---\n\n")

	if strings.TrimSpace(m.Body) != "" {
		sb.WriteString(strings.TrimSpace(m.Body))
		sb.WriteString("\n")
	}

	return sb.String()
}

// LoadTaskBoard 从指定文件（通常为 TASK.md）读取并解析任务接力单
func LoadTaskBoard(path string) (*TaskManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取任务看板失败 [%s]: %w", path, err)
	}
	return ParseTaskBoard(string(data))
}

// SaveTaskBoard 原子写入保存任务看板
func SaveTaskBoard(path string, m *TaskManifest) error {
	m.UpdatedAt = time.Now().Format("2006-01-02 15:04:05")
	content := FormatTaskBoard(m)

	dir := filepath.Dir(path)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("创建目录失败 [%s]: %w", dir, err)
		}
	}

	return harness.WriteFileAtomic(path, []byte(content), 0644)
}

// ExtractHandoverQuartet 从 Markdown 正文中智能提取接力四要素
func ExtractHandoverQuartet(body string) HandoverQuartet {
	var q HandoverQuartet
	lines := strings.Split(body, "\n")

	currSection := ""
	var sectionLines []string

	flushSection := func() {
		text := strings.TrimSpace(strings.Join(sectionLines, "\n"))
		switch currSection {
		case "done":
			q.Done = text
		case "progress":
			q.InProgress = text
		case "next":
			q.NextAction = text
		case "traps":
			q.Traps = text
		}
		sectionLines = nil
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.Contains(trimmed, "已完成事项") || strings.Contains(trimmed, "1. **已完成") {
			flushSection()
			currSection = "done"
			if after := extractAfterColon(trimmed); after != "" {
				sectionLines = append(sectionLines, after)
			}
			continue
		} else if strings.Contains(trimmed, "在途断点") || strings.Contains(trimmed, "2. **在途") {
			flushSection()
			currSection = "progress"
			if after := extractAfterColon(trimmed); after != "" {
				sectionLines = append(sectionLines, after)
			}
			continue
		} else if strings.Contains(trimmed, "接棒建议") || strings.Contains(trimmed, "下一步") || strings.Contains(trimmed, "3. **接棒") {
			flushSection()
			currSection = "next"
			if after := extractAfterColon(trimmed); after != "" {
				sectionLines = append(sectionLines, after)
			}
			continue
		} else if strings.Contains(trimmed, "暗坑警示") || strings.Contains(trimmed, "4. **暗坑") {
			flushSection()
			currSection = "traps"
			if after := extractAfterColon(trimmed); after != "" {
				sectionLines = append(sectionLines, after)
			}
			continue
		} else if strings.HasPrefix(trimmed, "## ") || strings.HasPrefix(trimmed, "# ") {
			// 遇到任何其他二级或一级标题（如 Handover Timeline、方案架构、交接备忘等），强制截断四要素提取
			flushSection()
			currSection = ""
			continue
		}

		if currSection != "" {
			sectionLines = append(sectionLines, line)
		}
	}
	flushSection()

	return q
}

// ExtractRecentTimeline 从 TASK.md 正文中提取最近的交接流转记录行
func ExtractRecentTimeline(body string, limit int) []string {
	if limit <= 0 {
		limit = 3
	}
	var rows []string
	lines := strings.Split(body, "\n")
	inTimeline := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## 📜") || strings.Contains(trimmed, "Handover Timeline") {
			inTimeline = true
			continue
		}
		if inTimeline {
			if strings.HasPrefix(trimmed, "## ") {
				break
			}
			if strings.HasPrefix(trimmed, "|") && !strings.Contains(trimmed, "交接时间") && !strings.Contains(trimmed, ":---") {
				rows = append(rows, trimmed)
			}
		}
	}

	if len(rows) > limit {
		rows = rows[len(rows)-limit:]
	}
	return rows
}

func extractAfterColon(line string) string {
	parts := strings.SplitN(line, ":", 2)
	if len(parts) == 2 {
		val := strings.TrimSpace(parts[1])
		val = strings.TrimLeft(val, "* ")
		return strings.TrimSpace(val)
	}
	parts = strings.SplitN(line, "：", 2)
	if len(parts) == 2 {
		val := strings.TrimSpace(parts[1])
		val = strings.TrimLeft(val, "* ")
		return strings.TrimSpace(val)
	}
	return ""
}
