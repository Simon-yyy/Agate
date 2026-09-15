package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	version = "0.1.0"
)

var rootCmd = &cobra.Command{
	Use:   "agate",
	Short: "agate - AI 研发协同防护与门禁治理底座",
	Long: `agate (Agent Gate)
零侵入、跨平台的 AI 原生开发工程护栏与安全闸门。
提供规则分发、Git 隐形隔离、三层门禁约束与自动化闭环自检物证。`,
	Version: version,
}

// Execute 暴露给 main 函数调用
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "\033[91m执行失败: %v\033[0m\n", err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.SetVersionTemplate("agate version {{.Version}}\n")
}
