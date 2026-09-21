package reporter

import (
	"bytes"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"agate/internal/templates"
	"agate/pkg/git"
	"agate/pkg/guard"
	"agate/pkg/harness"
)

// DiffLine 表示格式化后的单行 Git Diff
type DiffLine struct {
	Type    string // "add", "del", "hunk", "file", "plain"
	Content string
}

// ReportData 汇聚自包含审查报告所需全部数据模型
type ReportData struct {
	ProjectName    string
	GeneratedAt    string
	GitBranch      string
	GitCommit      string
	Status         string // "PASS" or "FAIL"
	TotalDuration  string
	TaskContent    string
	ContextContent string
	AuditResult    *guard.AuditResult
	TestOutput     string
	TestPassed     bool
	IsStaged       bool
	GitDiffLines   []DiffLine
}

// ReportOptions 导出选项
type ReportOptions struct {
	RootDir     string
	OutputFile  string // 可选，若为空则自动输出至 .ai-memory/reviews/
	Staged      bool
	TestOutput  string
	TestPassed  bool
	Duration    string
	AuditResult *guard.AuditResult
}

// ParseDiffLines 解析原始 git diff 文本为带类型的行流
func ParseDiffLines(rawDiff string) []DiffLine {
	if strings.TrimSpace(rawDiff) == "" {
		return nil
	}

	rawLines := strings.Split(rawDiff, "\n")
	var lines []DiffLine

	for _, line := range rawLines {
		t := "plain"
		if strings.HasPrefix(line, "diff --git") {
			t = "file"
		} else if strings.HasPrefix(line, "@@") {
			t = "hunk"
		} else if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			t = "add"
		} else if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
			t = "del"
		}

		lines = append(lines, DiffLine{
			Type:    t,
			Content: line,
		})
	}

	return lines
}

// RenderHTML 编译模板生成最终的自包含 HTML 字符串
func RenderHTML(data *ReportData) (string, error) {
	tmpl, err := template.New("review").Parse(string(templates.ReviewHtmlTpl))
	if err != nil {
		return "", fmt.Errorf("解析 HTML 审查模板失败: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("渲染审查报告模板失败: %w", err)
	}

	return buf.String(), nil
}

// BuildReport 自动化采集数据、编译并安全原子输出 HTML 报告至目标路径
func BuildReport(opts ReportOptions) (string, *ReportData, error) {
	root := opts.RootDir
	if root == "" {
		root = "."
	}

	repoRoot, err := git.GetRepoRoot(root)
	if err == nil && repoRoot != "" {
		root = repoRoot
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		absRoot = root
	}
	projectName := filepath.Base(absRoot)

	// 1. 采集 Git 元数据
	branch, commit, _ := git.GetGitMetadata(root)

	// 2. 采集 TASK.md 与 contexts/context.md
	taskContent := ""
	if taskData, err := os.ReadFile(filepath.Join(root, "TASK.md")); err == nil {
		taskContent = strings.TrimSpace(string(taskData))
	}

	contextContent := ""
	if ctxData, err := os.ReadFile(filepath.Join(root, "contexts", "context.md")); err == nil {
		contextContent = strings.TrimSpace(string(ctxData))
	}

	// 3. 采集 Phase 0 审计结果
	auditResult := opts.AuditResult
	if auditResult == nil {
		if opts.Staged {
			stagedFiles, _ := git.GetStagedFiles(root)
			auditResult = guard.RunStagedAudit(root, stagedFiles)
		} else {
			auditResult = guard.RunPreflightAudit(root)
		}
	}

	// 4. 采集 Git Diff
	rawDiff, _ := git.GetGitDiff(opts.Staged, root)
	diffLines := ParseDiffLines(rawDiff)

	// 5. 判定最终综合状态
	status := "PASS"
	if auditResult.HasErrors() || (!opts.TestPassed && opts.TestOutput != "") {
		status = "FAIL"
	}

	duration := opts.Duration
	if duration == "" {
		duration = "0ms"
	}

	data := &ReportData{
		ProjectName:    projectName,
		GeneratedAt:    time.Now().Format("2006-01-02 15:04:05"),
		GitBranch:      branch,
		GitCommit:      commit,
		Status:         status,
		TotalDuration:  duration,
		TaskContent:    taskContent,
		ContextContent: contextContent,
		AuditResult:    auditResult,
		TestOutput:     opts.TestOutput,
		TestPassed:     opts.TestPassed,
		IsStaged:       opts.Staged,
		GitDiffLines:   diffLines,
	}

	// 6. 渲染单文件 HTML
	htmlContent, err := RenderHTML(data)
	if err != nil {
		return "", nil, err
	}

	// 7. 确定最终输出目标路径
	outPath := opts.OutputFile
	if outPath == "" {
		reviewsDir := filepath.Join(root, ".ai-memory", "reviews")
		if err := os.MkdirAll(reviewsDir, 0755); err != nil {
			return "", nil, fmt.Errorf("创建审查报告目录失败 [%s]: %w", reviewsDir, err)
		}
		timestamp := time.Now().Format("20060102-150405.000000000")
		outPath = filepath.Join(reviewsDir, fmt.Sprintf("review-%s.html", timestamp))
	} else {
		if dir := filepath.Dir(outPath); dir != "." && dir != "" {
			_ = os.MkdirAll(dir, 0755)
		}
	}

	// 8. 崩溃安全原子写入
	if err := harness.WriteFileAtomic(outPath, []byte(htmlContent), 0644); err != nil {
		return "", nil, fmt.Errorf("落盘审查报告文件失败 [%s]: %w", outPath, err)
	}

	return outPath, data, nil
}

// FindLatestReport 在指定的 .ai-memory/reviews 目录下查找最新生成的 HTML 报告
func FindLatestReport(baseDir string) (string, error) {
	reviewsDir := filepath.Join(baseDir, ".ai-memory", "reviews")
	entries, err := os.ReadDir(reviewsDir)
	if err != nil {
		return "", fmt.Errorf("未找到审查报告目录 [%s]: %w", reviewsDir, err)
	}

	var reports []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasPrefix(entry.Name(), "review-") && strings.HasSuffix(entry.Name(), ".html") {
			reports = append(reports, filepath.Join(reviewsDir, entry.Name()))
		}
	}

	if len(reports) == 0 {
		return "", fmt.Errorf("在 [%s] 下未找到任何已生成的审查报告", reviewsDir)
	}

	// 按文件名升序排序，时间戳后生成的排在最后
	sort.Strings(reports)
	return reports[len(reports)-1], nil
}
