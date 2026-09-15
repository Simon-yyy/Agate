package guard

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

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

// RunPreflightAudit 执行全套工程预检审计
func RunPreflightAudit(root string) *AuditResult {
	res := &AuditResult{}

	// 1. 检查私有文件泄露与未隔离状态
	auditForbiddenFiles(root, res)

	// 2. 检查大文件与可疑二进制资产
	auditBinaryAndLargeFiles(root, res)

	// 3. 检查代码中的个人机器绝对路径硬编码
	auditHardcodedPaths(root, res)

	// 4. 检查临时残留文件与调试草稿（代码洁癖）
	auditTemporaryAndScratchFiles(root, res)

	// 5. 检查源码内容级洁癖（冲突标记、调试断点、伪代码占位符）
	auditContentHygiene(root, res)

	// 6. 检查 vendor 目录纯净度
	auditVendorCleanliness(root, res)

	return res
}

// auditForbiddenFiles 检查是否存在被追踪或遗留的私有配置文件
// isGitIgnored 检查文件是否已被 .gitignore 或 .git/info/exclude 忽略
func isGitIgnored(root, relPath string) bool {
	cmd := exec.Command("git", "check-ignore", "-q", relPath)
	cmd.Dir = root
	return cmd.Run() == nil
}

// isGitTracked 检查文件是否已经被 git 跟踪
func isGitTracked(root, relPath string) bool {
	cmd := exec.Command("git", "ls-files", "--error-unmatch", relPath)
	cmd.Dir = root
	return cmd.Run() == nil
}

// auditForbiddenFiles 检查是否存在被追踪或未隔离的私有配置文件
func auditForbiddenFiles(root string, res *AuditResult) {
	forbiddenBaseNames := map[string]string{
		".env":       "环境密钥配置，严禁提交至版本库",
		".env.local": "本地密钥配置，严禁提交至版本库",
		"TASK.md":    "AI 任务工作状态草稿，应放入 .git/info/exclude 隔离",
		"MEMORY.md":  "AI 记忆文件，应放入 .git/info/exclude 隔离",
	}

	for baseName, msg := range forbiddenBaseNames {
		p := filepath.Join(root, baseName)
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			tracked := isGitTracked(root, baseName)
			ignored := isGitIgnored(root, baseName)

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

// auditBinaryAndLargeFiles 扫描非代码大文件与二进制资产
func auditBinaryAndLargeFiles(root string, res *AuditResult) {
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

		// 忽略 .git 与 vendor（vendor 单独由 auditVendorCleanliness 检查）
		if d.IsDir() {
			if d.Name() == ".git" || d.Name() == "vendor" || d.Name() == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}

		slashRel := filepath.ToSlash(rel)
		ext := strings.ToLower(filepath.Ext(path))

		// 检查危险后缀
		if desc, ok := forbiddenExts[ext]; ok {
			tracked := isGitTracked(root, slashRel)
			ignored := isGitIgnored(root, slashRel)

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
			tracked := isGitTracked(root, slashRel)
			ignored := isGitIgnored(root, slashRel)

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
func auditHardcodedPaths(root string, res *AuditResult) {
	// 匹配类似 C:\Users\xxx 或 D:\hclaw\ 等典型机器绝对路径
	pathRegex := regexp.MustCompile(`(?i)[a-zA-Z]:[\\/](?:Users|hclaw|code_files|Software|AppData)[\\/][^\s"'` + "`" + `<>]+`)

	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			if d.Name() == ".git" || d.Name() == "vendor" || d.Name() == "docs" || d.Name() == ".agent" {
				return filepath.SkipDir
			}
			return nil
		}

		rel, _ := filepath.Rel(root, path)
		slashPath := filepath.ToSlash(rel)

		// 仅扫描源码与脚本文件
		ext := strings.ToLower(filepath.Ext(path))
		validExts := map[string]bool{
			".go": true, ".cmd": true, ".bat": true, ".sh": true,
			".py": true, ".js": true, ".ts": true, ".yaml": true, ".yml": true,
		}
		if !validExts[ext] {
			return nil
		}

		// 豁免当前 guard 包自身的规则定义源码
		if strings.HasPrefix(slashPath, "pkg/guard/") {
			return nil
		}

		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer f.Close()

		scanner := bufio.NewScanner(f)
		lineNum := 0
		for scanner.Scan() {
			lineNum++
			line := scanner.Text()

			// 忽略注释中的文档示例（包含 <YourUser> 等）
			if strings.Contains(line, "<YourUser>") || strings.Contains(line, "示例") {
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
			if strings.Contains(slashPath, "/site/") || strings.Contains(slashPath, "/doc/") && ext == ".md" {
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
func auditTemporaryAndScratchFiles(root string, res *AuditResult) {
	tempExts := map[string]string{
		".tmp":  "临时生成文件",
		".temp": "临时生成文件",
		".bak":  "临时备份文件",
		".swp":  "编辑器交换临时文件",
		".orig": "合并冲突备份文件",
	}

	tempPrefixes := []string{"temp_", "tmp_", "scratch_", "test_scratch_"}

	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if d.Name() == ".git" || d.Name() == "vendor" || d.Name() == "node_modules" || d.Name() == ".agent" {
				return filepath.SkipDir
			}
			return nil
		}

		rel, _ := filepath.Rel(root, path)
		slashRel := filepath.ToSlash(rel)
		fileName := strings.ToLower(d.Name())
		ext := strings.ToLower(filepath.Ext(path))

		isTemp := false
		reason := ""
		if desc, ok := tempExts[ext]; ok {
			isTemp = true
			reason = desc
		} else {
			for _, pfx := range tempPrefixes {
				if strings.HasPrefix(fileName, pfx) {
					isTemp = true
					reason = "临时/草稿脚本文件"
					break
				}
			}
		}

		if isTemp {
			tracked := isGitTracked(root, slashRel)
			ignored := isGitIgnored(root, slashRel)
			if tracked || !ignored {
				res.Violations = append(res.Violations, Violation{
					Level:      "ERROR",
					Category:   "代码洁癖-临时文件残留",
					File:       slashRel,
					Message:    fmt.Sprintf("检测到未隔离的%s: %s", reason, d.Name()),
					Suggestion: "请物理删除该临时文件，或将其添加至 .git/info/exclude 隐形隔离",
				})
			}
		}
		return nil
	})
}

// auditContentHygiene 扫描源码中的 Git 冲突标记、调试断点与伪代码占位符
func auditContentHygiene(root string, res *AuditResult) {
	conflictRegex := regexp.MustCompile(`^(<{7}|={7}|>{7})(\s|$)`)
	debuggerRegex := regexp.MustCompile(`\b(?:debugger|breakpoint\(\)|pdb\.set_trace\(\))`)
	lazyRegex := regexp.MustCompile(`(?:保持不变|保持原有逻辑不变|rest of code unchanged)`)

	validExts := map[string]bool{
		".go": true, ".js": true, ".ts": true, ".jsx": true, ".tsx": true,
		".py": true, ".java": true, ".vue": true, ".html": true, ".sh": true,
		".cmd": true, ".bat": true, ".rb": true, ".php": true, ".rs": true,
		".c": true, ".cpp": true, ".h": true,
	}

	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			if d.Name() == ".git" || d.Name() == "vendor" || d.Name() == "node_modules" || d.Name() == "docs" || d.Name() == "internal" || d.Name() == ".agent" {
				return filepath.SkipDir
			}
			return nil
		}

		rel, _ := filepath.Rel(root, path)
		slashPath := filepath.ToSlash(rel)
		ext := strings.ToLower(filepath.Ext(path))

		if !validExts[ext] {
			return nil
		}

		// 豁免 guard 包自身与单测中的反例定义
		if strings.HasPrefix(slashPath, "pkg/guard/") {
			return nil
		}

		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer f.Close()

		scanner := bufio.NewScanner(f)
		lineNum := 0
		for scanner.Scan() {
			lineNum++
			line := scanner.Text()
			trimmed := strings.TrimSpace(line)

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
			} else if debuggerRegex.MatchString(line) {
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
