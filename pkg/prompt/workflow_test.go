package prompt

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"agate/pkg/harness"
)

func TestRepairPlanRequiresDiagnosis(t *testing.T) {
	if err := RepairPlanAllowed(RequestState{Diagnosis: DiagnosisUndiagnosed}); err == nil {
		t.Fatal("未诊断不能进入修复方案")
	}
	if err := RepairPlanAllowed(RequestState{Diagnosis: DiagnosisHypothesis}); err == nil || !strings.Contains(err.Error(), "待验证假设") {
		t.Fatalf("假设不能写成已确认修复: %v", err)
	}
	if err := RepairPlanAllowed(RequestState{Diagnosis: DiagnosisConfirmed}); err != nil {
		t.Fatal(err)
	}
	if err := RepairPlanAllowed(RequestState{Diagnosis: DiagnosisNotApplicable}); err != nil {
		t.Fatal(err)
	}
}

func TestRequestStateDoesNotReuseAnotherInstruction(t *testing.T) {
	dir := t.TempDir()
	state := RequestState{
		RequestID:       RecordID("session raw/id", "request", "D:/work"),
		InstructionHash: hashText("修复采集"),
		WorkspaceID:     hashText("D:/work"),
		RuleVersion:     RuleVersion,
		CompileStatus:   "compiled",
		DeliveryMode:    "pending_read",
		Diagnosis:       DiagnosisConfirmed,
	}
	if strings.Contains(state.RequestID, "session") || strings.Contains(state.RequestID, "/") {
		t.Fatalf("记录标识不应包含原始会话或路径: %s", state.RequestID)
	}
	if err := SaveRequestState(dir, state); err != nil {
		t.Fatal(err)
	}
	if err := SaveRequestState(dir, state); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadRequestState(dir, state.RequestID)
	if err != nil {
		t.Fatal(err)
	}
	if err := ReuseAllowed(loaded, "修复采集", "D:/work"); err != nil {
		t.Fatal(err)
	}
	if err := ReuseAllowed(loaded, "另一个问题", "D:/work"); err == nil {
		t.Fatal("不同指令不应复用诊断或授权")
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 || filepath.Ext(entries[0].Name()) != ".json" {
		t.Fatalf("重复保存应保持一条记录，实际: %v %v", entries, err)
	}
}

func TestValidateDiagnosisOutputDetectsSkippedDiagnosis(t *testing.T) {
	// 1. 有 Plan 无原因说明必须拦截
	onlyPlan := "[Plan]\n拟修改 pkg/agent/collector.go\n"
	if err := ValidateDiagnosisOutput(onlyPlan, DiagnosisConfirmed); err == nil || !strings.Contains(err.Error(), "跳过诊断") {
		t.Fatalf("有 Plan 无原因说明应被拦截，实际: %v", err)
	}

	// 2. 把待验证假设写成已确认结论必须拦截
	hypothesisText := "现象与预期：识别未知。\n位置与证据：collector.go。\n原因：可能是环境变量缺失。\n不确定项：待验证。\n依据：先排查。"
	if err := ValidateDiagnosisOutput(hypothesisText, DiagnosisHypothesis); err == nil || !strings.Contains(err.Error(), "待验证假设") {
		t.Fatalf("待验证假设状态不应放行确认修复，实际: %v", err)
	}

	// 3. 完整诊断五要素应通过
	complete := "现象与预期：Windows Cursor 终端下 detect 返回 unknown，预期为 cursor。\n" +
		"问题位置和证据：pkg/agent/collector.go 仅实现了 Linux /proc。\n" +
		"因果解释：Windows 未采集祖先进程导致 Editor 证据为空。\n" +
		"不确定项：已确认 Windows 采集缺失；其他宿主暂未测。\n" +
		"方案依据：补齐 collector_windows.go 后用单测与实机验证。"
	if err := ValidateDiagnosisOutput(complete, DiagnosisConfirmed); err != nil {
		t.Fatalf("完整五要素诊断应通过，实际报错: %v", err)
	}

	// 4. 新功能或明确改动 (not_applicable) 不强求故障诊断
	if err := ValidateDiagnosisOutput("", DiagnosisNotApplicable); err != nil {
		t.Fatalf("not_applicable 不应强求故障诊断: %v", err)
	}
}

func TestUpdateDiagnosisStateEnforcesEvidence(t *testing.T) {
	dir := t.TempDir()
	id := RecordID("conv", "gen", "D:/work")
	state := RequestState{
		RequestID:       id,
		InstructionHash: hashText("修复采集"),
		WorkspaceID:     hashText("D:/work"),
		RuleVersion:     RuleVersion,
		CompileStatus:   "compiled",
		DeliveryMode:    "observed_read",
		DeliveryEvidence: "read:" + id + ".context.md",
		Diagnosis:       DiagnosisUndiagnosed,
	}
	if err := SaveRequestState(dir, state); err != nil {
		t.Fatal(err)
	}

	if _, err := UpdateDiagnosisState(dir, id, DiagnosisConfirmed, "[Plan] 直接改代码"); err == nil {
		t.Fatal("缺少诊断说明时不能将状态更新为 confirmed")
	}

	evidence := "现象：未知。位置与证据：collector.go。原因：未实现 Windows 采集导致。不确定项：已确认。依据：新增单测验证。"
	updated, err := UpdateDiagnosisState(dir, id, DiagnosisConfirmed, evidence)
	if err != nil {
		t.Fatalf("完整诊断应允许更新为 confirmed: %v", err)
	}
	if updated.Diagnosis != DiagnosisConfirmed {
		t.Fatalf("诊断状态应为 confirmed，实际: %s", updated.Diagnosis)
	}
}

func TestSaveRequestStateOverwriteAndConcurrent(t *testing.T) {
	dir := t.TempDir()
	id := "req-overwrite"
	first := RequestState{RequestID: id, CompileStatus: "first", Diagnosis: DiagnosisUndiagnosed}
	if err := SaveRequestState(dir, first); err != nil {
		t.Fatal(err)
	}
	second := first
	second.CompileStatus = "second"
	if err := SaveRequestState(dir, second); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadRequestState(dir, id)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.CompileStatus != "second" {
		t.Fatalf("覆盖后应读到新内容，实际: %s", loaded.CompileStatus)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("覆盖后应只保留目标文件，实际: %v %v", entries, err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			st := second
			st.CompileStatus = fmt.Sprintf("concurrent-%d", n)
			_ = SaveRequestState(dir, st)
		}(i)
	}
	wg.Wait()
	if _, err := LoadRequestState(dir, id); err != nil {
		t.Fatalf("并发保存后记录应仍可解析: %v", err)
	}
}

func TestWriteFileAtomicOverwritesDespiteReplacingDir(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "state.json")
	if err := os.WriteFile(dst, []byte(`{"compile_status":"old"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	backup := dst + ".replacing"
	if err := os.MkdirAll(backup, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(backup, "lock"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	newer := []byte(`{"compile_status":"new"}`)
	if err := harness.WriteFileAtomic(dst, newer, 0o600); err != nil {
		t.Fatalf("已有 .replacing 目录时仍应完成覆盖: %v", err)
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(newer) {
		t.Fatalf("覆盖后应读到新内容，实际: %s", got)
	}
}
