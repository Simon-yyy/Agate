package relay

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseTaskBoardWithFrontmatter(t *testing.T) {
	raw := `---
task_id: TASK-2026-001
title: 重构鉴权系统
status: HANDOVER_READY
current_agent: antigravity
next_agent: cursor
last_verified_at: 2026-09-17 14:00:00
receipt_html: .ai-memory/reviews/review-01.html
updated_at: 2026-09-17 14:05:00
---

# 协同任务看板 (TASK.md)

## 一、 任务目标与背景
- **任务描述**: 迁移 JWT 认证

## 二、 接力四要素 (Handover Quartet)
1. **已完成事项 (Done)**:
   - [x] 核心 OAuth2 逻辑
2. **在途断点 (In-Progress / Blocked)**:
   - 正在修改 cmd/server.go
3. **接棒建议与下一步 (Next Action)**:
   - 修复 Redis 单测
4. **暗坑警示 (Traps & Memory)**:
   - 严禁写死密钥
`

	manifest, err := ParseTaskBoard(raw)
	if err != nil {
		t.Fatalf("ParseTaskBoard 失败: %v", err)
	}

	if manifest.TaskId != "TASK-2026-001" {
		t.Errorf("TaskId 预期 TASK-2026-001，得到: %s", manifest.TaskId)
	}
	if manifest.Status != StatusHandoverReady {
		t.Errorf("Status 预期 HANDOVER_READY，得到: %s", manifest.Status)
	}
	if manifest.CurrentAgent != "antigravity" {
		t.Errorf("CurrentAgent 预期 antigravity，得到: %s", manifest.CurrentAgent)
	}
	if manifest.NextAgent != "cursor" {
		t.Errorf("NextAgent 预期 cursor，得到: %s", manifest.NextAgent)
	}

	quartet := ExtractHandoverQuartet(manifest.Body)
	if quartet.Done == "" || quartet.InProgress == "" || quartet.NextAction == "" || quartet.Traps == "" {
		t.Errorf("接力四要素解析不全: %+v", quartet)
	}
}

func TestParseLegacyTaskBoardWithoutFrontmatter(t *testing.T) {
	legacy := `# 协同任务看板 (TASK.md)
## 当前任务目标
- 任务描述: 老旧看板无 frontmatter`

	manifest, err := ParseTaskBoard(legacy)
	if err != nil {
		t.Fatalf("解析老旧看板失败: %v", err)
	}

	if manifest.Status != StatusTodo {
		t.Errorf("老旧看板默认状态应为 TODO，得到: %s", manifest.Status)
	}
	if manifest.Body != legacy {
		t.Errorf("正文应当完整保留老旧内容")
	}
}

func TestSaveAndLoadTaskBoardRoundtrip(t *testing.T) {
	tmpDir := t.TempDir()
	taskPath := filepath.Join(tmpDir, "TASK.md")

	m := &TaskManifest{
		TaskId:         "TASK-TEST-99",
		Title:          "测试往返",
		Status:         StatusClaimed,
		CurrentAgent:   "cursor",
		NextAgent:      "any",
		LastVerifiedAt: "none",
		ReceiptHTML:    "none",
		Body:           "# 任务内容\n- 步骤 1",
	}

	if err := SaveTaskBoard(taskPath, m); err != nil {
		t.Fatalf("SaveTaskBoard 失败: %v", err)
	}

	loaded, err := LoadTaskBoard(taskPath)
	if err != nil {
		t.Fatalf("LoadTaskBoard 失败: %v", err)
	}

	if loaded.TaskId != m.TaskId {
		t.Errorf("加载的 TaskId 不匹配: %s vs %s", loaded.TaskId, m.TaskId)
	}
	if loaded.CurrentAgent != m.CurrentAgent {
		t.Errorf("加载的 CurrentAgent 不匹配: %s vs %s", loaded.CurrentAgent, m.CurrentAgent)
	}
	if loaded.Status != m.Status {
		t.Errorf("加载的 Status 不匹配: %s vs %s", loaded.Status, m.Status)
	}

	data, _ := os.ReadFile(taskPath)
	if string(data) == "" {
		t.Errorf("文件不应为空")
	}
}
