package agent

import (
	"strings"
	"testing"
	"time"
)

var testMockEmpty = &mockProcessInspector{chain: nil}

func TestInspectContextRunnerAndEditorSeparation(t *testing.T) {
	// 场景 1：在 Cursor 编辑器宿主终端中运行 Codex 命令行 Agent
	env := map[string]string{
		"CODEX_SESSION_ID": "session-123456",
		"TERM_PROGRAM":     "cursor",
		"SSH_CONNECTION":   "10.0.0.1 52000 10.0.0.2 22",
	}
	getenv := func(k string) string { return env[k] }

	ctx := InspectContextFull(getenv, "cursor", "", nil, testMockEmpty, time.Now())

	if ctx.Runner != "codex" || ctx.RunnerStatus != StatusKnown {
		t.Fatalf("Runner 预期为 codex (known)，实际: %s (%s)", ctx.Runner, ctx.RunnerStatus)
	}
	if ctx.Editor != "cursor" || ctx.EditorStatus != StatusKnown {
		t.Fatalf("Editor 预期为 cursor (known)，实际: %s (%s)", ctx.Editor, ctx.EditorStatus)
	}
	if ctx.HasConflict() {
		t.Fatalf("Runner 与 Editor 职责分离时不应视为冲突")
	}

	eff, ok := ctx.EffectiveTarget()
	if !ok || eff != "codex" {
		t.Fatalf("首选装配目标应优先采用执行者 codex，实际: %s (%t)", eff, ok)
	}

	foundSSH := false
	for _, tr := range ctx.Transports {
		if tr == TransportSSH {
			foundSSH = true
			break
		}
	}
	if !foundSSH {
		t.Fatalf("预期检测到 SSH 传输环境")
	}
}

func TestInspectContextRunnerConflictDetection(t *testing.T) {
	// 场景 2：同时存在 CODEX_SESSION_ID 与 CLAUDE_CODE 环境变量（同级执行者冲突）
	env := map[string]string{
		"CODEX_SESSION_ID": "codex-test",
		"CLAUDE_CODE":      "1",
	}
	getenv := func(k string) string { return env[k] }

	ctx := InspectContextFull(getenv, "", "", nil, testMockEmpty, time.Now())

	if ctx.RunnerStatus != StatusAmbiguous {
		t.Fatalf("RunnerStatus 预期为 ambiguous，实际: %s", ctx.RunnerStatus)
	}
	if !ctx.HasConflict() {
		t.Fatalf("存在多个不同 Agent 强指纹时必须标记为 HasConflict=true")
	}

	eff, ok := ctx.EffectiveTarget()
	if ok || eff != "" {
		t.Fatalf("存在冲突时 EffectiveTarget 严禁放行，实际: %s (%t)", eff, ok)
	}

	if len(ctx.Warnings) == 0 {
		t.Fatalf("存在冲突时必须记录 Warnings")
	}
}

func TestInspectContextCustomProfilesRunnerAndEditor(t *testing.T) {
	// 场景 3：自定义工具档案 trae (env=TRAE_AGENT, term=trae)
	custom := map[string]Profile{
		"trae": {
			Path:         ".trae/rules/agate.mdc",
			Format:       "frontmatter-md",
			EnvKeys:      []string{"TRAE_AGENT"},
			TermPrograms: []string{"trae"},
		},
	}

	env := map[string]string{
		"TRAE_AGENT": "1",
	}
	getenv := func(k string) string { return env[k] }

	ctx := InspectContextFull(getenv, "trae", "", custom, testMockEmpty, time.Now())
	if ctx.Runner != "trae" || ctx.RunnerStatus != StatusKnown {
		t.Fatalf("自定义档案 Runner 预期为 trae，实际: %s (%s)", ctx.Runner, ctx.RunnerStatus)
	}
	if ctx.Editor != "trae" || ctx.EditorStatus != StatusKnown {
		t.Fatalf("自定义档案 Editor 预期为 trae，实际: %s (%s)", ctx.Editor, ctx.EditorStatus)
	}
}

func TestInspectContextPureVSCodeTerminalRemainsUnknownEditor(t *testing.T) {
	// 场景 4：仅有普通 TERM_PROGRAM=vscode，无任何 Cursor 专属环境变量或白名单路径
	// 按照规范：不把通用 vscode 猜测映射为 Cursor
	env := map[string]string{
		"TERM_PROGRAM": "vscode",
	}
	getenv := func(k string) string { return env[k] }

	ctx := InspectContextFull(getenv, "vscode", "", nil, testMockEmpty, time.Now())

	if ctx.RunnerStatus != StatusUnknown || ctx.Runner != "" {
		t.Fatalf("无执行者环境变量时 Runner 应为 unknown，实际: %s", ctx.Runner)
	}
	if ctx.EditorStatus != StatusUnknown || ctx.Editor != "" {
		t.Fatalf("普通 vscode 不应被盲目判定为已知特定编辑器，实际: %s (%s)", ctx.Editor, ctx.EditorStatus)
	}
}

func TestInspectContextRemoteCursorPathWhitelist(t *testing.T) {
	// 场景 5：Remote SSH 下 TERM_PROGRAM=vscode，但 PATH 中命中 .cursor-server 白名单路径
	env := map[string]string{
		"TERM_PROGRAM":   "vscode",
		"PATH":           "/usr/local/bin:/home/user/.cursor-server/bin/94060879.../bin",
		"SSH_CONNECTION": "192.168.1.100 50000 192.168.1.200 22",
	}
	getenv := func(k string) string { return env[k] }

	ctx := InspectContextFull(getenv, "vscode", "", nil, testMockEmpty, time.Now())

	if ctx.Editor != "cursor" || ctx.EditorStatus != StatusKnown {
		t.Fatalf("命中白名单路径时 Editor 预期为 cursor (known)，实际: %s (%s)", ctx.Editor, ctx.EditorStatus)
	}
	if ctx.RunnerStatus != StatusUnknown {
		t.Fatalf("无 runner 变量时 Runner 预期为 unknown，实际: %s", ctx.RunnerStatus)
	}

	eff, ok := ctx.EffectiveTarget()
	if !ok || eff != "cursor" {
		t.Fatalf("在无 runner 时装配目标应顺位采用 editor (cursor)，实际: %s (%t)", eff, ok)
	}
}

func TestInspectContextRemoteCursorAncestorProcess(t *testing.T) {
	// 场景 6：Remote SSH 下环境变量被剥离，但祖先进程命中 .cursor-server
	mock := &mockProcessInspector{
		chain: []ProcessInfo{
			{PID: 1024, PPID: 1020, Path: "/bin/bash", Name: "bash"},
			{PID: 1020, PPID: 900, Path: "/home/user/.cursor-server/bin/node", Name: "node"},
			{PID: 900, PPID: 1, Path: "/usr/lib/systemd/systemd", Name: "systemd"},
		},
	}

	env := map[string]string{
		"SSH_CLIENT": "10.0.0.1 52000 22",
	}
	getenv := func(k string) string { return env[k] }

	ctx := InspectContextFull(getenv, "", "", nil, mock, time.Now())
	if ctx.Editor != "cursor" || ctx.EditorStatus != StatusKnown {
		t.Fatalf("祖先进程命中白名单时 Editor 预期为 cursor (known)，实际: %s (%s)", ctx.Editor, ctx.EditorStatus)
	}
}

func TestInspectContextSessionContextPassing(t *testing.T) {
	// 场景 7：通过 AGATE_SESSION_CONTEXT 显式透传会话声明
	now := time.Unix(1700000000, 0)
	env := map[string]string{
		"AGATE_SESSION_CONTEXT": "v=1;editor=cursor;runner=codex;sid=sess-456;src=terminal;ts=1700000000;exp=1700001000",
	}
	getenv := func(k string) string { return env[k] }

	ctx := InspectContextFull(getenv, "", "", nil, testMockEmpty, now)
	if ctx.Runner != "codex" || ctx.RunnerStatus != StatusKnown {
		t.Fatalf("会话声明指定 Runner 预期为 codex (known)，实际: %s (%s)", ctx.Runner, ctx.RunnerStatus)
	}
	if ctx.Editor != "cursor" || ctx.EditorStatus != StatusKnown {
		t.Fatalf("会话声明指定 Editor 预期为 cursor (known)，实际: %s (%s)", ctx.Editor, ctx.EditorStatus)
	}
	if ctx.SessionContext == nil || ctx.SessionContext.SessionID != "sess-456" {
		t.Fatalf("SessionContext 预期解析成功，实际: %+v", ctx.SessionContext)
	}
}

func TestInspectContextSessionContextConflictWithLiveEnv(t *testing.T) {
	// 场景 8：现场环境变量为 CLAUDE_CODE，但会话声明为 runner=codex (R5-03: 补齐合法来源并在现场强指纹覆盖时断言分歧警告)
	now := time.Unix(1700000000, 0)
	env := map[string]string{
		"CLAUDE_CODE":           "1",
		"AGATE_SESSION_CONTEXT": "v=1;runner=codex;src=terminal;ts=1700000000;exp=1700001000",
	}
	getenv := func(k string) string { return env[k] }

	ctx := InspectContextFull(getenv, "", "", nil, testMockEmpty, now)
	if ctx.Runner != "claude" || ctx.RunnerStatus != StatusKnown {
		t.Fatalf("现场强指纹优先，Runner 预期为 claude，实际: %s (%s)", ctx.Runner, ctx.RunnerStatus)
	}
	foundConflictWarning := false
	for _, w := range ctx.Warnings {
		if strings.Contains(w, "分歧") || strings.Contains(w, "冲突") {
			foundConflictWarning = true
			break
		}
	}
	if !foundConflictWarning {
		t.Fatalf("现场变量与会话声明不一致时必须产生分歧告警，实际: %v", ctx.Warnings)
	}
}

func TestInspectContextIsolatedPathDowngradesToUnknown(t *testing.T) {
	// R2-F04: 普通 SSH 终端仅残留 PATH，无任何活动会话或进程证据
	env := map[string]string{
		"PATH":           "/usr/local/bin:/home/user/.cursor-server/bin/94060879.../bin",
		"SSH_CONNECTION": "192.168.1.100 50000 192.168.1.200 22",
	}
	getenv := func(k string) string { return env[k] }

	// R3-S02: 显式注入返回空链的 mock 检查器，杜绝真实宿主祖先进程干扰
	mockEmpty := &mockProcessInspector{chain: nil}
	ctx := InspectContextFull(getenv, "", "", nil, mockEmpty, time.Now())

	if ctx.EditorStatus != StatusUnknown || ctx.Editor != "" {
		t.Fatalf("孤立残留 PATH 严禁提升为 known editor，实际: %s (%s)", ctx.Editor, ctx.EditorStatus)
	}

	foundWarning := false
	for _, w := range ctx.Warnings {
		if strings.Contains(w, "仅残留 PATH") || strings.Contains(w, "无活动会话印证") {
			foundWarning = true
			break
		}
	}
	if !foundWarning {
		t.Fatalf("孤立残留 PATH 必须产生未知状态防误认告警，实际告警: %v", ctx.Warnings)
	}

	if target, ok := ctx.EffectiveTarget(); ok || target != "" {
		t.Fatalf("孤立残留 PATH 不得作为有效装配目标，实际: %s (%t)", target, ok)
	}
}

func TestInspectContextConflictingProcessOverridesResidualPath(t *testing.T) {
	// R2-F04: 祖先进程为 vscode，但 PATH 中残留了 cursor
	mock := &mockProcessInspector{
		chain: []ProcessInfo{
			{PID: 1024, PPID: 1020, Path: "/bin/bash", Name: "bash"},
			{PID: 1020, PPID: 900, Path: "/home/user/.vscode-server/bin/node", Name: "node"},
			{PID: 900, PPID: 1, Path: "/usr/lib/systemd/systemd", Name: "systemd"},
		},
	}

	env := map[string]string{
		"PATH": "/usr/local/bin:/home/user/.cursor-server/bin/94060879.../bin",
	}
	getenv := func(k string) string { return env[k] }

	ctx := InspectContextFull(getenv, "", "", nil, mock, time.Now())

	if ctx.Editor != "vscode" || ctx.EditorStatus != StatusKnown {
		t.Fatalf("活跃进程证据应明确判定为 vscode，不应被残留 PATH 搅乱，实际: %s (%s)", ctx.Editor, ctx.EditorStatus)
	}
}

func TestInspectContextIrrelevantTerminalDoesNotCorroborateResidualPath(t *testing.T) {
	// R3-F02: TERM_PROGRAM=xterm 无关终端绝不能作为残留 PATH 的活跃佐证
	env := map[string]string{
		"TERM_PROGRAM": "xterm",
		"PATH":         "/usr/local/bin:/home/user/.cursor-server/bin/94060879.../bin",
	}
	getenv := func(k string) string { return env[k] }
	mockEmpty := &mockProcessInspector{chain: nil}

	ctx := InspectContextFull(getenv, "xterm", "", nil, mockEmpty, time.Now())

	if ctx.EditorStatus != StatusUnknown || ctx.Editor != "" {
		t.Fatalf("无关终端 xterm 不得作为活跃编辑器印证提升 cursor 为 known，实际: %s (%s)", ctx.Editor, ctx.EditorStatus)
	}
}

func TestInspectContextResidualPathsDoNotBlockKnownRunner(t *testing.T) {
	// R3-F02: 存在已知 Runner 时，多个残留厂商 PATH 不得造成 conflict 阻断 Runner
	env := map[string]string{
		"CODEX_SESSION_ID": "codex-123",
		"PATH":             "/home/user/.cursor-server/bin:/home/user/.vscode-server/bin:/usr/bin",
	}
	getenv := func(k string) string { return env[k] }
	mockEmpty := &mockProcessInspector{chain: nil}

	ctx := InspectContextFull(getenv, "", "", nil, mockEmpty, time.Now())

	if ctx.Runner != "codex" || ctx.RunnerStatus != StatusKnown {
		t.Fatalf("Runner 预期为 codex (known)，实际: %s (%s)", ctx.Runner, ctx.RunnerStatus)
	}
	if ctx.HasConflict() {
		t.Fatalf("已存在唯一确凿 Runner 时，Editor 残留路径不应造成 HasConflict=true 阻断 Runner！")
	}
	eff, ok := ctx.EffectiveTarget()
	if !ok || eff != "codex" {
		t.Fatalf("EffectiveTarget 应稳定采纳 codex，实际: %s (%t)", eff, ok)
	}
}

func TestInspectContextDualResidualPathInVSCodeTerminalDowngradesToUnknown(t *testing.T) {
	// R4-02: 在通用 TERM_PROGRAM=vscode 终端下，若 PATH 同时残留 cursor-server 与 vscode-server 且无特异证据
	// 应判定为多残留冲突未决，降级为 StatusUnknown，严禁误报 StatusAmbiguous
	env := map[string]string{
		"TERM_PROGRAM": "vscode",
		"PATH":         "/home/user/.cursor-server/bin:/home/user/.vscode-server/bin:/usr/bin",
	}
	getenv := func(k string) string { return env[k] }
	mockEmpty := &mockProcessInspector{chain: nil}

	ctx := InspectContextFull(getenv, "vscode", "", nil, mockEmpty, time.Now())

	if ctx.EditorStatus != StatusUnknown {
		t.Fatalf("双残留 PATH 在无区分 IPC 时应降级为 StatusUnknown，实际: %s (%s)", ctx.Editor, ctx.EditorStatus)
	}
	if ctx.HasConflict() {
		t.Fatalf("双残留降级为 Unknown 后不应触发 HasConflict，实际: %v", ctx.Warnings)
	}
}

func TestInspectContextWindsurfTerminalDoesNotCorroborateCursorPath(t *testing.T) {
	// R4-02: windsurf 终端绝不得去互证 cursor 的残留 PATH
	env := map[string]string{
		"TERM_PROGRAM": "windsurf",
		"PATH":         "/home/user/.cursor-server/bin:/usr/bin",
	}
	getenv := func(k string) string { return env[k] }
	mockEmpty := &mockProcessInspector{chain: nil}

	ctx := InspectContextFull(getenv, "windsurf", "", nil, mockEmpty, time.Now())

	if ctx.Editor != "windsurf" || ctx.EditorStatus != StatusKnown {
		t.Fatalf("Editor 预期为 windsurf (known)，实际: %s (%s)", ctx.Editor, ctx.EditorStatus)
	}
	// cursor 应该在 Warnings 中被作为无活动会话印证而忽略，而非升级为冲突
	if ctx.HasConflict() {
		t.Fatalf("windsurf 终端不应与 cursor 残留 PATH 产生冲突，实际: %v", ctx.Warnings)
	}
}

func TestInspectContextRealEditorConflictReflectedInHasConflict(t *testing.T) {
	// R4-01: 当环境中真实存在冲突的特定编辑器时，HasConflict 必须如实反映 true
	ctx := &DetectionContext{
		Runner:       "codex",
		RunnerStatus: StatusKnown,
		Editor:       "",
		EditorStatus: StatusAmbiguous,
	}
	if !ctx.HasConflict() {
		t.Fatalf("即使 Runner 为 known，当 Editor 处于 Ambiguous 真实冲突时，HasConflict 必须返回 true 防诊断矛盾")
	}
}

func TestDetectEnvironmentWithProfilesKnownRunnerWinsOverEditorConflict(t *testing.T) {
	// R5-01 & R6-04: 当 Runner 为 known (codex) 且 Editor 为 ambiguous 冲突时，DetectEnvironmentFull 仍应确定性返回 codex
	// 并显式注入 mockProcessInspector 隔离 Linux 宿主机 /proc 真实进程树
	env := map[string]string{
		"CODEX_SESSION_ID": "123",
	}
	getenv := func(k string) string { return env[k] }
	// 构造具有冲突 Editor 的环境
	custom := map[string]Profile{
		"custom1": {TermPrograms: []string{"term_conflict"}},
		"custom2": {TermPrograms: []string{"term_conflict"}},
	}
	mockEmpty := &mockProcessInspector{chain: nil}
	kind, detected := DetectEnvironmentFull(getenv, "term_conflict", "", custom, mockEmpty)
	if !detected || kind != Codex {
		t.Fatalf("当 Runner 为 known 时，即使宿主 Editor 冲突，DetectEnvironmentFull 仍应返回 codex，实际: %s (%t)", kind, detected)
	}
}

func TestInspectSessionContextRejectsCustomAndUntrustedSource(t *testing.T) {
	// R5-02: 传未受控/非法 source (如 custom 或任意路径) 必须被拒绝拦截，严禁升级为确定身份
	now := time.Unix(1700000000, 0)
	for _, badSrc := range []string{"custom", "unknown", "/bin/bash", "untrusted_channel"} {
		env := map[string]string{
			"AGATE_SESSION_CONTEXT": "v=1;runner=codex;src=" + badSrc + ";exp=1700001000",
		}
		getenv := func(k string) string { return env[k] }
		sc, evs, warnings := InspectSessionContext(getenv, now)
		if sc != nil || len(evs) > 0 {
			t.Fatalf("来源为 %q 的会话声明不得生效，实际: sc=%+v", badSrc, sc)
		}
		foundWarn := false
		for _, w := range warnings {
			if strings.Contains(w, "缺失合规受控来源特征") || strings.Contains(w, "缺失合规来源特征") {
				foundWarn = true
				break
			}
		}
		if !foundWarn {
			t.Fatalf("来源为 %q 时必须产生拦截告警，实际告警: %v", badSrc, warnings)
		}
	}
}

func TestInspectContextSessionPathVarTakesPrecedenceOverResidualPath(t *testing.T) {
	// R5-04: 当前会话专有路径变量 (VSCODE_GIT_ASKPASS_NODE 命中 cursor-server)
	// 与残留的全局 PATH (包含 vscode-server) 并存时，Cursor 应作为 Level 1 专有会话证据命中 known，不被降级
	env := map[string]string{
		"TERM_PROGRAM":            "vscode",
		"PATH":                    "/usr/local/bin:/home/user/.vscode-server/bin:/usr/bin",
		"VSCODE_GIT_ASKPASS_NODE": "/home/user/.cursor-server/bin/node",
	}
	getenv := func(k string) string { return env[k] }
	mockEmpty := &mockProcessInspector{chain: nil}

	ctx := InspectContextFull(getenv, "vscode", "", nil, mockEmpty, time.Now())
	if ctx.Editor != "cursor" || ctx.EditorStatus != StatusKnown {
		t.Fatalf("命中活跃会话专有路径变量时 Editor 预期为 cursor (known)，实际: %s (%s)", ctx.Editor, ctx.EditorStatus)
	}
}

func TestInspectContextActiveProcessSuppressesResidualSessionPathVar(t *testing.T) {
	// R6-01: 当活跃父进程明确为 cursor 时，环境中残留的 VSCODE_GIT_ASKPASS_NODE (指向 vscode)
	// 应当被判定为跨宿主残留特征并被抑制，cursor 维持 StatusKnown，不产生 Ambiguous 假冲突
	env := map[string]string{
		"TERM_PROGRAM":            "vscode",
		"PATH":                    "/usr/local/bin:/usr/bin",
		"VSCODE_GIT_ASKPASS_NODE": "/home/user/.vscode-server/bin/node",
	}
	getenv := func(k string) string { return env[k] }
	mockCursorProcess := &mockProcessInspector{
		chain: []ProcessInfo{
			{PID: 100, Name: "cursor-server", Path: "/home/user/.cursor-server/bin/cursor-server"},
		},
	}

	ctx := InspectContextFull(getenv, "vscode", "", nil, mockCursorProcess, time.Now())
	if ctx.Editor != "cursor" || ctx.EditorStatus != StatusKnown {
		t.Fatalf("活跃父进程 cursor 应当压制跨宿主残留环境变量，预期 Editor 为 cursor (known)，实际: %s (%s)", ctx.Editor, ctx.EditorStatus)
	}

	foundWarning := false
	for _, w := range ctx.Warnings {
		if strings.Contains(w, "忽略残留会话特征: vscode") {
			foundWarning = true
			break
		}
	}
	if !foundWarning {
		t.Fatalf("被压制的残留会话变量应当输出警告日志，实际告警: %v", ctx.Warnings)
	}
}

func TestInspectContextCustomVSCodeProfileRecognizedAsKnown(t *testing.T) {
	// R6-01: 用户显式声明自定义 Profile [agent.profiles.vscode] (term_programs = ["vscode"])
	// 在 TERM_PROGRAM=vscode 且无冲突时，应正常被识别为 vscode (StatusKnown)，不再被硬编码降级为未知
	env := map[string]string{
		"TERM_PROGRAM": "vscode",
	}
	getenv := func(k string) string { return env[k] }
	mockEmpty := &mockProcessInspector{chain: nil}
	custom := map[string]Profile{
		"vscode": {
			TermPrograms: []string{"vscode"},
		},
	}

	ctx := InspectContextFull(getenv, "vscode", "", custom, mockEmpty, time.Now())
	if ctx.Editor != "vscode" || ctx.EditorStatus != StatusKnown {
		t.Fatalf("显式自定义 vscode profile 应当被识别为 vscode (known)，实际: %s (%s)", ctx.Editor, ctx.EditorStatus)
	}
}

func TestInspectContextLocalExecutableIdentifiesEditorOnly(t *testing.T) {
	mock := &mockProcessInspector{chain: []ProcessInfo{
		{PID: 8, PPID: 4, Path: `C:\Program Files\cursor\Cursor.exe`, Name: "Cursor.exe"},
		{PID: 4, PPID: 1, Path: `C:\Program Files\cursor\Cursor.exe`, Name: "Cursor.exe"},
	}}
	ctx := InspectContextFull(func(string) string { return "" }, "", "", nil, mock, time.Now())
	if ctx.Editor != "cursor" || ctx.EditorStatus != StatusKnown {
		t.Fatalf("同一本地编辑器的多条进程证据应识别为 cursor，实际: %s (%s)", ctx.Editor, ctx.EditorStatus)
	}
	if ctx.RunnerStatus != StatusUnknown || ctx.Runner != "" {
		t.Fatalf("本地 Editor 证据不应成为 Runner，实际: %s (%s)", ctx.Runner, ctx.RunnerStatus)
	}
	eff, ok := ctx.EffectiveTarget()
	if !ok || eff != "cursor" {
		t.Fatalf("没有 Runner 时应采用 Editor，实际: %s (%t)", eff, ok)
	}
	if mock.lastDepth != defaultAncestorDepth() {
		t.Fatalf("采集深度应使用平台默认值 %d，实际 %d", defaultAncestorDepth(), mock.lastDepth)
	}
}

func TestInspectContextLocalEditorsConflict(t *testing.T) {
	mock := &mockProcessInspector{chain: []ProcessInfo{
		{PID: 3, PPID: 2, Path: `C:\Program Files\cursor\Cursor.exe`, Name: "Cursor.exe"},
		{PID: 2, PPID: 1, Path: `C:\Program Files\Microsoft VS Code\Code.exe`, Name: "Code.exe"},
	}}
	ctx := InspectContextFull(func(string) string { return "" }, "", "", nil, mock, time.Now())
	if ctx.EditorStatus != StatusAmbiguous {
		t.Fatalf("不同本地宿主应保持 ambiguous，实际: %s (%s)", ctx.Editor, ctx.EditorStatus)
	}
	if _, ok := ctx.EffectiveTarget(); ok {
		t.Fatal("无法仲裁的宿主不应产生装配目标")
	}
}

func TestInspectContextCodexRunnerKeepsPriorityOverLocalEditor(t *testing.T) {
	env := map[string]string{"CODEX_SESSION_ID": "session"}
	mock := &mockProcessInspector{chain: []ProcessInfo{
		{PID: 9, PPID: 1, Path: `D:\Apps\Cursor.exe`, Name: "Cursor.exe"},
	}}
	ctx := InspectContextFull(func(k string) string { return env[k] }, "", "", nil, mock, time.Now())
	if ctx.Editor != "cursor" || ctx.Runner != "codex" {
		t.Fatalf("预期 Editor=cursor Runner=codex，实际 editor=%s runner=%s", ctx.Editor, ctx.Runner)
	}
	eff, ok := ctx.EffectiveTarget()
	if !ok || eff != "codex" {
		t.Fatalf("Runner 应优先于本地 Editor，实际: %s (%t)", eff, ok)
	}
	if IsAutomationEnvironmentWithProfiles(func(k string) string { return env[k] }, nil) != (ctx.RunnerStatus == StatusKnown) {
		t.Fatal("自动化判断应继续只看 Runner")
	}
	if isAutomationEnvironmentFull(func(string) string { return "" }, nil, mock) {
		t.Fatal("仅有本地 Editor 进程证据时不应判定为自动化环境")
	}
}
