package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"agate/pkg/agent"
	"agate/pkg/config"
	"agate/pkg/git"

	"github.com/spf13/cobra"
)

type sanitizedSessionOutput struct {
	Version string `json:"version"`
	Editor  string `json:"editor,omitempty"`
	Runner  string `json:"runner,omitempty"`
	Source  string `json:"source,omitempty"`
}

type detectJSONOutput struct {
	Version         string                  `json:"version"`
	Editor          string                  `json:"editor"`
	EditorStatus    agent.IdentityStatus    `json:"editor_status"`
	Runner          string                  `json:"runner"`
	RunnerStatus    agent.IdentityStatus    `json:"runner_status"`
	Transports      []agent.Transport       `json:"transports"`
	EffectiveTarget string                  `json:"effective_target"`
	HasConflict     bool                    `json:"has_conflict"`
	Candidates      []agent.Candidate       `json:"candidates"`
	Evidences       []agent.Evidence        `json:"evidences"`
	SessionContext  *sanitizedSessionOutput `json:"session_context,omitempty"`
	Warnings        []string                `json:"warnings,omitempty"`
}

func newDetectCommand(inspector agent.ProcessInspector) *cobra.Command {
	var explain bool
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "detect",
		Short: "只读诊断当前宿主编辑器、执行 Agent 及网络传输环境指纹",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			workDir := "."
			if repoRoot, err := git.GetRepoRoot("."); err == nil && repoRoot != "" {
				workDir = repoRoot
			}

			cfg, _, configErr := config.LoadWithGlobal(workDir)
			if configErr != nil {
				sanitizedErr := sanitizeConfigError(configErr)
				if asJSON {
					out := map[string]interface{}{
						"version":      "1.0",
						"config_error": sanitizedErr,
					}
					enc := json.NewEncoder(cmd.OutOrStdout())
					enc.SetIndent("", "  ")
					_ = enc.Encode(out)
					return fmt.Errorf("配置文件加载失败: %s", sanitizedErr)
				}
				cmd.Printf("\033[91m[!] 配置文件加载错误: %s\033[0m\n", sanitizedErr)
				return fmt.Errorf("配置文件加载失败: %s", sanitizedErr)
			}

			termProgram := os.Getenv("TERM_PROGRAM")
			gitIPC := os.Getenv("VSCODE_GIT_IPC_HANDLE")

			ctx := agent.InspectContextFull(os.Getenv, termProgram, gitIPC, cfg.Agent.Profiles, inspector, time.Now())
			effective, _ := ctx.EffectiveTarget()
			hasConflict := ctx.HasConflict()

			var sanitizedSession *sanitizedSessionOutput
			if ctx.SessionContext != nil {
				sanitizedSession = &sanitizedSessionOutput{
					Version: ctx.SessionContext.Version,
					Editor:  ctx.SessionContext.Editor,
					Runner:  ctx.SessionContext.Runner,
					Source:  ctx.SessionContext.Source,
				}
			}

			if asJSON {
				out := detectJSONOutput{
					Version:         "1.0",
					Editor:          ctx.Editor,
					EditorStatus:    ctx.EditorStatus,
					Runner:          ctx.Runner,
					RunnerStatus:    ctx.RunnerStatus,
					Transports:      ctx.Transports,
					EffectiveTarget: effective,
					HasConflict:     hasConflict,
					Candidates:      ctx.Candidates,
					Evidences:       ctx.Evidences,
					SessionContext:  sanitizedSession,
					Warnings:        ctx.Warnings,
				}
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(out)
			}

			cmd.Println("\033[94m=== [agate detect] Agent 协同环境诊断 ===\033[0m")

			runnerDisplay := "未知 (unknown)"
			if ctx.RunnerStatus == agent.StatusKnown {
				runnerDisplay = fmt.Sprintf("\033[92m%s\033[0m (known)", ctx.Runner)
			} else if ctx.RunnerStatus == agent.StatusAmbiguous {
				runnerDisplay = fmt.Sprintf("\033[91m冲突 (ambiguous)\033[0m")
			}
			cmd.Printf("  执行者 (Runner):       %s\n", runnerDisplay)

			editorDisplay := "未知 (unknown)"
			if ctx.EditorStatus == agent.StatusKnown {
				editorDisplay = fmt.Sprintf("\033[92m%s\033[0m (known)", ctx.Editor)
			} else if ctx.EditorStatus == agent.StatusAmbiguous {
				editorDisplay = fmt.Sprintf("\033[91m冲突 (ambiguous)\033[0m")
			}
			cmd.Printf("  宿主编辑器 (Editor):   %s\n", editorDisplay)

			var transportStrs []string
			for _, t := range ctx.Transports {
				transportStrs = append(transportStrs, string(t))
			}
			cmd.Printf("  传输环境 (Transports): %s\n", strings.Join(transportStrs, ", "))

			effectiveDisplay := "\033[90m无 (需显式选择 --targets)\033[0m"
			if effective != "" {
				if hasConflict {
					effectiveDisplay = fmt.Sprintf("\033[92m%s\033[0m \033[93m(注意: 宿主环境存在冲突候选)\033[0m", effective)
				} else {
					effectiveDisplay = fmt.Sprintf("\033[92m%s\033[0m", effective)
				}
			} else if hasConflict {
				effectiveDisplay = "\033[91m存在冲突 (需人工指定 --targets)\033[0m"
			}
			cmd.Printf("  首选装配目标:         %s\n", effectiveDisplay)
			cmd.Printf("  是否存在冲突:         %t\n", hasConflict)

			for _, w := range ctx.Warnings {
				cmd.Printf("  \033[93m[!] 告警: %s\033[0m\n", w)
			}

			if explain {
				if sanitizedSession != nil {
					cmd.Println("\n\033[1m--- 跨会话传递声明上下文 (已脱敏) ---\033[0m")
					cmd.Printf("  协议版本:  %s\n", sanitizedSession.Version)
					if sanitizedSession.Editor != "" {
						cmd.Printf("  声明宿主:  %s\n", sanitizedSession.Editor)
					}
					if sanitizedSession.Runner != "" {
						cmd.Printf("  声明执行:  %s\n", sanitizedSession.Runner)
					}
					if sanitizedSession.Source != "" {
						cmd.Printf("  声明来源:  %s\n", sanitizedSession.Source)
					}
				}

				cmd.Println("\n\033[1m--- 命中的探测证据摘要 (已脱敏) ---\033[0m")
				if len(ctx.Evidences) == 0 {
					cmd.Println("  (当前未命中任何进程专属、白名单路径或终端特征证据)")
				} else {
					for _, ev := range ctx.Evidences {
						cmd.Printf("  - [%s] %s -> %s: %s (%s)\n",
							ev.Source, ev.Key, ev.TargetRole, ev.TargetAgent, ev.Description)
					}
				}

				cmd.Println("\n\033[1m--- 收集到的候选列表 ---\033[0m")
				if len(ctx.Candidates) == 0 {
					cmd.Println("  (无任何候选工具)")
				} else {
					for _, c := range ctx.Candidates {
						cmd.Printf("  - [%s] %s (支持证据数: %d)\n", c.Role, c.Agent, len(c.Evidences))
					}
				}
			} else {
				cmd.Println("\n\033[90m提示: 可通过 `agate detect --explain` 查看详细脱敏证据链，或通过 `--json` 输出机器可读结果。\033[0m")
			}

			return nil
		},
	}
	cmd.Flags().BoolVar(&explain, "explain", false, "显示详细的探测证据与各候选匹配链")
	cmd.Flags().BoolVar(&asJSON, "json", false, "以结构化 JSON 输出诊断结果")
	return cmd
}

func init() {
	rootCmd.AddCommand(newDetectCommand(nil))
}

// sanitizeConfigError 归一化配置错误文本，严禁泄漏绝对私有文件路径或任意敏感输入值。(R3-S01)
func sanitizeConfigError(err error) string {
	if err == nil {
		return ""
	}
	s := err.Error()
	if strings.Contains(s, "格式错误") || strings.Contains(s, "TOML") || strings.Contains(s, "syntax") {
		return "配置文件格式错误"
	}
	if strings.Contains(s, "重复配置项") {
		return "配置文件存在重复配置项"
	}
	if strings.Contains(s, "无法读取") || strings.Contains(s, "打开项目配置失败") || strings.Contains(s, "no such file") {
		return "配置文件读取失败"
	}
	if strings.Contains(s, "未确认生效") || strings.Contains(s, "安全单调收紧") {
		return "配置策略违反安全约束"
	}
	if strings.Contains(s, "仅支持 warning/warn") {
		return "配置策略取值非法（仅支持 warning/warn、error 或 off/ignore）"
	}
	return "配置文件解析异常"
}
