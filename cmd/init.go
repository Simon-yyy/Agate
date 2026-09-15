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

		// 3. 挂载规约 (智能按需探测或按指定分发)
		targets := adapter.ResolveTargets(flagTargets, ".")
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

		// 6. 安装本地 Git 物理双重门禁
		if git.IsGitRepo() && !flagNoHook {
			if err := git.InstallHooks(); err != nil {
				fmt.Printf("  \033[93m[!] 挂载 Git 物理门禁提示: %v\033[0m\n", err)
			} else {
				fmt.Println("  \033[92m[+] 已挂载私有 Git 双重物理门禁 (pre-commit 验证 + pre-push 阻断)\033[0m")
			}
		}

		fmt.Println("\n\033[92m=== [完成] 工程护栏挂载完毕 ===\033[0m")
		fmt.Println("\033[93m💡 后续协同建议：\033[0m")
		fmt.Println("  1. 规约已在本地注入生效，在此项目中与 AI 对话将默认遵守“方案对齐 + 闭环自检”安全门禁；")
		fmt.Println("  2. (可选冷启动) 如需为 AI 注入全局架构认知，可向 AI 发送：")
		fmt.Println("     \033[96m“阅读 AGENTS.md 骨架，结合当前代码目录，简要补齐各模块核心职责与关键入口。”\033[0m")
		return nil
	},
}

func init() {
	initCmd.Flags().StringSliceVarP(&flagTargets, "targets", "t", nil, "指定要分发的 Agent 目标，可选：antigravity,cursor,claude,windsurf")
	initCmd.Flags().BoolVar(&flagNoHook, "no-hook", false, "跳过 Git pre-commit 钩子的自动挂载")
	rootCmd.AddCommand(initCmd)
}
