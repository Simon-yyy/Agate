package cmd

import (
	"fmt"

	"agate/pkg/git"

	"github.com/spf13/cobra"
)

var hookCmd = &cobra.Command{
	Use:   "hook",
	Short: "管理本地 Git Pre-commit 验证门禁钩子",
}

var hookInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "安装本地 pre-commit 钩子并重定向至 agate verify",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := git.InstallPreCommitHook(); err != nil {
			return fmt.Errorf("挂载钩子失败: %w", err)
		}
		fmt.Println("\033[92m[+] 成功挂载本地 pre-commit 门禁 -> agate verify\033[0m")
		return nil
	},
}

var hookUninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "卸载并移除本地 core.hooksPath 与 pre-commit 钩子",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := git.UninstallPreCommitHook(); err != nil {
			return fmt.Errorf("卸载钩子失败: %w", err)
		}
		fmt.Println("\033[92m[+] 成功卸载私有 Git 钩子配置\033[0m")
		return nil
	},
}

func init() {
	hookCmd.AddCommand(hookInstallCmd)
	hookCmd.AddCommand(hookUninstallCmd)
	rootCmd.AddCommand(hookCmd)
}
