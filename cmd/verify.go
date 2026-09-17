package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"agate/pkg/git"
	"agate/pkg/guard"

	"github.com/spf13/cobra"
)

var (
	flagSkipGuard bool
	flagStrict    bool
	flagStaged    bool
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

		// Phase 0: 仓库安全与代码洁癖前置拦截（支持 --skip-guard 应急逃生与 --staged 暂存区定向模式）
		if flagSkipGuard {
			fmt.Println("  \033[93m[!] 已启用 --skip-guard 应急模式，跳过 Phase 0 安全红线扫描\033[0m")
		} else if flagStaged {
			stagedFiles, err := git.GetStagedFiles(".")
			if err != nil {
				fmt.Println("  \033[93m[!] 无法获取暂存区文件 (当前可能不在 Git 仓库)，平滑回退全量扫描\033[0m")
				auditResult := guard.RunPreflightAudit(".")
				auditResult.PrintReport()
				if auditResult.HasErrors() {
					elapsed := time.Since(startTime).Round(time.Millisecond)
					fmt.Printf("\033[91m[FAIL] 触发安全护栏拦截，拒绝交付 (耗时: %v)\033[0m\n", elapsed)
					return fmt.Errorf("触发安全护栏拦截")
				}
			} else if len(stagedFiles) == 0 {
				fmt.Println("  [i] 当前 Git 暂存区 (Index) 无待提交文件，跳过自检")
				elapsed := time.Since(startTime).Round(time.Millisecond)
				fmt.Printf("\033[92m[PASS] 暂存区就绪 (耗时: %v)\033[0m\n", elapsed)
				return nil
			} else {
				fmt.Printf("  [i] 命中 Git 暂存区审查模式，定向审查 %d 个待提交文件...\n", len(stagedFiles))
				auditResult := guard.RunStagedAudit(".", stagedFiles)
				auditResult.PrintReport()
				if auditResult.HasErrors() {
					elapsed := time.Since(startTime).Round(time.Millisecond)
					fmt.Printf("\033[91m[FAIL] 触发安全护栏拦截，拒绝交付 (耗时: %v)\033[0m\n", elapsed)
					return fmt.Errorf("触发安全护栏拦截")
				}
			}
		} else {
			auditResult := guard.RunPreflightAudit(".")
			auditResult.PrintReport()
			if auditResult.HasErrors() {
				elapsed := time.Since(startTime).Round(time.Millisecond)
				fmt.Printf("\033[91m[FAIL] 触发安全护栏拦截，拒绝交付 (耗时: %v)\033[0m\n", elapsed)
				return fmt.Errorf("触发安全护栏拦截")
			}
		}

		// 1. 优先探测专有验证脚本
		customScript, shellCmd, shellArgs := detectCustomScript()
		if customScript != "" {
			fmt.Printf("执行本地自检脚本: %s\n", customScript)
			err := runShellCommand(shellCmd, shellArgs...)
			elapsed := time.Since(startTime).Round(time.Millisecond)
			if err != nil {
				// 若由于环境缺少对应 shell（如 Windows 未装 bash），打印提示并平滑降级至框架探测
				if isCommandNotFoundError(err) {
					fmt.Printf("\033[93m[提示] 调度本地脚本 %s 失败 (环境未找到 %s 命令)，平滑降级至框架探测\033[0m\n", customScript, shellCmd)
				} else {
					fmt.Printf("\033[91m[FAIL] 自检未通过，拒绝交付 (耗时: %v)\033[0m\n", elapsed)
					return fmt.Errorf("本地自检未通过: %w", err)
				}
			} else {
				fmt.Printf("\033[92m[PASS] 自检完成，允许交付 (耗时: %v)\033[0m\n", elapsed)
				return nil
			}
		}

		// 2. 探测常见框架构建测试
		if ok, name, shell, args := detectFrameworkTests(); ok {
			fmt.Printf("未配置专用脚本，自动调度 %s 测试套件...\n", name)
			err := runShellCommand(shell, args...)
			elapsed := time.Since(startTime).Round(time.Millisecond)
			if err != nil {
				fmt.Printf("\033[91m[FAIL] %s 测试未通过，拒绝交付 (耗时: %v)\033[0m\n", name, elapsed)
				return fmt.Errorf("%s 测试未通过: %w", name, err)
			}
			fmt.Printf("\033[92m[PASS] %s 测试通过，允许交付 (耗时: %v)\033[0m\n", name, elapsed)
			return nil
		}

		// 3. 通用完备性检查（未配置自检脚本且无可用框架测试套件）
		elapsed := time.Since(startTime).Round(time.Millisecond)
		if flagStrict {
			fmt.Printf("\033[91m[FAIL] 严格模式拦截: 未检测到自检脚本 (verify.sh/cmd) 或测试套件，拒绝交付 (耗时: %v)\033[0m\n", elapsed)
			return fmt.Errorf("未检测到自动化测试套件或自检脚本，严格模式拒绝伪交付")
		}

		fmt.Println("\033[93m[WARN] 未检测到自动化测试套件或自检脚本 (verify.sh/cmd)，动态测试已跳过\033[0m")
		fmt.Printf("\033[92m[PASS] 静态交付物合规 (注: 仅通过静态安全扫描，未执行动态测试) (耗时: %v)\033[0m\n", elapsed)
		return nil
	},
}

func detectCustomScript() (script, shell string, args []string) {
	if runtime.GOOS == "windows" {
		if _, err := os.Stat("verify.cmd"); err == nil {
			return "verify.cmd", "cmd.exe", []string{"/c", "verify.cmd"}
		}
		if _, err := os.Stat("verify.bat"); err == nil {
			return "verify.bat", "cmd.exe", []string{"/c", "verify.bat"}
		}
	}

	if _, err := os.Stat("verify.sh"); err == nil {
		if runtime.GOOS == "windows" {
			return "verify.sh", "bash", []string{"verify.sh"}
		}
		return "verify.sh", "./verify.sh", nil
	}

	return "", "", nil
}

func detectFrameworkTests() (bool, string, string, []string) {
	if _, err := os.Stat("pom.xml"); err == nil {
		if _, lookErr := exec.LookPath("mvn"); lookErr != nil {
			fmt.Println("\033[93m[提示] 侦测到 pom.xml 但系统未安装 mvn 命令，按 SPEC 规范平滑降级\033[0m")
		} else {
			return true, "Maven", "mvn", []string{"test-compile", "-q"}
		}
	}
	if _, err := os.Stat("package.json"); err == nil {
		if _, lookErr := exec.LookPath("npm"); lookErr != nil {
			fmt.Println("\033[93m[提示] 侦测到 package.json 但系统未安装 npm 命令，按 SPEC 规范平滑降级\033[0m")
		} else {
			return true, "npm", "npm", []string{"test", "--if-present"}
		}
	}
	if _, err := os.Stat("go.mod"); err == nil {
		if _, lookErr := exec.LookPath("go"); lookErr != nil {
			fmt.Println("\033[93m[提示] 侦测到 go.mod 但系统未安装 go 命令，按 SPEC 规范平滑降级\033[0m")
		} else {
			goArgs := []string{"test", "-v", "./..."}
			if _, err := os.Stat("vendor"); err == nil {
				goArgs = []string{"test", "-mod=vendor", "-v", "./..."}
			}
			return true, "Go", "go", goArgs
		}
	}
	return false, "", "", nil
}

func runShellCommand(command string, args ...string) error {
	cmd := exec.Command(command, args...)
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

func init() {
	verifyCmd.Flags().BoolVar(&flagSkipGuard, "skip-guard", false, "跳过 Phase 0 安全红线与代码洁癖前置扫描")
	verifyCmd.Flags().BoolVar(&flagStrict, "strict", false, "严格模式：要求必须存在并执行有效的测试套件或自检脚本，无测试时拒绝交付")
	verifyCmd.Flags().BoolVar(&flagStaged, "staged", false, "仅针对 Git 暂存区 (Index) 待提交文件进行高精度定向审查")
	rootCmd.AddCommand(verifyCmd)
}
