package relay

import (
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"agate/internal/templates"
	"agate/pkg/adapter"
	agentinfo "agate/pkg/agent"
	"agate/pkg/config"
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
	CurrentRole  string
	NextRole     string
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
	Warnings    []string
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
	if !force && !lock.IsExpired() {
		if lock.OwnerAgent != agent {
			return fmt.Errorf("无权操作由 [%s] 持有的任务锁 (请使用 --force 强制操作)", lock.OwnerAgent)
		}
		currentSessionId := DetectCurrentSessionId()
		if lock.SessionId != "" && currentSessionId != "" && lock.SessionId != currentSessionId {
			return fmt.Errorf("任务当前正被同一工具 [%s] 的另一会话 (%s) 锁定施工，请等待其交接或使用 --force",
				lock.OwnerAgent, lock.SessionId)
		}
	}
	return nil
}

// ComputeWorkspaceDigest 计算当前工作区源码快照摘要，用于校验交接物证是否过期
func ComputeWorkspaceDigest(root string) string {
	if root == "" {
		root = "."
	}
	h := sha256.New()
	if headOut, err := exec.Command("git", "-C", root, "rev-parse", "HEAD").Output(); err == nil {
		_, _ = fmt.Fprintf(h, "HEAD:%s\n", strings.TrimSpace(string(headOut)))
	}

	var files []string
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil || rel == "." {
			return nil
		}
		slashRel := filepath.ToSlash(rel)
		if d.IsDir() {
			switch d.Name() {
			case ".git", ".ai-memory", "node_modules", "vendor":
				return filepath.SkipDir
			}
			return nil
		}
		if slashRel == "TASK.md" || slashRel == "MEMORY.md" {
			return nil
		}
		files = append(files, slashRel)
		return nil
	})
	sort.Strings(files)
	for _, slashRel := range files {
		full := filepath.Join(root, filepath.FromSlash(slashRel))
		info, err := os.Stat(full)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		_, _ = fmt.Fprintf(h, "FILE:%s:%d:", slashRel, info.Size())
		if info.Size() <= 10*1024*1024 {
			if data, readErr := os.ReadFile(full); readErr == nil {
				_, _ = h.Write(data)
			}
		} else {
			_, _ = fmt.Fprintf(h, "%d", info.ModTime().UnixNano())
		}
		_, _ = h.Write([]byte{'\n'})
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

// DetectCurrentAgent 智能嗅探当前执行环境对应的 Agent 名称
func DetectCurrentAgent(rootDir string) string {
	if rootDir == "" {
		rootDir = "."
	}

	termProgram := strings.ToLower(os.Getenv("TERM_PROGRAM"))
	gitIpc := strings.ToLower(os.Getenv("VSCODE_GIT_IPC_HANDLE"))
	cfg, _, cfgErr := config.Load(rootDir)

	// 1. 环境变量与活动终端优先探测（含自定义工具档案的 env_keys / term_programs）
	if detected, ok := agentinfo.DetectEnvironmentWithProfiles(os.Getenv, termProgram, gitIpc, cfg.Agent.Profiles); ok {
		return string(detected)
	}

	// 2. 扫描工作区已有规则目标（含 AGENTS.md 受管标记与自定义档案）
	existing := adapter.DetectExistingTargets(rootDir)
	if data, err := os.ReadFile(filepath.Join(rootDir, "AGENTS.md")); err == nil && strings.Contains(string(data), "<!-- agate:codex-rules:start -->") {
		existing = append(existing, adapter.TargetCodex)
	}
	if cfgErr == nil {
		var customNames []string
		for name, profile := range cfg.Agent.Profiles {
			if info, err := os.Stat(filepath.Join(rootDir, filepath.FromSlash(profile.Path))); err == nil && info.Mode().IsRegular() {
				customNames = append(customNames, name)
			}
		}
		sort.Strings(customNames)
		for _, name := range customNames {
			existing = append(existing, adapter.AgentTarget(name))
		}
	}

	seen := make(map[adapter.AgentTarget]bool)
	var unique []adapter.AgentTarget
	for _, t := range existing {
		if !seen[t] {
			seen[t] = true
			unique = append(unique, t)
		}
	}
	if len(unique) == 1 {
		return string(unique[0])
	}

	// 3. 多目标并存或无规则文件时，检查 .agate/config.toml 是否配置了唯一默认目标
	if cfgErr == nil && len(cfg.Agent.Targets) == 1 {
		target := strings.ToLower(strings.TrimSpace(cfg.Agent.Targets[0]))
		if target != "" && target != "all" {
			if parsed, ok := agentinfo.Parse(target); ok {
				return string(parsed)
			}
			return target
		}
	}

	// 4. 回退检查当前 TASK.md 中的持锁/执行 Agent
	if manifest, err := LoadTaskBoard(filepath.Join(rootDir, "TASK.md")); err == nil && manifest != nil {
		curr := strings.TrimSpace(manifest.CurrentAgent)
		if curr != "" && !strings.EqualFold(curr, "none") {
			return curr
		}
	}

	return "antigravity"
}

// GetActiveRoleCardPath 获取当前会话角色边界卡片路径（私有会话级，不污染根目录共享规则）
func GetActiveRoleCardPath(rootDir string) string {
	if rootDir == "" {
		rootDir = "."
	}
	return filepath.Join(rootDir, ".ai-memory", "sessions", "active-role.md")
}

func writeActiveRoleCard(rootDir, agent, role, taskId string) error {
	normRole, err := NormalizeRole(role)
	if err != nil {
		return err
	}
	if normRole != RoleDev && normRole != RoleTest && normRole != RoleOps {
		return nil
	}

	var roleDesc string
	switch normRole {
	case RoleDev:
		roleDesc = "负责依据 MAP.md 与 contexts/context.md 开展架构寻路、核心功能编码实现与本地单测编写，完成后执行 agate task handover 移交测试角色。"
	case RoleTest:
		roleDesc = "负责独立复核前序开发快照与审查物证、补充边界测试并运行 agate verify 验收，通过后移交运维部署角色。"
	case RoleOps:
		roleDesc = "负责核验测试验收物证与工作区快照指纹 (snapshot_digest) 一致性，确认全量门禁绿灯后执行交付归档。"
	}

	cardPath := GetActiveRoleCardPath(rootDir)
	if err := os.MkdirAll(filepath.Dir(cardPath), 0755); err != nil {
		return fmt.Errorf("创建会话角色卡片目录失败: %w", err)
	}

	content := fmt.Sprintf(`# Agate 会话角色边界卡 (Session Active Role)

- **任务编号**: %s
- **当前执行工具**: %s
- **当前绑定角色**: %s
- **更新时间**: %s

## 角色核心职责
%s

## 绝对安全边界
> ⚠️ **安全边界声明**：角色职责不自动授予 Git 提交、推送、发布或生产部署权限，外溢动作仍须人类显式指令授权。
`, taskId, agent, normRole, time.Now().Format("2006-01-02 15:04:05"), roleDesc)

	return harness.WriteFileAtomic(cardPath, []byte(content), 0644)
}

// ClaimTask 认领当前任务，抢占租约锁并将状态流转至 CLAIMED
func ClaimTask(rootDir string, agent string, force bool) (*TaskManifest, *TaskLock, error) {
	return ClaimTaskWithRole(rootDir, agent, "", force)
}

// ClaimTaskWithRole 以指定角色认领当前任务，抢占租约锁并将状态流转至 CLAIMED
func ClaimTaskWithRole(rootDir, agent, role string, force bool) (*TaskManifest, *TaskLock, error) {
	normRole, err := NormalizeRole(role)
	if err != nil {
		return nil, nil, err
	}
	return claimTaskWithStatus(rootDir, agent, normRole, force, StatusClaimed)
}

func claimTaskWithStatus(rootDir string, agent string, role string, force bool, targetStatus TaskStatus) (*TaskManifest, *TaskLock, error) {
	root := rootDir
	if root == "" {
		root = "."
	}
	if repoRoot, err := git.GetRepoRoot(root); err == nil && repoRoot != "" {
		root = repoRoot
	}
	taskPath := filepath.Join(root, "TASK.md")

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

	lock, err := AcquireLockWithRole(root, manifest.TaskId, agent, role, DefaultLeaseDuration, allowSteal)
	if err != nil {
		return nil, nil, err
	}

	manifest.Status = targetStatus
	manifest.CurrentAgent = agent
	if role != "" {
		manifest.CurrentRole = role
	} else if manifest.CurrentRole == "" {
		manifest.CurrentRole = "none"
	}
	if manifest.NextRole == "" {
		manifest.NextRole = "any"
	}
	if err := SaveTaskBoard(taskPath, manifest); err != nil {
		return nil, nil, err
	}

	if err := writeActiveRoleCard(root, agent, manifest.CurrentRole, manifest.TaskId); err != nil {
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

	curRole, err := NormalizeRole(opts.CurrentRole)
	if err != nil {
		return nil, "", err
	}
	nextRole, err := NormalizeRole(opts.NextRole)
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
		if !opts.TestPassed {
			msg := opts.TestOutput
			if msg == "" {
				msg = "(动态测试未通过，无标准输出)"
			}
			return nil, "", fmt.Errorf("交接前动态测试未通过: %s，禁止交接！请先修复", msg)
		}
	}

	// 2. 编译自包含 HTML 审查报告
	start := time.Now()
	testPassed := true
	testOutput := opts.TestOutput
	if !opts.SkipVerify && !opts.TestPassed {
		testPassed = false
		if testOutput == "" {
			testOutput = "(动态测试未通过，无标准输出)"
		}
	}
	outPath, _, rErr := reporter.BuildReport(reporter.ReportOptions{
		RootDir:     root,
		Duration:    time.Since(start).Round(time.Millisecond).String(),
		AuditResult: auditResult,
		TestPassed:  testPassed,
		TestOutput:  testOutput,
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
	if curRole != "" {
		manifest.CurrentRole = curRole
	} else if manifest.CurrentRole == "" {
		manifest.CurrentRole = "none"
	}
	if nextRole != "" {
		manifest.NextRole = nextRole
	} else if manifest.NextRole == "" {
		manifest.NextRole = "any"
	}
	relReportPath := outPath
	if rel, err := filepath.Rel(root, outPath); err == nil && !strings.HasPrefix(rel, "..") {
		relReportPath = rel
	}
	manifest.ReceiptHTML = relReportPath
	manifest.SnapshotDigest = ComputeWorkspaceDigest(root)
	manifest.LastVerifiedAt = time.Now().Format("2006-01-02 15:04:05")

	if err := writeActiveRoleCard(root, currentAgent, manifest.CurrentRole, manifest.TaskId); err != nil {
		return nil, "", err
	}

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

	curRole, err := NormalizeRole(opts.CurrentRole)
	if err != nil {
		return nil, "", err
	}
	effectiveRole := curRole
	if effectiveRole == "" {
		effectiveRole = manifest.CurrentRole
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
		if !opts.TestPassed {
			msg := opts.TestOutput
			if msg == "" {
				msg = "(动态测试未通过，无标准输出)"
			}
			return nil, "", fmt.Errorf("任务归档动态自检未通过: %s，禁止交付！请先修复", msg)
		}
	}

	// 2. 编译终态审查报告物证
	start := time.Now()
	testPassed := true
	testOutput := opts.TestOutput
	if !opts.SkipVerify && !opts.TestPassed {
		testPassed = false
		if testOutput == "" {
			testOutput = "(动态测试未通过，无标准输出)"
		}
	}
	outPath, _, rErr := reporter.BuildReport(reporter.ReportOptions{
		RootDir:     root,
		Duration:    time.Since(start).Round(time.Millisecond).String(),
		AuditResult: auditResult,
		TestPassed:  testPassed,
		TestOutput:  testOutput,
	})
	if rErr != nil {
		return nil, "", fmt.Errorf("编译交付物证报告失败: %w", rErr)
	}

	latestDigest := ComputeWorkspaceDigest(root)
	if effectiveRole == RoleOps && !opts.SkipVerify {
		if _, statErr := os.Stat(outPath); statErr != nil {
			return nil, "", fmt.Errorf("运维角色 [ops] 交付失败: 缺少有效审查物证文件 [%s]", outPath)
		}
		if latestDigest == "" || latestDigest == "none" {
			return nil, "", fmt.Errorf("运维角色 [ops] 交付失败: 无法计算有效源码快照摘要 snapshot_digest")
		}
	}

	// 3. 更新 TASK.md 状态为 DONE
	manifest.Status = StatusDone
	manifest.CurrentAgent = currentAgent
	manifest.NextAgent = "none"
	if effectiveRole != "" {
		manifest.CurrentRole = effectiveRole
	} else if manifest.CurrentRole == "" {
		manifest.CurrentRole = "none"
	}
	manifest.NextRole = "none"

	relReportPath := outPath
	if rel, err := filepath.Rel(root, outPath); err == nil && !strings.HasPrefix(rel, "..") {
		relReportPath = rel
	}
	manifest.ReceiptHTML = relReportPath
	manifest.SnapshotDigest = latestDigest
	manifest.LastVerifiedAt = time.Now().Format("2006-01-02 15:04:05")

	if err := writeActiveRoleCard(root, currentAgent, manifest.CurrentRole, manifest.TaskId); err != nil {
		return nil, "", err
	}

	note := opts.Note
	if strings.TrimSpace(note) == "" {
		note = "任务完成，经全量自检通过并归档"
	}
	if effectiveRole == RoleOps {
		note = fmt.Sprintf("%s [role=ops, snapshot_digest=%s]", note, manifest.SnapshotDigest)
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
	return ResumeTaskWithRole(rootDir, agent, "")
}

// ResumeTaskWithRole 新 Agent 按指定或继承角色唤醒接棒
func ResumeTaskWithRole(rootDir, agent, role string) (*ResumeResult, error) {
	normRole, err := NormalizeRole(role)
	if err != nil {
		return nil, err
	}

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

	if manifest.Status == StatusHandoverReady &&
		manifest.NextAgent != "" &&
		!strings.EqualFold(manifest.NextAgent, "any") &&
		!strings.EqualFold(manifest.NextAgent, "none") &&
		!strings.EqualFold(manifest.NextAgent, agent) {
		return nil, fmt.Errorf("当前任务指定由 [%s] 接棒，拒绝 [%s] 越权接棒 (如需强制接管请使用 agate task claim --force)", manifest.NextAgent, agent)
	}

	effectiveRole := normRole
	if manifest.Status == StatusHandoverReady &&
		manifest.NextRole != "" &&
		!strings.EqualFold(manifest.NextRole, "any") &&
		!strings.EqualFold(manifest.NextRole, "none") {
		if effectiveRole != "" && effectiveRole != "any" && effectiveRole != "none" && !strings.EqualFold(manifest.NextRole, effectiveRole) {
			return nil, fmt.Errorf("当前任务指定由角色 [%s] 接棒，拒绝角色 [%s] 越权接棒 (如需强制接管请使用 agate task claim --force)", manifest.NextRole, effectiveRole)
		}
		if effectiveRole == "" {
			effectiveRole = strings.ToLower(strings.TrimSpace(manifest.NextRole))
		}
	}

	var warnings []string
	receiptMissing := false
	snapshotStale := false
	if manifest.ReceiptHTML == "" || manifest.ReceiptHTML == "none" {
		receiptMissing = true
	} else {
		receiptFullPath := filepath.Join(root, filepath.FromSlash(manifest.ReceiptHTML))
		if _, err := os.Stat(receiptFullPath); os.IsNotExist(err) {
			receiptMissing = true
			warnings = append(warnings, "交接审查物证文件缺失: "+manifest.ReceiptHTML)
		}
	}
	if manifest.SnapshotDigest != "" && manifest.SnapshotDigest != "none" {
		if ComputeWorkspaceDigest(root) != manifest.SnapshotDigest {
			snapshotStale = true
			warnings = append(warnings, "工作区代码在交接后已发生变化，历史交接物证已过期，请重新运行 agate verify")
		}
	}
	if (effectiveRole == RoleTest || effectiveRole == RoleOps) && (snapshotStale || receiptMissing) {
		warnings = append(warnings, fmt.Sprintf("[角色复验门禁:%s] 检测到前序交接快照已过期或审查物证缺失，阻塞直接放行，请立即运行 agate verify 完成复验", effectiveRole))
	}

	// 认领接棒并直接流转至 IN_PROGRESS，避免重复写盘
	manifest, lock, cErr := claimTaskWithStatus(root, agent, effectiveRole, manifest.Status == StatusHandoverReady, StatusInProgress)
	if cErr != nil {
		return nil, cErr
	}

	quartet := ExtractHandoverQuartet(manifest.Body)

	return &ResumeResult{
		Manifest:    manifest,
		Quartet:     quartet,
		ReceiptHTML: manifest.ReceiptHTML,
		Lock:        lock,
		Warnings:    warnings,
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
	if strings.Contains(m.Body, "3. **接棒建议与下一步 (Next Action)**:") {
		m.Body = strings.Replace(m.Body, "3. **接棒建议与下一步 (Next Action)**:", "3. **接棒建议与下一步 (Next Action)**:"+noteLine, 1)
	} else if strings.Contains(m.Body, "3. **接棒建议与下一步**:") {
		m.Body = strings.Replace(m.Body, "3. **接棒建议与下一步**:", "3. **接棒建议与下一步**:"+noteLine, 1)
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
	noteClean := strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ", "|", `\|`).Replace(strings.TrimSpace(note))
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
