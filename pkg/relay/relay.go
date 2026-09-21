package relay

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"agate/internal/templates"
	agentinfo "agate/pkg/agent"
	"agate/pkg/git"
	"agate/pkg/guard"
	"agate/pkg/harness"
	"agate/pkg/reporter"
)

// HandoverOptions 交接参数选项
type HandoverOptions struct {
	RootDir      string
	CurrentAgent string
	NextAgent    string
	Note         string
	SkipVerify   bool
	Force        bool
	TestPassed   bool
	TestOutput   string
}

// ResumeResult 接棒唤醒结果
type ResumeResult struct {
	Manifest    *TaskManifest
	Quartet     HandoverQuartet
	ReceiptHTML string
	Lock        *TaskLock
}

// TaskStatusResult 任务状态查询模型
type TaskStatusResult struct {
	Manifest       *TaskManifest
	Lock           *TaskLock
	Quartet        HandoverQuartet
	RecentTimeline []string
}

func ensureLockOwner(root, agent string, force bool) error {
	lock, err := ReadLock(root)
	if err != nil {
		return err
	}
	if lock == nil {
		return fmt.Errorf("当前任务未被认领，禁止执行状态流转")
	}
	if lock.OwnerAgent != agent && !force && !lock.IsExpired() {
		return fmt.Errorf("无权操作由 [%s] 持有的任务锁 (请使用 --force 强制操作)", lock.OwnerAgent)
	}
	return nil
}

// DetectCurrentAgent 智能嗅探当前执行环境对应的 Agent 名称
func DetectCurrentAgent(rootDir string) string {
	if rootDir == "" {
		rootDir = "."
	}

	termProgram := strings.ToLower(os.Getenv("TERM_PROGRAM"))
	gitIpc := strings.ToLower(os.Getenv("VSCODE_GIT_IPC_HANDLE"))

	// 1. 环境变量与活动终端优先探测
	if detected, ok := agentinfo.DetectEnvironment(os.Getenv, termProgram, gitIpc); ok {
		return string(detected)
	}

	// 2. 独占式规约文件标记探测
	hasGemini := false
	hasCursor := false
	hasClaude := false
	hasWindsurf := false

	if _, err := os.Stat(filepath.Join(rootDir, ".gemini")); err == nil {
		hasGemini = true
	}
	if _, err := os.Stat(filepath.Join(rootDir, ".cursorrules")); err == nil {
		hasCursor = true
	}
	if _, err := os.Stat(filepath.Join(rootDir, "CLAUDE.md")); err == nil {
		hasClaude = true
	}
	if _, err := os.Stat(filepath.Join(rootDir, ".windsurfrules")); err == nil {
		hasWindsurf = true
	}

	// 独占命中判断
	if hasCursor && !hasGemini && !hasClaude && !hasWindsurf {
		return "cursor"
	}
	if hasClaude && !hasGemini && !hasCursor && !hasWindsurf {
		return "claude"
	}
	if hasWindsurf && !hasGemini && !hasCursor && !hasClaude {
		return "windsurf"
	}

	return "antigravity"
}

// ClaimTask 认领当前任务，抢占租约锁并将状态流转至 CLAIMED
func ClaimTask(rootDir string, agent string, force bool) (*TaskManifest, *TaskLock, error) {
	if rootDir == "" {
		rootDir = "."
	}
	taskPath := filepath.Join(rootDir, "TASK.md")

	// 确保 TASK.md 存在
	if _, err := os.Stat(taskPath); os.IsNotExist(err) {
		if err := harness.WriteFileAtomic(taskPath, templates.DefaultTaskTpl, 0644); err != nil {
			return nil, nil, fmt.Errorf("初始化 TASK.md 失败: %w", err)
		}
	}

	manifest, err := LoadTaskBoard(taskPath)
	if err != nil {
		return nil, nil, err
	}
	if manifest.Status == StatusDone {
		return nil, nil, fmt.Errorf("任务 [%s] 已处于 DONE 终态，禁止重新认领", manifest.TaskId)
	}

	// 若当前处于 HANDOVER_READY 状态，接棒者可以正常接单（强制放行抢锁）
	allowSteal := force || manifest.Status == StatusHandoverReady

	lock, err := AcquireLock(rootDir, manifest.TaskId, agent, DefaultLeaseDuration, allowSteal)
	if err != nil {
		return nil, nil, err
	}

	manifest.Status = StatusClaimed
	manifest.CurrentAgent = agent
	if err := SaveTaskBoard(taskPath, manifest); err != nil {
		return nil, nil, err
	}

	return manifest, lock, nil
}

// HandoverTask 执行安全自检、编译物证、流转至 HANDOVER_READY 并释放锁
func HandoverTask(opts HandoverOptions) (*TaskManifest, string, error) {
	root := opts.RootDir
	if root == "" {
		root = "."
	}
	repoRoot, err := git.GetRepoRoot(root)
	if err == nil && repoRoot != "" {
		root = repoRoot
	}

	taskPath := filepath.Join(root, "TASK.md")
	manifest, err := LoadTaskBoard(taskPath)
	if err != nil {
		return nil, "", err
	}

	currentAgent := opts.CurrentAgent
	if currentAgent == "" {
		currentAgent = DetectCurrentAgent(root)
	}
	if err := ensureLockOwner(root, currentAgent, opts.Force); err != nil {
		return nil, "", err
	}

	// 1. 验证门禁拦截
	var auditResult *guard.AuditResult
	if !opts.SkipVerify {
		auditResult = guard.RunPreflightAudit(root)
		if auditResult.HasErrors() {
			return nil, "", fmt.Errorf("交接前门禁自检未通过，发现 %d 处阻断违规项，禁止交接！请先修复", len(auditResult.Violations))
		}
		if !opts.TestPassed && opts.TestOutput != "" {
			return nil, "", fmt.Errorf("交接前动态测试未通过: %s，禁止交接！请先修复", opts.TestOutput)
		}
	}

	// 2. 编译自包含 HTML 审查报告
	start := time.Now()
	testPassed := true
	if !opts.SkipVerify && !opts.TestPassed && opts.TestOutput != "" {
		testPassed = false
	}
	outPath, _, rErr := reporter.BuildReport(reporter.ReportOptions{
		RootDir:     root,
		Duration:    time.Since(start).Round(time.Millisecond).String(),
		AuditResult: auditResult,
		TestPassed:  testPassed,
		TestOutput:  opts.TestOutput,
	})
	if rErr != nil {
		return nil, "", fmt.Errorf("编译交接物证报告失败: %w", rErr)
	}

	// 3. 更新 TASK.md 状态与接棒信息
	manifest.Status = StatusHandoverReady
	manifest.CurrentAgent = currentAgent
	if opts.NextAgent != "" {
		manifest.NextAgent = opts.NextAgent
	} else {
		manifest.NextAgent = "any"
	}
	relReportPath := outPath
	if rel, err := filepath.Rel(root, outPath); err == nil && !strings.HasPrefix(rel, "..") {
		relReportPath = rel
	}
	manifest.ReceiptHTML = relReportPath
	manifest.LastVerifiedAt = time.Now().Format("2006-01-02 15:04:05")

	if strings.TrimSpace(opts.Note) != "" {
		appendNoteToBody(manifest, opts.Note)
	}
	appendHandoverTimeline(manifest, currentAgent, manifest.NextAgent, relReportPath, opts.Note)

	if err := SaveTaskBoard(taskPath, manifest); err != nil {
		return nil, "", fmt.Errorf("保存交接任务看板失败: %w", err)
	}

	// 4. 释放锁
	if err := ReleaseLock(root, currentAgent, opts.Force); err != nil {
		return nil, "", fmt.Errorf("交接状态已保存，但释放任务锁失败: %w", err)
	}

	return manifest, outPath, nil
}

// DoneTask 完成任务并流转至 DONE 终态，生成终态审查物证并释放互斥锁
func DoneTask(opts HandoverOptions) (*TaskManifest, string, error) {
	root := opts.RootDir
	if root == "" {
		root = "."
	}
	repoRoot, err := git.GetRepoRoot(root)
	if err == nil && repoRoot != "" {
		root = repoRoot
	}

	taskPath := filepath.Join(root, "TASK.md")
	manifest, err := LoadTaskBoard(taskPath)
	if err != nil {
		return nil, "", err
	}

	currentAgent := opts.CurrentAgent
	if currentAgent == "" {
		currentAgent = DetectCurrentAgent(root)
	}
	if err := ensureLockOwner(root, currentAgent, opts.Force); err != nil {
		return nil, "", err
	}

	// 1. 验证门禁拦截
	var auditResult *guard.AuditResult
	if !opts.SkipVerify {
		auditResult = guard.RunPreflightAudit(root)
		if auditResult.HasErrors() {
			return nil, "", fmt.Errorf("任务归档自检未通过，发现 %d 处阻断违规项，禁止交付！请先修复", len(auditResult.Violations))
		}
		if !opts.TestPassed && opts.TestOutput != "" {
			return nil, "", fmt.Errorf("任务归档动态自检未通过: %s，禁止交付！请先修复", opts.TestOutput)
		}
	}

	// 2. 编译终态审查报告物证
	start := time.Now()
	testPassed := true
	if !opts.SkipVerify && !opts.TestPassed && opts.TestOutput != "" {
		testPassed = false
	}
	outPath, _, rErr := reporter.BuildReport(reporter.ReportOptions{
		RootDir:     root,
		Duration:    time.Since(start).Round(time.Millisecond).String(),
		AuditResult: auditResult,
		TestPassed:  testPassed,
		TestOutput:  opts.TestOutput,
	})
	if rErr != nil {
		return nil, "", fmt.Errorf("编译交付物证报告失败: %w", rErr)
	}

	// 3. 更新 TASK.md 状态为 DONE
	manifest.Status = StatusDone
	manifest.CurrentAgent = currentAgent
	manifest.NextAgent = "none"

	relReportPath := outPath
	if rel, err := filepath.Rel(root, outPath); err == nil && !strings.HasPrefix(rel, "..") {
		relReportPath = rel
	}
	manifest.ReceiptHTML = relReportPath
	manifest.LastVerifiedAt = time.Now().Format("2006-01-02 15:04:05")

	note := opts.Note
	if strings.TrimSpace(note) == "" {
		note = "任务完成，经全量自检通过并归档"
	}
	appendHandoverTimeline(manifest, currentAgent, "none (DONE)", relReportPath, "🏁 "+note)

	if err := SaveTaskBoard(taskPath, manifest); err != nil {
		return nil, "", fmt.Errorf("保存任务看板失败: %w", err)
	}

	// 4. 彻底释放锁
	if err := ReleaseLock(root, currentAgent, opts.Force); err != nil {
		return nil, "", fmt.Errorf("任务归档状态已保存，但释放任务锁失败: %w", err)
	}

	return manifest, outPath, nil
}

// ResumeTask 新 Agent 唤醒接棒
func ResumeTask(rootDir string, agent string) (*ResumeResult, error) {
	root := rootDir
	if root == "" {
		root = "."
	}
	repoRoot, err := git.GetRepoRoot(root)
	if err == nil && repoRoot != "" {
		root = repoRoot
	}

	taskPath := filepath.Join(root, "TASK.md")
	if _, err := os.Stat(taskPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("未检测到 TASK.md，项目尚未初始化任务看板")
	}

	manifest, err := LoadTaskBoard(taskPath)
	if err != nil {
		return nil, err
	}

	if agent == "" {
		agent = DetectCurrentAgent(root)
	}

	// 认领接棒并抢占锁
	manifest, lock, cErr := ClaimTask(root, agent, manifest.Status == StatusHandoverReady)
	if cErr != nil {
		return nil, cErr
	}

	manifest.Status = StatusInProgress
	_ = SaveTaskBoard(taskPath, manifest)

	quartet := ExtractHandoverQuartet(manifest.Body)

	return &ResumeResult{
		Manifest:    manifest,
		Quartet:     quartet,
		ReceiptHTML: manifest.ReceiptHTML,
		Lock:        lock,
	}, nil
}

// GetTaskStatus 获取当前任务看板与锁综合状态
func GetTaskStatus(rootDir string) (*TaskStatusResult, error) {
	root := rootDir
	if root == "" {
		root = "."
	}
	repoRoot, err := git.GetRepoRoot(root)
	if err == nil && repoRoot != "" {
		root = repoRoot
	}

	taskPath := filepath.Join(root, "TASK.md")
	manifest, err := LoadTaskBoard(taskPath)
	if err != nil {
		return nil, err
	}

	lock, _ := ReadLock(root)
	quartet := ExtractHandoverQuartet(manifest.Body)

	return &TaskStatusResult{
		Manifest:       manifest,
		Lock:           lock,
		Quartet:        quartet,
		RecentTimeline: ExtractRecentTimeline(manifest.Body, 3),
	}, nil
}

func appendNoteToBody(m *TaskManifest, note string) {
	noteLine := fmt.Sprintf("\n- **最新交接批注 (%s)**: %s", time.Now().Format("15:04:05"), note)
	if strings.Contains(m.Body, "3. **接棒建议与下一步") {
		m.Body = strings.Replace(m.Body, "3. **接棒建议与下一步 (Next Action)**:", "3. **接棒建议与下一步 (Next Action)**:"+noteLine, 1)
	} else if strings.Contains(m.Body, "接力四要素") {
		m.Body = m.Body + noteLine
	} else {
		m.Body = m.Body + "\n\n## 交接备忘\n" + noteLine
	}
}

func appendHandoverTimeline(m *TaskManifest, fromAgent, toAgent, receiptPath, note string) {
	timeStr := time.Now().Format("2006-01-02 15:04:05")
	if strings.TrimSpace(note) == "" {
		note = "常规交接就绪"
	}
	noteClean := strings.ReplaceAll(strings.TrimSpace(note), "\n", " ")
	receiptDisplay := receiptPath
	if receiptPath != "" && receiptPath != "none" {
		receiptDisplay = fmt.Sprintf("[%s](%s)", filepath.Base(receiptPath), receiptPath)
	}

	tableHeader := "\n\n## 📜 跨 Agent 交接流转日志 (Handover Timeline)\n| 交接时间 | 交接者 (From) | 接棒者 (To) | 审查物证凭单 | 核心备忘批注 |\n| :--- | :--- | :--- | :--- | :--- |\n"
	row := fmt.Sprintf("| %s | %s | %s | %s | %s |\n", timeStr, fromAgent, toAgent, receiptDisplay, noteClean)

	if strings.Contains(m.Body, "## 📜 跨 Agent 交接流转日志 (Handover Timeline)") {
		m.Body = strings.TrimRight(m.Body, "\n") + "\n" + row
	} else {
		m.Body = strings.TrimRight(m.Body, "\n") + tableHeader + row
	}
}
