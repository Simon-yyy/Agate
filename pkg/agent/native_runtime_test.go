package agent

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestEvidenceExecutableIsNotSerialized(t *testing.T) {
	body, err := json.Marshal(Evidence{
		Source:      EvidenceSourceProcess,
		Key:         "pid_1",
		TargetRole:  "editor",
		TargetAgent: "cursor",
		Matched:     true,
		Executable:  `C:\secret\Cursor.exe`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "secret") || strings.Contains(string(body), "Executable") {
		t.Fatalf("可执行文件路径不应进入 JSON: %s", body)
	}
}

func TestCursorVersionComesFromExecutableNotEnv(t *testing.T) {
	orig := readExecutableProductVersion
	t.Cleanup(func() { readExecutableProductVersion = orig })
	readExecutableProductVersion = func(path string) string {
		if path == `C:\cursor\Cursor.exe` {
			return "3.30.14"
		}
		return ""
	}
	ctx := DetectionContext{
		Editor:       "cursor",
		EditorStatus: StatusKnown,
		RunnerStatus: StatusUnknown,
		Evidences: []Evidence{{
			Source:      EvidenceSourceProcess,
			TargetRole:  "editor",
			TargetAgent: "cursor",
			Matched:     true,
			Executable:  `C:\cursor\Cursor.exe`,
		}},
	}
	rt := DetectNativeRuntimeEnv(ctx, `C:\work`, func(key string) string {
		if key == "CURSOR_VERSION" {
			return "9.9.9"
		}
		return ""
	})
	if rt.ClientVer != "3.30.14" {
		t.Fatalf("应采用可执行文件产品版本，实际: %q", rt.ClientVer)
	}
}

func TestCursorVersionStaysEmptyWithoutExecutable(t *testing.T) {
	orig := readExecutableProductVersion
	t.Cleanup(func() { readExecutableProductVersion = orig })
	readExecutableProductVersion = func(string) string { return "3.30.14" }
	ctx := DetectionContext{Editor: "cursor", EditorStatus: StatusKnown, RunnerStatus: StatusUnknown}
	rt := DetectNativeRuntimeEnv(ctx, `C:\work`, func(key string) string {
		if key == "CURSOR_VERSION" {
			return "9.9.9"
		}
		return ""
	})
	if rt.ClientVer != "" {
		t.Fatalf("没有已匹配可执行文件时版本应为空，实际: %q", rt.ClientVer)
	}
}

func TestCursorVersionStaysEmptyWhenReaderFails(t *testing.T) {
	orig := readExecutableProductVersion
	t.Cleanup(func() { readExecutableProductVersion = orig })
	readExecutableProductVersion = func(string) string { return "" }
	ctx := DetectionContext{
		Editor:       "cursor",
		EditorStatus: StatusKnown,
		RunnerStatus: StatusUnknown,
		Evidences: []Evidence{{
			Source:      EvidenceSourceProcess,
			TargetRole:  "editor",
			TargetAgent: "cursor",
			Matched:     true,
			Executable:  `C:\cursor\Cursor.exe`,
		}},
	}
	rt := DetectNativeRuntimeEnv(ctx, `C:\work`, func(string) string { return "9.9.9" })
	if rt.ClientVer != "" {
		t.Fatalf("读不到产品版本时应为空，实际: %q", rt.ClientVer)
	}
}

func TestCursorAcceptsAnyReadVersion(t *testing.T) {
	for _, version := range []string{"3.31.15", "3.30.14", "9.9.9"} {
		if !NativeHookVersionAccepted(Cursor, version) {
			t.Fatalf("Cursor 读到版本 %s 时应视为已验收", version)
		}
	}
	if NativeHookVersionAccepted(Cursor, "") || NativeHookVersionAccepted(Cursor, "  ") {
		t.Fatal("Cursor 没有读到版本时不应视为已验收")
	}
	if NativeHookVersionAccepted(Claude, "3.31.15") {
		t.Fatal("其他宿主不能因 Cursor 的规则被放行")
	}
}
