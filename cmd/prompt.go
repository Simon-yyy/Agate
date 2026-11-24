package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"agate/pkg/git"
	"agate/pkg/prompt"

	"github.com/spf13/cobra"
)

var (
	flagPromptJSON        bool
	flagPromptHook        bool
	flagPromptScopes      []string
	flagPromptIgnoreDirty bool
)

var promptCmd = &cobra.Command{
	Use:   "prompt [instruction]",
	Short: "自然语言意图物理锚定与结构化 Prompt 编译器",
	Long: `agate prompt 将口语化、简短自然语言指令与本地 Git 现场、MAP.md 架构地图、
TASK.md 任务看板与 MEMORY.md 避坑卡片进行确定性动态锚定，生成符合工程两阶段门禁规范的高密度结构化 Prompt。
支持通过 --scope (-s) 显式指定或纠偏目标文件/模块，或通过 --no-dirty 忽略工作区无关脏文件干扰。`,
	RunE: func(cmd *cobra.Command, args []string) error {
		isJSON, _ := cmd.Flags().GetBool("json")
		isHook, _ := cmd.Flags().GetBool("hook")
		scopeChanged := cmd.Flags().Changed("scope")
		scopes, _ := cmd.Flags().GetStringSlice("scope")
		ignoreDirty, _ := cmd.Flags().GetBool("no-dirty")
		defer func() {
			flagPromptJSON = false
			flagPromptHook = false
			flagPromptScopes = nil
			flagPromptIgnoreDirty = false
			_ = cmd.Flags().Set("json", "false")
			_ = cmd.Flags().Set("hook", "false")
			_ = cmd.Flags().Set("no-dirty", "false")
			_ = cmd.Flags().Set("host", "")
			_ = cmd.Flags().Set("event", "")
			_ = cmd.Flags().Set("set-diagnosis", "")
			_ = cmd.Flags().Set("request-id", "")
			_ = cmd.Flags().Set("evidence", "")
			for _, name := range []string{"json", "hook", "scope", "no-dirty", "host", "event", "set-diagnosis", "request-id", "evidence"} {
				if f := cmd.Flags().Lookup(name); f != nil {
					f.Changed = false
				}
			}
		}()

		if scopeChanged && len(scopes) == 0 {
			return &UsageError{Err: errors.New("--scope (-s) 参数不能为空白字符，请传入合法的仓库内相对路径或模块名")}
		}

		setDiag, _ := cmd.Flags().GetString("set-diagnosis")
		reqID, _ := cmd.Flags().GetString("request-id")
		evidence, _ := cmd.Flags().GetString("evidence")
		setDiag = strings.TrimSpace(setDiag)
		if setDiag != "" {
			reqID = strings.TrimSpace(reqID)
			if reqID == "" {
				return &UsageError{Err: errors.New("--set-diagnosis 必须同时指定 --request-id")}
			}
			commonDir, dirErr := git.GetGitCommonDir(".")
			if dirErr != nil {
				return dirErr
			}
			recordDir := filepath.Join(commonDir, "agate", "requests")
			updated, diagErr := prompt.UpdateDiagnosisState(recordDir, reqID, prompt.DiagnosisStatus(setDiag), evidence)
			if diagErr != nil {
				return diagErr
			}
			cmd.Printf("[i] 请求 %s 诊断状态已更新为 %s\n", updated.RequestID, updated.Diagnosis)
			return nil
		}

		host, _ := cmd.Flags().GetString("host")
		eventName, _ := cmd.Flags().GetString("event")
		host = strings.TrimSpace(host)
		eventName = strings.TrimSpace(eventName)
		if host != "" || eventName != "" {
			if host == "" || eventName == "" || !isHook {
				return &UsageError{Err: errors.New("宿主原生模式必须同时指定 --hook、--host 和 --event")}
			}
			if isJSON {
				return &UsageError{Err: errors.New("宿主原生模式不能与 --json 同时使用")}
			}
			raw, readErr := io.ReadAll(cmd.InOrStdin())
			if readErr != nil {
				return readErr
			}
			workDir, err := os.Getwd()
			if err != nil {
				return err
			}
			if _, ok := prompt.GetNativeAdapter(host); ok {
				res, handleErr := prompt.HandleNativeEvent(prompt.NativeEventOptions{
					Host:      host,
					EventName: eventName,
					Payload:   raw,
					WorkDir:   workDir,
				})
				if res.Stderr != "" {
					fmt.Fprintln(cmd.ErrOrStderr(), res.Stderr)
				}
				if len(res.Stdout) > 0 {
					cmd.Print(string(res.Stdout))
				}
				// 宿主协议退出码优先（如 Windsurf 预写拒绝为 2），避免被通用错误映射成 1
				if res.ExitCode != 0 {
					os.Exit(res.ExitCode)
				}
				if handleErr != nil {
					if errors.Is(handleErr, prompt.ErrUnsupportedHookTarget) {
						return &UsageError{Err: handleErr}
					}
					return handleErr
				}
				return nil
			}

			result, handleErr := prompt.HandleCursorEvent(prompt.CursorEventOptions{
				Host:    host,
				Event:   eventName,
				Payload: raw,
				WorkDir: workDir,
			})
			if result.Stderr != "" {
				fmt.Fprintln(cmd.ErrOrStderr(), result.Stderr)
			}
			if len(result.Stdout) > 0 {
				cmd.Print(string(result.Stdout))
			}
			if handleErr != nil {
				if errors.Is(handleErr, prompt.ErrUnsupportedHookTarget) {
					return &UsageError{Err: handleErr}
				}
				return handleErr
			}
			return nil
		}

		var instruction string
		if len(args) > 0 {
			instruction = strings.Join(args, " ")
		} else if isHook {
			// Hook 模式下优先从标准输入流读取
			in := cmd.InOrStdin()
			bytes, err := io.ReadAll(in)
			if err == nil && len(bytes) > 0 {
				instruction = extractInstructionFromPayload(string(bytes))
			}
		}

		instruction = strings.TrimSpace(instruction)
		if instruction == "" {
			return &UsageError{Err: errors.New("缺少自然语言指令内容，请传入指令或在 hook 模式下通过标准输入传入")}
		}

		res, err := prompt.CompilePrompt(prompt.CompileOptions{
			Instruction: instruction,
			Scopes:      scopes,
			IgnoreDirty: ignoreDirty,
			JSONOutput:  isJSON,
			HookMode:    isHook,
		})
		if err != nil {
			if strings.Contains(err.Error(), "--scope") {
				return &UsageError{Err: err}
			}
			return err
		}

		if isJSON {
			jsonStr, formatErr := res.FormatJSON()
			if formatErr != nil {
				return formatErr
			}
			cmd.Println(jsonStr)
			return nil
		}

		cmd.Print(res.CompiledPrompt)
		return nil
	},
}

// extractInstructionFromPayload 尝试从 JSON 载荷（宿主 IDE Hook 格式）或纯文本中提取有效指令
func extractInstructionFromPayload(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if strings.HasPrefix(trimmed, "{") && strings.HasSuffix(trimmed, "}") {
		var payload map[string]interface{}
		if err := json.Unmarshal([]byte(trimmed), &payload); err == nil {
			candidateKeys := []string{"prompt", "user_prompt", "message", "instruction", "content", "text"}
			for _, key := range candidateKeys {
				if val, exists := payload[key]; exists {
					if strVal, ok := val.(string); ok && strings.TrimSpace(strVal) != "" {
						return strings.TrimSpace(strVal)
					}
				}
			}
			// 合法 JSON 对象但未包含有效文本指令时，严禁将原始 JSON（可能含 session_id 等元数据）回退当成指令
			return ""
		}
	}
	return trimmed
}

func init() {
	rootCmd.AddCommand(promptCmd)
	promptCmd.Flags().BoolVar(&flagPromptJSON, "json", false, "以结构化 JSON 格式输出编译结果")
	promptCmd.Flags().BoolVar(&flagPromptHook, "hook", false, "IDE Hook 模式，支持从标准输入读取提问并输出受管信封")
	promptCmd.Flags().StringSliceVarP(&flagPromptScopes, "scope", "s", nil, "显式指定或纠偏物理锚定的目标文件/模块路径（可逗号分隔或重复传入）")
	promptCmd.Flags().BoolVar(&flagPromptIgnoreDirty, "no-dirty", false, "禁用 Git 活跃脏文件兜底关联，防止无关在途改动污染锚定结果")
	promptCmd.Flags().String("host", "", "宿主原生协议名称；当前已核对的值是 cursor")
	promptCmd.Flags().String("event", "", "宿主原生事件；cursor 支持 beforeSubmitPrompt、preToolUse、postToolUse、afterAgentResponse")
	promptCmd.Flags().String("set-diagnosis", "", "校验并更新指定请求的诊断状态 (confirmed|hypothesis|not_applicable)")
	promptCmd.Flags().String("request-id", "", "与 --set-diagnosis 配合使用的请求记录标识")
	promptCmd.Flags().String("evidence", "", "与 --set-diagnosis confirmed 配合使用的诊断说明文本")
}
