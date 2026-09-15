package cmd

import (
	"fmt"

	"agate/pkg/git"

	"github.com/spf13/cobra"
)

var isolateCmd = &cobra.Command{
	Use:   "isolate",
	Short: "配置 .git/info/exclude 隐形隔离 AI 私有状态",
	Long: `向当前 Git 仓库的本地私有排除配置（.git/info/exclude）注入敏感与 AI 元数据清单。
该操作仅对当前开发者本地生效，不会修改 .gitignore，对团队公共仓库保持零侵入。`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if !git.IsGitRepo() {
			return fmt.Errorf("当前目录不是 Git 仓库根目录")
		}

		addedCount, err := git.ApplyPrivateExclusions(nil)
		if err != nil {
			return fmt.Errorf("应用隐形隔离失败: %w", err)
		}

		if addedCount > 0 {
			fmt.Printf("\033[92m[+] 成功注入 %d 项隔离配置至 .git/info/exclude\033[0m\n", addedCount)
		} else {
			fmt.Println("\033[92m[+] 隔离清单已是最最新状态，无需追加\033[0m")
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(isolateCmd)
}
