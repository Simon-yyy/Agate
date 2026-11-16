package agent

import (
	"errors"
	"strings"
	"testing"
)

type mockProcessInspector struct {
	chain         []ProcessInfo
	err           error
	returnPartial bool
	calls         int
	lastDepth     int
}

func (m *mockProcessInspector) GetAncestors(pid int, maxDepth int) ([]ProcessInfo, error) {
	m.calls++
	m.lastDepth = maxDepth
	chain := m.chain
	if maxDepth > 0 && len(chain) > maxDepth {
		chain = chain[:maxDepth]
	}
	if m.err != nil && !m.returnPartial {
		return nil, m.err
	}
	return chain, m.err
}

func TestMatchPathWhitelist(t *testing.T) {
	tests := []struct {
		name      string
		rawPath   string
		wantAgent string
		wantMatch bool
	}{
		{
			name:      "Cursor 远程 Linux 常见路径",
			rawPath:   "/home/user/.cursor-server/bin/94060879.../node",
			wantAgent: "cursor",
			wantMatch: true,
		},
		{
			name:      "Cursor 远程 Windows 路径转换",
			rawPath:   `C:\Users\test\.cursor-server\bin\node.exe`,
			wantAgent: "cursor",
			wantMatch: true,
		},
		{
			name:      "VS Code 远程服务端对照路径",
			rawPath:   "/root/.vscode-server/bin/abcdef/node",
			wantAgent: "vscode",
			wantMatch: true,
		},
		{
			name:      "普通项目代码路径包含 cursor 单词不应误判",
			rawPath:   "/var/projects/my-cursor-app/main.go",
			wantAgent: "",
			wantMatch: false,
		},
		{
			name:      "空路径",
			rawPath:   "",
			wantAgent: "",
			wantMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rule, matched := MatchPathWhitelist(tt.rawPath)
			if matched != tt.wantMatch {
				t.Fatalf("MatchPathWhitelist(%q) matched = %v, want %v", tt.rawPath, matched, tt.wantMatch)
			}
			if matched && rule.Agent != tt.wantAgent {
				t.Fatalf("MatchPathWhitelist(%q) agent = %v, want %v", tt.rawPath, rule.Agent, tt.wantAgent)
			}
		})
	}
}

func TestInspectPathEnvironments(t *testing.T) {
	env := map[string]string{
		"PATH":                    "/usr/local/bin:/home/developer/.cursor-server/bin/123456:/opt/.vscode-server/bin",
		"VSCODE_GIT_ASKPASS_NODE": "/home/developer/.cursor-server/bin/123456/node",
		"OTHER_VAR":               "nothing",
	}

	evidences := InspectPathEnvironments(func(k string) string { return env[k] })
	// PATH 命中 cursor 和 vscode 各一条，VSCODE_GIT_ASKPASS_NODE 命中 cursor 一条，共 3 条
	if len(evidences) != 3 {
		t.Fatalf("预期命中 3 条环境变量路径证据 (含 PATH 拆分多候选)，实际: %d", len(evidences))
	}
	foundCursor := false
	foundVSCode := false
	for _, ev := range evidences {
		if ev.TargetAgent == "cursor" {
			foundCursor = true
		}
		if ev.TargetAgent == "vscode" {
			foundVSCode = true
		}
	}
	if !foundCursor || !foundVSCode {
		t.Errorf("PATH 拆分扫描应同时发现 cursor 与 vscode，实际: cursor=%t, vscode=%t", foundCursor, foundVSCode)
	}
}

func TestInspectProcessAncestors(t *testing.T) {
	mock := &mockProcessInspector{
		chain: []ProcessInfo{
			{PID: 100, PPID: 99, Path: "/usr/bin/bash", Name: "bash"},
			{PID: 99, PPID: 50, Path: "/home/user/.cursor-server/bin/node", Name: "node"},
			{PID: 50, PPID: 1, Path: "/lib/systemd/systemd", Name: "systemd"},
		},
	}

	evs, warnings := InspectProcessAncestors(mock, 100, 4)
	if len(warnings) != 0 {
		t.Errorf("不应产生警告: %v", warnings)
	}
	if len(evs) != 1 {
		t.Fatalf("预期命中 1 条祖先进程证据，实际: %d", len(evs))
	}
	if evs[0].TargetAgent != "cursor" || evs[0].Key != "pid_99" {
		t.Errorf("意外的进程证据: %+v", evs[0])
	}
}

func TestInspectProcessAncestors_LocalEditors(t *testing.T) {
	tests := []struct {
		name      string
		chain     []ProcessInfo
		wantAgent string
		wantHit   bool
	}{
		{
			name: "本地 Cursor.exe 生成 Editor 证据",
			chain: []ProcessInfo{
				{PID: 20, PPID: 10, Path: `C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe`, Name: "powershell.exe"},
				{PID: 10, PPID: 1, Path: `C:\Users\dev\AppData\Local\Programs\cursor\Cursor.exe`, Name: "Cursor.exe"},
			},
			wantAgent: "cursor",
			wantHit:   true,
		},
		{
			name: "本地 Code.exe 识别为 vscode",
			chain: []ProcessInfo{
				{PID: 11, PPID: 1, Path: `D:\Program Files\Microsoft VS Code\Code.exe`, Name: "Code.exe"},
			},
			wantAgent: "vscode",
			wantHit:   true,
		},
		{
			name: "Code - Insiders.exe 识别为 vscode",
			chain: []ProcessInfo{
				{PID: 12, PPID: 1, Path: `D:\VS Code Insiders\Code - Insiders.exe`, Name: "Code - Insiders.exe"},
			},
			wantAgent: "vscode",
			wantHit:   true,
		},
		{
			name: "普通 PowerShell 不产生宿主证据",
			chain: []ProcessInfo{
				{PID: 30, PPID: 1, Path: `C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe`, Name: "powershell.exe"},
			},
		},
		{
			name: "路径包含 cursor 但不是编辑器可执行文件",
			chain: []ProcessInfo{
				{PID: 31, PPID: 1, Path: `D:\projects\cursor-helper.exe`, Name: "cursor-helper.exe"},
			},
		},
		{
			name: "node.exe 即使位于 cursor 目录也不命中",
			chain: []ProcessInfo{
				{PID: 32, PPID: 1, Path: `C:\Program Files\cursor\resources\app\node.exe`, Name: "node.exe"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			evs, warnings := InspectProcessAncestors(&mockProcessInspector{chain: tt.chain}, 0, 8)
			if len(warnings) != 0 {
				t.Fatalf("不应产生警告: %v", warnings)
			}
			if !tt.wantHit {
				if len(evs) != 0 {
					t.Fatalf("不应产生宿主证据，实际: %+v", evs)
				}
				return
			}
			if len(evs) != 1 {
				t.Fatalf("预期 1 条 Editor 证据，实际: %+v", evs)
			}
			ev := evs[0]
			if ev.TargetAgent != tt.wantAgent || ev.TargetRole != "editor" || ev.Source != EvidenceSourceProcess {
				t.Fatalf("证据归属不符: %+v", ev)
			}
			if strings.Contains(ev.Description, `C:\Users`) || strings.Contains(ev.Description, `D:\`) {
				t.Fatalf("证据说明不应包含完整安装路径: %s", ev.Description)
			}
		})
	}
}

func TestInspectProcessAncestors_PartialChainKept(t *testing.T) {
	mock := &mockProcessInspector{
		returnPartial: true,
		err:           errors.New("父进程已退出"),
		chain: []ProcessInfo{
			{PID: 10, PPID: 1, Path: `C:\Users\dev\AppData\Local\Programs\cursor\Cursor.exe`, Name: "Cursor.exe"},
		},
	}
	evs, warnings := InspectProcessAncestors(mock, 10, 8)
	if len(evs) != 1 || evs[0].TargetAgent != "cursor" {
		t.Fatalf("已核验节点应保留，实际证据: %+v", evs)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "父进程已退出") {
		t.Fatalf("应记录降级警告，实际: %v", warnings)
	}
}

func TestInspectProcessAncestors_Degraded(t *testing.T) {
	mock := &mockProcessInspector{
		err: errors.New("permission denied (/proc unreadable in container)"),
	}

	evs, warnings := InspectProcessAncestors(mock, 100, 4)
	if len(evs) != 0 {
		t.Errorf("降级时不应有命中证据，实际: %d", len(evs))
	}
	if len(warnings) != 1 {
		t.Fatalf("降级时预期产生 1 条警告，实际: %d", len(warnings))
	}
}
