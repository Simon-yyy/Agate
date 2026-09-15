package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"agate/pkg/adapter"
	"agate/pkg/git"
	"agate/pkg/harness"

	"github.com/spf13/cobra"
)

var (
	flagTargets []string
	flagNoHook  bool
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "为当前项目挂载 agate 护栏体系",
	Long: `自动化执行工程护栏初始化：
- 生成 .ignore 索引过滤，避免文件检索爆仓；
- 配置 .git/info/exclude 私有追踪隔离，防止 AI 元数据污染 Git 提交；
- 分发单一事实源规约至 Cursor / Antigravity / Claude / Windsurf 等目标；
- 生成 AGENTS.md 模块地图与 contexts/context.md 契约骨架；
- 挂载本地 Git pre-commit 闭环门禁。`,
	RunE: func(cmd *cobra.Command, args []string) error {
		pwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("获取当前工作目录失败: %w", err)
		}
		projectName := filepath.Base(pwd)

		fmt.Printf("\033[94m[agate] 正在为工程 [%s] 挂载防护体系...\033[0m\n", projectName)

		// 1. 生成 .ignore
		if created, err := harness.EnsureIgnore(); err != nil {
			return err
		} else if created {
			fmt.Println("  \033[92m[+] 已生成 .ignore (索引防爆仓)\033[0m")
		}

		// 2. 隔离私有文件至 .git/info/exclude
		if git.IsGitRepo() {
			addedCount, err := git.ApplyPrivateExclusions(nil)
			if err != nil {
				return err
			}
			if addedCount > 0 {
				fmt.Printf("  \033[92m[+] 已向 .git/info/exclude 注入 %d 项隔离清单 (私有配置完全隐形)\033[0m\n", addedCount)
			}
		}

		// 3. 挂载规约
		var targets []adapter.AgentTarget
		for _, t := range flagTargets {
			targets = append(targets, adapter.AgentTarget(t))
		}
		globalRules, _ := adapter.LoadGlobalRules()
		if err := adapter.DistributeRules(globalRules, targets); err != nil {
			return err
		}

		// 4. 生成架构地图骨架 AGENTS.md
		if created, err := harness.EnsureAgentsMap(projectName); err != nil {
			return err
		} else if created {
			fmt.Println("  \033[92m[+] 已生成 AGENTS.md 骨架\033[0m")
		}

		// 5. 生成技术上下文基线 contexts/context.md
		if created, err := harness.EnsureContext(); err != nil {
			return err
		} else if created {
			fmt.Println("  \033[92m[+] 已生成 contexts/context.md 基线\033[0m")
		}

		// 6. 安装本地 Git Hook
		if git.IsGitRepo() && !flagNoHook {
			if err := git.InstallPreCommitHook(); err != nil {
				fmt.Printf("  \033[93m[!] 挂载 pre-commit 钩子提示: %v\033[0m\n", err)
			} else {
				fmt.Println("  \033[92m[+] 已挂载私有 pre-commit 门禁 -> agate verify\033[0m")
			}
		}

		fmt.Println("\n\033[92m=== [完成] 工程初始化完毕 ===\033[0m")
		fmt.Println("\033[93m👉 请直接复制以下指令发送给 AI 开启冷启动：\033[0m")
		fmt.Println("\033[96m分析当前工程代码与配置，完善 AGENTS.md 与 contexts/context.md。\033[0m")
		return nil
	},
}

func init() {
	initCmd.Flags().StringSliceVarP(&flagTargets, "targets", "t", nil, "指定要分发的 Agent 目标，可选：antigravity,cursor,claude,windsurf")
	initCmd.Flags().BoolVar(&flagNoHook, "no-hook", false, "跳过 Git pre-commit 钩子的自动挂载")
	rootCmd.AddCommand(initCmd)
}
