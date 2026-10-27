package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	agentinfo "agate/pkg/agent"
	"agate/pkg/config"
	"agate/pkg/git"

	"github.com/spf13/cobra"
)

func resolveProjectRoot() (string, error) {
	pwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("获取当前工作目录失败: %w", err)
	}
	if repoRoot, err := git.GetRepoRoot(pwd); err == nil && repoRoot != "" {
		return repoRoot, nil
	}
	return pwd, nil
}

func newProfileCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "profile",
		Short: "管理开放式 AI 协同工具档案（内置与自定义扩展）",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return nil
			}
			return &UsageError{Err: fmt.Errorf("unknown command %q for %q", args[0], cmd.CommandPath())}
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	command.AddCommand(newProfileListCommand(), newProfileAddCommand())
	return command
}

func newProfileListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "列出所有内置工具档案与已加载的自定义工具档案",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := resolveProjectRoot()
			if err != nil {
				return err
			}
			cfg, _, err := config.LoadWithGlobal(root)
			if err != nil {
				return err
			}
			builtins := agentinfo.BuiltinProfiles()
			cmd.Println("=== Agate 工具档案列表 ===")
			for _, kind := range agentinfo.All() {
				p := builtins[kind]
				scope := p.Scope
				if scope == "" {
					scope = "builtin"
				}
				cmd.Printf("[%s] %s -> %s (format=%s, env=%v, term=%v, source=%s)\n",
					scope, kind, p.Path, p.Format, p.EnvKeys, p.TermPrograms, p.Source)
			}
			if len(cfg.Agent.Profiles) > 0 {
				names := make([]string, 0, len(cfg.Agent.Profiles))
				for name := range cfg.Agent.Profiles {
					names = append(names, name)
				}
				sort.Strings(names)
				for _, name := range names {
					p := cfg.Agent.Profiles[name]
					scope := p.Scope
					if scope == "" {
						scope = "project"
					}
					cmd.Printf("[%s] %s -> %s (format=%s, env=%v, term=%v, source=%s)\n",
						scope, name, p.Path, p.Format, p.EnvKeys, p.TermPrograms, p.Source)
				}
			}
			return nil
		},
	}
}

func newProfileAddCommand() *cobra.Command {
	var (
		rulePath     string
		format       string
		source       string
		version      string
		envKeys      []string
		termPrograms []string
		reads        []string
		frontmatter  string
		useGlobal    bool
		useLocal     bool
	)
	resetFlags := func() {
		rulePath = ""
		format = "markdown"
		source = "用户通过 agate profile add 注册"
		version = "1.0"
		envKeys = nil
		termPrograms = nil
		reads = nil
		frontmatter = ""
		useGlobal, useLocal = false, false
	}
	command := &cobra.Command{
		Use:   "add <name>",
		Short: "注册自定义 AI 工具档案至项目、本机或全局配置",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			defer resetFlags()
			if useGlobal && useLocal {
				return fmt.Errorf("--global 与 --local 不能同时指定")
			}
			name := strings.ToLower(strings.TrimSpace(args[0]))
			root, err := resolveProjectRoot()
			if err != nil {
				return err
			}

			profile := agentinfo.Profile{
				Path:         strings.TrimSpace(rulePath),
				Format:       strings.TrimSpace(format),
				Confirmed:    true,
				Source:       strings.TrimSpace(source),
				Version:      strings.TrimSpace(version),
				Reads:        reads,
				EnvKeys:      envKeys,
				TermPrograms: termPrograms,
				Frontmatter:  frontmatter,
			}

			validated, err := agentinfo.Profiles(map[string]agentinfo.Profile{name: profile})
			if err != nil {
				return err
			}
			cleanProfile := validated[agentinfo.Kind(name)]

			var targetFile string
			switch {
			case useGlobal:
				targetFile = config.GlobalConfigPath("")
				if targetFile == "" {
					return fmt.Errorf("无法解析全局配置路径")
				}
			case useLocal:
				targetFile = filepath.Join(root, filepath.FromSlash(config.LocalFileName))
			default:
				targetFile = filepath.Join(root, filepath.FromSlash(config.FileName))
			}

			if err := config.UpsertProfile(targetFile, name, cleanProfile); err != nil {
				return err
			}
			if useLocal && git.IsGitRepo() {
				_, _ = git.ApplyPrivateExclusions([]string{config.LocalFileName}, root)
			}
			cmd.Printf("[+] 已注册工具档案 %s -> %s (入口: %s, 格式: %s)\n", name, targetFile, cleanProfile.Path, cleanProfile.Format)
			return nil
		},
	}
	command.Flags().StringVar(&rulePath, "path", "", "项目内相对规则路径（如 .trae/rules.md 或 .cursor/rules/agate.mdc）")
	command.Flags().StringVar(&format, "format", "markdown", "规则渲染格式: markdown | frontmatter-md | pointer")
	command.Flags().StringVar(&source, "source", "用户通过 agate profile add 注册", "入口路径确认来源说明")
	command.Flags().StringVar(&version, "version", "1.0", "适用工具版本说明")
	command.Flags().StringSliceVar(&envKeys, "env", nil, "自动嗅探的环境变量指纹列表")
	command.Flags().StringSliceVar(&termPrograms, "term-program", nil, "自动嗅探的 TERM_PROGRAM 子串列表")
	command.Flags().StringSliceVar(&reads, "reads", nil, "该工具可能同时读取的其他规则路径")
	command.Flags().StringVar(&frontmatter, "frontmatter", "", "frontmatter-md 模式的自定义 YAML Frontmatter 内容")
	command.Flags().BoolVar(&useGlobal, "global", false, "写入个人全局配置 (~/.agate/config.toml 或 AGATE_GLOBAL_CONFIG)")
	command.Flags().BoolVar(&useLocal, "local", false, "写入本机私有配置 (.agate/config.local.toml)")
	return command
}

var profileCmd = newProfileCommand()

func init() { rootCmd.AddCommand(profileCmd) }
