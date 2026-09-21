// Package config loads optional project-level Agate configuration.
package config

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

const FileName = ".agate/config.toml"

type Config struct {
	Agent    AgentConfig
	Guard    GuardConfig
	Hooks    HooksConfig
	Loaded   bool
	Warnings []string
}

type AgentConfig struct{ Targets []string }
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

// Load reads the optional project config. It intentionally supports the small
// TOML subset needed by Agate, avoiding a runtime dependency for one file.
func Load(root string) (Config, string, error) {
	cfg := Default()
	path := root + string(os.PathSeparator) + FileName
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return cfg, "", nil
	}
	if err != nil {
		return cfg, path, fmt.Errorf("打开项目配置失败: %w", err)
	}
	defer f.Close()

	section := ""
	scanner := bufio.NewScanner(f)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.TrimSpace(line[1 : len(line)-1])
			continue
		}
		line = stripInlineComment(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			return cfg, path, fmt.Errorf("项目配置第 %d 行格式错误", lineNo)
		}
		if err := apply(&cfg, section, strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])); err != nil {
			return cfg, path, fmt.Errorf("项目配置第 %d 行: %w", lineNo, err)
		}
	}
	if err := scanner.Err(); err != nil {
		return cfg, path, fmt.Errorf("读取项目配置失败: %w", err)
	}
	cfg.Loaded = true
	if cfg.Guard.Todo != "" {
		cfg.Warnings = append(cfg.Warnings, "guard.todo 已读取，当前版本暂未改变 TODO 审计行为")
	}
	if cfg.Guard.AbsolutePath != "" {
		cfg.Warnings = append(cfg.Warnings, "guard.absolute_path 已读取，当前版本暂未改变绝对路径审计行为")
	}
	if cfg.Guard.LargeFileMB > 0 {
		cfg.Warnings = append(cfg.Warnings, "guard.large_file_mb 已读取，当前版本暂未改变大文件审计阈值")
	}
	return cfg, path, nil
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

func apply(cfg *Config, section, key, raw string) error {
	switch section {
	case "agent":
		if key != "targets" {
			return fmt.Errorf("未知 agent 配置项 %q", key)
		}
		values, err := parseArray(raw)
		if err != nil {
			return err
		}
		cfg.Agent.Targets = values
	case "guard":
		switch key {
		case "strict":
			return parseBool(raw, &cfg.Guard.Strict)
		case "todo":
			return parseString(raw, &cfg.Guard.Todo)
		case "absolute_path":
			return parseString(raw, &cfg.Guard.AbsolutePath)
		case "large_file_mb":
			value, err := strconv.Atoi(raw)
			if err != nil || value < 0 {
				return fmt.Errorf("large_file_mb 必须是非负整数")
			}
			cfg.Guard.LargeFileMB = value
		default:
			return fmt.Errorf("未知 guard 配置项 %q", key)
		}
	case "hooks":
		if key != "install" {
			return fmt.Errorf("未知 hooks 配置项 %q", key)
		}
		return parseBool(raw, &cfg.Hooks.Install)
	default:
		return fmt.Errorf("未知配置区块 %q", section)
	}
	return nil
}

func parseString(raw string, out *string) error {
	if len(raw) < 2 || raw[0] != '"' || raw[len(raw)-1] != '"' {
		return fmt.Errorf("必须是双引号字符串")
	}
	*out = raw[1 : len(raw)-1]
	return nil
}

func parseBool(raw string, out *bool) error {
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
	var values []string
	for _, item := range strings.Split(inner, ",") {
		var value string
		if err := parseString(strings.TrimSpace(item), &value); err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, nil
}
