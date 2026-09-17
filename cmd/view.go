package cmd

import (
	"fmt"
	"time"

	"agate/pkg/reporter"

	"github.com/spf13/cobra"
)

var viewCmd = &cobra.Command{
	Use:   "view",
	Short: "在浏览器中查看最新的自包含 HTML 协同审查报告",
	Long: `自动寻找 .ai-memory/reviews/ 目录下最新生成的 HTML 审查报告并在浏览器中打开。
若尚未生成过任何报告，则自动现场生成一份最新审查报告并打开。`,
	RunE: func(cmd *cobra.Command, args []string) error {
		latestPath, err := reporter.FindLatestReport(".")
		if err != nil {
			// 若尚未生成过报告，自动现场执行导出
			cmd.Println("\033[93m[i] 未检测到既有审查报告，正在现场生成最新物证报告...\033[0m")
			start := time.Now()
			outPath, _, bErr := reporter.BuildReport(reporter.ReportOptions{
				Duration: time.Since(start).Round(time.Millisecond).String(),
			})
			if bErr != nil {
				return fmt.Errorf("自动生成审查报告失败: %w", bErr)
			}
			latestPath = outPath
		}

		cmd.Printf("\033[92m[>] 正在打开审查报告: %s\033[0m\n", latestPath)
		if err := reporter.OpenBrowser(latestPath); err != nil {
			cmd.Printf("\033[93m[!] 自动打开浏览器失败 (%v)，请手动在浏览器打开上述文件。\033[0m\n", err)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(viewCmd)
}
