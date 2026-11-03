package cmd

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"

	"agate/pkg/agent"
	"agate/pkg/config"
	"agate/pkg/git"
	"agate/pkg/workspace"

	"github.com/spf13/cobra"
)

var (
	flagWorkspaceTargets string
	flagWorkspaceDryRun  bool
)

var workspaceCmd = &cobra.Command{
	Use:   "workspace",
	Short: "生成与管理多工具独立窗口视图 (.code-workspace)，实现规则展示隔离",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return nil
		}
		return &UsageError{Err: fmt.Errorf("unknown command %q for %q", args[0], cmd.CommandPath())}
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
	Long: `在不修改团队共享 .vscode/settings.json、不删除任何物理规则文件的前提下，
按工具生成本机私有、窗口独立的 .code-workspace 视图配置文件（位于 .ai-memory/workspaces/）。
注：隐藏仅影响当前窗口展示，不代表停止客户端加载底层物理文件。`,
}

// parseWorkspaceTargetList 拆分逗号分隔的目标字符串并去重过滤空白。
func parseWorkspaceTargetList(raw string) []string {
	parts := strings.Split(raw, ",")
	var result []string
	seen := make(map[string]bool)
	for _, p := range parts {
		cleaned := strings.TrimSpace(p)
		if cleaned != "" && !seen[cleaned] {
			seen[cleaned] = true
			result = append(result, cleaned)
		}
	}
	return result
}

// resolveWorkspaceTargets 解析工作区目标工具列表。
// (R6-03: 逗号分隔解析、显式空白参数拦截、配置项空白过滤与默认目标回退)
func resolveWorkspaceTargets(cmd *cobra.Command, cfgTargets []string) ([]string, error) {
	if cmd != nil && cmd.Flags().Changed("targets") {
		clean := strings.TrimSpace(flagWorkspaceTargets)
		if clean == "" {
			return nil, fmt.Errorf("参数 -t/--targets 不能为空白字符串")
		}
		parsed := parseWorkspaceTargetList(clean)
		if len(parsed) == 0 {
			return nil, fmt.Errorf("参数 -t/--targets 未包含有效的目标工具名称")
		}
		return parsed, nil
	}

	var validCfg []string
	seen := make(map[string]bool)
	for _, t := range cfgTargets {
		c := strings.TrimSpace(t)
		if c != "" && !seen[c] {
			seen[c] = true
			validCfg = append(validCfg, c)
		}
	}
	if len(validCfg) > 0 {
		return validCfg, nil
	}

	return []string{"cursor", "antigravity", "windsurf"}, nil
}

func resolveWorkspaceContext(cmd *cobra.Command) (string, []string, map[agent.Kind]agent.Profile, error) {
	workDir := "."
	if repoRoot, err := git.GetRepoRoot("."); err == nil && repoRoot != "" {
		workDir = repoRoot
	}
	cfg, _, err := config.LoadWithGlobal(workDir)
	if err != nil {
		return "", nil, nil, err
	}

	profiles := make(map[agent.Kind]agent.Profile, len(cfg.Agent.Profiles))
	for name, prof := range cfg.Agent.Profiles {
		profiles[agent.Kind(name)] = prof
	}

	targets, err := resolveWorkspaceTargets(cmd, cfg.Agent.Targets)
	if err != nil {
		return "", nil, nil, err
	}
	return workDir, targets, profiles, nil
}

func printWorkspaceViewPlan(cmd *cobra.Command, workDir string, plan *workspace.ViewPlan) {
	cmd.Println("\033[94m=== [agate workspace] 独立窗口视图隔离计划 ===\033[0m")
	for _, view := range plan.Views {
		if !view.Supported {
			cmd.Printf("  \033[93m[!] [%s] 降级跳过:\033[0m %s\n", view.Target, view.Reason)
			continue
		}
		relWS := view.WorkspacePath
		if rel, err := filepath.Rel(workDir, view.WorkspacePath); err == nil {
			relWS = filepath.ToSlash(rel)
		}
		cmd.Printf("  \033[92m[•] 工具视图: %s\033[0m\n", view.Target)
		cmd.Printf("      独立视图文件: %s\n", relWS)
		cmd.Printf("      本窗可见规则: %s (及 MAP.md, contexts/context.md, TASK.md, MEMORY.md)\n", view.SelfRulePath)
		cmd.Printf("      本窗隐藏规则: %s\n", strings.Join(view.HiddenPaths, ", "))
	}
	cmd.Println("\n\033[90m提示: 隐藏仅影响当前窗口展示，不代表停止客户端加载；不会修改团队共享 .vscode/settings.json。\033[0m")
}

var workspacePlanCmd = &cobra.Command{
	Use:   "plan",
	Short: "只读预览各工具的独立视图文件路径与将隐藏的其他规则文件列表",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		workDir, targets, profiles, err := resolveWorkspaceContext(cmd)
		if err != nil {
			return err
		}
		plan, err := workspace.BuildViewPlan(workDir, targets, profiles)
		if err != nil {
			return err
		}
		printWorkspaceViewPlan(cmd, workDir, plan)
		return nil
	},
}

var workspaceApplyCmd = &cobra.Command{
	Use:   "apply",
	Short: "生成 .ai-memory/workspaces/<target>.code-workspace 独立视图文件",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		workDir, targets, profiles, err := resolveWorkspaceContext(cmd)
		if err != nil {
			return err
		}
		plan, err := workspace.BuildViewPlan(workDir, targets, profiles)
		if err != nil {
			return err
		}
		printWorkspaceViewPlan(cmd, workDir, plan)

		if flagWorkspaceDryRun {
			cmd.Println("\033[93m[Dry-Run] 仅预览视图计划，未写入任何 .code-workspace 文件。\033[0m")
			return nil
		}

		written, err := workspace.ApplyViewPlan(plan)
		if err != nil {
			return err
		}
		cmd.Printf("\n\033[92m[✓] 已生成 %d 个独立工作区视图文件:\033[0m\n", len(written))
		for _, p := range written {
			rel := p
			if r, err := filepath.Rel(workDir, p); err == nil {
				rel = filepath.ToSlash(r)
			}
			cmd.Printf("  - %s\n", rel)
		}
		cmd.Println("打开方式: 可执行 `agate workspace open <target>`，或在对应 IDE 中选择 `File -> Open Workspace from File...` 打开上述视图文件。")
		return nil
	},
}

var workspaceRestoreCmd = &cobra.Command{
	Use:   "restore",
	Short: "清理生成的私有工作区视图配置，恢复原始全量文件展示",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		workDir := "."
		if repoRoot, err := git.GetRepoRoot("."); err == nil && repoRoot != "" {
			workDir = repoRoot
		}
		var targets []string
		if cmd.Flags().Changed("targets") {
			clean := strings.TrimSpace(flagWorkspaceTargets)
			if clean == "" {
				return fmt.Errorf("参数 -t/--targets 不能为空白字符串")
			}
			targets = parseWorkspaceTargetList(clean)
			if len(targets) == 0 {
				return fmt.Errorf("参数 -t/--targets 未包含有效的目标工具名称")
			}
		}

		removed, err := workspace.RestoreViews(workDir, targets)
		if err != nil {
			return err
		}
		if len(removed) == 0 {
			cmd.Println("\033[90m[agate workspace] 当前无已生成的私有工作区视图文件需要清理。\033[0m")
			return nil
		}
		cmd.Printf("\033[92m[✓] 已清理 %d 个私有工作区视图文件:\033[0m\n", len(removed))
		for _, p := range removed {
			rel := p
			if r, err := filepath.Rel(workDir, p); err == nil {
				rel = filepath.ToSlash(r)
			}
			cmd.Printf("  - %s\n", rel)
		}
		return nil
	},
}

var workspaceOpenCmd = &cobra.Command{
	Use:   "open <target>",
	Short: "生成对应 .code-workspace 并启动客户端打开该独立视图窗口",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			return &UsageError{Err: fmt.Errorf("workspace open 需要且仅需要 1 个 <target> 参数")}
		}
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		target := workspace.NormalizeTarget(args[0])
		workDir, _, profiles, err := resolveWorkspaceContext(cmd)
		if err != nil {
			return err
		}

		plan, err := workspace.BuildViewPlan(workDir, []string{target}, profiles)
		if err != nil {
			return err
		}
		if len(plan.Views) == 0 || !plan.Views[0].Supported {
			reason := fmt.Sprintf("[%s] 不支持 GUI .code-workspace 独立视图", target)
			if len(plan.Views) > 0 && plan.Views[0].Reason != "" {
				reason = plan.Views[0].Reason
			}
			return fmt.Errorf("无法打开独立视图窗口: %s", reason)
		}

		written, err := workspace.ApplyViewPlan(plan)
		if err != nil {
			return err
		}
		wsFile := written[0]
		openExec, err := workspace.BuildOpenCommand(runtime.GOOS, target, wsFile)
		if err != nil {
			return err
		}
		if err := openExec.Start(); err != nil {
			return fmt.Errorf("启动 [%s] 客户端失败 (%v)，请手动打开工作区文件: %s", target, err, wsFile)
		}
		cmd.Printf("\033[92m[✓] 已为 [%s] 启动独立视图窗口: %s\033[0m\n", target, wsFile)
		return nil
	},
}

func init() {
	workspacePlanCmd.Flags().StringVarP(&flagWorkspaceTargets, "targets", "t", "", "目标工具列表 (逗号分隔，如 cursor,antigravity,windsurf 或 all)")
	workspaceApplyCmd.Flags().StringVarP(&flagWorkspaceTargets, "targets", "t", "", "目标工具列表 (逗号分隔，如 cursor,antigravity,windsurf 或 all)")
	workspaceApplyCmd.Flags().BoolVar(&flagWorkspaceDryRun, "dry-run", false, "仅预览视图计划，不写入文件")
	workspaceRestoreCmd.Flags().StringVarP(&flagWorkspaceTargets, "targets", "t", "", "待清理的目标工具列表 (为空则清理全部)")

	workspaceCmd.AddCommand(workspacePlanCmd)
	workspaceCmd.AddCommand(workspaceApplyCmd)
	workspaceCmd.AddCommand(workspaceRestoreCmd)
	workspaceCmd.AddCommand(workspaceOpenCmd)

	rootCmd.AddCommand(workspaceCmd)
}
