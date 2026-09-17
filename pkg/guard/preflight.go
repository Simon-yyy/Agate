// Package guard 提供了工程准入与安全红线的核心判定引擎。
// 涵盖私有文件泄露、二进制/大文件拦截、个人绝对路径硬编码、
// 临时草稿文件遗留以及源码级内容洁癖（冲突、断点、伪代码占位符）等多维度的安全合规静态审计。
package guard

import (
	"bufio"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

const maxScanTokenSize = 10 * 1024 * 1024 // 支持最大 10MB 单行源码扫描，防止超长行静默截断

func newSafeScanner(r io.Reader) *bufio.Scanner {
	scanner := bufio.NewScanner(r)
	buf := make([]byte, 64*1024)
	scanner.Buffer(buf, maxScanTokenSize)
	return scanner
}

// Violation 记录一处具体的合规违规项
type Violation struct {
	Level       string // "ERROR" 或 "WARN"
	Category    string // "硬编码路径", "私有文件泄露", "大文件拦截", "Vendor依赖污染"
	File        string
	LineNumber  int
	LineContent string
	Message     string
	Suggestion  string
}

// AuditResult 汇总所有违规检查结果
type AuditResult struct {
	Violations []Violation
}

// HasErrors 是否存在阻断级错误
func (r *AuditResult) HasErrors() bool {
	for _, v := range r.Violations {
		if v.Level == "ERROR" {
			return true
		}
	}
	return false
}

// PrintReport 打印彩色审计报告
func (r *AuditResult) PrintReport() {
	if len(r.Violations) == 0 {
		fmt.Println("  \033[92m[✓] 仓库安全与代码洁癖合规\033[0m")
		return
	}

	fmt.Printf("\n\033[91m[!] 触发工程护栏拦截，发现 %d 处违规项:\033[0m\n", len(r.Violations))
	for i, v := range r.Violations {
		prefix := "\033[91m[ERROR]\033[0m"
		if v.Level == "WARN" {
			prefix = "\033[93m[WARN]\033[0m"
		}

		location := v.File
		if v.LineNumber > 0 {
			location = fmt.Sprintf("%s:%d", v.File, v.LineNumber)
		}

		fmt.Printf("  %d. %s [%s] %s\n", i+1, prefix, v.Category, location)
		fmt.Printf("     问题: %s\n", v.Message)
		if v.LineContent != "" {
			fmt.Printf("     代码: %s\n", strings.TrimSpace(v.LineContent))
		}
		if v.Suggestion != "" {
			fmt.Printf("     建议: \033[96m%s\033[0m\n", v.Suggestion)
		}
		fmt.Println()
	}
}

type gitContext struct {
	root         string
	isGitRepo    bool
	trackedMap   map[string]bool
	ignoredCache map[string]bool
}

func newGitContext(root string) *gitContext {
	ctx := &gitContext{
		root:         root,
		trackedMap:   make(map[string]bool),
		ignoredCache: make(map[string]bool),
	}

	// 探测是否为 git 仓库
	checkCmd := exec.Command("git", "rev-parse", "--is-inside-work-tree")
	checkCmd.Dir = root
	if err := checkCmd.Run(); err != nil {
		ctx.isGitRepo = false
		return ctx
	}
	ctx.isGitRepo = true

	// 一次性批量加载已跟踪文件 (耗时约 10-20ms)
	lsCmd := exec.Command("git", "-c", "core.quotepath=false", "ls-files")
	lsCmd.Dir = root
	out, err := lsCmd.Output()
	if err == nil {
		scanner := newSafeScanner(strings.NewReader(string(out)))
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line != "" {
				ctx.trackedMap[filepath.ToSlash(line)] = true
			}
		}
	}
	return ctx
}

func (c *gitContext) isTracked(slashPath string) bool {
	if !c.isGitRepo {
		return false
	}
	return c.trackedMap[slashPath]
}

func (c *gitContext) isIgnored(slashPath string) bool {
	if !c.isGitRepo {
		return false
	}
	// 若已被版本库跟踪，直接视为未被忽略
	if c.trackedMap[slashPath] {
		return false
	}
	if ignored, ok := c.ignoredCache[slashPath]; ok {
		return ignored
	}
	cmd := exec.Command("git", "check-ignore", "-q", slashPath)
	cmd.Dir = c.root
	ignored := (cmd.Run() == nil)
	c.ignoredCache[slashPath] = ignored
	return ignored
}

// RunPreflightAudit 执行全套工程预检审计
func RunPreflightAudit(root string) *AuditResult {
	res := &AuditResult{}
	gitCtx := newGitContext(root)

	// 1. 检查私有文件泄露与未隔离状态
	auditForbiddenFiles(root, res, gitCtx)

	// 2. 检查大文件与可疑二进制资产
	auditBinaryAndLargeFiles(root, res, gitCtx)

	// 3. 检查代码中的个人机器绝对路径硬编码
	auditHardcodedPaths(root, res, gitCtx)

	// 4. 检查临时残留文件与调试草稿（代码洁癖）
	auditTemporaryAndScratchFiles(root, res, gitCtx)

	// 5. 检查源码内容级洁癖（冲突标记、调试断点、伪代码占位符）
	auditContentHygiene(root, res, gitCtx)

	// 6. 检查 vendor 目录纯净度
	auditVendorCleanliness(root, res)

	// 7. 检查 3-Hop 双地图完备性 (AGENTS.md 与 contexts/context.md 架构契约)
	auditArchitectureMaps(root, res)

	// 若当前目录非 Git 仓库，不存在版本库提交风险，将依赖 Git 隔离机制的违规降级为 WARN（平滑降级原则）
	if !gitCtx.isGitRepo {
		for i := range res.Violations {
			switch res.Violations[i].Category {
			case "私有文件泄露", "二进制文件拦截", "大文件图片拦截", "代码洁癖-临时文件残留":
				res.Violations[i].Level = "WARN"
			}
		}
	}

	return res
}

// auditForbiddenFiles 检查是否存在被追踪或未隔离的私有配置文件
func auditForbiddenFiles(root string, res *AuditResult, gitCtx *gitContext) {
	forbiddenBaseNames := map[string]string{
		".env":       "环境密钥配置，严禁提交至版本库",
		".env.local": "本地密钥配置，严禁提交至版本库",
		"TASK.md":    "AI 任务工作状态草稿，应放入 .git/info/exclude 隔离",
		"MEMORY.md":  "AI 记忆文件，应放入 .git/info/exclude 隔离",
	}

	for baseName, msg := range forbiddenBaseNames {
		p := filepath.Join(root, baseName)
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			tracked := gitCtx.isTracked(baseName)
			ignored := gitCtx.isIgnored(baseName)

			// 若已跟踪入库，或未被本地忽略，则报告违规
			if tracked || !ignored {
				level := "ERROR"
				detailMsg := msg
				if tracked {
					detailMsg = fmt.Sprintf("%s (当前已被 Git 跟踪，随时会被提交)", msg)
				} else {
					detailMsg = fmt.Sprintf("%s (当前未被 .git/info/exclude 忽略，存在泄露风险)", msg)
				}

				res.Violations = append(res.Violations, Violation{
					Level:      level,
					Category:   "私有文件泄露",
					File:       baseName,
					Message:    detailMsg,
					Suggestion: "运行 `agate isolate` 将其加入本地忽略，若已跟踪请执行 `git rm --cached <文件>`",
				})
			}
		}
	}
}

// isCommonIgnoredDir 统一判定第三方依赖包、编译器构建产物与缓存目录，执行秒级剪枝
func isCommonIgnoredDir(dirName string) bool {
	switch strings.ToLower(dirName) {
	case ".git", ".github", ".agent", ".idea", ".vscode",
		"node_modules", "vendor", "venv", ".venv",
		"dist", "build", "out", "target", "release", "bin", "obj",
		"testdata", ".next", ".nuxt", ".cache", ".temp":
		return true
	default:
		return false
	}
}

// auditBinaryAndLargeFiles 扫描非代码大文件与二进制资产
func auditBinaryAndLargeFiles(root string, res *AuditResult, gitCtx *gitContext) {
	forbiddenExts := map[string]string{
		".exe":   "编译可执行二进制",
		".dll":   "动态链接库",
		".so":    "动态链接库",
		".dylib": "动态链接库",
		".zip":   "压缩包文件",
		".tar":   "压缩包归档",
		".gz":    "压缩包文件",
	}

	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		if rel == "." {
			return nil
		}

		// 忽略依赖、构建产物与缓存目录
		if d.IsDir() {
			if isCommonIgnoredDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}

		slashRel := filepath.ToSlash(rel)
		ext := strings.ToLower(filepath.Ext(path))

		// 检查危险后缀
		if desc, ok := forbiddenExts[ext]; ok {
			tracked := gitCtx.isTracked(slashRel)
			ignored := gitCtx.isIgnored(slashRel)

			// 仅当未被 ignore 或是已被 tracked 时报警
			if tracked || !ignored {
				res.Violations = append(res.Violations, Violation{
					Level:      "ERROR",
					Category:   "二进制文件拦截",
					File:       slashRel,
					Message:    fmt.Sprintf("检测到未忽略的%s (%s)", desc, ext),
					Suggestion: "请在 .gitignore 中添加该后缀，并从版本库中删除该文件",
				})
			}
			return nil
		}

		// 图片文件大小检查 (超过 50KB 触发警告或拦截)
		if ext == ".png" || ext == ".jpg" || ext == ".jpeg" || ext == ".gif" {
			tracked := gitCtx.isTracked(slashRel)
			ignored := gitCtx.isIgnored(slashRel)

			if tracked || !ignored {
				if fi, err := d.Info(); err == nil && fi.Size() > 50*1024 {
					res.Violations = append(res.Violations, Violation{
						Level:      "ERROR",
						Category:   "大文件图片拦截",
						File:       slashRel,
						Message:    fmt.Sprintf("图片文件过大 (%d KB > 50 KB)，严禁直接入库", fi.Size()/1024),
						Suggestion: "请压缩图片、使用外部 CDN，或将其移至文档附件目录",
					})
				}
			}
		}

		return nil
	})
}

// auditHardcodedPaths 扫描代码中写死的个人开发机绝对路径
func auditHardcodedPaths(root string, res *AuditResult, gitCtx *gitContext) {
	// 匹配类似 C:\Users\xxx 或 D:\hclaw\ 等典型机器绝对路径
	pathRegex := regexp.MustCompile(`(?i)[a-zA-Z]:[\\/](?:Users|hclaw|code_files|Software|AppData)[\\/][^\s"'` + "`" + `<>]+`)

	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if isCommonIgnoredDir(d.Name()) || d.Name() == "docs" {
				return filepath.SkipDir
			}
			return nil
		}

		rel, _ := filepath.Rel(root, path)
		slashPath := filepath.ToSlash(rel)

		// 若当前文件未被 Git 跟踪且已被 Git 忽略（如本地构建产物），直接跳过
		if !gitCtx.isTracked(slashPath) && gitCtx.isIgnored(slashPath) {
			return nil
		}

		// 仅扫描源码与脚本文件
		ext := strings.ToLower(filepath.Ext(path))
		validExts := map[string]bool{
			".go": true, ".cmd": true, ".bat": true, ".sh": true,
			".py": true, ".js": true, ".ts": true, ".yaml": true, ".yml": true,
		}
		if !validExts[ext] {
			return nil
		}

		// 豁免当前 guard 包自身的规则定义源码与单元测试文件
		if strings.HasPrefix(slashPath, "pkg/guard/") || strings.HasSuffix(slashPath, "_test.go") {
			return nil
		}

		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer f.Close()

		scanner := newSafeScanner(f)
		lineNum := 0
		for scanner.Scan() {
			lineNum++
			line := scanner.Text()

			// 忽略注释中的文档与路径示例（以 //、#、/*、* 开头，或包含 <YourUser>、示例 等）
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "#") ||
				strings.HasPrefix(trimmed, "/*") || strings.HasPrefix(trimmed, "*") ||
				strings.Contains(line, "<YourUser>") || strings.Contains(line, "示例") {
				continue
			}

			if matches := pathRegex.FindString(line); matches != "" {
				res.Violations = append(res.Violations, Violation{
					Level:       "ERROR",
					Category:    "机器绝对路径泄露",
					File:        slashPath,
					LineNumber:  lineNum,
					LineContent: line,
					Message:     fmt.Sprintf("代码中检测到写死的个人本地磁盘路径: %s", matches),
					Suggestion:  "请改用相对路径、环境变量探测（如 os.UserHomeDir()、os.Getenv）",
				})
			}
		}

		return nil
	})
}

// auditVendorCleanliness 检查 vendor 目录是否混入了上游第三方库的工作流和文档资产
func auditVendorCleanliness(root string, res *AuditResult) {
	vendorDir := filepath.Join(root, "vendor")
	if fi, err := os.Stat(vendorDir); err != nil || !fi.IsDir() {
		return
	}

	_ = filepath.WalkDir(vendorDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(vendorDir, path)
		slashPath := filepath.ToSlash(rel)

		// 检查是否有上游的 .github 工作流
		if d.IsDir() && d.Name() == ".github" {
			res.Violations = append(res.Violations, Violation{
				Level:      "ERROR",
				Category:   "Vendor依赖污染",
				File:       filepath.ToSlash(filepath.Join("vendor", rel)),
				Message:    "vendor 目录混入了上游第三方库的 .github 工作流文件",
				Suggestion: "请运行 `go mod tidy && go mod vendor` 自动净化上游非代码文件",
			})
			return filepath.SkipDir
		}

		// 检查是否有上游的静态网站或大图片
		if !d.IsDir() {
			ext := strings.ToLower(filepath.Ext(path))
			if ext == ".png" || ext == ".jpg" || ext == ".jpeg" || ext == ".gif" {
				res.Violations = append(res.Violations, Violation{
					Level:      "ERROR",
					Category:   "Vendor依赖污染",
					File:       filepath.ToSlash(filepath.Join("vendor", rel)),
					Message:    fmt.Sprintf("vendor 依赖包中存在非代码图片资产 (%s)", d.Name()),
					Suggestion: "请剔除 vendor 中的非必要多媒体资产或运行 `go mod vendor` 重新精简",
				})
			}
			if (strings.Contains(slashPath, "/site/") || strings.Contains(slashPath, "/doc/")) && ext == ".md" {
				res.Violations = append(res.Violations, Violation{
					Level:      "WARN",
					Category:   "Vendor依赖冗余",
					File:       filepath.ToSlash(filepath.Join("vendor", rel)),
					Message:    "vendor 中存在上游开源库自带的网页或文档目录",
					Suggestion: "运行 `go mod vendor` 重新瘦身依赖包",
				})
			}
		}

		return nil
	})
}

// auditTemporaryAndScratchFiles 检查未隔离的临时文件与草稿测试文件
func auditTemporaryAndScratchFiles(root string, res *AuditResult, gitCtx *gitContext) {
	tempExts := map[string]string{
		".tmp":  "临时生成文件",
		".temp": "临时生成文件",
		".bak":  "临时备份文件",
		".swp":  "编辑器交换临时文件",
		".orig": "合并冲突备份文件",
	}

	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if isCommonIgnoredDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}

		rel, _ := filepath.Rel(root, path)
		slashRel := filepath.ToSlash(rel)
		ext := strings.ToLower(filepath.Ext(path))

		if desc, ok := tempExts[ext]; ok {
			tracked := gitCtx.isTracked(slashRel)
			ignored := gitCtx.isIgnored(slashRel)
			if tracked || !ignored {
				res.Violations = append(res.Violations, Violation{
					Level:      "ERROR",
					Category:   "代码洁癖-临时文件残留",
					File:       slashRel,
					Message:    fmt.Sprintf("检测到未隔离的%s: %s", desc, d.Name()),
					Suggestion: "请物理删除该临时文件，或将其添加至 .git/info/exclude 隐形隔离",
				})
			}
		}
		return nil
	})
}

// auditContentHygiene 扫描源码中的 Git 冲突标记、调试断点与伪代码占位符
func auditContentHygiene(root string, res *AuditResult, gitCtx *gitContext) {
	conflictRegex := regexp.MustCompile(`^(<{7}|={7}|>{7})(\s|$)`)
	debuggerRegex := regexp.MustCompile(`(?:^debugger(?:\s*;)?$|breakpoint\(\)|pdb\.set_trace\(\))`)
	lazyRegex := regexp.MustCompile(`(?i)(?://|/\*|#)\s*(?:\.{2,}|…)\s*(?:保持.*不变|其余.*不变|原有.*不变|代码.*不变|现有.*不变|rest of code unchanged)`)

	validExts := map[string]bool{
		".go": true, ".js": true, ".ts": true, ".jsx": true, ".tsx": true,
		".py": true, ".java": true, ".vue": true, ".html": true, ".sh": true,
		".cmd": true, ".bat": true, ".rb": true, ".php": true, ".rs": true,
		".c": true, ".cpp": true, ".h": true,
	}

	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		slashPath := filepath.ToSlash(rel)

		if d.IsDir() {
			if isCommonIgnoredDir(d.Name()) || d.Name() == "docs" || slashPath == "internal/templates" || strings.HasPrefix(slashPath, "internal/templates/") {
				return filepath.SkipDir
			}
			return nil
		}

		// 若当前文件未被 Git 跟踪且已被 Git 忽略（如构建产物 release/builder-debug.yml 等），直接跳过
		if !gitCtx.isTracked(slashPath) && gitCtx.isIgnored(slashPath) {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		if !validExts[ext] {
			return nil
		}

		// 豁免 guard 包自身与单测中的反例定义与单元测试文件
		if strings.HasPrefix(slashPath, "pkg/guard/") || strings.HasSuffix(slashPath, "_test.go") {
			return nil
		}

		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer f.Close()

		scanner := newSafeScanner(f)
		lineNum := 0
		for scanner.Scan() {
			lineNum++
			line := scanner.Text()
			trimmed := strings.TrimSpace(line)

			// 忽略单行注释（以 //、#、/*、* 开头），避免注释中提及 debugger 误报
			isComment := strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "#") ||
				strings.HasPrefix(trimmed, "/*") || strings.HasPrefix(trimmed, "*")

			if conflictRegex.MatchString(trimmed) {
				res.Violations = append(res.Violations, Violation{
					Level:       "ERROR",
					Category:    "代码洁癖-Git冲突残留",
					File:        slashPath,
					LineNumber:  lineNum,
					LineContent: line,
					Message:     "检测到未解决的 Git 冲突标记",
					Suggestion:  "请人工核对并消除 Git 冲突标记",
				})
			} else if !isComment && debuggerRegex.MatchString(trimmed) {
				res.Violations = append(res.Violations, Violation{
					Level:       "ERROR",
					Category:    "代码洁癖-调试断点残留",
					File:        slashPath,
					LineNumber:  lineNum,
					LineContent: line,
					Message:     "检测到残留的调试断点代码",
					Suggestion:  "请清理 debugger 或 breakpoint 等临时排错语句",
				})
			} else if lazyRegex.MatchString(line) {
				res.Violations = append(res.Violations, Violation{
					Level:       "ERROR",
					Category:    "代码洁癖-伪代码占位符",
					File:        slashPath,
					LineNumber:  lineNum,
					LineContent: line,
					Message:     "检测到未落地的伪代码占位符（防伪代码红线）",
					Suggestion:  "代码必须完整可运行，请补齐真实完整实现",
				})
			}
		}
		return nil
	})
}

// auditArchitectureMaps 检查 3-Hop 寻路地图是否完整（若工程已挂载护栏，双地图必须成对完备）
func auditArchitectureMaps(root string, res *AuditResult) {
	agentsPath := filepath.Join(root, "AGENTS.md")
	contextPath := filepath.Join(root, "contexts", "context.md")

	hasAgents := false
	if fi, err := os.Stat(agentsPath); err == nil && !fi.IsDir() && fi.Size() > 0 {
		hasAgents = true
	}

	hasContext := false
	if fi, err := os.Stat(contextPath); err == nil && !fi.IsDir() && fi.Size() > 0 {
		hasContext = true
	}

	// 若工程未挂载 agate 护栏且两个地图均不存在，不做强制拦截（平滑兼容普通工程）
	if !hasAgents && !hasContext {
		return
	}

	if hasAgents && !hasContext {
		res.Violations = append(res.Violations, Violation{
			Level:      "ERROR",
			Category:   "架构地图缺失",
			File:       filepath.Join("contexts", "context.md"),
			Message:    "检测到 AGENTS.md 但缺失 contexts/context.md 技术契约地图，3-Hop 寻路链条断裂",
			Suggestion: "执行 `agate init` 补齐 contexts/context.md 或手动创建",
		})
	} else if !hasAgents && hasContext {
		res.Violations = append(res.Violations, Violation{
			Level:      "ERROR",
			Category:   "架构地图缺失",
			File:       "AGENTS.md",
			Message:    "检测到 contexts/context.md 但缺失 AGENTS.md 模块地图，3-Hop 寻路链条断裂",
			Suggestion: "执行 `agate init` 补齐 AGENTS.md 架构地图或手动创建",
		})
	}
}

var (
	stagedPathRegex     = regexp.MustCompile(`(?i)[a-zA-Z]:[\\/](?:Users|hclaw|code_files|Software|AppData)[\\/][^\s"'` + "`" + `<>]+`)
	stagedConflictRegex = regexp.MustCompile(`^(<{7}|={7}|>{7})(\s|$)`)
	stagedDebuggerRegex = regexp.MustCompile(`(?:^debugger(?:\s*;)?$|breakpoint\(\)|pdb\.set_trace\(\))`)
	stagedLazyRegex     = regexp.MustCompile(`(?i)(?://|/\*|#)\s*(?:\.{2,}|…)\s*(?:保持.*不变|其余.*不变|原有.*不变|代码.*不变|现有.*不变|rest of code unchanged)`)

	stagedForbiddenExts = map[string]string{
		".exe":   "编译可执行二进制",
		".dll":   "动态链接库",
		".so":    "动态链接库",
		".dylib": "动态链接库",
		".zip":   "压缩包文件",
		".tar":   "压缩包归档",
		".gz":    "压缩包文件",
	}

	stagedTempExts = map[string]string{
		".tmp":  "临时生成文件",
		".temp": "临时生成文件",
		".bak":  "临时备份文件",
		".swp":  "编辑器交换临时文件",
		".orig": "合并冲突备份文件",
	}

	stagedForbiddenBaseNames = map[string]string{
		".env":       "环境密钥配置，严禁提交至版本库",
		".env.local": "本地密钥配置，严禁提交至版本库",
		"TASK.md":    "AI 任务工作状态草稿，应放入 .git/info/exclude 隔离",
		"MEMORY.md":  "AI 记忆文件，应放入 .git/info/exclude 隔离",
	}

	stagedValidCodeExts = map[string]bool{
		".go": true, ".js": true, ".ts": true, ".jsx": true, ".tsx": true,
		".py": true, ".java": true, ".vue": true, ".html": true, ".sh": true,
		".cmd": true, ".bat": true, ".rb": true, ".php": true, ".rs": true,
		".c": true, ".cpp": true, ".h": true, ".yaml": true, ".yml": true,
	}
)

// RunStagedAudit 针对 Git 暂存区 (Index) 待提交文件执行定向安全与代码洁癖审查
func RunStagedAudit(root string, stagedFiles []string) *AuditResult {
	res := &AuditResult{}
	if len(stagedFiles) == 0 {
		return res
	}

	for _, stagedFile := range stagedFiles {
		slashPath := filepath.ToSlash(stagedFile)
		baseName := filepath.Base(slashPath)
		ext := strings.ToLower(filepath.Ext(slashPath))

		// 1. 检查私有文件泄露
		if msg, ok := stagedForbiddenBaseNames[baseName]; ok {
			res.Violations = append(res.Violations, Violation{
				Level:      "ERROR",
				Category:   "私有文件泄露",
				File:       slashPath,
				Message:    fmt.Sprintf("%s (当前位于暂存区，一旦提交将泄露至版本库)", msg),
				Suggestion: fmt.Sprintf("执行 `git rm --cached %s` 移出暂存区，并运行 `agate isolate` 加入隐形隔离", slashPath),
			})
			continue
		}

		// 2. 检查危险二进制与归档大文件
		if desc, ok := stagedForbiddenExts[ext]; ok {
			res.Violations = append(res.Violations, Violation{
				Level:      "ERROR",
				Category:   "二进制文件拦截",
				File:       slashPath,
				Message:    fmt.Sprintf("检测到待提交的%s (%s)", desc, ext),
				Suggestion: fmt.Sprintf("执行 `git rm --cached %s` 并将其添加至 .gitignore", slashPath),
			})
			continue
		}

		// 3. 检查临时残留文件
		if desc, ok := stagedTempExts[ext]; ok {
			res.Violations = append(res.Violations, Violation{
				Level:      "ERROR",
				Category:   "代码洁癖-临时文件残留",
				File:       slashPath,
				Message:    fmt.Sprintf("检测到待提交的%s: %s", desc, baseName),
				Suggestion: fmt.Sprintf("执行 `git rm --cached %s` 并清理本地临时文件", slashPath),
			})
			continue
		}

		// 4. 图片文件大小检查 (> 50KB 阻断)
		if ext == ".png" || ext == ".jpg" || ext == ".jpeg" || ext == ".gif" {
			size := getStagedFileSize(root, slashPath)
			if size > 50*1024 {
				res.Violations = append(res.Violations, Violation{
					Level:      "ERROR",
					Category:   "大文件图片拦截",
					File:       slashPath,
					Message:    fmt.Sprintf("图片文件过大 (%d KB > 50 KB)，严禁直接入库", size/1024),
					Suggestion: "请压缩图片、使用外部 CDN，或将其移至文档附件目录",
				})
			}
			continue
		}

		// 5. 源码内容级洁癖检查
		if !stagedValidCodeExts[ext] {
			continue
		}

		// 忽略 docs 或 internal/templates 等豁免目录
		if strings.HasPrefix(slashPath, "docs/") || slashPath == "docs" ||
			strings.HasPrefix(slashPath, "internal/templates/") || slashPath == "internal/templates" {
			continue
		}

		// 豁免 guard 自身的测试与规则定义
		if strings.HasPrefix(slashPath, "pkg/guard/") || strings.HasSuffix(slashPath, "_test.go") {
			continue
		}

		// 读取暂存区文件的真实内容 (优先通过 git show :path 读取 Index 内容)
		lines, err := readStagedFileLines(root, slashPath)
		if err == nil {
			auditStagedLines(slashPath, lines, res)
		}
	}

	// 6. 检查 3-Hop 双地图完备性
	auditArchitectureMaps(root, res)

	return res
}

func getStagedFileSize(root, slashPath string) int64 {
	cmd := exec.Command("git", "cat-file", "-s", ":"+slashPath)
	cmd.Dir = root
	if out, err := cmd.Output(); err == nil {
		var size int64
		if _, scanErr := fmt.Sscanf(strings.TrimSpace(string(out)), "%d", &size); scanErr == nil {
			return size
		}
	}
	if fi, err := os.Stat(filepath.Join(root, filepath.FromSlash(slashPath))); err == nil {
		return fi.Size()
	}
	return 0
}

func readStagedFileLines(root, slashPath string) ([]string, error) {
	cmd := exec.Command("git", "show", ":"+slashPath)
	cmd.Dir = root
	out, err := cmd.Output()
	if err == nil {
		var lines []string
		scanner := newSafeScanner(strings.NewReader(string(out)))
		for scanner.Scan() {
			lines = append(lines, scanner.Text())
		}
		return lines, nil
	}

	// 回退从磁盘读取
	f, err := os.Open(filepath.Join(root, filepath.FromSlash(slashPath)))
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var lines []string
	scanner := newSafeScanner(f)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines, nil
}

func auditStagedLines(slashPath string, lines []string, res *AuditResult) {
	for lineIdx, line := range lines {
		actualLine := lineIdx + 1
		trimmed := strings.TrimSpace(line)

		// Git 冲突标记
		if stagedConflictRegex.MatchString(trimmed) {
			res.Violations = append(res.Violations, Violation{
				Level:       "ERROR",
				Category:    "代码洁癖-Git冲突残留",
				File:        slashPath,
				LineNumber:  actualLine,
				LineContent: line,
				Message:     "检测到未解决的 Git 冲突标记",
				Suggestion:  "请人工核对并消除 Git 冲突标记",
			})
		}

		isComment := strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "#") ||
			strings.HasPrefix(trimmed, "/*") || strings.HasPrefix(trimmed, "*")

		// 调试断点
		if !isComment && stagedDebuggerRegex.MatchString(trimmed) {
			res.Violations = append(res.Violations, Violation{
				Level:       "ERROR",
				Category:    "代码洁癖-调试断点残留",
				File:        slashPath,
				LineNumber:  actualLine,
				LineContent: line,
				Message:     "检测到残留的调试断点代码",
				Suggestion:  "请清理 debugger 或 breakpoint 等临时排错语句",
			})
		}

		// 伪代码占位符
		if stagedLazyRegex.MatchString(line) {
			res.Violations = append(res.Violations, Violation{
				Level:       "ERROR",
				Category:    "代码洁癖-伪代码占位符",
				File:        slashPath,
				LineNumber:  actualLine,
				LineContent: line,
				Message:     "检测到未落地的伪代码占位符（防伪代码红线）",
				Suggestion:  "代码必须完整可运行，请补齐真实完整实现",
			})
		}

		// 机器绝对路径
		if !isComment && !strings.Contains(line, "<YourUser>") && !strings.Contains(line, "示例") {
			if matches := stagedPathRegex.FindString(line); matches != "" {
				res.Violations = append(res.Violations, Violation{
					Level:       "ERROR",
					Category:    "机器绝对路径泄露",
					File:        slashPath,
					LineNumber:  actualLine,
					LineContent: line,
					Message:     fmt.Sprintf("代码中检测到写死的个人本地磁盘路径: %s", matches),
					Suggestion:  "请改用相对路径、环境变量探测（如 os.UserHomeDir()、os.Getenv）",
				})
			}
		}
	}
}
