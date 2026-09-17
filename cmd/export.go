package cmd

import (
	"fmt"
	"time"

	"agate/pkg/reporter"

	"github.com/spf13/cobra"
)

var (
	flagExportOutput string
	flagExportOpen   bool
	flagExportStaged bool
)

var exportCmd = &cobra.Command{
	Use:   "export",
	Short: "导出自包含单文件 HTML 协同审查与物证报告",
	Long: `将当前工程的阶段任务（TASK.md）、Phase 0 安全红线审计、
自检测试物证及 Git Diff 变动编译为离线单文件 HTML 审查报告。
默认安全输出至 .ai-memory/reviews/，受 Git 隐形隔离保护，团队零污染。`,
	RunE: func(cmd *cobra.Command, args []string) error {
		start := time.Now()
		cmd.Println("\033[94m[agate export] 正在汇聚审计数据并编译自包含 HTML 报告...\033[0m")

		outPath, data, err := reporter.BuildReport(reporter.ReportOptions{
			OutputFile: flagExportOutput,
			Staged:     flagExportStaged,
			Duration:   time.Since(start).Round(time.Millisecond).String(),
		})
		if err != nil {
			return fmt.Errorf("生成审查报告失败: %w", err)
		}

		cmd.Printf("  \033[92m[+] 报告生成成功: %s\033[0m\n", outPath)
		cmd.Printf("  [i] 审查状态: [%s] | 分支: %s | 违规项: %d 处\n",
			data.Status, data.GitBranch, len(data.AuditResult.Violations))

		if flagExportOpen {
			cmd.Printf("  \033[96m[>] 正在启动系统浏览器查看报告...\033[0m\n")
			if err := reporter.OpenBrowser(outPath); err != nil {
				cmd.Printf("  \033[93m[!] 自动启动浏览器失败 (%v)，请手动在浏览器打开文件。\033[0m\n", err)
			}
		}

		return nil
	},
}

func init() {
	exportCmd.Flags().StringVarP(&flagExportOutput, "output", "o", "", "自定义输出的 HTML 文件路径（默认保存在 .ai-memory/reviews/）")
	exportCmd.Flags().BoolVar(&flagExportOpen, "open", false, "报告生成后自动在默认浏览器中打开预览")
	exportCmd.Flags().BoolVar(&flagExportStaged, "staged", false, "仅审查暂存区 (Staged) 改动与 Diff")
	rootCmd.AddCommand(exportCmd)
}
