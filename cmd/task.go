package cmd

import (
	"fmt"
	"strings"

	"agate/pkg/relay"

	"github.com/spf13/cobra"
)

var (
	flagTaskAgent      string
	flagTaskForce      bool
	flagTaskNextAgent  string
	flagTaskNote       string
	flagTaskSkipVerify bool
)

var taskCmd = &cobra.Command{
	Use:   "task",
	Short: "跨 Agent 协同任务接力与状态机管理",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return nil
		}
		return &UsageError{Err: fmt.Errorf("unknown command %q for %q", args[0], cmd.CommandPath())}
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
	Long: `跨编程工具 (Cursor, Antigravity, Claude Code, Windsurf) 任务接力中枢。
负责管理 TASK.md 状态机流转、多 Agent 租约互斥锁、自检门禁核验与机器交付物证留痕。`,
}

var taskStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "查看当前任务看板状态、持锁人与交付物证",
	RunE: func(cmd *cobra.Command, args []string) error {
		res, err := relay.GetTaskStatus(".")
		if err != nil {
			return fmt.Errorf("获取任务状态失败: %w", err)
		}

		cmd.Println("\033[94m=== [agate task] 协同任务看板状态 ===\033[0m")
		cmd.Printf("  任务编号: \033[1m%s\033[0m\n", res.Manifest.TaskId)
		cmd.Printf("  任务标题: %s\n", res.Manifest.Title)

		statusColor := "\033[92m" // green
		if res.Manifest.Status == relay.StatusBlocked {
			statusColor = "\033[91m" // red
		} else if res.Manifest.Status == relay.StatusHandoverReady {
			statusColor = "\033[96m" // cyan
		} else if res.Manifest.Status == relay.StatusInProgress {
			statusColor = "\033[93m" // yellow
		}
		cmd.Printf("  当前状态: %s[%s]\033[0m\n", statusColor, res.Manifest.Status)
		cmd.Printf("  当前执行: %s | 指定接棒: %s\n", res.Manifest.CurrentAgent, res.Manifest.NextAgent)
		cmd.Printf("  更新时间: %s\n", res.Manifest.UpdatedAt)

		if res.Lock != nil {
			if res.Lock.IsExpired() {
				cmd.Printf("  互斥锁状态: \033[90m已过期 (前持有人: %s)\033[0m\n", res.Lock.OwnerAgent)
			} else {
				cmd.Printf("  互斥锁状态: \033[93m施工锁定中 (持有人: %s, 租约至: %s)\033[0m\n",
					res.Lock.OwnerAgent, res.Lock.ExpiresAt.Format("15:04:05"))
			}
		} else {
			cmd.Printf("  互斥锁状态: \033[90m空闲无锁\033[0m\n")
		}

		if res.Manifest.ReceiptHTML != "none" && res.Manifest.ReceiptHTML != "" {
			cmd.Printf("  最新交付物证: \033[94m%s\033[0m\n", res.Manifest.ReceiptHTML)
		}

		if strings.TrimSpace(res.Quartet.NextAction) != "" {
			cmd.Printf("\n  \033[1m[下一步建议]\033[0m:\n    %s\n",
				strings.ReplaceAll(strings.TrimSpace(res.Quartet.NextAction), "\n", "\n    "))
		}

		if len(res.RecentTimeline) > 0 {
			cmd.Printf("\n  \033[1m[最近交接记录]\033[0m:\n")
			for _, row := range res.RecentTimeline {
				cmd.Printf("    %s\n", row)
			}
		}

		return nil
	},
}

var taskClaimCmd = &cobra.Command{
	Use:   "claim",
	Short: "认领任务并抢占租约互斥锁",
	RunE: func(cmd *cobra.Command, args []string) error {
		agent := flagTaskAgent
		if agent == "" {
			agent = relay.DetectCurrentAgent(".")
		}

		manifest, lock, err := relay.ClaimTask(".", agent, flagTaskForce)
		if err != nil {
			return fmt.Errorf("认领任务失败: %w", err)
		}

		cmd.Printf("\033[92m[+] 任务认领成功！\033[0m\n")
		cmd.Printf("  任务编号: %s (%s)\n", manifest.TaskId, manifest.Title)
		cmd.Printf("  当前执行者: \033[1m%s\033[0m\n", agent)
		cmd.Printf("  租约锁已生效至: \033[93m%s\033[0m (防多 Agent 冲突)\n", lock.ExpiresAt.Format("15:04:05"))
		return nil
	},
}

var taskHandoverCmd = &cobra.Command{
	Use:   "handover",
	Short: "执行安全自检、编译 HTML 物证并流转至交接就绪状态",
	RunE: func(cmd *cobra.Command, args []string) error {
		agent := flagTaskAgent
		if agent == "" {
			agent = relay.DetectCurrentAgent(".")
		}

		cmd.Printf("\033[94m[agate task] [%s] 正在准备交接并编译可核验物证...\033[0m\n", agent)

		var testPassed = true
		var testOutput = ""
		if !flagTaskSkipVerify {
			cmd.Printf("  [1/2] 调度动态自检套件与安全门禁...\n")
			passed, summary, err := runDynamicTests(".", false)
			if err != nil || !passed {
				return fmt.Errorf("交接拒绝: 动态自检未通过 (%s)", summary)
			}
			testPassed = passed
			testOutput = summary
		} else {
			cmd.Printf("  \033[93m[提示] 启用 --skip-verify 跳过动态自检\033[0m\n")
		}

		manifest, reportPath, err := relay.HandoverTask(relay.HandoverOptions{
			RootDir:      ".",
			CurrentAgent: agent,
			NextAgent:    flagTaskNextAgent,
			Note:         flagTaskNote,
			SkipVerify:   flagTaskSkipVerify,
			Force:        flagTaskForce,
			TestPassed:   testPassed,
			TestOutput:   testOutput,
		})
		if err != nil {
			return fmt.Errorf("任务交接失败: %w", err)
		}

		cmd.Printf("  \033[92m[✓] 任务交接就绪！状态已流转为 [HANDOVER_READY]\033[0m\n")
		cmd.Printf("  指定接棒目标: \033[1m%s\033[0m\n", manifest.NextAgent)
		cmd.Printf("  可核验证明单: \033[94m%s\033[0m\n", reportPath)
		cmd.Printf("  租约锁已释放，请切换至接棒工具执行 \033[93magate task resume\033[0m\n")
		return nil
	},
}

var taskResumeCmd = &cobra.Command{
	Use:   "resume",
	Short: "新 Agent 唤醒接棒，提取前序断点与接力四要素",
	RunE: func(cmd *cobra.Command, args []string) error {
		agent := flagTaskAgent
		if agent == "" {
			agent = relay.DetectCurrentAgent(".")
		}

		res, err := relay.ResumeTask(".", agent)
		if err != nil {
			return fmt.Errorf("接棒失败: %w", err)
		}

		cmd.Printf("\033[92m=== [agate task] 成功接棒任务: %s ===\033[0m\n", res.Manifest.TaskId)
		cmd.Printf("  任务标题: %s\n", res.Manifest.Title)
		cmd.Printf("  接棒执行者: \033[1m%s\033[0m (状态已流转为 IN_PROGRESS)\n", agent)
		if res.ReceiptHTML != "none" && res.ReceiptHTML != "" {
			cmd.Printf("  前任物证凭单: \033[94m%s\033[0m\n", res.ReceiptHTML)
		}

		cmd.Println("\n\033[1m📋 接力四要素感知基线 (Handover Quartet):\033[0m")
		if strings.TrimSpace(res.Quartet.Done) != "" {
			cmd.Printf("  \033[92m[已完成]\033[0m: %s\n", strings.ReplaceAll(res.Quartet.Done, "\n", " "))
		}
		if strings.TrimSpace(res.Quartet.InProgress) != "" {
			cmd.Printf("  \033[93m[在途断点]\033[0m: %s\n", strings.ReplaceAll(res.Quartet.InProgress, "\n", " "))
		}
		if strings.TrimSpace(res.Quartet.NextAction) != "" {
			cmd.Printf("  \033[96m[下一步建议]\033[0m: %s\n", strings.ReplaceAll(res.Quartet.NextAction, "\n", " "))
		}
		if strings.TrimSpace(res.Quartet.Traps) != "" {
			cmd.Printf("  \033[91m[在途暗坑]\033[0m: %s\n", strings.ReplaceAll(res.Quartet.Traps, "\n", " "))
		}

		cmd.Println("\n\033[90m提示: 租约锁已生效，请严格遵循 3-Hop 寻径开展工作。\033[0m")
		return nil
	},
}

var taskDoneCmd = &cobra.Command{
	Use:   "done",
	Short: "完成任务交付，执行全量门禁自检，流转至 [DONE] 终态并释放锁",
	RunE: func(cmd *cobra.Command, args []string) error {
		agent := flagTaskAgent
		if agent == "" {
			agent = relay.DetectCurrentAgent(".")
		}

		cmd.Printf("\033[94m[agate task] [%s] 正在执行任务终态自检与归档...\033[0m\n", agent)

		var testPassed = true
		var testOutput = ""
		if !flagTaskSkipVerify {
			cmd.Printf("  [1/2] 调度终态动态测试套件与安全门禁...\n")
			passed, summary, err := runDynamicTests(".", false)
			if err != nil || !passed {
				return fmt.Errorf("交付被拒绝: 动态自检未通过 (%s)", summary)
			}
			testPassed = passed
			testOutput = summary
		} else {
			cmd.Printf("  \033[93m[提示] 启用 --skip-verify 跳过动态自检\033[0m\n")
		}

		manifest, reportPath, err := relay.DoneTask(relay.HandoverOptions{
			RootDir:      ".",
			CurrentAgent: agent,
			Note:         flagTaskNote,
			SkipVerify:   flagTaskSkipVerify,
			Force:        flagTaskForce,
			TestPassed:   testPassed,
			TestOutput:   testOutput,
		})
		if err != nil {
			return fmt.Errorf("任务归档失败: %w", err)
		}

		cmd.Printf("  \033[92m[✓] 任务已终态交付！状态流转为 [DONE]\033[0m\n")
		cmd.Printf("  任务编号: %s (%s)\n", manifest.TaskId, manifest.Title)
		cmd.Printf("  交付证明单: \033[94m%s\033[0m\n", reportPath)
		cmd.Printf("  租约锁已彻底释放，工程已具备正式上线/合并条件。\033[0m\n")
		return nil
	},
}

func init() {
	taskClaimCmd.Flags().StringVarP(&flagTaskAgent, "agent", "a", "", "指定当前执行认领的 Agent 名称 (默认智能推断)")
	taskClaimCmd.Flags().BoolVarP(&flagTaskForce, "force", "f", false, "强制认领并覆盖现有租约锁")

	taskHandoverCmd.Flags().StringVarP(&flagTaskAgent, "agent", "a", "", "指定当前交接的 Agent 名称 (默认智能推断)")
	taskHandoverCmd.Flags().StringVarP(&flagTaskNextAgent, "to", "t", "any", "指定接棒 Agent 工具 (默认 any)")
	taskHandoverCmd.Flags().StringVarP(&flagTaskNote, "note", "n", "", "附加交接备忘或下一步建议批注")
	taskHandoverCmd.Flags().BoolVar(&flagTaskSkipVerify, "skip-verify", false, "跳过交接前门禁自检 (应急模式)")
	taskHandoverCmd.Flags().BoolVarP(&flagTaskForce, "force", "f", false, "强制释放他人持有的锁")

	taskDoneCmd.Flags().StringVarP(&flagTaskAgent, "agent", "a", "", "指定当前执行交付的 Agent 名称 (默认智能推断)")
	taskDoneCmd.Flags().StringVarP(&flagTaskNote, "note", "n", "", "附加交付备忘或完成说明")
	taskDoneCmd.Flags().BoolVar(&flagTaskSkipVerify, "skip-verify", false, "跳过最终自检 (应急模式)")
	taskDoneCmd.Flags().BoolVarP(&flagTaskForce, "force", "f", false, "强制释放并完成他人持有的任务")

	taskResumeCmd.Flags().StringVarP(&flagTaskAgent, "agent", "a", "", "指定接棒的 Agent 名称 (默认智能推断)")

	taskCmd.AddCommand(taskStatusCmd)
	taskCmd.AddCommand(taskClaimCmd)
	taskCmd.AddCommand(taskHandoverCmd)
	taskCmd.AddCommand(taskResumeCmd)
	taskCmd.AddCommand(taskDoneCmd)

	rootCmd.AddCommand(taskCmd)
}
