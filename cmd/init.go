package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"agate/pkg/adapter"
	agentinfo "agate/pkg/agent"
	"agate/pkg/config"
	"agate/pkg/git"
	"agate/pkg/harness"

	"github.com/spf13/cobra"
)

// newInitCommand 让测试通过与用户相同的命令入口，并隔离各实例的参数状态。
func newInitCommand() *cobra.Command {
	var targets []string
	var noHook, dryRun, allowTracked bool
	command := &cobra.Command{
		Use:   "init",
		Short: "为当前项目挂载 agate 护栏体系",
		Long: `预览并安全装配工程护栏：
- 解析内置工具及 .agate/config.toml 中声明的 Markdown 工具档案；
- 展示规则分发计划、已有入口和可能的重叠读取；
- 通过受管区块保留用户内容，已跟踪文件需要 --allow-tracked；
- 生成地图、任务看板并按项目策略安装 Git 门禁。
--dry-run 只预览，不生成文件、不修改 Git 配置。规则落盘不表示客户端已加载。`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			defer func() {
				targets = nil
				noHook, dryRun, allowTracked = false, false, false
			}()
			origPwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("获取当前工作目录失败: %w", err)
			}
			pwd := origPwd
			if repoRoot, err := git.GetRepoRoot(pwd); err == nil && repoRoot != "" {
				pwd = repoRoot
			}
			if pwd != origPwd {
				if err := os.Chdir(pwd); err == nil {
					defer func() { _ = os.Chdir(origPwd) }()
				}
			}
			cfg, configPath, err := config.Load(pwd)
			if err != nil {
				return err
			}
			if configPath != "" {
				cmd.Printf("[i] 已加载团队级项目策略 -> %s\n", configPath)
				for _, warning := range cfg.Warnings {
					cmd.Printf("[i] %s\n", warning)
				}
			}
			detectedAgent, detected := agentinfo.DetectEnvironmentWithProfiles(os.Getenv, os.Getenv("TERM_PROGRAM"), os.Getenv("VSCODE_GIT_IPC_HANDLE"), cfg.Agent.Profiles)
			inputs := targets
			selection := "显式目标"
			if len(inputs) == 0 && len(cfg.Agent.Targets) > 0 {
				inputs, selection = cfg.Agent.Targets, "项目默认装配集合"
			} else if len(inputs) == 0 && detected {
				selection = "当前 Agent 环境"
			} else if len(inputs) == 0 {
				selection = "已有规则入口（当前 Agent 未识别）"
			}
			resolved, err := adapter.ResolveTargetsWithProfiles(inputs, pwd, detectedAgent, detected, cfg.Agent.Profiles)
			if err != nil {
				return err
			}
			cmd.Printf("[i] 目标选择依据: %s；所选目标: %v\n", selection, resolved)
			rules, source, err := adapter.LoadGlobalRules()
			if err != nil {
				return fmt.Errorf("全局规约读取失败，未执行初始化: %w", err)
			}
			if source == "" {
				cmd.Println("[i] 使用 Agate 内置规约")
			} else {
				cmd.Printf("[i] 使用用户全局规约 -> %s（替代内置正文）\n", source)
				for _, warning := range adapter.CheckDeprecatedRulePatterns(rules) {
					cmd.Printf("[!] 全局规约需复核: %s\n", warning)
				}
			}
			plan, err := adapter.BuildPlan(pwd, rules, resolved, cfg.Agent.Profiles, allowTracked)
			if err != nil {
				return err
			}
			cmd.Println("=== 规则分发计划 ===")
			for _, entry := range plan.Entries() {
				cmd.Printf("[%s] %s -> %s (Git 跟踪=%t): %s\n", entry.Action, entry.Target, entry.Path, entry.Tracked, entry.Note)
			}
			for _, file := range plan.Inventory() {
				cmd.Printf("[现有入口] %s (Git 跟踪=%t，受管=%t，客户端加载=未知)\n", file.Path, file.Tracked, file.Managed)
			}
			for _, diagnostic := range plan.Diagnostics() {
				cmd.Printf("[!] %s\n", diagnostic)
			}
			printEditorSuggestions(cmd, plan, resolved)
			if plan.HasConflicts() {
				return fmt.Errorf("分发计划存在冲突，工程未初始化；请处理冲突后重新预览")
			}
			if dryRun {
				cmd.Println("[预览完成] 未写入规则、地图或 Git 配置")
				return nil
			}
			if err := plan.Apply(); err != nil {
				return err
			}
			projectName := filepath.Base(pwd)
			cmd.Printf("[agate] 正在为工程 [%s] 装配其余护栏...\n", projectName)
			steps := []struct {
				name string
				run  func() (bool, error)
			}{
				{".ignore", harness.EnsureIgnore},
				{"MAP.md", func() (bool, error) { return harness.EnsureMap(projectName) }},
				{"contexts/context.md", harness.EnsureContext},
				{"TASK.md", harness.EnsureTaskBoard},
				{"MEMORY.md", harness.EnsureMemory},
			}
			for _, step := range steps {
				created, err := step.run()
				if err != nil {
					return err
				}
				if created {
					cmd.Printf("[+] 已生成 %s\n", step.name)
				}
			}
			if git.IsGitRepo() {
				extraItems := append([]string(nil), git.DefaultExcludedItems...)
				if _, err := os.Stat(filepath.Join(pwd, filepath.FromSlash(config.LocalFileName))); err == nil {
					extraItems = append(extraItems, config.LocalFileName)
				}
				for _, entry := range plan.Entries() {
					if !entry.Tracked && entry.Path != "" {
						extraItems = append(extraItems, entry.Path)
					}
				}
				added, err := git.ApplyPrivateExclusions(extraItems, pwd)
				if err != nil {
					return err
				}
				cmd.Printf("[i] Git 私有排除新增 %d 项；不影响编辑器展示或客户端加载\n", added)
				if !noHook && cfg.Hooks.Install {
					if err := git.InstallHooks(); err != nil {
						return fmt.Errorf("规则和地图已写入，但 Git 门禁未就绪，请运行 agate hook install 重试: %w", err)
					}
					cmd.Println("[+] 已安装 pre-commit 与 pre-push 门禁")
				}
			}
			cmd.Println("=== [完成] 工程护栏装配完成 ===")
			cmd.Println("规约已落盘；请在实际客户端中验证入口加载与关键约束，不以落盘结果代替生效证明。")
			cmd.Println("可选冷启动：阅读 MAP.md 与 contexts/context.md，结合当前代码目录，简要补齐各模块核心职责与关键入口。")
			return nil
		},
	}
	command.Flags().StringSliceVarP(&targets, "targets", "t", nil, "目标工具名称或 all；未知工具先在项目配置声明档案")
	command.Flags().BoolVar(&noHook, "no-hook", false, "跳过本地 Git 门禁安装")
	command.Flags().BoolVar(&dryRun, "dry-run", false, "只读预览分发计划与并存诊断")
	command.Flags().BoolVar(&allowTracked, "allow-tracked", false, "明确允许更新计划中的 Git 已跟踪规则文件")
	return command
}

func printEditorSuggestions(cmd *cobra.Command, plan *adapter.Plan, targets []adapter.AgentTarget) {
	selected := make(map[adapter.AgentTarget]bool)
	for _, target := range targets {
		selected[target] = true
	}
	var paths []string
	for _, file := range plan.Inventory() {
		if !selected[file.Target] {
			paths = append(paths, file.Path)
		}
	}
	if len(paths) > 0 {
		cmd.Printf("[展示建议] 如编辑器支持本地文件树过滤，可按需隐藏 %v；保留检查全部入口的途径。隐藏不阻止规则加载，Agate 未修改编辑器设置。\n", paths)
	}
}

var initCmd = newInitCommand()

func init() { rootCmd.AddCommand(initCmd) }
