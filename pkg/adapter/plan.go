package adapter

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"agate/internal/templates"
	"agate/pkg/agent"
	"agate/pkg/harness"
)

// Entry 是只读计划的展示信息，不暴露待写正文或用户文件内容。
type Entry struct {
	Target  AgentTarget
	Path    string
	Action  string
	Tracked bool
	Note    string
}

// RuleFile 区分物理存在、Git 跟踪和内容受管；不宣称客户端已加载。
type RuleFile struct {
	Target  AgentTarget
	Path    string
	Tracked bool
	Managed bool
	Digest  string
}

type plannedFile struct {
	entry  Entry
	before []byte
	after  []byte
	exists bool
	mode   os.FileMode
}

// Plan 将读取、冲突检查与写入分开；不提供跨文件事务或会话刷新保证。
type Plan struct {
	mu           sync.Mutex
	root         string
	files        []plannedFile
	inventory    []RuleFile
	diagnostics  []string
	allowTracked bool
	isGit        bool
	trackedFiles []string
}

func (p *Plan) Entries() []Entry {
	entries := make([]Entry, 0, len(p.files))
	for _, file := range p.files {
		entries = append(entries, file.entry)
	}
	return entries
}

func (p *Plan) Inventory() []RuleFile { return append([]RuleFile(nil), p.inventory...) }
func (p *Plan) Diagnostics() []string { return append([]string(nil), p.diagnostics...) }

func (p *Plan) HasConflicts() bool {
	for _, file := range p.files {
		if file.entry.Action == "conflict" {
			return true
		}
	}
	return false
}

// BuildPlan 只读取项目，所有目标都完成检查后调用方才能决定是否执行。
func BuildPlan(root string, rules []byte, targets []AgentTarget, custom map[string]agent.Profile, allowTracked bool) (*Plan, error) {
	profiles, err := agent.Profiles(custom)
	if err != nil {
		return nil, err
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return nil, fmt.Errorf("解析项目目录失败: %w", err)
	}
	if len(targets) == 0 {
		return nil, fmt.Errorf("没有待分发目标，请显式指定 --targets")
	}
	if len(rules) == 0 {
		rules = templates.DefaultSkill
	}
	if strings.Contains(string(rules), "<!-- agate:") {
		return nil, fmt.Errorf("规约正文不能包含 Agate 受管标记")
	}
	p := &Plan{root: root, allowTracked: allowTracked}
	if err := p.refreshGit(); err != nil {
		return nil, err
	}
	seen := make(map[AgentTarget]bool)
	for _, target := range targets {
		profile, ok := profiles[target]
		if !ok {
			return nil, fmt.Errorf("未知工具 %q，请在项目配置中声明经确认的档案", target)
		}
		if seen[target] {
			continue
		}
		seen[target] = true
		file := plannedFile{entry: Entry{Target: target, Path: profile.Path, Action: "create"}, mode: 0644}
		full, pathErr := safeRulePath(root, profile.Path)
		if pathErr != nil {
			file.entry.Action, file.entry.Note = "conflict", pathErr.Error()
			p.files = append(p.files, file)
			continue
		}
		info, statErr := os.Lstat(full)
		if statErr == nil {
			if !info.Mode().IsRegular() {
				file.entry.Action, file.entry.Note = "conflict", "目标不是普通文件"
				p.files = append(p.files, file)
				continue
			}
			file.exists, file.mode = true, info.Mode().Perm()
			file.before, err = os.ReadFile(full)
			if err != nil {
				return nil, fmt.Errorf("读取规则入口 %s 失败: %w", profile.Path, err)
			}
		} else if !os.IsNotExist(statErr) {
			return nil, fmt.Errorf("检查规则入口 %s 失败: %w", profile.Path, statErr)
		}
		file.entry.Tracked, err = p.tracked(profile.Path)
		if err != nil {
			return nil, err
		}
		file.after, file.entry.Note, err = mergeRulesWithProfile(file.before, target, profile, rules)
		if err != nil {
			file.entry.Action, file.entry.Note = "conflict", err.Error()
		} else if file.exists && bytes.Equal(file.before, file.after) {
			file.entry.Action = "skip"
		} else if file.entry.Tracked && !allowTracked {
			file.entry.Action, file.entry.Note = "conflict", "已被 Git 跟踪；审阅计划后使用 --allow-tracked 明确允许修改"
		} else if file.exists {
			file.entry.Action = "update"
		}
		p.files = append(p.files, file)
	}
	if err := p.inspect(profiles, targets, ruleDigest(rules)); err != nil {
		return nil, err
	}
	return p, nil
}

// Apply 在任何写入前核对全部快照，防止旧计划覆盖期间的用户修改。
func (p *Plan) Apply() (applyErr error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.HasConflicts() {
		return fmt.Errorf("分发计划存在冲突，未写入任何规则")
	}
	allSkip := true
	for _, file := range p.files {
		if file.entry.Action != "skip" {
			allSkip = false
			break
		}
	}
	if allSkip {
		if err := p.refreshGit(); err != nil {
			return err
		}
		for _, file := range p.files {
			if err := p.checkSnapshot(file); err != nil {
				return err
			}
		}
		return nil
	}
	release, err := p.acquireDistributionLock()
	if err != nil {
		return err
	}
	defer func() {
		if err := release(); err != nil && applyErr == nil {
			applyErr = err
		}
	}()
	if err := p.refreshGit(); err != nil {
		return err
	}
	for _, file := range p.files {
		if err := p.checkSnapshot(file); err != nil {
			return err
		}
	}
	for _, file := range p.files {
		if file.entry.Action == "skip" {
			continue
		}
		if err := p.checkSnapshot(file); err != nil {
			return fmt.Errorf("分发中止，之前已写入的入口保留，请重新预览: %w", err)
		}
		full, err := safeRulePath(p.root, file.entry.Path)
		if err != nil {
			return err
		}
		if err := harness.WriteFileAtomic(full, file.after, file.mode); err != nil {
			return fmt.Errorf("写入 %s 失败，之前已写入的入口保留: %w", file.entry.Path, err)
		}
	}
	return nil
}

func (p *Plan) checkSnapshot(file plannedFile) error {
	full, err := safeRulePath(p.root, file.entry.Path)
	if err != nil {
		return err
	}
	current, readErr := os.ReadFile(full)
	if (!file.exists && !os.IsNotExist(readErr)) || (file.exists && (readErr != nil || !bytes.Equal(current, file.before))) {
		return fmt.Errorf("规则入口 %s 在计划生成后发生变化，请重新预览", file.entry.Path)
	}
	if file.exists {
		info, err := os.Lstat(full)
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != file.mode {
			return fmt.Errorf("规则入口 %s 的类型或权限在计划生成后变化", file.entry.Path)
		}
	}
	tracked, err := p.tracked(file.entry.Path)
	if err != nil {
		return err
	}
	if tracked && !p.allowTracked && file.entry.Action != "skip" {
		return fmt.Errorf("规则入口 %s 现在已被 Git 跟踪，拒绝旧计划写入", file.entry.Path)
	}
	return nil
}

func (p *Plan) refreshGit() error {
	p.trackedFiles = nil
	probe := exec.Command("git", "-C", p.root, "rev-parse", "--is-inside-work-tree")
	out, err := probe.Output()
	if err == nil {
		p.isGit = strings.TrimSpace(string(out)) == "true"
		if p.isGit {
			lsCmd := exec.Command("git", "--literal-pathspecs", "-c", "core.quotepath=false", "-C", p.root, "ls-files", "-z")
			lsOut, lsErr := lsCmd.Output()
			if lsErr != nil {
				return fmt.Errorf("无法读取项目 Git 索引文件列表: %w", lsErr)
			}
			for _, item := range bytes.Split(lsOut, []byte{0}) {
				if len(item) > 0 {
					p.trackedFiles = append(p.trackedFiles, filepath.ToSlash(string(item)))
				}
			}
		}
		return nil
	}
	if _, statErr := os.Stat(filepath.Join(p.root, ".git")); statErr == nil || p.isGit {
		return fmt.Errorf("无法核对项目 Git 跟踪状态: %w", err)
	}
	return nil
}

func (p *Plan) tracked(path string) (bool, error) {
	if !p.isGit {
		return false, nil
	}
	normPath := filepath.ToSlash(path)
	for _, entry := range p.trackedFiles {
		if entry == normPath {
			return true, nil
		}
	}
	targetFull := filepath.Join(p.root, filepath.FromSlash(normPath))
	targetInfo, targetErr := os.Stat(targetFull)
	for _, entry := range p.trackedFiles {
		if strings.EqualFold(entry, normPath) {
			if targetErr == nil {
				entryFull := filepath.Join(p.root, filepath.FromSlash(entry))
				if entryInfo, entryErr := os.Stat(entryFull); entryErr == nil && os.SameFile(targetInfo, entryInfo) {
					return true, nil
				}
			}
		}
	}
	cmd := exec.Command("git", "--literal-pathspecs", "-C", p.root, "ls-files", "--error-unmatch", "--", path)
	if _, err := cmd.Output(); err != nil {
		if exit, ok := err.(*exec.ExitError); ok && exit.ExitCode() == 1 {
			return false, nil
		}
		return false, fmt.Errorf("核对 %s 的 Git 跟踪状态失败: %w", path, err)
	}
	return true, nil
}

func safeRulePath(root, relative string) (string, error) {
	clean, err := agent.NormalizeRulePath(relative)
	if err != nil {
		return "", err
	}
	return noSymlinkPath(root, clean)
}

func noSymlinkPath(root, relative string) (string, error) {
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("项目根目录不存在、类型变化或被替换为符号链接")
	}
	current := root
	for _, part := range strings.Split(relative, "/") {
		current = filepath.Join(current, part)
		info, statErr := os.Lstat(current)
		if statErr != nil && !os.IsNotExist(statErr) {
			return "", statErr
		}
		if statErr == nil && info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("规则路径不能经过符号链接: %s", relative)
		}
	}
	return current, nil
}

// acquireDistributionLock 仅串行化遵循本接口的规则写入，不替代任务锁或文件访问隔离。
func (p *Plan) acquireDistributionLock() (func() error, error) {
	relative := ".ai-memory/locks/rules-distribution.lock"
	full, err := noSymlinkPath(p.root, relative)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
		return nil, err
	}
	full, err = noSymlinkPath(p.root, relative)
	if err != nil {
		return nil, err
	}
	lock, err := os.OpenFile(full, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return nil, fmt.Errorf("无法取得规则分发锁；已有初始化可能正在执行，请检查 %s: %w", relative, err)
	}
	info, err := lock.Stat()
	if err != nil {
		_ = lock.Close()
		_ = os.Remove(full)
		return nil, err
	}
	return func() error {
		if err := lock.Close(); err != nil {
			return fmt.Errorf("关闭规则分发锁失败: %w", err)
		}
		current, err := os.Lstat(full)
		if err != nil || !os.SameFile(info, current) {
			return fmt.Errorf("规则分发锁被外部修改，未自动删除: %s", relative)
		}
		if err := os.Remove(full); err != nil {
			return fmt.Errorf("释放规则分发锁失败: %w", err)
		}
		return nil
	}, nil
}

func markers(target AgentTarget) (string, string) {
	return "<!-- agate:" + string(target) + "-rules:start -->", "<!-- agate:" + string(target) + "-rules:end -->"
}

func managedRange(existing string, target AgentTarget) (int, int, error) {
	startMarker, endMarker := markers(target)
	starts, ends := strings.Count(existing, startMarker), strings.Count(existing, endMarker)
	if starts == 0 && ends == 0 {
		return -1, -1, nil
	}
	start, end := strings.Index(existing, startMarker), strings.Index(existing, endMarker)
	if starts != 1 || ends != 1 || end < start {
		return 0, 0, fmt.Errorf("%s 受管区块标记不完整、重复或顺序错误", target)
	}
	return start, end + len(endMarker), nil
}

func ruleDigest(rules []byte) string {
	normalized := strings.TrimSpace(strings.ReplaceAll(string(rules), "\r\n", "\n"))
	return fmt.Sprintf("%x", sha256.Sum256([]byte(normalized)))
}

func hasForeignManagedMarkers(text string, target AgentTarget) bool {
	startMarker, endMarker := markers(target)
	search := text
	for {
		idx := strings.Index(search, "<!-- agate:")
		if idx < 0 {
			return false
		}
		rest := search[idx:]
		endIdx := strings.Index(rest, "-->")
		if endIdx < 0 {
			return false
		}
		token := rest[:endIdx+3]
		if (strings.HasSuffix(token, "-rules:start -->") || strings.HasSuffix(token, "-rules:end -->")) &&
			token != startMarker && token != endMarker {
			return true
		}
		search = rest[endIdx+3:]
	}
}

func pointerBody() string {
	return strings.Join([]string{
		"本文件为 Agate 轻量引导指针入口。处理本仓库任何任务前，必须严格执行以下规约：",
		"1. **3-Hop 寻路**：按顺序读取 `MAP.md` (架构地图) -> `contexts/context.md` (技术契约) -> `AGENTS.md` (核心规约)，再进入目标源码，严禁全仓盲扫。",
		"2. **两阶段门禁**：排查或修改任务首轮仅输出根因、精简方案与拟改动文件清单，提示“👉 请确认方案，确认请回复‘执行’”，等待明确指令后再落盘。",
		"3. **闭环验证**：代码落盘后必须调用 `agate verify`（或 `agent-verify`）通过自检；未获显式指令严禁执行 git commit/push 等外溢操作。",
	}, "\n")
}

func splitFrontmatter(text string) (string, string, bool) {
	var firstEnd int
	if strings.HasPrefix(text, "---\r\n") {
		firstEnd = 5
	} else if strings.HasPrefix(text, "---\n") {
		firstEnd = 4
	} else {
		return "", text, false
	}
	pos := firstEnd
	for pos < len(text) {
		nextNL := strings.IndexByte(text[pos:], '\n')
		var line string
		var lineEnd int
		if nextNL < 0 {
			line = strings.TrimSuffix(text[pos:], "\r")
			lineEnd = len(text)
		} else {
			line = strings.TrimSuffix(text[pos:pos+nextNL], "\r")
			lineEnd = pos + nextNL
		}
		if line == "---" {
			return text[:lineEnd], text[lineEnd:], true
		}
		if nextNL < 0 {
			break
		}
		pos = pos + nextNL + 1
	}
	return "", text, false
}

func mergeRules(existing []byte, target AgentTarget, rules []byte) ([]byte, string, error) {
	return mergeRulesWithProfile(existing, target, agent.Profile{Format: "markdown"}, rules)
}

func mergeRulesWithProfile(existing []byte, target AgentTarget, profile agent.Profile, rules []byte) ([]byte, string, error) {
	text := string(existing)
	if hasForeignManagedMarkers(text, target) {
		return nil, "", fmt.Errorf("包含其他工具的受管区块标记，拒绝混入")
	}
	start, end, err := managedRange(text, target)
	if err != nil {
		return nil, "", err
	}
	newline := "\n"
	if strings.Contains(text, "\r\n") {
		newline = "\r\n"
	}
	startMarker, endMarker := markers(target)
	body := strings.TrimSpace(strings.ReplaceAll(string(rules), "\r\n", "\n"))
	if profile.Format == "pointer" {
		body = pointerBody()
	}
	heading := "## Agate " + string(target) + " Guardrails"
	if target == TargetCodex {
		heading = "## Agate Codex Guardrails"
	}
	block := startMarker + newline + heading + newline + "<!-- agate:rules-sha256:" + ruleDigest(rules) + " -->" + newline + strings.ReplaceAll(body, "\n", newline) + newline + endMarker

	if profile.Format == "frontmatter-md" {
		fmBlock, rest, hasFM := splitFrontmatter(text)
		if hasFM {
			if start >= 0 {
				if start < len(fmBlock) {
					return nil, "", fmt.Errorf("%s 受管区块标记不能位于 YAML Frontmatter 内部", target)
				}
				return []byte(text[:start] + block + text[end:]), "仅更新受管区块，保留 Frontmatter 与用户内容", nil
			}
			if strings.TrimSpace(rest) == "" {
				return []byte(fmBlock + newline + newline + block + newline), "保留已有 Frontmatter 并建立受管区块", nil
			}
			separator := newline + newline
			if strings.HasSuffix(text, newline) {
				separator = newline
			}
			return []byte(text + separator + block + newline), "保留已有 Frontmatter 与原正文并追加受管区块；请核验原规则是否重复或冲突", nil
		}
		fmContent := strings.TrimSpace(strings.ReplaceAll(profile.Frontmatter, "\r\n", "\n"))
		if fmContent == "" {
			fmContent = "description: Agate Engineering Guardrails\nalwaysApply: true"
		}
		generatedFM := "---" + newline + strings.ReplaceAll(fmContent, "\n", newline) + newline + "---"
		if start >= 0 {
			updated := strings.TrimLeft(text[:start]+block+text[end:], "\r\n")
			return []byte(generatedFM + newline + newline + updated), "补齐 Frontmatter 并更新受管区块，保留用户内容", nil
		}
		if len(bytes.TrimSpace(existing)) == 0 {
			return []byte(generatedFM + newline + newline + block + newline), "建立 Frontmatter 与受管区块", nil
		}
		separator := newline + newline
		if strings.HasSuffix(text, newline) {
			separator = newline
		}
		return []byte(generatedFM + newline + newline + text + separator + block + newline), "生成 Frontmatter、保留原正文并追加受管区块；请核验原规则是否重复或冲突", nil
	}

	if start >= 0 {
		return []byte(text[:start] + block + text[end:]), "仅更新受管区块，保留用户内容", nil
	}
	if len(bytes.TrimSpace(existing)) == 0 {
		return []byte(block + newline), "建立受管区块", nil
	}
	separator := newline + newline
	if strings.HasSuffix(text, newline) {
		separator = newline
	}
	return []byte(text + separator + block + newline), "保留原正文并追加受管区块；请核验原规则是否重复或冲突", nil
}

func managedDigest(data []byte, target AgentTarget) string {
	start, end, err := managedRange(string(data), target)
	if err != nil || start < 0 {
		return ""
	}
	_, endMarker := markers(target)
	block := strings.ReplaceAll(string(data[start:end-len(endMarker)]), "\r\n", "\n")
	lines := strings.Split(block, "\n")
	if len(lines) < 3 {
		return ""
	}
	lines = lines[2:]
	if len(lines) > 0 && strings.HasPrefix(lines[0], "<!-- agate:rules-sha256:") {
		if strings.HasSuffix(lines[0], "-->") {
			hex := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(lines[0], "<!-- agate:rules-sha256:"), "-->"))
			if hex != "" {
				return hex
			}
		}
		lines = lines[1:]
	}
	return ruleDigest([]byte(strings.Join(lines, "\n")))
}

func (p *Plan) inspect(profiles map[agent.Kind]agent.Profile, targets []AgentTarget, digest string) error {
	names := make([]string, 0, len(profiles))
	for name := range profiles {
		names = append(names, string(name))
	}
	sort.Strings(names)
	present := make(map[string]bool)
	for _, name := range names {
		profile := profiles[agent.Kind(name)]
		full, err := safeRulePath(p.root, profile.Path)
		if err != nil {
			p.diagnostics = append(p.diagnostics, name+" 入口无法安全检查: "+err.Error())
			continue
		}
		data, err := os.ReadFile(full)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			p.diagnostics = append(p.diagnostics, name+" 入口无法读取，加载关系未知")
			continue
		}
		tracked, err := p.tracked(profile.Path)
		if err != nil {
			return err
		}
		version := managedDigest(data, AgentTarget(name))
		p.inventory = append(p.inventory, RuleFile{Target: AgentTarget(name), Path: profile.Path, Tracked: tracked, Managed: version != "", Digest: version})
		present[profile.Path] = true
		if version != "" && version != digest {
			p.diagnostics = append(p.diagnostics, profile.Path+" 的当前受管正文与待分发规约不同；只更新所选目标，不代表全部入口同版")
		}
	}
	for _, file := range p.files {
		if file.entry.Action != "conflict" {
			present[file.entry.Path] = true
		}
	}
	selectedSet := make(map[AgentTarget]bool, len(targets))
	for _, target := range targets {
		selectedSet[target] = true
	}
	seenDiag := make(map[string]bool)
	addDiag := func(msg string) {
		if !seenDiag[msg] {
			seenDiag[msg] = true
			p.diagnostics = append(p.diagnostics, msg)
		}
	}
	for _, target := range targets {
		profile := profiles[target]
		addDiag(fmt.Sprintf("%s: 仅确认规则写入路径，客户端加载及会话刷新未验证 (来源: %s，适用版本: %s)", target, profile.Source, versionLabel(profile.Version)))
		for _, read := range profile.Reads {
			if read == profile.Path {
				continue
			}
			full, err := safeRulePath(p.root, read)
			if err != nil {
				addDiag(fmt.Sprintf("%s 的声明读取入口 %s 无法安全核验", target, read))
				continue
			}
			if _, err := os.Stat(full); err == nil || present[read] {
				addDiag(fmt.Sprintf("%s 可能同时读取 %s 与 %s；请核验实际版本、配置和规则冲突", target, profile.Path, read))
			}
		}
		for _, name := range names {
			otherTarget := AgentTarget(name)
			if otherTarget == target || selectedSet[otherTarget] {
				continue
			}
			otherProfile := profiles[otherTarget]
			if !present[otherProfile.Path] || otherProfile.Path == profile.Path {
				continue
			}
			for _, read := range otherProfile.Reads {
				if read == profile.Path {
					addDiag(fmt.Sprintf("项目中已存在 %s (%s)，该工具可能同时读取 %s；请核验实际版本、配置和规则冲突", otherTarget, otherProfile.Path, profile.Path))
				}
			}
		}
	}
	return nil
}

func versionLabel(version string) string {
	if version == "" {
		return "未核验"
	}
	return version
}
