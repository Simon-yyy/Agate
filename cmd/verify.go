package cmd

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"agate/pkg/config"
	"agate/pkg/git"
	"agate/pkg/guard"
	"agate/pkg/harness"
	"agate/pkg/reporter"

	"github.com/spf13/cobra"
)

var (
	flagSkipGuard bool
	flagStrict    bool
	flagStaged    bool
	flagReport    bool
)

var verifyCmd = &cobra.Command{
	Use:   "verify",
	Short: "执行工程闭环自检，输出 PASS 物证",
	Long: `优先调度工程自定义自检脚本（verify.sh 或 verify.cmd）；
若未配置自定义脚本，则自动探测工程类型（Maven / npm / Go 等）执行测试构建；
无任何工程描述文件时，执行通用完备性扫描，确保代码无破坏并输出物证。`,
	RunE: func(cmd *cobra.Command, args []string) error {
		startTime := time.Now()
		fmt.Println("=== [agate verify] 闭环自检 ===")

		// 解析统一的仓库/工程根目录，兼容子目录调用 (AG-016)
		workDir := "."
		if repoRoot, err := git.GetRepoRoot("."); err == nil && repoRoot != "" {
			workDir = repoRoot
		}
		projectConfig, configPath, configErr := config.Load(workDir)
		if configErr != nil {
			return configErr
		}
		if configPath != "" {
			for _, warning := range projectConfig.Warnings {
				fmt.Printf("  \033[93m[i] %s\033[0m\n", warning)
			}
		}

		var latestAuditResult *guard.AuditResult
		triggerReport := func(testPassed bool, testOutput string) {
			if flagReport {
				outPath, _, err := reporter.BuildReport(reporter.ReportOptions{
					RootDir:     workDir,
					Staged:      flagStaged,
					Duration:    time.Since(startTime).Round(time.Millisecond).String(),
					TestPassed:  testPassed,
					TestOutput:  testOutput,
					AuditResult: latestAuditResult,
				})
				if err == nil {
					fmt.Printf("  \033[92m[+] 已生成自包含 HTML 审查物证报告: %s\033[0m\n", outPath)
				}
			}
		}

		// Phase 0: 仓库安全与代码洁癖前置拦截（支持 --skip-guard 应急逃生与 --staged 暂存区定向模式）
		onFail := func(summary string, err error) error {
			elapsed := time.Since(startTime).Round(time.Millisecond)
			fmt.Printf("\033[91m[FAIL] %s (耗时: %v)\033[0m\n", summary, elapsed)
			triggerReport(false, summary)
			streak := recordVerifyFailure(workDir)
			printCircuitBreakerWarning(cmd.OutOrStdout(), streak)
			return err
		}

		onSuccess := func(summary string) error {
			elapsed := time.Since(startTime).Round(time.Millisecond)
			fmt.Printf("\033[92m[PASS] %s (耗时: %v)\033[0m\n", summary, elapsed)
			triggerReport(true, summary)
			recordVerifySuccess(workDir)
			return nil
		}

		// Phase 0 扫描
		if flagSkipGuard {
			fmt.Println("  \033[93m[!] 已启用 --skip-guard 应急模式，跳过 Phase 0 安全红线扫描\033[0m")
		} else if flagStaged {
			stagedFiles, err := git.GetStagedFiles(workDir)
			if err != nil {
				fmt.Println("  \033[93m[!] 无法获取暂存区文件 (当前可能不在 Git 仓库)，平滑回退全量扫描\033[0m")
				auditResult := guard.RunPreflightAudit(workDir)
				latestAuditResult = auditResult
				auditResult.PrintReport()
				if auditResult.HasErrors() {
					return onFail("Phase 0 安全红线拦截", fmt.Errorf("触发安全护栏拦截"))
				}
			} else if len(stagedFiles) == 0 {
				fmt.Println("  [i] 当前 Git 暂存区 (Index) 无待提交文件，跳过自检")
				return onSuccess("暂存区就绪")
			} else {
				fmt.Printf("  [i] 命中 Git 暂存区审查模式，定向审查 %d 个待提交文件...\n", len(stagedFiles))
				auditResult := guard.RunStagedAudit(workDir, stagedFiles)
				latestAuditResult = auditResult
				auditResult.PrintReport()
				if auditResult.HasErrors() {
					return onFail("Phase 0 暂存区安全红线拦截", fmt.Errorf("触发安全护栏拦截"))
				}
			}
		} else {
			auditResult := guard.RunPreflightAudit(workDir)
			latestAuditResult = auditResult
			auditResult.PrintReport()
			if auditResult.HasErrors() {
				return onFail("Phase 0 全量安全红线拦截", fmt.Errorf("触发安全护栏拦截"))
			}
		}

		// Phase 1 动态测试调度。未显式指定 --strict 时继承项目配置；
		// 显式传入 --strict=false 也必须能够覆盖项目中的 strict=true。
		effectiveStrict := projectConfig.Guard.Strict
		if flagStrict || cmd.Flags().Changed("strict") {
			effectiveStrict = flagStrict
		}
		passed, summary, testErr := runDynamicTests(workDir, effectiveStrict)
		if testErr != nil || !passed {
			return onFail("自检未通过，拒绝交付", testErr)
		}

		return onSuccess(summary)
	},
}

// runDynamicTests 调度 Phase 1 动态测试（优先自检脚本，其次框架探测）
func runDynamicTests(workDir string, strict bool) (passed bool, summary string, err error) {
	// 1. 优先探测专有验证脚本
	customScript, shellCmd, shellArgs := detectCustomScript(workDir)
	if customScript != "" {
		fmt.Printf("执行本地自检脚本: %s\n", customScript)
		runErr := runShellCommandInDir(workDir, shellCmd, shellArgs...)
		if runErr != nil {
			if isCommandNotFoundError(runErr) {
				fmt.Printf("\033[93m[提示] 调度本地脚本 %s 失败 (环境未找到 %s 命令)，平滑降级至框架探测\033[0m\n", customScript, shellCmd)
			} else {
				return false, fmt.Sprintf("本地自检脚本 %s 执行失败: %v", customScript, runErr), fmt.Errorf("本地自检未通过: %w", runErr)
			}
		} else {
			return true, fmt.Sprintf("本地自检脚本 %s 验证通过", customScript), nil
		}
	}

	// 2. 探测常见框架构建测试
	if ok, name, shell, args := detectFrameworkTests(workDir); ok {
		fmt.Printf("未配置专用脚本，自动调度 %s 测试套件...\n", name)
		runErr := runShellCommandInDir(workDir, shell, args...)
		if runErr != nil {
			return false, fmt.Sprintf("%s 测试执行失败: %v", name, runErr), fmt.Errorf("%s 测试未通过: %w", name, runErr)
		}
		return true, fmt.Sprintf("%s 测试套件验证通过", name), nil
	}

	// 3. 通用完备性检查（未配置自检脚本且无可用框架测试套件）
	if strict {
		return false, "严格模式拦截: 未检测到自检脚本或测试套件", fmt.Errorf("未检测到自动化测试套件或自检脚本，严格模式拒绝伪交付")
	}

	fmt.Println("\033[93m[WARN] 未检测到自动化测试套件或自检脚本 (verify.sh/cmd)，动态测试已跳过\033[0m")
	return true, "静态合规通过 (未执行动态测试)", nil
}

func detectCustomScript(dir ...string) (script, shell string, args []string) {
	base := "."
	if len(dir) > 0 && dir[0] != "" {
		base = dir[0]
	}
	if runtime.GOOS == "windows" {
		if _, err := os.Stat(filepath.Join(base, "verify.cmd")); err == nil {
			return "verify.cmd", "cmd.exe", []string{"/c", "verify.cmd"}
		}
		if _, err := os.Stat(filepath.Join(base, "verify.bat")); err == nil {
			return "verify.bat", "cmd.exe", []string{"/c", "verify.bat"}
		}
	}

	if _, err := os.Stat(filepath.Join(base, "verify.sh")); err == nil {
		if runtime.GOOS == "windows" {
			return "verify.sh", "bash", []string{"verify.sh"}
		}
		return "verify.sh", "./verify.sh", nil
	}

	return "", "", nil
}

func detectFrameworkTests(dir ...string) (bool, string, string, []string) {
	base := "."
	if len(dir) > 0 && dir[0] != "" {
		base = dir[0]
	}
	if _, err := os.Stat(filepath.Join(base, "pom.xml")); err == nil {
		if _, lookErr := exec.LookPath("mvn"); lookErr != nil {
			fmt.Println("\033[93m[提示] 侦测到 pom.xml 但系统未安装 mvn 命令，按 SPEC 规范平滑降级\033[0m")
		} else {
			return true, "Maven", "mvn", []string{"test-compile", "-q"}
		}
	}
	if _, err := os.Stat(filepath.Join(base, "package.json")); err == nil {
		if _, lookErr := exec.LookPath("npm"); lookErr != nil {
			fmt.Println("\033[93m[提示] 侦测到 package.json 但系统未安装 npm 命令，按 SPEC 规范平滑降级\033[0m")
		} else {
			return true, "npm", "npm", []string{"test", "--if-present"}
		}
	}
	if _, err := os.Stat(filepath.Join(base, "go.mod")); err == nil {
		if _, lookErr := exec.LookPath("go"); lookErr != nil {
			fmt.Println("\033[93m[提示] 侦测到 go.mod 但系统未安装 go 命令，按 SPEC 规范平滑降级\033[0m")
		} else {
			goArgs := []string{"test", "-v", "./..."}
			if _, err := os.Stat(filepath.Join(base, "vendor")); err == nil {
				goArgs = []string{"test", "-mod=vendor", "-v", "./..."}
			}
			return true, "Go", "go", goArgs
		}
	}
	return false, "", "", nil
}

func runShellCommand(command string, args ...string) error {
	return runShellCommandInDir(".", command, args...)
}

func runShellCommandInDir(dir string, command string, args ...string) error {
	cmd := exec.Command(command, args...)
	if dir != "" && dir != "." {
		cmd.Dir = dir
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

func isCommandNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "executable file not found") || strings.Contains(msg, "not found") || strings.Contains(msg, "cannot find the file")
}

func recordVerifySuccess(workDir string) {
	streakFile := filepath.Join(workDir, ".ai-memory", ".verify_streak")
	_ = os.Remove(streakFile)
}

func recordVerifyFailure(workDir string) int {
	streakDir := filepath.Join(workDir, ".ai-memory")
	_ = os.MkdirAll(streakDir, 0755)
	streakFile := filepath.Join(streakDir, ".verify_streak")

	count := 0
	if data, err := os.ReadFile(streakFile); err == nil {
		_, _ = fmt.Sscanf(strings.TrimSpace(string(data)), "%d", &count)
	}
	count++
	_ = harness.WriteFileAtomic(streakFile, []byte(fmt.Sprintf("%d", count)), 0644)
	return count
}

func printCircuitBreakerWarning(out io.Writer, count int) {
	if count >= 2 {
		if out == nil {
			out = os.Stdout
		}
		fmt.Fprintln(out)
		fmt.Fprintln(out, "\033[91m╔════════════════════════════════════════════════════════════════════════════╗\033[0m")
		fmt.Fprintf(out, "\033[91m║ 🛑 触发两振熔断警示 (Two-Strike Circuit Breaker: 连续失败第 %d 次)          ║\033[0m\n", count)
		fmt.Fprintln(out, "\033[91m║ ────────────────────────────────────────────────────────────────────────── ║\033[0m")
		fmt.Fprintln(out, "\033[91m║ 根据 AI 协同工程规约：连续 2 次自检未通过，严禁 Agent 继续盲目重试！       ║\033[0m")
		fmt.Fprintln(out, "\033[91m║ 请立即停止自动化尝试，主动向人类用户发起求助，或重新核对技术方案与契约。   ║\033[0m")
		fmt.Fprintln(out, "\033[91m╚════════════════════════════════════════════════════════════════════════════╝\033[0m")
		fmt.Fprintln(out)
	}
}

func init() {
	verifyCmd.Flags().BoolVar(&flagSkipGuard, "skip-guard", false, "跳过 Phase 0 安全红线与代码洁癖前置扫描")
	verifyCmd.Flags().BoolVar(&flagStrict, "strict", false, "严格模式：要求必须存在并执行有效的测试套件或自检脚本，无测试时拒绝交付")
	verifyCmd.Flags().BoolVar(&flagStaged, "staged", false, "仅针对 Git 暂存区 (Index) 待提交文件进行高精度定向审查")
	verifyCmd.Flags().BoolVar(&flagReport, "report", false, "自检完成后自动在 .ai-memory/reviews/ 生成自包含 HTML 审查物证报告")
	rootCmd.AddCommand(verifyCmd)
}
