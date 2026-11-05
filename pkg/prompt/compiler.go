package prompt

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"agate/pkg/git"
	"agate/pkg/relay"
)

// AnchorConfidence 标识物理目标锚定的置信度等级
const (
	ConfidenceExplicit      = "explicit"       // 指令或 --scope 显式命中物理文件/模块
	ConfidenceInferredDirty = "inferred_dirty" // 指令未指明路径，仅根据 Git 活跃脏文件推测兜底
	ConfidenceUnresolved    = "unresolved"     // 未命中任何路径且无可关联的活跃脏文件
)

// CompileOptions 提示词编译输入参数
type CompileOptions struct {
	RootDir     string   // 仓库根目录，为空时自动向上寻径
	Instruction string   // 用户输入的自然语言指令
	Scopes      []string // 显式指定或纠偏的目标文件/模块路径（优先于自动探测）
	IgnoreDirty bool     // 是否忽略 Git 活跃脏文件兜底关联（防无关脏文件干扰）
	JSONOutput  bool     // 是否输出 JSON 格式
	HookMode    bool     // 是否为 IDE Hook 模式
}

// CompileResult 提示词编译结果模型
type CompileResult struct {
	RawInstruction    string   `json:"raw_instruction"`
	AnchorConfidence  string   `json:"anchor_confidence"`
	TargetDescription string   `json:"target_description"`
	ExplicitScopes    []string `json:"explicit_scopes,omitempty"`
	IgnoreDirty       bool     `json:"ignore_dirty,omitempty"`
	MatchedModules    []string `json:"matched_modules,omitempty"`
	MatchedFiles      []string `json:"matched_files,omitempty"`
	GitBranch         string   `json:"git_branch,omitempty"`
	GitCommit         string   `json:"git_commit,omitempty"`
	DirtyFiles        []string `json:"dirty_files,omitempty"`
	VerifyStreak      int      `json:"verify_streak"`
	TaskActive        bool     `json:"task_active"`
	TaskTitle         string   `json:"task_title,omitempty"`
	TaskStatus        string   `json:"task_status,omitempty"`
	TaskRole          string   `json:"task_role,omitempty"`
	TaskNext          string   `json:"task_next,omitempty"`
	TaskBreakpoint    string   `json:"task_breakpoint,omitempty"`
	TaskNextStep      string   `json:"task_next_step,omitempty"`
	MapExcerpts       []string `json:"map_excerpts,omitempty"`
	MemoryCards       []string `json:"memory_cards,omitempty"`
	CompiledPrompt    string   `json:"compiled_prompt"`
}

// CompilePrompt 执行四源物理现场收集与结构化 Prompt 编译
func CompilePrompt(opts CompileOptions) (*CompileResult, error) {
	instruction := strings.TrimSpace(opts.Instruction)
	if instruction == "" {
		return nil, fmt.Errorf("自然语言指令不能为空")
	}

	root := opts.RootDir
	if root == "" {
		r, err := git.GetRepoRoot()
		if err == nil && r != "" {
			root = r
		} else {
			root = "."
		}
	}
	root, _ = filepath.Abs(root)

	res := &CompileResult{
		RawInstruction: instruction,
		IgnoreDirty:    opts.IgnoreDirty,
		MatchedModules: make([]string, 0),
		MatchedFiles:   make([]string, 0),
		DirtyFiles:     make([]string, 0),
		MapExcerpts:    make([]string, 0),
		MemoryCards:    make([]string, 0),
	}

	// 1. 采集 Git 活跃现场
	branch, commit, dirty, streak := collectGitSnapshot(root)
	res.GitBranch = branch
	res.GitCommit = commit
	res.DirtyFiles = dirty
	res.VerifyStreak = streak

	// 2. 物理实体与模块碰撞（支持 --scope 显式覆写纠偏，以及 --no-dirty 隔离无关脏文件）
	var matchedMods, matchedFiles []string
	cleanScopes, scopeMods, scopeFiles, scopeErr := resolveExplicitScopes(root, opts.Scopes)
	if scopeErr != nil {
		return nil, scopeErr
	}
	if len(cleanScopes) > 0 {
		res.ExplicitScopes = cleanScopes
		matchedMods = scopeMods
		matchedFiles = scopeFiles
	} else {
		var dirtyForDetect []string
		if !opts.IgnoreDirty {
			dirtyForDetect = dirty
		}
		matchedMods, matchedFiles = detectMatchedEntities(instruction, root, dirtyForDetect)
	}
	sort.Strings(matchedMods)
	sort.Strings(matchedFiles)
	res.MatchedModules = matchedMods
	res.MatchedFiles = matchedFiles

	hasExplicitTarget := len(matchedFiles) > 0 || len(matchedMods) > 0

	// 目标描述与锚定置信度推导
	if hasExplicitTarget {
		res.AnchorConfidence = ConfidenceExplicit
		var parts []string
		if len(res.ExplicitScopes) > 0 {
			parts = append(parts, fmt.Sprintf("[显式纠偏锚定] 锁定范围: %s", strings.Join(res.ExplicitScopes, ", ")))
		}
		if len(matchedFiles) > 0 {
			parts = append(parts, fmt.Sprintf("明确命中文件: %s", strings.Join(matchedFiles, ", ")))
		}
		if len(matchedMods) > 0 {
			parts = append(parts, fmt.Sprintf("关联模块: %s", strings.Join(matchedMods, ", ")))
		}
		res.TargetDescription = strings.Join(parts, " | ")
	} else if !opts.IgnoreDirty && len(dirty) > 0 {
		res.AnchorConfidence = ConfidenceInferredDirty
		res.TargetDescription = fmt.Sprintf("[推测锚定-待核验] 指令未显式指明路径，暂关联当前 Git 活跃工作区 (%d 项脏文件: %s)；⚠️ 若原始指令语义与上述脏文件无关，严禁强行采信，请对照 MAP.md 重新定位或调用 `agate prompt \"<指令>\" -s <真实模块> --no-dirty` 纠偏", len(dirty), strings.Join(truncateList(dirty, 8), ", "))
	} else {
		res.AnchorConfidence = ConfidenceUnresolved
		if opts.IgnoreDirty {
			res.TargetDescription = "已启用 --no-dirty 忽略工作区脏文件且未命中具体路径；请宿主模型结合上下文与 3-Hop 双地图澄清目标"
		} else {
			res.TargetDescription = "指令未指定具体文件且工作区干净；请宿主模型结合上下文与 3-Hop 双地图澄清目标"
		}
	}

	// 3. MAP.md 模块动态切片（明确指定目标时仅切目标模块，未指定且未禁用脏文件时才用脏文件兜底）
	var fallbackDirtyForMap []string
	if !hasExplicitTarget && !opts.IgnoreDirty {
		fallbackDirtyForMap = dirty
	}
	res.MapExcerpts = extractMapExcerpts(root, matchedMods, fallbackDirtyForMap)

	// 4. TASK.md 在途断点采集（严格排除 [DONE] 终态任务）
	taskFile := filepath.Join(root, "TASK.md")
	if data, readErr := os.ReadFile(taskFile); readErr == nil {
		if manifest, err := relay.ParseTaskBoard(string(data)); err == nil && manifest != nil {
			if manifest.Status != relay.StatusDone && manifest.Status != "" {
				res.TaskActive = true
				res.TaskTitle = manifest.Title
				res.TaskStatus = string(manifest.Status)
				res.TaskRole = manifest.CurrentRole
				res.TaskNext = manifest.NextRole

				// 提取接力四要素中的在途断点与下一步建议
				quartet := relay.ExtractHandoverQuartet(manifest.Body)
				res.TaskBreakpoint = strings.TrimSpace(quartet.InProgress)
				res.TaskNextStep = strings.TrimSpace(quartet.NextAction)
			}
		}
	}

	// 5. MEMORY.md 按需准入（明确目标时仅匹配目标，模糊指令且未禁用脏文件时匹配活跃脏文件）
	var targets []string
	if hasExplicitTarget {
		targets = append(targets, matchedFiles...)
		targets = append(targets, matchedMods...)
	} else if !opts.IgnoreDirty {
		targets = append(targets, dirty...)
	}
	res.MemoryCards = extractMemoryCards(root, targets)

	// 6. 合成结构化 Prompt
	res.CompiledPrompt = buildCompiledPrompt(res, opts.HookMode)

	return res, nil
}

// collectGitSnapshot 读取分支、commit、dirty 列表与 verify_streak
func collectGitSnapshot(root string) (string, string, []string, int) {
	branch, commit, _ := git.GetGitMetadata(root)

	var dirty []string
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = root
	if out, err := cmd.Output(); err == nil {
		scanner := bufio.NewScanner(bytes.NewReader(out))
		for scanner.Scan() {
			rawLine := scanner.Text()
			if len(rawLine) < 4 {
				continue
			}
			filePath := strings.TrimSpace(rawLine[3:])
			if idx := strings.Index(filePath, " -> "); idx != -1 {
				filePath = filePath[idx+4:]
			}
			filePath = strings.Trim(filePath, "\"")
			if filePath != "" {
				dirty = append(dirty, filepath.ToSlash(filePath))
			}
		}
	}
	sort.Strings(dirty)

	streak := 0
	streakFile := filepath.Join(root, ".ai-memory", ".verify_streak")
	if data, err := os.ReadFile(streakFile); err == nil {
		if s, parseErr := strconv.Atoi(strings.TrimSpace(string(data))); parseErr == nil {
			streak = s
		}
	}

	return branch, commit, dirty, streak
}

// resolveExplicitScopes 校验并解析用户或 Agent 通过 --scope 显式指定的纠偏目标范围
func resolveExplicitScopes(root string, rawScopes []string) ([]string, []string, []string, error) {
	if len(rawScopes) == 0 {
		return nil, nil, nil, nil
	}

	var cleanScopes []string
	seenScopes := make(map[string]bool)
	modsMap := make(map[string]bool)
	filesMap := make(map[string]bool)

	for _, raw := range rawScopes {
		for _, part := range strings.Split(raw, ",") {
			token := strings.TrimSpace(part)
			if token == "" {
				return nil, nil, nil, fmt.Errorf("--scope 参数包含空白路径，请传入合法的仓库内相对路径或模块名")
			}

			cleanToken := strings.Trim(filepath.ToSlash(token), "/")
			if cleanToken == "pkg" || cleanToken == "internal" {
				return nil, nil, nil, fmt.Errorf("--scope 不支持单独指定顶层容器目录 %q，请指定具体子模块路径（如 pkg/prompt 或 internal/templates）", cleanToken)
			}

			isDir, isFile, relPath, ok := inspectRepoCandidate(root, token)
			if !ok {
				// 兼容单独传入顶层模块名（如 cmd, docs, scripts, contexts, test）
				standaloneTopDirs := map[string]bool{"cmd": true, "scripts": true, "docs": true, "contexts": true, "test": true}
				if standaloneTopDirs[cleanToken] {
					isDir = true
					relPath = cleanToken
					ok = true
				}
			}
			if !ok {
				return nil, nil, nil, fmt.Errorf("无效的 --scope 目标路径 %q：仅允许仓库内合法的相对模块或代码路径", token)
			}
			if relPath == "pkg" || relPath == "internal" {
				return nil, nil, nil, fmt.Errorf("--scope 不支持单独指定顶层容器目录 %q，请指定具体子模块路径（如 pkg/prompt 或 internal/templates）", relPath)
			}

			if !seenScopes[relPath] {
				seenScopes[relPath] = true
				cleanScopes = append(cleanScopes, relPath)
			}
			if isDir {
				modsMap[relPath] = true
			}
			if isFile {
				filesMap[relPath] = true
				if dir := filepath.ToSlash(filepath.Dir(relPath)); dir != "." && dir != "/" {
					modsMap[dir] = true
				}
			}
		}
	}

	pruneContainerModules(modsMap)

	var mods []string
	for m := range modsMap {
		mods = append(mods, m)
	}
	sort.Strings(mods)

	var files []string
	for f := range filesMap {
		files = append(files, f)
	}
	sort.Strings(files)

	return cleanScopes, mods, files, nil
}

// detectMatchedEntities 扫描指令中出现的物理文件或目录
func detectMatchedEntities(instruction string, root string, dirty []string) ([]string, []string) {
	matchedModsMap := make(map[string]bool)
	matchedFilesMap := make(map[string]bool)

	instrNorm := filepath.ToSlash(instruction)

	// 1. 尝试碰撞独立顶层模块目录（不含 pkg/internal 容器目录，避免泛化命中全部子包）
	standaloneTopDirs := []string{"cmd", "scripts", "docs", "contexts", "test"}
	for _, dir := range standaloneTopDirs {
		if strings.Contains(instrNorm, dir+"/") || strings.Contains(instrNorm, dir+"\\") ||
			strings.HasPrefix(instrNorm, dir+" ") || strings.HasSuffix(instrNorm, " "+dir) || strings.Contains(instrNorm, " "+dir+" ") {
			matchedModsMap[dir] = true
		}
	}

	// 2. 检查指令中是否直接包含当前脏文件或未跟踪目录
	for _, d := range dirty {
		isDirtyDir := strings.HasSuffix(d, "/")
		cleanDirty := strings.TrimSuffix(d, "/")
		if cleanDirty == "" {
			continue
		}
		if stat, err := os.Stat(filepath.Join(root, filepath.FromSlash(cleanDirty))); err == nil && stat.IsDir() {
			isDirtyDir = true
		}

		base := filepath.Base(cleanDirty)
		if strings.Contains(instrNorm, cleanDirty) || (!isDirtyDir && len(base) > 3 && strings.Contains(instrNorm, base)) {
			if isDirtyDir {
				matchedModsMap[cleanDirty] = true
			} else {
				matchedFilesMap[cleanDirty] = true
				if dir := filepath.ToSlash(filepath.Dir(cleanDirty)); dir != "." && dir != "/" {
					matchedModsMap[dir] = true
				}
			}
		}
	}

	// 3. 扫描指令中的 token 是否匹配工作区实际文件或合法的相对代码路径
	words := strings.Fields(instruction)
	for _, w := range words {
		wClean := strings.Trim(w, "\"`'(),:;<>[]{}")
		if len(wClean) < 2 {
			continue
		}

		isDir, isFile, relPath, ok := inspectRepoCandidate(root, wClean)
		if ok {
			if isDir {
				matchedModsMap[relPath] = true
			}
			if isFile {
				matchedFilesMap[relPath] = true
				if dir := filepath.ToSlash(filepath.Dir(relPath)); dir != "." && dir != "/" {
					matchedModsMap[dir] = true
				}
			}
		}
	}

	// 4. 剔除泛化容器目录 ("pkg", "internal")，若已存在更具体的子模块或作为孤立容器词
	pruneContainerModules(matchedModsMap)

	var mods []string
	for m := range matchedModsMap {
		mods = append(mods, m)
	}
	sort.Strings(mods)

	var files []string
	for f := range matchedFilesMap {
		files = append(files, f)
	}
	sort.Strings(files)

	return mods, files
}

func pruneContainerModules(modsMap map[string]bool) {
	for _, container := range []string{"pkg", "internal"} {
		if !modsMap[container] {
			continue
		}
		for m := range modsMap {
			if strings.HasPrefix(m, container+"/") {
				delete(modsMap, container)
				break
			}
		}
	}
}

// inspectRepoCandidate 校验并提取仓库内合法候选相对路径（防御 URL、路径穿越与非代码中文斜杠）
func inspectRepoCandidate(root, token string) (isDir, isFile bool, relPath string, ok bool) {
	// 1. 排除 URL 协议标识符 (http://, file://, git@)
	if strings.Contains(token, "://") || strings.HasPrefix(token, "git@") {
		return false, false, "", false
	}

	// 2. 排除纯中文词汇斜杠组合（如“输入/输出”、“前端/后端”）
	hasCJK := false
	for _, r := range token {
		if unicode.Is(unicode.Han, r) {
			hasCJK = true
			break
		}
	}
	if hasCJK && (strings.Contains(token, "/") || strings.Contains(token, "\\")) {
		ext := strings.ToLower(filepath.Ext(token))
		if ext == "" || !isCommonSourceExtension(ext) {
			return false, false, "", false
		}
	}

	// 3. 归一化路径，防御越界与绝对路径
	trimmedTrailingSlash := strings.HasSuffix(token, "/") || strings.HasSuffix(token, "\\")
	cleanRel := filepath.ToSlash(filepath.Clean(filepath.FromSlash(token)))
	if cleanRel == "." || filepath.IsAbs(cleanRel) || strings.HasPrefix(cleanRel, "../") || cleanRel == ".." {
		return false, false, "", false
	}
	// Windows 盘符排除 (如 C:/)
	if len(cleanRel) >= 2 && cleanRel[1] == ':' {
		return false, false, "", false
	}

	absPath := filepath.Join(root, filepath.FromSlash(cleanRel))

	// 4. 优先检查磁盘实际存在性
	if stat, err := os.Stat(absPath); err == nil {
		if stat.IsDir() {
			return true, false, cleanRel, true
		}
		return false, true, cleanRel, true
	}

	// 5. 对磁盘尚不存在的路径，若为已知顶层目录下的子目录或带合法源码后缀，准入
	if strings.Contains(cleanRel, "/") {
		ext := strings.ToLower(filepath.Ext(cleanRel))
		topDir := strings.Split(cleanRel, "/")[0]
		knownTopDirs := map[string]bool{"cmd": true, "pkg": true, "internal": true, "scripts": true, "docs": true, "contexts": true, "test": true}
		if trimmedTrailingSlash && knownTopDirs[topDir] {
			return true, false, cleanRel, true
		}
		if isCommonSourceExtension(ext) || knownTopDirs[topDir] {
			if ext == "" && knownTopDirs[topDir] {
				return true, false, cleanRel, true
			}
			return false, true, cleanRel, true
		}
	}

	return false, false, "", false
}

func isCommonSourceExtension(ext string) bool {
	switch ext {
	case ".go", ".md", ".toml", ".yaml", ".yml", ".sh", ".ps1", ".cmd", ".bat",
		".json", ".ts", ".js", ".py", ".java", ".html", ".css", ".tpl":
		return true
	default:
		return false
	}
}

// extractMapExcerpts 针对命中的模块切出 MAP.md 的内容段
func extractMapExcerpts(root string, mods []string, fallbackDirty []string) []string {
	mapPath := filepath.Join(root, "MAP.md")
	content, err := os.ReadFile(mapPath)
	if err != nil {
		return nil
	}

	kwSet := make(map[string]bool)
	for _, m := range mods {
		clean := strings.Trim(filepath.ToSlash(m), "/")
		// 忽略孤立的顶层容器词 "pkg" / "internal"，避免把所有子包全部切出
		if clean == "" || clean == "pkg" || clean == "internal" {
			continue
		}
		kwSet[clean] = true
	}
	for _, d := range fallbackDirty {
		cleanDirty := strings.TrimSuffix(filepath.ToSlash(d), "/")
		dir := cleanDirty
		if filepath.Ext(cleanDirty) != "" {
			dir = filepath.ToSlash(filepath.Dir(cleanDirty))
		}
		if dir != "." && dir != "/" && dir != "pkg" && dir != "internal" {
			kwSet[dir] = true
		}
	}

	var targetKeywords []string
	for kw := range kwSet {
		targetKeywords = append(targetKeywords, kw)
	}
	sort.Strings(targetKeywords)

	lines := strings.Split(string(content), "\n")
	var excerpts []string

	var currentHeading string
	var currentSection strings.Builder
	recording := false

	flushSection := func() {
		if recording && currentSection.Len() > 0 {
			text := strings.TrimSpace(currentSection.String())
			if text != "" {
				excerpts = append(excerpts, text)
			}
		}
		currentSection.Reset()
		recording = false
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "### ") {
			flushSection()
			currentHeading = trimmed
			lowerHeading := strings.ToLower(currentHeading)
			for _, kw := range targetKeywords {
				lowerKW := strings.ToLower(kw)
				if lowerKW != "" && (strings.Contains(lowerHeading, lowerKW+"/") ||
					strings.Contains(lowerHeading, "`"+lowerKW+"`") ||
					strings.Contains(lowerHeading, "("+lowerKW+")")) {
					recording = true
					break
				}
			}
			if recording {
				currentSection.WriteString(currentHeading + "\n")
			}
			continue
		}

		if strings.HasPrefix(trimmed, "## ") {
			flushSection()
			continue
		}

		if recording {
			currentSection.WriteString(line + "\n")
		}
	}
	flushSection()

	if len(excerpts) == 0 {
		var intro strings.Builder
		count := 0
		for _, l := range lines {
			if strings.HasPrefix(strings.TrimSpace(l), "## 二、") {
				break
			}
			intro.WriteString(l + "\n")
			count++
			if count > 20 {
				break
			}
		}
		if s := strings.TrimSpace(intro.String()); s != "" {
			excerpts = append(excerpts, s)
		}
	}

	return excerpts
}

// extractMemoryCards 提取与目标文件/模块匹配的 MEMORY.md 卡片
func extractMemoryCards(root string, targets []string) []string {
	memPath := filepath.Join(root, "MEMORY.md")
	data, err := os.ReadFile(memPath)
	if err != nil {
		return nil
	}

	var matchedCards []string
	cards := strings.Split(string(data), "\n### ")

	for idx, c := range cards {
		if idx == 0 && !strings.HasPrefix(c, "### ") {
			continue
		}
		cardText := c
		if !strings.HasPrefix(cardText, "### ") {
			cardText = "### " + cardText
		}

		scope := extractCardScope(cardText)
		cleanScope := strings.Trim(strings.TrimSpace(scope), "` \t\r\n")

		// 严格防线：缺少生效范围的卡片绝不盲目准入
		if cleanScope == "" {
			continue
		}

		// 显式全局生效卡片准入
		if cleanScope == "all" || cleanScope == "*" || cleanScope == "global" {
			matchedCards = append(matchedCards, strings.TrimSpace(cardText))
			continue
		}

		// 支持多生效范围逗号拆分
		scopeList := strings.Split(cleanScope, ",")
		hit := false

		for _, s := range scopeList {
			prefix := filepath.ToSlash(strings.Trim(strings.TrimSpace(s), "`* /"))
			if prefix == "" {
				continue
			}

			for _, t := range targets {
				if t == "" {
					continue
				}
				normTarget := strings.TrimSuffix(filepath.ToSlash(t), "/")
				// 忽略顶层超宽目录，防止全局误穿透
				if normTarget == "pkg" || normTarget == "cmd" || normTarget == "internal" || normTarget == "docs" {
					continue
				}

				if normTarget == prefix || strings.HasPrefix(normTarget, prefix+"/") || strings.HasPrefix(prefix, normTarget+"/") {
					hit = true
					break
				}
			}
			if hit {
				break
			}
		}

		if hit {
			matchedCards = append(matchedCards, strings.TrimSpace(cardText))
		}
	}

	return matchedCards
}

func extractCardScope(card string) string {
	for _, line := range strings.Split(card, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.Contains(trimmed, "生效范围") {
			// 优先兼容 3 字节中文冒号 "："
			if idx := strings.Index(trimmed, "："); idx != -1 {
				return strings.TrimSpace(trimmed[idx+len("："):])
			}
			if idx := strings.Index(trimmed, ":"); idx != -1 {
				return strings.TrimSpace(trimmed[idx+1:])
			}
		}
	}
	return ""
}

// buildCompiledPrompt 组装结构化 Prompt 文本
func buildCompiledPrompt(res *CompileResult, hookMode bool) string {
	var sb strings.Builder

	if hookMode {
		sb.WriteString("<!-- agate:prompt-guard:start -->\n")
	}

	sb.WriteString("# 【宿主模型指令执行信封 (Agate Prompt Envelope)】\n\n")

	sb.WriteString("## 1. 用户原始指令\n")
	for _, line := range strings.Split(strings.ReplaceAll(res.RawInstruction, "\r\n", "\n"), "\n") {
		sb.WriteString(fmt.Sprintf("> %s\n", line))
	}
	sb.WriteString("\n")

	sb.WriteString("## 2. 物理现场与上下文锚定 (Workspace Grounding)\n")
	sb.WriteString(fmt.Sprintf("- **锚定置信度**: `%s`\n", res.AnchorConfidence))
	sb.WriteString(fmt.Sprintf("- **物理目标**: %s\n", res.TargetDescription))
	if res.GitBranch != "" {
		sb.WriteString(fmt.Sprintf("- **Git 现场**: 分支 `%s` (Commit: `%s`)", res.GitBranch, res.GitCommit))
		if len(res.DirtyFiles) > 0 {
			if res.AnchorConfidence == ConfidenceExplicit {
				sb.WriteString(fmt.Sprintf(" | 存在 %d 项工作区改动 (已精准锁定目标，跳过无关脏文件展开)", len(res.DirtyFiles)))
			} else {
				truncated := truncateList(res.DirtyFiles, 15)
				sb.WriteString(fmt.Sprintf(" | 暂存/在途改动 (%d 项): %s", len(res.DirtyFiles), strings.Join(truncated, ", ")))
				if len(res.DirtyFiles) > len(truncated) {
					sb.WriteString(fmt.Sprintf(" ...等共 %d 项", len(res.DirtyFiles)))
				}
			}
		}
		sb.WriteString("\n")
	}
	if res.VerifyStreak > 0 {
		sb.WriteString(fmt.Sprintf("- ⚠️ **物理熔断状态**: 当前存在未解除的连续验证失败计数: %d 次 (请优先排错并谨慎修改)\n", res.VerifyStreak))
	}

	if res.TaskActive {
		sb.WriteString(fmt.Sprintf("- **TASK.md 看板**: [%s] %s (当前角色: %s | 下一角色: %s)\n",
			res.TaskStatus, res.TaskTitle, res.TaskRole, res.TaskNext))
		if res.TaskBreakpoint != "" {
			sb.WriteString(fmt.Sprintf("  - *在途断点*: %s\n", res.TaskBreakpoint))
		}
		if res.TaskNextStep != "" {
			sb.WriteString(fmt.Sprintf("  - *下一步建议*: %s\n", res.TaskNextStep))
		}
	} else {
		sb.WriteString("- **TASK.md 看板**: 当前无在途活跃接力任务\n")
	}

	if len(res.MapExcerpts) > 0 {
		sb.WriteString("\n### 架构地图参考 (MAP.md 切片):\n")
		for _, ex := range res.MapExcerpts {
			sb.WriteString(ex + "\n\n")
		}
	}

	if len(res.MemoryCards) > 0 {
		sb.WriteString("### 生效记忆约束 (MEMORY.md 准入卡片):\n")
		for _, card := range res.MemoryCards {
			sb.WriteString(card + "\n\n")
		}
	} else {
		sb.WriteString("- **生效记忆约束**: 未检索到针对当期修改范围的特异性生效记忆卡片\n")
	}

	singleLineInstr := strings.Join(strings.Fields(res.RawInstruction), " ")

	sb.WriteString("\n## 3. 宿主模型执行硬门禁与纠偏协议 (Two-Phase Gate & Correction Loop)\n")
	sb.WriteString("处理上述指令时，你必须在**首轮回复首部强制输出以下结构化解码代码块**，未输出即视为门禁违规，严禁直接动手修改代码：\n\n")
	sb.WriteString("```agate-intent\n")
	sb.WriteString("【指令语义解码与隐式需求展开】\n")
	sb.WriteString(fmt.Sprintf("1. 原始指令: %s\n", strconv.Quote(singleLineInstr)))
	if res.AnchorConfidence == ConfidenceExplicit {
		sb.WriteString("2. 物理目标锚定: <指令已明确目标，锁定目标文件/模块，遵循最小影响原则，严禁借机扩大范围>\n")
		sb.WriteString("3. 隐式关联展开: <收敛模式：仅限核对直接调用方签名与对应单测；若无连带影响请填“无（指令边界已明确）”，严禁过度设计>\n")
	} else {
		sb.WriteString("2. 物理目标锚定: <明确拟修改的文件或模块；若信封为 inferred_dirty 且与指令语义不符，须主动剔除无关脏文件并按 MAP.md 修正或向用户反问>\n")
		sb.WriteString("3. 隐式关联展开: <主动补齐调用方兼容性、入参校验、异常分支、配置与契约同步等隐含工程项>\n")
	}
	sb.WriteString("4. 动作分级: <[A 级：只读探查] | [C 级：核心修改-需出 Plan 待确认]>\n")
	sb.WriteString("```\n\n")
	sb.WriteString("### 局部纠偏与重锚定闭环 (Correction Loop)\n")
	sb.WriteString("1. **首轮双通道门禁提示**：\n")
	sb.WriteString("   - 若为修改任务，首轮末尾必须显式输出：\n")
	sb.WriteString("     > 👉 请确认方案：确认无误请回复“执行”；若上述【指令语义解码】的目标锚定或隐式需求有偏差，请直接指出修正项（如“目标改为 X”、“剔除条件 Y”），我将重新锚定对齐。\n")
	sb.WriteString("   - 若为纯只读探查/提问任务（A级）且无需后续修改，直接呈现探查结果与答案，无需强加“确认请回复执行”提示。\n")
	sb.WriteString("2. **纠偏禁止抢跑**：当人类用户回复修正意见（而非单纯确认“执行”）时，**严禁边改方案边直接修改业务代码**！你必须先调用 `agate prompt \"<指令>\" -s <修正目标路径> [--no-dirty]`（或根据用户修正项更新上下文），重新输出修正后的 `agate-intent` 块与 `[Plan]`，再次等待用户明确回复“执行”后方可落盘。\n")
	sb.WriteString("3. **闭环交付**：代码落盘后必须在终端运行 `agate verify` 获取 [PASS] 物证交付。\n")

	if hookMode {
		sb.WriteString("<!-- agate:prompt-guard:end -->\n")
	}

	return sb.String()
}

func truncateList(items []string, limit int) []string {
	if len(items) <= limit {
		return items
	}
	return items[:limit]
}

// FormatJSON 返回结构化 JSON 文本
func (r *CompileResult) FormatJSON() (string, error) {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}
