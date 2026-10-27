package agent

import (
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

// Profile 记录工具入口与加载关系的声明，不将路径确认当作客户端加载证明。
type Profile struct {
	Path         string
	Format       string
	Confirmed    bool
	Source       string
	Version      string
	Reads        []string
	EnvKeys      []string
	TermPrograms []string
	Frontmatter  string
	Scope        string
}

var profileName = regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`)
var reservedDevice = regexp.MustCompile(`^(COM|LPT)[1-9]$`)

// BuiltinProfiles 保留既有写入路径；Reads 仅用于提示可能的重叠读取。
func BuiltinProfiles() map[Kind]Profile {
	return map[Kind]Profile{
		Codex: {
			Path:         "AGENTS.md",
			Format:       "markdown",
			Confirmed:    true,
			Source:       "Agate 内置入口映射",
			Reads:        []string{"AGENTS.md"},
			EnvKeys:      []string{"CODEX_SESSION_ID", "CODEX_THREAD_ID", "CODEX_VERSION", "CODEX_CI", "CODEX_SHELL"},
			TermPrograms: []string{"codex"},
			Scope:        "builtin",
		},
		Cursor: {
			Path:         ".cursorrules",
			Format:       "markdown",
			Confirmed:    true,
			Source:       "Agate 内置入口映射",
			Reads:        []string{".cursorrules", "AGENTS.md", ".cursor/rules"},
			EnvKeys:      []string{"CURSOR_AGENT", "CURSOR_VERSION"},
			TermPrograms: []string{"cursor"},
			Scope:        "builtin",
		},
		Antigravity: {
			Path:         ".gemini/GEMINI.md",
			Format:       "markdown",
			Confirmed:    true,
			Source:       "Agate 内置入口映射",
			Reads:        []string{".gemini/GEMINI.md"},
			EnvKeys:      []string{"ANTIGRAVITY_AGENT", "GEMINI_AGENT", "ANTIGRAVITY_IDE"},
			TermPrograms: []string{"antigravity"},
			Scope:        "builtin",
		},
		Claude: {
			Path:         "CLAUDE.md",
			Format:       "markdown",
			Confirmed:    true,
			Source:       "Agate 内置入口映射",
			Reads:        []string{"CLAUDE.md", "AGENTS.md"},
			EnvKeys:      []string{"CLAUDE_CODE", "CLAUDE_AGENT"},
			TermPrograms: []string{"claude"},
			Scope:        "builtin",
		},
		Windsurf: {
			Path:         ".windsurfrules",
			Format:       "markdown",
			Confirmed:    true,
			Source:       "Agate 内置入口映射",
			Reads:        []string{".windsurfrules"},
			EnvKeys:      []string{"WINDSURF_AGENT"},
			TermPrograms: []string{"windsurf"},
			Scope:        "builtin",
		},
	}
}

// NormalizeRulePath 在所有平台统一拒绝越界路径、控制字符和保留资产。
func NormalizeRulePath(raw string) (string, error) {
	if raw == "" || strings.TrimSpace(raw) != raw || strings.IndexFunc(raw, unicode.IsControl) >= 0 || strings.ContainsAny(raw, "\\:*?\"<>|") || strings.HasPrefix(raw, "/") {
		return "", fmt.Errorf("规则路径必须是项目内的相对路径: %q", raw)
	}
	for _, part := range strings.Split(raw, "/") {
		if part == "" || part == "." || part == ".." || strings.HasSuffix(part, ".") || strings.HasSuffix(part, " ") {
			return "", fmt.Errorf("规则路径包含不安全片段: %q", raw)
		}
		lower := strings.ToLower(part)
		if lower == ".git" || lower == ".agate" || lower == ".ai-memory" || lower == ".env" || strings.HasPrefix(lower, ".env.") {
			return "", fmt.Errorf("规则路径不能经过配置或私有资产目录: %q", raw)
		}
		base := strings.ToUpper(strings.SplitN(part, ".", 2)[0])
		if base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" || reservedDevice.MatchString(base) {
			return "", fmt.Errorf("规则路径包含系统保留名称: %q", raw)
		}
	}
	clean := path.Clean(raw)
	first := strings.ToLower(strings.Split(clean, "/")[0])
	switch first {
	case ".git", ".agate", ".ai-memory", ".env", "map.md", "task.md", "memory.md", "contexts", "go.mod", "go.sum", "main.go", "verify.cmd", "verify.sh", "verify.bat", ".ignore":
		return "", fmt.Errorf("规则路径不能覆盖项目保留资产: %q", raw)
	}
	if strings.HasPrefix(first, ".env.") {
		return "", fmt.Errorf("规则路径不能覆盖环境配置: %q", raw)
	}
	return clean, nil
}

func isSupportedFormat(format string) bool {
	return format == "markdown" || format == "frontmatter-md" || format == "pointer"
}

func validateCleanToken(raw string) bool {
	if raw == "" {
		return false
	}
	return strings.IndexFunc(raw, func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsControl(r)
	}) < 0
}

func validateFrontmatter(fm string) error {
	for _, r := range fm {
		if unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t' {
			return fmt.Errorf("frontmatter 不能包含控制字符")
		}
	}
	normalized := strings.ReplaceAll(fm, "\r\n", "\n")
	for _, line := range strings.Split(normalized, "\n") {
		if strings.TrimSpace(line) == "---" {
			return fmt.Errorf("frontmatter 不能包含 --- 分隔行")
		}
	}
	return nil
}

// Profiles 将项目补充档案加入内置集合；禁止静默覆写内置档案和跨平台路径碰撞。
func Profiles(custom map[string]Profile) (map[Kind]Profile, error) {
	result := BuiltinProfiles()
	paths := make(map[string]string)
	for name, p := range result {
		paths[strings.ToLower(p.Path)] = string(name)
	}
	names := make([]string, 0, len(custom))
	for name := range custom {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		p := custom[name]
		if !profileName.MatchString(name) || name == "all" {
			return nil, fmt.Errorf("工具档案名称非法: %q", name)
		}
		if _, known := Parse(name); known {
			return nil, fmt.Errorf("项目档案不能覆写内置工具 %q", name)
		}
		if !isSupportedFormat(p.Format) || !p.Confirmed || strings.TrimSpace(p.Source) == "" || strings.TrimSpace(p.Version) == "" {
			return nil, fmt.Errorf("工具档案 %q 需要 format=markdown/frontmatter-md/pointer、confirmed=true、source 和 version；加载结果仍须人工验证", name)
		}
		if strings.IndexFunc(p.Source+p.Version, unicode.IsControl) >= 0 {
			return nil, fmt.Errorf("工具档案 %q 的来源和版本不能包含控制字符", name)
		}
		if err := validateFrontmatter(p.Frontmatter); err != nil {
			return nil, fmt.Errorf("工具档案 %q: %w", name, err)
		}
		clean, err := NormalizeRulePath(p.Path)
		if err != nil {
			return nil, fmt.Errorf("工具档案 %q: %w", name, err)
		}
		p.Path = clean
		ext, base := strings.ToLower(path.Ext(clean)), strings.ToLower(path.Base(clean))
		if ext != ".md" && ext != ".mdc" && ext != ".rules" && !(strings.HasPrefix(base, ".") && strings.HasSuffix(base, "rules")) {
			return nil, fmt.Errorf("工具档案 %q 仅支持 .md、.mdc、.rules 或隐藏 rules 文件，不能写入源码或可执行配置", name)
		}
		for used, owner := range paths {
			current := strings.ToLower(clean)
			if used == current || strings.HasPrefix(used, current+"/") || strings.HasPrefix(current, used+"/") {
				return nil, fmt.Errorf("工具档案 %q 与 %q 存在路径碰撞: %s", name, owner, clean)
			}
		}
		p.Reads = append([]string(nil), p.Reads...)
		for i, read := range p.Reads {
			p.Reads[i], err = NormalizeRulePath(read)
			if err != nil {
				return nil, fmt.Errorf("工具档案 %q 的 reads: %w", name, err)
			}
		}
		p.EnvKeys = append([]string(nil), p.EnvKeys...)
		for _, k := range p.EnvKeys {
			if !validateCleanToken(k) {
				return nil, fmt.Errorf("工具档案 %q 的 env_keys 不能包含空值、空白或控制字符: %q", name, k)
			}
		}
		p.TermPrograms = append([]string(nil), p.TermPrograms...)
		for _, tp := range p.TermPrograms {
			if !validateCleanToken(tp) {
				return nil, fmt.Errorf("工具档案 %q 的 term_programs 不能包含空值、空白或控制字符: %q", name, tp)
			}
		}
		if p.Scope == "" {
			p.Scope = "project"
		}
		paths[strings.ToLower(clean)] = name
		result[Kind(name)] = p
	}
	return result, nil
}
