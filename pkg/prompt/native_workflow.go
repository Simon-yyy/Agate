package prompt

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// ParseNativePlan 解析模型回复中的 ```agate-plan 标准声明块。
func ParseNativePlan(response, workspace string) *NativePlanCandidate {
	const opening = "\n```agate-plan\n"
	text := "\n" + strings.ReplaceAll(response, "\r\n", "\n")
	if strings.Count(text, opening) != 1 {
		return nil
	}
	_, block, _ := strings.Cut(text, opening)
	body, rest, closed := strings.Cut(block+"\n", "\n```\n")
	if !closed || !strings.Contains(rest, "执行") {
		return nil
	}
	var raw struct {
		Diagnosis DiagnosisStatus `json:"diagnosis"`
		Files     []string        `json:"files"`
	}
	decoder := json.NewDecoder(strings.NewReader(body))
	decoder.DisallowUnknownFields()
	if !json.Valid([]byte(body)) || decoder.Decode(&raw) != nil || len(raw.Files) == 0 ||
		(raw.Diagnosis != DiagnosisConfirmed && raw.Diagnosis != DiagnosisNotApplicable) {
		return nil
	}
	if raw.Diagnosis == DiagnosisConfirmed {
		if err := ValidateDiagnosisOutput(response, raw.Diagnosis); err != nil {
			return nil
		}
	}
	summary, _, _ := strings.Cut(response, strings.TrimPrefix(opening, "\n"))
	cleanedFiles := make([]string, 0, len(raw.Files))
	for _, file := range raw.Files {
		clean := filepath.Clean(filepath.FromSlash(file))
		if strings.TrimSpace(file) != file || clean == "." || !filepath.IsLocal(clean) {
			return nil
		}
		cleanedFiles = append(cleanedFiles, filepath.ToSlash(clean))
	}
	return &NativePlanCandidate{
		PlanSummary: strings.TrimSpace(summary),
		Diagnosis:   raw.Diagnosis,
		Files:       cleanedFiles,
		CreatedAt:   time.Now().UTC(),
	}
}

// IsTargetInConfirmedScope 判断 target 是否完全在 confirmedFiles 授权范围之内。
func IsTargetInConfirmedScope(target string, confirmedFiles []string, workspace string) bool {
	if strings.TrimSpace(target) == "" || strings.TrimSpace(workspace) == "" || len(confirmedFiles) == 0 {
		return false
	}
	root, err := filepath.Abs(workspace)
	if err != nil {
		return false
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(root, target)
	}
	normTarget, err := filepath.Abs(target)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(root, normTarget)
	if err != nil || rel == "." || !filepath.IsLocal(rel) {
		return false
	}
	current := root
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			break
		}
		if err != nil || info.Mode()&os.ModeSymlink != 0 {
			return false
		}
		if current == normTarget && !info.Mode().IsRegular() {
			return false
		}
	}
	for _, cf := range confirmedFiles {
		if strings.TrimSpace(cf) == "" {
			continue
		}
		if !filepath.IsAbs(cf) {
			cf = filepath.Join(root, cf)
		}
		confirmed, err := filepath.Abs(cf)
		if err == nil && (normTarget == confirmed || (runtime.GOOS == "windows" && strings.EqualFold(normTarget, confirmed))) {
			return true
		}
	}
	return false
}

// isContextReadHit 判定目标事件是否读到了当前轮生成的上下文文件。
func isContextReadHit(targets []string, expectedContextPath string) bool {
	if len(targets) == 0 || strings.TrimSpace(expectedContextPath) == "" {
		return false
	}
	expectedNorm, err := filepath.Abs(filepath.Clean(expectedContextPath))
	if err != nil {
		return false
	}
	for _, raw := range targets {
		if strings.TrimSpace(raw) == "" {
			continue
		}
		norm, err := filepath.Abs(filepath.Clean(raw))
		if err != nil {
			continue
		}
		if norm == expectedNorm || (runtime.GOOS == "windows" && strings.EqualFold(norm, expectedNorm)) {
			return true
		}
	}
	return false
}

func sameWorkspacePath(a, b string) bool {
	absA, errA := filepath.Abs(filepath.Clean(a))
	absB, errB := filepath.Abs(filepath.Clean(b))
	if errA != nil || errB != nil {
		return false
	}
	if absA == absB {
		return true
	}
	return runtime.GOOS == "windows" && strings.EqualFold(absA, absB)
}

func claimedRootsConflict(workspace string, claimed []string) bool {
	if len(claimed) == 0 {
		return false
	}
	for _, root := range claimed {
		if strings.TrimSpace(root) == "" {
			continue
		}
		if !sameWorkspacePath(workspace, root) {
			return true
		}
	}
	return false
}

func clearNativeApproval(snap *NativeSnapshot) {
	snap.PlanConfirmed = false
	snap.ConfirmedFiles = nil
	snap.ConfirmedTurnID = ""
	snap.ConfirmEventID = ""
	snap.ApprovedAt = nil
	snap.Consumed = false
}

func clearNativePlan(snap *NativeSnapshot) {
	snap.PlanCandidate = nil
	snap.Diagnosis = DiagnosisUndiagnosed
	snap.DiagnosisSummary = ""
}

const mcpDenyReason = "MCP 不能借用文件方案授权"

func requireControlledSession(event NativeEvent, opts NativeEventOptions, snap NativeSnapshot, workspace string) (NativeDecision, bool) {
	if strings.TrimSpace(event.SessionID) == "" {
		return NativeDecision{Block: true, Reason: "缺少会话标识，拒绝受控写入"}, true
	}
	if claimedRootsConflict(workspace, event.ClaimedRoots) {
		return NativeDecision{Block: true, Reason: "声明工作区与当前根目录冲突，拒绝受控写入"}, true
	}
	if snap.Host != "" && opts.Host != "" && !strings.EqualFold(snap.Host, opts.Host) {
		return NativeDecision{Block: true, Reason: "宿主标识与会话快照不一致，拒绝受控写入"}, true
	}
	if snap.WorkspaceRoot != "" && !sameWorkspacePath(snap.WorkspaceRoot, workspace) {
		return NativeDecision{Block: true, Reason: "工作区根目录冲突，拒绝受控写入"}, true
	}
	return NativeDecision{}, false
}

// ApplyNativeWorkflow 实现多宿主通用的原生生命周期流转与写入判定。
func ApplyNativeWorkflow(opts NativeEventOptions, event NativeEvent) (NativeDecision, error) {
	workspace, err := filepath.Abs(firstText(opts.WorkDir, opts.RootDir, "."))
	if err != nil {
		return NativeDecision{Block: true, Reason: "无法确定工作区路径"}, err
	}
	storeDir, err := ResolveNativeStoreDir(firstText(opts.RootDir, workspace), opts.Host, workspace, opts.RecordDir)
	if err != nil {
		return NativeDecision{Block: true, Reason: "无法解析会话存储目录"}, err
	}
	store := NewNativeStore(storeDir)

	sessionID := strings.TrimSpace(event.SessionID)
	if sessionID == "" {
		// 受控操作必须带会话标识。读取可以没有会话。
		if event.Kind == NativeEventBeforeTool {
			if event.Action == NativeActionRead {
				return NativeDecision{Block: false}, nil
			}
			return NativeDecision{Block: true, Reason: "缺少会话标识，拒绝受控写入"}, fmt.Errorf("缺少会话标识")
		}
		if event.Kind == NativeEventSubmit || event.Kind == NativeEventReadCompleted || event.Kind == NativeEventResponseCompleted {
			return NativeDecision{Block: true, Reason: "缺少会话标识"}, fmt.Errorf("缺少会话标识")
		}
		return NativeDecision{Block: false}, nil
	}

	var decision NativeDecision
	var actionErr error

	lockErr := store.WithLock(sessionID, func() error {
		snap, err := store.LoadSnapshot(sessionID, opts.Host, workspace)
		if err != nil {
			decision = NativeDecision{Block: true, Reason: err.Error()}
			actionErr = err
			return nil
		}
		if snap.Host == "" {
			snap.Host = opts.Host
		}
		if snap.WorkspaceRoot == "" {
			snap.WorkspaceRoot = workspace
		}
		if snap.SessionID == "" {
			snap.SessionID = sessionID
		}

		switch event.Kind {
		case NativeEventSubmit:
			if claimedRootsConflict(workspace, event.ClaimedRoots) {
				decision = NativeDecision{Block: true, Reason: "声明工作区与当前根目录冲突"}
				actionErr = fmt.Errorf("工作区冲突")
				return nil
			}

			instruction := strings.TrimSpace(event.Text)
			confirmTurn := firstText(event.TurnID, event.EventID)
			isConfirm := event.TrustedUser && instruction == "执行"

			// 同一条确认提交重复到达：保持幂等，不更换批准范围
			if isConfirm && snap.PlanConfirmed && snap.ConfirmEventID != "" && event.EventID != "" &&
				snap.ConfirmEventID == event.EventID && snap.ConfirmedTurnID == confirmTurn {
				decision = NativeDecision{Block: false, AdditionalContext: snap.ContextPath}
				return nil
			}

			canConsume := isConfirm && snap.PlanCandidate != nil && len(snap.PlanCandidate.Files) > 0 &&
				snap.ObservedRead && !snap.Consumed

			// 新的用户提交先清除旧批准，再判断是否消费当前待确认方案
			if canConsume {
				now := time.Now().UTC()
				snap.PlanConfirmed = true
				snap.ConfirmedFiles = append([]string(nil), snap.PlanCandidate.Files...)
				snap.ConfirmEventID = event.EventID
				snap.ApprovedAt = &now
				snap.Consumed = true
				snap.Diagnosis = snap.PlanCandidate.Diagnosis
				snap.DiagnosisSummary = snap.PlanCandidate.PlanSummary
			} else {
				clearNativeApproval(&snap)
				if !isConfirm {
					clearNativePlan(&snap)
				} else {
					// 没有新方案的「执行」不得沿用旧范围
					clearNativePlan(&snap)
				}
			}
			// 先落本轮 ActiveTurnID；宿主未提供轮次时，确认绑定回退到该值
			snap.ActiveTurnID = firstText(event.TurnID, event.EventID, fmt.Sprintf("%d", time.Now().UnixNano()))
			if canConsume {
				snap.ConfirmedTurnID = firstText(confirmTurn, snap.ActiveTurnID)
			}
			snap.ObservedRead = false
			snap.ObservedReadPath = ""
			snap.WorkspaceRoot = workspace
			snap.Host = opts.Host

			compiled, compErr := CompilePrompt(CompileOptions{
				RootDir:     firstText(opts.RootDir, workspace),
				Instruction: instruction,
				HookMode:    true,
			})
			if compErr != nil || compiled == nil || strings.TrimSpace(compiled.CompiledPrompt) == "" {
				if compErr == nil {
					compErr = fmt.Errorf("编译产物为空")
				}
				snap.DeliveryMode = "failed"
				_ = store.SaveSnapshot(snap)
				decision = NativeDecision{Block: true, Reason: "上下文编译失败: " + compErr.Error()}
				actionErr = compErr
				return nil
			}

			requestRecordKey := RecordID(sessionID, snap.ActiveTurnID, workspace)
			contextPath, artErr := SaveContextArtifact(storeDir, requestRecordKey, compiled.CompiledPrompt)
			if artErr != nil {
				snap.DeliveryMode = "failed"
				_ = store.SaveSnapshot(snap)
				decision = NativeDecision{Block: true, Reason: "上下文产物保存失败: " + artErr.Error()}
				actionErr = artErr
				return nil
			}

			snap.ContextPath = contextPath
			snap.DeliveryMode = "pending_read"
			if saveErr := store.SaveSnapshot(snap); saveErr != nil {
				decision = NativeDecision{Block: true, Reason: "保存会话快照失败: " + saveErr.Error()}
				actionErr = saveErr
				return nil
			}

			decision = NativeDecision{
				Block:             false,
				AdditionalContext: contextPath,
			}
			return nil

		case NativeEventReadCompleted:
			if dec, blocked := requireControlledSession(event, opts, snap, workspace); blocked {
				decision = dec
				return nil
			}
			// 成功的普通读取在待交付时返回本轮路径；只有精确命中本轮上下文文件才登记回执。
			turnOK := event.TurnID == "" || snap.ActiveTurnID == "" || event.TurnID == snap.ActiveTurnID
			if event.Success && turnOK && isContextReadHit(event.Targets, snap.ContextPath) {
				snap.ObservedRead = true
				snap.ObservedReadPath = snap.ContextPath
				snap.DeliveryMode = "observed_read"
				decision = NativeDecision{Block: false, AdditionalContext: snap.ContextPath}
				if saveErr := store.SaveSnapshot(snap); saveErr != nil {
					actionErr = saveErr
					decision.Reason = "保存会话快照失败: " + saveErr.Error()
					return nil
				}
				return nil
			}
			if event.Success && turnOK && snap.DeliveryMode == "pending_read" && strings.TrimSpace(snap.ContextPath) != "" &&
				(event.Action == NativeActionRead || event.Action == "") {
				decision = NativeDecision{Block: false, AdditionalContext: snap.ContextPath}
				return nil
			}
			decision = NativeDecision{Block: false}
			return nil

		case NativeEventResponseCompleted:
			if dec, blocked := requireControlledSession(event, opts, snap, workspace); blocked {
				decision = dec
				return nil
			}
			// 迟到的旧轮回复不得登记方案
			if event.TurnID != "" && snap.ActiveTurnID != "" && event.TurnID != snap.ActiveTurnID {
				decision = NativeDecision{Block: false}
				return nil
			}
			if snap.ObservedRead {
				if candidate := ParseNativePlan(event.Text, workspace); candidate != nil {
					candidate.TurnID = snap.ActiveTurnID
					snap.PlanCandidate = candidate
					snap.Diagnosis = candidate.Diagnosis
					snap.DiagnosisSummary = candidate.PlanSummary
					snap.Consumed = false
					if saveErr := store.SaveSnapshot(snap); saveErr != nil {
						actionErr = saveErr
						return nil
					}
				}
			}
			decision = NativeDecision{Block: false}
			return nil

		case NativeEventBeforeTool:
			if event.Action == NativeActionRead {
				decision = NativeDecision{Block: false}
				return nil
			}

			if dec, blocked := requireControlledSession(event, opts, snap, workspace); blocked {
				decision = dec
				actionErr = fmt.Errorf("%s", dec.Reason)
				return nil
			}

			if event.Action == NativeActionShell {
				decision = NativeDecision{Block: false}
				return nil
			}

			if !snap.ObservedRead {
				decision = NativeDecision{Block: true, Reason: "上下文尚未交付，不能执行写入"}
				return nil
			}

			switch snap.Diagnosis {
			case DiagnosisConfirmed, DiagnosisNotApplicable:
			case DiagnosisHypothesis:
				decision = NativeDecision{Block: true, Reason: "诊断状态为待验证假设，只能继续排查，不能进入已确认根因的修复方案"}
				return nil
			default:
				decision = NativeDecision{Block: true, Reason: "诊断状态为未诊断，不能仅凭解码标题、explicit 或 Plan 进入修复方案"}
				return nil
			}

			if !snap.PlanConfirmed || len(snap.ConfirmedFiles) == 0 {
				decision = NativeDecision{Block: true, Reason: "方案尚未获得用户显式确认（缺少‘执行’授权），不能执行修改"}
				return nil
			}

			// 预写必须核对确认轮：旧轮写入一律拒绝。
			// 宿主未提供轮次字段时，回退到本会话活跃轮，仅允许与确认轮同一 ActiveTurnID。
			eventTurn := firstText(event.TurnID, event.EventID, snap.ActiveTurnID)
			if snap.ConfirmedTurnID == "" || eventTurn == "" || eventTurn != snap.ConfirmedTurnID {
				decision = NativeDecision{Block: true, Reason: "写入事件轮次与确认轮不一致，拒绝复用旧批准"}
				return nil
			}

			if event.Action == NativeActionMCP {
				decision = NativeDecision{Block: true, Reason: mcpDenyReason}
				return nil
			}

			if event.Action == NativeActionEdit {
				if len(event.Targets) == 0 {
					decision = NativeDecision{Block: true, Reason: "缺少明确目标文件，无法核验修改范围"}
					return nil
				}
				for _, target := range event.Targets {
					if !IsTargetInConfirmedScope(target, snap.ConfirmedFiles, workspace) {
						decision = NativeDecision{
							Block:  true,
							Reason: fmt.Sprintf("目标文件 %q 超出已确认方案范围 %v", target, snap.ConfirmedFiles),
						}
						return nil
					}
				}
				decision = NativeDecision{Block: false}
				return nil
			}

			decision = NativeDecision{Block: true, Reason: "未知工具动作，默认阻断"}
			return nil

		case NativeEventObserve:
			decision = NativeDecision{Block: false}
			return nil

		default:
			decision = NativeDecision{Block: false}
			return nil
		}
	})

	if lockErr != nil {
		return NativeDecision{Block: true, Reason: "会话锁执行失败: " + lockErr.Error()}, lockErr
	}
	return decision, actionErr
}
