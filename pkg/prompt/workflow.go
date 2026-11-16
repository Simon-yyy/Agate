package prompt

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const RuleVersion = "intent-diagnosis-1"

// DiagnosisStatus 是进入修复方案前必须单独记录的状态。
type DiagnosisStatus string

const (
	DiagnosisUndiagnosed   DiagnosisStatus = "undiagnosed"
	DiagnosisHypothesis    DiagnosisStatus = "hypothesis"
	DiagnosisConfirmed     DiagnosisStatus = "confirmed"
	DiagnosisNotApplicable DiagnosisStatus = "not_applicable"
)

// RequestState 保存一次请求的编译、交付和诊断结果。回执不是用户授权。
type RequestState struct {
	RequestID        string          `json:"request_id"`
	InstructionHash  string          `json:"instruction_hash"`
	WorkspaceID      string          `json:"workspace_id"`
	RuleVersion      string          `json:"rule_version"`
	CompileStatus    string          `json:"compile_status"`
	ContextPath      string          `json:"context_path,omitempty"`
	DeliveryMode     string          `json:"delivery_mode"`
	DeliveryEvidence string          `json:"delivery_evidence,omitempty"`
	Diagnosis        DiagnosisStatus `json:"diagnosis"`
	DiagnosisSummary string          `json:"diagnosis_summary,omitempty"`
	Failure          string          `json:"failure,omitempty"`
}

// RecordID 把会话和请求标识规范化成文件名，避免把原始标识拼进路径。
func RecordID(sessionKey, requestKey, workspace string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(sessionKey) + "\n" + strings.TrimSpace(requestKey) + "\n" + strings.TrimSpace(workspace)))
	return hex.EncodeToString(sum[:])
}

func hashText(value string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(value)))
	return hex.EncodeToString(sum[:])
}

// RepairPlanAllowed 判断当前记录能否进入已确认根因的修复方案。
func RepairPlanAllowed(state RequestState) error {
	switch state.Diagnosis {
	case DiagnosisConfirmed, DiagnosisNotApplicable:
		return nil
	case DiagnosisHypothesis:
		return fmt.Errorf("诊断状态为待验证假设，只能继续排查，不能进入已确认根因的修复方案")
	default:
		return fmt.Errorf("诊断状态为未诊断，不能仅凭解码标题、explicit 或 Plan 进入修复方案")
	}
}

// ReuseAllowed 拒绝把另一条指令或另一个工作区的成功状态借过来。
func ReuseAllowed(saved RequestState, instruction, workspace string) error {
	if saved.InstructionHash != hashText(instruction) || saved.WorkspaceID != hashText(workspace) {
		return fmt.Errorf("目标或指令已变化，不能复用旧的编译、诊断或授权状态")
	}
	return nil
}

// SaveRequestState 幂等写入私有运行时记录。相同内容重复保存不改变结果。
func SaveRequestState(dir string, state RequestState) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("创建请求记录目录失败: %w", err)
	}
	if state.RequestID == "" || strings.Contains(state.RequestID, ".") || strings.Contains(state.RequestID, "/") || strings.Contains(state.RequestID, "\\") {
		return fmt.Errorf("请求标识不能作为路径")
	}
	body, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	target := filepath.Join(dir, state.RequestID+".json")
	temp := target + ".tmp"
	if err := os.WriteFile(temp, body, 0o600); err != nil {
		return err
	}
	return os.Rename(temp, target)
}

// LoadRequestState 读取一条请求记录。
func LoadRequestState(dir, requestID string) (RequestState, error) {
	var state RequestState
	body, err := os.ReadFile(filepath.Join(dir, requestID+".json"))
	if err != nil {
		return state, err
	}
	if err := json.Unmarshal(body, &state); err != nil {
		return state, err
	}
	return state, nil
}

// SaveContextArtifact 将编译后的执行信封落盘为独立私有上下文文件，供宿主模型读取。
func SaveContextArtifact(dir, requestID, compiledPrompt string) (string, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("创建上下文产物目录失败: %w", err)
	}
	if requestID == "" || strings.Contains(requestID, ".") || strings.Contains(requestID, "/") || strings.Contains(requestID, "\\") {
		return "", fmt.Errorf("请求标识不能作为路径")
	}
	target := filepath.Join(dir, requestID+".context.md")
	temp := target + ".tmp"
	if err := os.WriteFile(temp, []byte(compiledPrompt), 0o600); err != nil {
		return "", err
	}
	if err := os.Rename(temp, target); err != nil {
		return "", err
	}
	return target, nil
}

// SaveSessionActiveRequest 记录某会话最近一次提交生成的请求 ID，供无 generation_id 的后续事件关联。
func SaveSessionActiveRequest(dir, sessionKey, workspace, requestID string) error {
	if strings.TrimSpace(sessionKey) == "" || strings.TrimSpace(requestID) == "" {
		return nil
	}
	sessionsDir := filepath.Join(dir, "sessions")
	if err := os.MkdirAll(sessionsDir, 0o700); err != nil {
		return err
	}
	key := hashText(strings.TrimSpace(sessionKey) + "\n" + strings.TrimSpace(workspace))
	target := filepath.Join(sessionsDir, key+".active")
	temp := target + ".tmp"
	if err := os.WriteFile(temp, []byte(requestID), 0o600); err != nil {
		return err
	}
	return os.Rename(temp, target)
}

// LoadSessionActiveRequest 读取某会话最近一次激活的请求 ID。
func LoadSessionActiveRequest(dir, sessionKey, workspace string) (string, error) {
	if strings.TrimSpace(sessionKey) == "" {
		return "", fmt.Errorf("缺少会话标识")
	}
	key := hashText(strings.TrimSpace(sessionKey) + "\n" + strings.TrimSpace(workspace))
	data, err := os.ReadFile(filepath.Join(dir, "sessions", key+".active"))
	if err != nil {
		return "", err
	}
	id := strings.TrimSpace(string(data))
	if id == "" {
		return "", fmt.Errorf("会话活跃请求记录为空")
	}
	return id, nil
}

// ValidateDiagnosisOutput 检查诊断说明或修复前输出是否具备最小诊断要素，拦截有 Plan 无原因说明及假设伪装结论。
func ValidateDiagnosisOutput(output string, status DiagnosisStatus) error {
	if status == DiagnosisNotApplicable {
		return nil
	}
	trimmed := strings.TrimSpace(output)
	if trimmed == "" {
		return fmt.Errorf("缺少诊断说明内容：必须提供现象与预期、位置与证据、因果解释、不确定项及方案依据")
	}

	hasPlanMarker := containsAny(trimmed, "[Plan]", "修复方案", "拟修改", "拟改动")
	hasPhenomenon := containsAny(trimmed, "现象", "预期", "实际")
	hasEvidence := containsAny(trimmed, "位置", "证据", "报错", "日志", "文件", "行")
	hasCausal := containsAny(trimmed, "原因", "导致", "因果", "触发", "因为")
	hasUncertainty := containsAny(trimmed, "不确定", "假设", "待验证", "已确认")
	hasBasis := containsAny(trimmed, "依据", "验证")

	if status == DiagnosisHypothesis {
		if hasPlanMarker && !containsAny(trimmed, "排查计划") {
			return fmt.Errorf("诊断状态为待验证假设，仅可交付排查计划，不能输出已确认根因的修复方案")
		}
		return fmt.Errorf("诊断状态为待验证假设，只能继续排查，不能进入已确认根因的修复方案")
	}

	var missing []string
	if !hasPhenomenon {
		missing = append(missing, "现象与预期")
	}
	if !hasEvidence {
		missing = append(missing, "问题位置与证据")
	}
	if !hasCausal {
		missing = append(missing, "因果解释")
	}
	if !hasUncertainty {
		missing = append(missing, "不确定项")
	}
	if !hasBasis {
		missing = append(missing, "方案依据")
	}
	if len(missing) > 0 {
		if hasPlanMarker {
			return fmt.Errorf("检测到修复方案但缺少诊断说明（缺失：%s），严禁仅凭解码标题、explicit 或 Plan 跳过诊断", strings.Join(missing, "、"))
		}
		return fmt.Errorf("诊断说明不完整，缺失：%s", strings.Join(missing, "、"))
	}
	return nil
}

// UpdateDiagnosisState 校验并更新请求记录的诊断状态，防止未提供诊断证据即放行写入。
func UpdateDiagnosisState(dir, requestID string, status DiagnosisStatus, summary string) (RequestState, error) {
	state, err := LoadRequestState(dir, requestID)
	if err != nil {
		return state, err
	}
	switch status {
	case DiagnosisUndiagnosed, DiagnosisHypothesis, DiagnosisConfirmed, DiagnosisNotApplicable:
	default:
		return state, fmt.Errorf("非法的诊断状态 %q", status)
	}
	if status == DiagnosisConfirmed {
		if err := ValidateDiagnosisOutput(summary, status); err != nil {
			return state, err
		}
	}
	state.Diagnosis = status
	if strings.TrimSpace(summary) != "" {
		state.DiagnosisSummary = strings.TrimSpace(summary)
	}
	if err := SaveRequestState(dir, state); err != nil {
		return state, err
	}
	return state, nil
}

func containsAny(text string, keywords ...string) bool {
	lower := strings.ToLower(text)
	for _, kw := range keywords {
		if strings.Contains(lower, strings.ToLower(kw)) {
			return true
		}
	}
	return false
}
