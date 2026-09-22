package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var (
	version = "0.2.5"
)

// UsageError 标识命令行参数解析或用法错误（SPEC 要求退出码 2）
type UsageError struct {
	Err error
}

func (e *UsageError) Error() string {
	return e.Err.Error()
}

func (e *UsageError) Unwrap() error {
	return e.Err
}

// IsUsageError 判断是否为参数或命令用法错误
func IsUsageError(err error) bool {
	if err == nil {
		return false
	}
	var ue *UsageError
	if errors.As(err, &ue) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "unknown flag") ||
		strings.Contains(msg, "unknown command") ||
		strings.Contains(msg, "unknown shorthand flag") ||
		strings.Contains(msg, "flag needs an argument") ||
		strings.Contains(msg, "bad flag syntax") ||
		strings.Contains(msg, "invalid argument") ||
		strings.Contains(msg, "accepts ") ||
		strings.Contains(msg, "requires ")
}

var rootCmd = &cobra.Command{
	Use:   "agate",
	Short: "agate - AI 研发协同防护与门禁治理底座",
	Long: `agate (Agent Gate)
零侵入、跨平台的 AI 原生开发工程护栏与安全闸门。
提供规则分发、Git 隐形隔离、三层门禁约束与自动化闭环自检物证。`,
	Version: version,
}

// ExecuteWithArgs 执行命令并返回三态退出码 (0: 成功, 1: 失败/门禁拦截, 2: 参数用法错误)
func ExecuteWithArgs(args []string, out io.Writer, errOut io.Writer) int {
	rootCmd.SetArgs(args)
	if out != nil {
		rootCmd.SetOut(out)
	} else {
		rootCmd.SetOut(os.Stdout)
	}
	if errOut != nil {
		rootCmd.SetErr(errOut)
	} else {
		rootCmd.SetErr(os.Stderr)
	}

	err := rootCmd.Execute()
	if err == nil {
		return 0
	}

	if IsUsageError(err) {
		if errOut != nil {
			fmt.Fprintf(errOut, "\033[91m参数或用法错误 (退出码 2): %v\033[0m\n", err)
		}
		return 2
	}

	if errOut != nil {
		fmt.Fprintf(errOut, "\033[91m执行失败 (退出码 1): %v\033[0m\n", err)
	}
	return 1
}

// Execute 暴露给 main 函数调用
func Execute() {
	exitCode := ExecuteWithArgs(os.Args[1:], os.Stdout, os.Stderr)
	os.Exit(exitCode)
}

func init() {
	rootCmd.SetVersionTemplate("agate version {{.Version}}\n")
	rootCmd.SilenceUsage = true
	rootCmd.SilenceErrors = true
	rootCmd.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error {
		return &UsageError{Err: err}
	})
}
