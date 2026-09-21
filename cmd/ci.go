package cmd

import "github.com/spf13/cobra"

// ciVerifyCmd 提供面向 CI 的稳定入口，固定启用严格验证与审查报告。
var ciVerifyCmd = &cobra.Command{
	Use:   "verify",
	Short: "在 CI 环境执行严格验证并生成审查报告",
	RunE: func(cmd *cobra.Command, args []string) error {
		oldStrict, oldReport, oldStaged, oldSkipGuard := flagStrict, flagReport, flagStaged, flagSkipGuard
		defer func() {
			flagStrict, flagReport, flagStaged, flagSkipGuard = oldStrict, oldReport, oldStaged, oldSkipGuard
		}()
		flagStrict = true
		flagReport = true
		flagStaged = false
		flagSkipGuard = false
		return verifyCmd.RunE(verifyCmd, args)
	},
}

var ciCmd = &cobra.Command{
	Use:   "ci",
	Short: "CI 环境专用门禁命令",
}

func init() {
	ciCmd.AddCommand(ciVerifyCmd)
	rootCmd.AddCommand(ciCmd)
}
