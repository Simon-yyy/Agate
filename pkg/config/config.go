// Package config loads optional project-level Agate configuration.
package config

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"agate/pkg/agent"
)

const (
	FileName      = ".agate/config.toml"
	LocalFileName = ".agate/config.local.toml"
)

type Config struct {
	Agent    AgentConfig
	Guard    GuardConfig
	Hooks    HooksConfig
	Loaded   bool
	Warnings []string
}

type AgentConfig struct {
	// Targets 仍表示未显式指定 -t 时的默认装配集合。
	Targets  []string
	Profiles map[string]agent.Profile
}
type GuardConfig struct {
	Strict       bool
	Todo         string
	AbsolutePath string
	LargeFileMB  int
}
type HooksConfig struct{ Install bool }

func Default() Config {
	return Config{Hooks: HooksConfig{Install: true}}
}

// GlobalConfigPath 返回全局配置文件路径，优先读取 AGATE_GLOBAL_CONFIG 环境变量。
func GlobalConfigPath(homeDir string) string {
	if env := strings.TrimSpace(os.Getenv("AGATE_GLOBAL_CONFIG")); env != "" {
		return env
	}
	if strings.TrimSpace(homeDir) != "" {
		return filepath.Join(homeDir, ".agate", "config.toml")
	}
	if h, err := os.UserHomeDir(); err == nil && strings.TrimSpace(h) != "" {
		return filepath.Join(h, ".agate", "config.toml")
	}
	return ""
}

func resolveGlobalPath(homeDir string) string {
	if env := strings.TrimSpace(os.Getenv("AGATE_GLOBAL_CONFIG")); env != "" {
		return env
	}
	if strings.TrimSpace(homeDir) != "" {
		return filepath.Join(homeDir, ".agate", "config.toml")
	}
	return ""
}

type layerData struct {
	path            string
	scope           string
	targets         []string
	hasTargets      bool
	profiles        map[string]agent.Profile
	strict          bool
	hasStrict       bool
	todo            string
	hasTodo         bool
	absolutePath    string
	hasAbsolutePath bool
	largeFileMB     int
	hasLargeFileMB  bool
	hooksInstall    bool
	hasHooksInstall bool
}

// Load reads the optional project config (and .agate/config.local.toml if present,
// plus AGATE_GLOBAL_CONFIG when explicitly set).
func Load(root string) (Config, string, error) {
	return LoadMerged(root, "")
}

// LoadWithGlobal loads global (~/.agate/config.toml or AGATE_GLOBAL_CONFIG),
// project (.agate/config.toml), and local (.agate/config.local.toml) configurations.
func LoadWithGlobal(root string) (Config, string, error) {
	homeDir, _ := os.UserHomeDir()
	return LoadMerged(root, homeDir)
}

// LoadMerged merges global, project, and local configurations with monotonic security tightening.
func LoadMerged(root, homeDir string) (Config, string, error) {
	cfg := Default()
	type layerSpec struct {
		path  string
		scope string
	}
	var specs []layerSpec
	if globalPath := resolveGlobalPath(homeDir); globalPath != "" {
		specs = append(specs, layerSpec{path: globalPath, scope: "global"})
	}
	if strings.TrimSpace(root) != "" {
		specs = append(specs,
			layerSpec{path: filepath.Join(root, filepath.FromSlash(FileName)), scope: "project"},
			layerSpec{path: filepath.Join(root, filepath.FromSlash(LocalFileName)), scope: "local"},
		)
	}

	var loadedPaths []string
	for _, spec := range specs {
		layer, exists, err := loadLayerFile(spec.path, spec.scope)
		if err != nil {
			return cfg, spec.path, err
		}
		if !exists {
			continue
		}
		loadedPaths = append(loadedPaths, spec.path)
		cfg.Loaded = true
		if err := mergeLayer(&cfg, layer); err != nil {
			return cfg, spec.path, err
		}
	}

	if !cfg.Loaded {
		return cfg, "", nil
	}
	if cfg.Guard.Todo != "" {
		cfg.Warnings = append(cfg.Warnings, "guard.todo 已读取，当前版本暂未改变 TODO 审计行为")
	}
	if cfg.Guard.AbsolutePath != "" {
		cfg.Warnings = append(cfg.Warnings, "guard.absolute_path 已读取，当前版本暂未改变绝对路径审计行为")
	}
	if cfg.Guard.LargeFileMB > 0 {
		cfg.Warnings = append(cfg.Warnings, "guard.large_file_mb 已读取，当前版本暂未改变大文件审计阈值")
	}
	return cfg, strings.Join(loadedPaths, ", "), nil
}

func loadLayerFile(path, scope string) (layerData, bool, error) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return layerData{}, false, nil
	}
	if err != nil {
		return layerData{}, false, fmt.Errorf("打开项目配置失败: %w", err)
	}
	defer f.Close()
	layer, err := parseLayerReader(f, path, scope)
	if err != nil {
		return layerData{}, true, err
	}
	return layer, true, nil
}

func parseLayerReader(r io.Reader, path, scope string) (layerData, error) {
	layer := layerData{path: path, scope: scope}
	section := ""
	scanner := bufio.NewScanner(r)
	lineNo := 0
	seen := make(map[string]bool)
	for scanner.Scan() {
		lineNo++
		line := stripInlineComment(strings.TrimSpace(scanner.Text()))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.TrimSpace(line[1 : len(line)-1])
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			return layer, fmt.Errorf("项目配置第 %d 行格式错误", lineNo)
		}
		key := strings.TrimSpace(parts[0])
		identity := section + "." + key
		if seen[identity] {
			return layer, fmt.Errorf("项目配置第 %d 行: 重复配置项 %s", lineNo, identity)
		}
		seen[identity] = true
		if err := applyToLayer(&layer, section, key, strings.TrimSpace(parts[1])); err != nil {
			return layer, fmt.Errorf("项目配置第 %d 行: %w", lineNo, err)
		}
	}
	if err := scanner.Err(); err != nil {
		return layer, fmt.Errorf("读取项目配置失败: %w", err)
	}
	if _, err := agent.Profiles(layer.profiles); err != nil {
		return layer, err
	}
	return layer, nil
}

func policyRank(val string) int {
	base := strings.ToLower(strings.TrimSpace(val))
	if idx := strings.IndexByte(base, '#'); idx >= 0 {
		base = strings.TrimSpace(base[:idx])
	}
	switch base {
	case "error":
		return 2
	case "warning", "warn":
		return 1
	default:
		return 0
	}
}

func mergeLayer(cfg *Config, layer layerData) error {
	if layer.hasTargets {
		cfg.Agent.Targets = append([]string(nil), layer.targets...)
	}
	if len(layer.profiles) > 0 {
		if cfg.Agent.Profiles == nil {
			cfg.Agent.Profiles = make(map[string]agent.Profile)
		}
		names := make([]string, 0, len(layer.profiles))
		for name := range layer.profiles {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			p := layer.profiles[name]
			p.Scope = layer.scope
			cfg.Agent.Profiles[name] = p
		}
		if _, err := agent.Profiles(cfg.Agent.Profiles); err != nil {
			return err
		}
	}
	if layer.scope == "local" {
		if layer.hasStrict {
			if cfg.Guard.Strict && !layer.strict {
				cfg.Warnings = append(cfg.Warnings, "config.local.toml 不能降低安全门禁 guard.strict=false，已保持 true")
			} else {
				cfg.Guard.Strict = layer.strict
			}
		}
		if layer.hasHooksInstall {
			if cfg.Hooks.Install && !layer.hooksInstall {
				cfg.Warnings = append(cfg.Warnings, "config.local.toml 不能降低安全门禁 hooks.install=false，已保持 true")
			} else {
				cfg.Hooks.Install = layer.hooksInstall
			}
		}
		if layer.hasTodo {
			if policyRank(layer.todo) < policyRank(cfg.Guard.Todo) {
				cfg.Warnings = append(cfg.Warnings, "config.local.toml 不能降低安全门禁 guard.todo 级别，已保持原配置")
			} else {
				cfg.Guard.Todo = layer.todo
			}
		}
		if layer.hasAbsolutePath {
			if policyRank(layer.absolutePath) < policyRank(cfg.Guard.AbsolutePath) {
				cfg.Warnings = append(cfg.Warnings, "config.local.toml 不能降低安全门禁 guard.absolute_path 级别，已保持原配置")
			} else {
				cfg.Guard.AbsolutePath = layer.absolutePath
			}
		}
		if layer.hasLargeFileMB {
			cfg.Guard.LargeFileMB = layer.largeFileMB
		}
		return nil
	}

	if layer.hasStrict {
		cfg.Guard.Strict = layer.strict
	}
	if layer.hasTodo {
		cfg.Guard.Todo = layer.todo
	}
	if layer.hasAbsolutePath {
		cfg.Guard.AbsolutePath = layer.absolutePath
	}
	if layer.hasLargeFileMB {
		cfg.Guard.LargeFileMB = layer.largeFileMB
	}
	if layer.hasHooksInstall {
		cfg.Hooks.Install = layer.hooksInstall
	}
	return nil
}

// stripInlineComment removes a TOML-style trailing comment while preserving
// hash characters inside quoted strings.
func stripInlineComment(line string) string {
	inString := false
	escaped := false
	for i, r := range line {
		if escaped {
			escaped = false
			continue
		}
		if r == '\\' && inString {
			escaped = true
			continue
		}
		if r == '"' {
			inString = !inString
			continue
		}
		if r == '#' && !inString {
			return strings.TrimSpace(line[:i])
		}
	}
	return strings.TrimSpace(line)
}

func applyToLayer(layer *layerData, section, key, raw string) error {
	if strings.HasPrefix(section, "agent.profiles.") {
		name := strings.TrimPrefix(section, "agent.profiles.")
		if layer.profiles == nil {
			layer.profiles = make(map[string]agent.Profile)
		}
		profile := layer.profiles[name]
		profile.Scope = layer.scope
		var err error
		switch key {
		case "path":
			err = parseString(raw, &profile.Path)
		case "format":
			err = parseString(raw, &profile.Format)
		case "confirmed":
			err = parseBool(raw, &profile.Confirmed)
		case "source":
			err = parseString(raw, &profile.Source)
		case "version":
			err = parseString(raw, &profile.Version)
		case "reads":
			profile.Reads, err = parseArray(raw)
		case "env_keys":
			profile.EnvKeys, err = parseArray(raw)
		case "term_programs":
			profile.TermPrograms, err = parseArray(raw)
		case "frontmatter":
			err = parseString(raw, &profile.Frontmatter)
		default:
			err = fmt.Errorf("未知工具档案配置项 %q", key)
		}
		layer.profiles[name] = profile
		return err
	}
	switch section {
	case "agent":
		if key != "targets" {
			return fmt.Errorf("未知 agent 配置项 %q", key)
		}
		values, err := parseArray(raw)
		if err != nil {
			return err
		}
		layer.targets = values
		layer.hasTargets = true
	case "guard":
		switch key {
		case "strict":
			if err := parseBool(raw, &layer.strict); err != nil {
				return err
			}
			layer.hasStrict = true
			return nil
		case "todo":
			if err := parseString(raw, &layer.todo); err != nil {
				return err
			}
			if err := validateGuardPolicy("todo", layer.todo); err != nil {
				return err
			}
			layer.hasTodo = true
			return nil
		case "absolute_path":
			if err := parseString(raw, &layer.absolutePath); err != nil {
				return err
			}
			if err := validateGuardPolicy("absolute_path", layer.absolutePath); err != nil {
				return err
			}
			layer.hasAbsolutePath = true
			return nil
		case "large_file_mb":
			value, err := strconv.Atoi(raw)
			if err != nil || value < 0 {
				return fmt.Errorf("large_file_mb 必须是非负整数")
			}
			layer.largeFileMB = value
			layer.hasLargeFileMB = true
			return nil
		default:
			return fmt.Errorf("未知 guard 配置项 %q", key)
		}
	case "hooks":
		if key != "install" {
			return fmt.Errorf("未知 hooks 配置项 %q", key)
		}
		if err := parseBool(raw, &layer.hooksInstall); err != nil {
			return err
		}
		layer.hasHooksInstall = true
		return nil
	default:
		return fmt.Errorf("未知配置区块 %q", section)
	}
	return nil
}

func formatStringArray(items []string) string {
	quoted := make([]string, 0, len(items))
	for _, item := range items {
		quoted = append(quoted, strconv.Quote(item))
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}

func formatProfileSection(name string, p agent.Profile) string {
	var b strings.Builder
	fmt.Fprintf(&b, "[agent.profiles.%s]\n", name)
	fmt.Fprintf(&b, "path = %s\n", strconv.Quote(p.Path))
	fmt.Fprintf(&b, "format = %s\n", strconv.Quote(p.Format))
	fmt.Fprintf(&b, "confirmed = %t\n", p.Confirmed)
	fmt.Fprintf(&b, "source = %s\n", strconv.Quote(p.Source))
	fmt.Fprintf(&b, "version = %s\n", strconv.Quote(p.Version))
	if len(p.Reads) > 0 {
		fmt.Fprintf(&b, "reads = %s\n", formatStringArray(p.Reads))
	}
	if len(p.EnvKeys) > 0 {
		fmt.Fprintf(&b, "env_keys = %s\n", formatStringArray(p.EnvKeys))
	}
	if len(p.TermPrograms) > 0 {
		fmt.Fprintf(&b, "term_programs = %s\n", formatStringArray(p.TermPrograms))
	}
	if p.Frontmatter != "" {
		fmt.Fprintf(&b, "frontmatter = %s\n", strconv.Quote(p.Frontmatter))
	}
	return b.String()
}

// UpsertProfile 校验并以规范 TOML 区块写入或更新指定配置文件中的工具档案。
func UpsertProfile(filePath, name string, profile agent.Profile) error {
	validated, err := agent.Profiles(map[string]agent.Profile{name: profile})
	if err != nil {
		return err
	}
	cleanProfile := validated[agent.Kind(name)]
	sectionHeader := "[agent.profiles." + name + "]"
	block := formatProfileSection(name, cleanProfile)

	existingBytes, readErr := os.ReadFile(filePath)
	if readErr != nil && !os.IsNotExist(readErr) {
		return fmt.Errorf("读取配置文件 %s 失败: %w", filePath, readErr)
	}

	var updated string
	if os.IsNotExist(readErr) || len(bytes.TrimSpace(existingBytes)) == 0 {
		updated = block
	} else {
		text := strings.ReplaceAll(string(existingBytes), "\r\n", "\n")
		lines := strings.Split(text, "\n")
		secStart, secEnd := -1, len(lines)
		for i, rawLine := range lines {
			trimmed := stripInlineComment(strings.TrimSpace(rawLine))
			if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
				if secStart >= 0 {
					secEnd = i
					break
				}
				if strings.TrimSpace(trimmed[1:len(trimmed)-1]) == "agent.profiles."+name {
					secStart = i
				}
			}
		}
		if secStart >= 0 {
			prefix := strings.Join(lines[:secStart], "\n")
			suffix := strings.Join(lines[secEnd:], "\n")
			var parts []string
			if strings.TrimSpace(prefix) != "" {
				parts = append(parts, strings.TrimRight(prefix, "\n"))
			}
			parts = append(parts, strings.TrimRight(block, "\n"))
			if strings.TrimSpace(suffix) != "" {
				parts = append(parts, strings.TrimLeft(suffix, "\n"))
			}
			updated = strings.Join(parts, "\n\n") + "\n"
		} else {
			_ = sectionHeader
			updated = strings.TrimRight(text, "\n") + "\n\n" + block
		}
	}

	if _, err := parseLayerReader(strings.NewReader(updated), filePath, "project"); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		return fmt.Errorf("创建配置目录失败: %w", err)
	}
	return os.WriteFile(filePath, []byte(updated), 0644)
}

func validateGuardPolicy(field, value string) error {
	base := strings.ToLower(strings.TrimSpace(value))
	if idx := strings.IndexByte(base, '#'); idx >= 0 {
		base = strings.TrimSpace(base[:idx])
	}
	switch base {
	case "", "warning", "warn", "error", "off", "ignore":
		return nil
	default:
		return fmt.Errorf("guard.%s 仅支持 warning/warn、error 或 off/ignore，当前值: %q", field, value)
	}
}

func parseString(raw string, out *string) error {
	if len(raw) < 2 || raw[0] != '"' || raw[len(raw)-1] != '"' {
		return fmt.Errorf("必须是双引号字符串")
	}
	value, err := strconv.Unquote(raw)
	if err != nil {
		return fmt.Errorf("字符串格式错误: %w", err)
	}
	*out = value
	return nil
}

func parseBool(raw string, out *bool) error {
	if raw != "true" && raw != "false" {
		return fmt.Errorf("必须是 true 或 false")
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return fmt.Errorf("必须是 true 或 false")
	}
	*out = value
	return nil
}

func parseArray(raw string) ([]string, error) {
	if len(raw) < 2 || raw[0] != '[' || raw[len(raw)-1] != ']' {
		return nil, fmt.Errorf("必须是字符串数组")
	}
	inner := strings.TrimSpace(raw[1 : len(raw)-1])
	if inner == "" {
		return nil, nil
	}
	var items []string
	start, inString, escaped := 0, false, false
	for i, r := range inner {
		if escaped {
			escaped = false
			continue
		}
		if r == '\\' && inString {
			escaped = true
			continue
		}
		if r == '"' {
			inString = !inString
		}
		if r == ',' && !inString {
			items = append(items, inner[start:i])
			start = i + 1
		}
	}
	if inString {
		return nil, fmt.Errorf("数组字符串未闭合")
	}
	if strings.TrimSpace(inner[start:]) != "" {
		items = append(items, inner[start:])
	}
	var values []string
	for _, item := range items {
		var value string
		if err := parseString(strings.TrimSpace(item), &value); err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, nil
}
