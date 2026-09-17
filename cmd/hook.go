package cmd

import (
	"fmt"

	"agate/pkg/git"

	"github.com/spf13/cobra"
)

var hookCmd = &cobra.Command{
	Use:   "hook",
	Short: "管理本地 Git 门禁钩子（pre-commit 提纯自检 + pre-push 防偷跑物理阻断）",
}

var hookInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "安装本地 Git 物理双门禁（pre-commit 验证 + pre-push 阻断）",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := git.InstallHooks(); err != nil {
			return fmt.Errorf("挂载钩子失败: %w", err)
		}
		cmd.Println("\033[92m[+] 成功挂载本地 Git 双重物理门禁:\033[0m")
		cmd.Println("    - \033[96mpre-commit\033[0m: 调度 agate verify 执行代码与纯净度自检")
		cmd.Println("    - \033[96mpre-push\033[0m:   物理阻断自动化/AI静默推流，锁定提交权归用户")
		return nil
	},
}

var hookUninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "卸载并移除本地 core.hooksPath 与物理钩子",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := git.UninstallHooks(); err != nil {
			return fmt.Errorf("卸载钩子失败: %w", err)
		}
		cmd.Println("\033[92m[+] 成功卸载私有 Git 物理钩子配置\033[0m")
		return nil
	},
}

var hookStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "查看当前 Git 仓库本地门禁挂载状态",
	RunE: func(cmd *cobra.Command, args []string) error {
		status, err := git.GetHookStatus()
		if err != nil {
			return fmt.Errorf("获取门禁状态失败: %w", err)
		}
		if !status.IsGitRepo {
			cmd.Println("\033[93m[!] 当前目录不是有效 Git 仓库或工作树\033[0m")
			return nil
		}
		cmd.Println("=== [Git 本地物理门禁状态] ===")
		hooksPathDesc := status.HooksPath
		if hooksPathDesc == "" {
			hooksPathDesc = ".git/hooks (Git 默认)"
		}
		cmd.Printf("  • core.hooksPath: \033[96m%s\033[0m\n", hooksPathDesc)

		printHookLine := func(name string, exists, isAgate bool) {
			if !exists {
				cmd.Printf("  • %-12s: \033[90m未安装\033[0m\n", name)
			} else if isAgate {
				cmd.Printf("  • %-12s: \033[92m已挂载 (Agate 托管)\033[0m\n", name)
			} else {
				cmd.Printf("  • %-12s: \033[93m已存在 (第三方脚本)\033[0m\n", name)
			}
		}

		printHookLine("pre-commit", status.PreCommitExists, status.PreCommitAgate)
		printHookLine("pre-push", status.PrePushExists, status.PrePushAgate)
		return nil
	},
}

func init() {
	hookCmd.AddCommand(hookInstallCmd)
	hookCmd.AddCommand(hookUninstallCmd)
	hookCmd.AddCommand(hookStatusCmd)
	rootCmd.AddCommand(hookCmd)
}
