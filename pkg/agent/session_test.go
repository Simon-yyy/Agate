package agent

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestParseSessionContext(t *testing.T) {
	now := time.Unix(1700000000, 0)

	t.Run("合法声明完整解析", func(t *testing.T) {
		raw := "v=1;editor=cursor;runner=codex;sid=sess-123;src=terminal;ts=1699999000;exp=1700001000"
		sc, warnings, err := ParseSessionContext(raw, now)
		if err != nil {
			t.Fatalf("意外解析失败: %v", err)
		}
		if len(warnings) != 0 {
			t.Errorf("不应产生告警: %v", warnings)
		}
		if sc.Version != "1" || sc.Editor != "cursor" || sc.Runner != "codex" || sc.SessionID != "sess-123" {
			t.Errorf("意外的解析结果: %+v", sc)
		}
	})

	t.Run("空输入返回空", func(t *testing.T) {
		sc, warnings, err := ParseSessionContext("", now)
		if err != nil || sc != nil || len(warnings) != 0 {
			t.Errorf("空输入预期全空，实际: sc=%v, w=%v, err=%v", sc, warnings, err)
		}
	})

	t.Run("版本缺失或不支持", func(t *testing.T) {
		raw := "v=2;editor=cursor"
		sc, warnings, err := ParseSessionContext(raw, now)
		if err == nil || sc != nil {
			t.Fatalf("预期版本错误，实际成功: %+v", sc)
		}
		if len(warnings) == 0 {
			t.Errorf("预期产生版本告警")
		}
	})

	t.Run("长度超限截断拒绝", func(t *testing.T) {
		raw := "v=1;editor=cursor;long=" + strings.Repeat("a", 600)
		sc, warnings, err := ParseSessionContext(raw, now)
		if err == nil || sc != nil {
			t.Fatalf("预期长度超限拒绝，实际成功")
		}
		if len(warnings) == 0 {
			t.Errorf("预期产生超长告警")
		}
	})

	t.Run("凭据防火墙拦截", func(t *testing.T) {
		raw := "v=1;editor=cursor;token=secret_token_12345"
		sc, warnings, err := ParseSessionContext(raw, now)
		if err == nil || sc != nil {
			t.Fatalf("预期凭据拦截，实际成功")
		}
		if len(warnings) == 0 || !strings.Contains(warnings[0], "疑似凭据特征") {
			t.Errorf("预期产生凭据拦截告警: %v", warnings)
		}
	})

	t.Run("过期声明被拒绝", func(t *testing.T) {
		raw := "v=1;editor=cursor;exp=1699999000" // 早于 now (1700000000)
		sc, warnings, err := ParseSessionContext(raw, now)
		if err == nil || sc != nil {
			t.Fatalf("预期过期拒绝，实际成功")
		}
		if len(warnings) == 0 || !strings.Contains(warnings[0], "过期") {
			t.Errorf("预期产生过期告警: %v", warnings)
		}
	})

	t.Run("非法时间格式被拒绝拦截", func(t *testing.T) {
		raw := "v=1;runner=codex;exp=oops"
		sc, warnings, err := ParseSessionContext(raw, now)
		if err == nil || sc != nil {
			t.Fatalf("预期 exp=oops 被拒绝，实际成功")
		}
		if len(warnings) == 0 || !strings.Contains(warnings[0], "格式非法") {
			t.Errorf("预期产生时间格式非法告警: %v", warnings)
		}
	})

	t.Run("时序倒挂被拒绝拦截", func(t *testing.T) {
		raw := "v=1;runner=codex;ts=1700001000;exp=1700000000"
		sc, warnings, err := ParseSessionContext(raw, now)
		if err == nil || sc != nil {
			t.Fatalf("预期 exp <= ts 被拒绝，实际成功")
		}
		if len(warnings) == 0 || !strings.Contains(warnings[0], "不得早于或等于生成时间") {
			t.Errorf("预期产生时序逻辑非法告警: %v", warnings)
		}
	})

	t.Run("缺失时效约束被拒绝拦截 (R2-F03)", func(t *testing.T) {
		raw := "v=1;runner=codex"
		sc, warnings, err := ParseSessionContext(raw, now)
		if err == nil || sc != nil {
			t.Fatalf("预期缺失 exp 被拒绝，实际成功")
		}
		if len(warnings) == 0 || !strings.Contains(warnings[0], "缺失时效约束") {
			t.Errorf("预期产生缺失时效告警: %v", warnings)
		}
	})

	t.Run("私有路径来源脱敏与告警脱敏 (R2-S01)", func(t *testing.T) {
		// 1. source 传入私有 socket 路径，应脱敏为 custom
		rawValid := "v=1;runner=codex;src=/home/user/private/socket;exp=1700001000"
		sc, _, err := ParseSessionContext(rawValid, now)
		if err != nil || sc == nil {
			t.Fatalf("解析有效声明失败: %v", err)
		}
		if sc.Source != "custom" {
			t.Errorf("私有路径 source 预期脱敏为 custom，实际: %s", sc.Source)
		}

		// 2. 传入包含私有路径的非法 exp，告警文本中绝不反射泄露该私有路径
		rawBad := "v=1;runner=codex;exp=/home/secret/path/to/leak"
		_, badWarnings, badErr := ParseSessionContext(rawBad, now)
		if badErr == nil {
			t.Fatalf("预期非法 exp 报错")
		}
		if len(badWarnings) == 0 {
			t.Fatalf("预期产生告警")
		}
		for _, w := range badWarnings {
			if strings.Contains(w, "/home/secret/path/to/leak") {
				t.Fatalf("安全红线拦截：告警中反射泄露了私有路径！内容: %s", w)
			}
		}
	})

	t.Run("身份字段路径及控制字符脱敏拦截 (R3-S01)", func(t *testing.T) {
		// runner 或 editor 包含私有绝对路径，必须报错拦截，且告警不包含该私有路径
		rawPathRunner := "v=1;runner=/home/example/private;exp=1700001000"
		_, badWarnings, err := ParseSessionContext(rawPathRunner, now)
		if err == nil {
			t.Fatalf("预期包含路径的 runner 被拒绝拦截")
		}
		for _, w := range badWarnings {
			if strings.Contains(w, "/home/example/private") {
				t.Fatalf("安全红线拦截：非法 runner 告警反射泄露了私有路径: %s", w)
			}
		}

		rawPathEditor := "v=1;editor=/var/run/cursor.sock;exp=1700001000"
		_, badEditorWarnings, errEditor := ParseSessionContext(rawPathEditor, now)
		if errEditor == nil {
			t.Fatalf("预期包含路径的 editor 被拒绝拦截")
		}
		for _, w := range badEditorWarnings {
			if strings.Contains(w, "/var/run/cursor.sock") {
				t.Fatalf("安全红线拦截：非法 editor 告警反射泄露了私有路径: %s", w)
			}
		}
	})

	t.Run("会话适用范围不匹配拦截 (R3-F01)", func(t *testing.T) {
		// 声明为 src=ssh，但当前环境并非 SSH（无 SSH_CONNECTION/SSH_CLIENT）
		env := map[string]string{
			"AGATE_SESSION_CONTEXT": "v=1;runner=codex;src=ssh;exp=1700001000",
		}
		getenv := func(k string) string { return env[k] }

		sc, evs, warnings := InspectSessionContext(getenv, now)
		if sc != nil || len(evs) > 0 {
			t.Fatalf("范围不匹配的声明不得生成确定证据，实际: sc=%+v, evs=%v", sc, evs)
		}
		foundMismatch := false
		for _, w := range warnings {
			if strings.Contains(w, "声明适用范围不匹配") {
				foundMismatch = true
				break
			}
		}
		if !foundMismatch {
			t.Fatalf("预期产生适用范围不匹配警告，实际: %v", warnings)
		}
	})

	t.Run("畸形段与非法SessionID及脱敏拦截 (R4-03 & R4-04)", func(t *testing.T) {
		// 1. 不含等号的非法片段
		rawMalformed := "v=1;runner=codex;raw_malformed_token;exp=1700001000"
		_, malformedWarnings, errMalformed := ParseSessionContext(rawMalformed, now)
		if errMalformed == nil {
			t.Fatalf("预期不含 '=' 的畸形片段直接拒绝")
		}
		if len(malformedWarnings) == 0 {
			t.Fatalf("预期产生畸形片段警告")
		}

		// 2. 包含非法字符（如空格与注入符号）的 sid
		rawBadSID := "v=1;runner=codex;sid=bad sid;exp=1700001000"
		_, badSIDWarnings, errBadSID := ParseSessionContext(rawBadSID, now)
		if errBadSID == nil {
			t.Fatalf("预期包含非法字符的 sid 被拦截拒绝")
		}
		for _, w := range badSIDWarnings {
			if strings.Contains(w, "bad sid") {
				t.Fatalf("安全红线拦截：非法 sid 告警反射泄露了私有信息: %s", w)
			}
		}

		// 3. 验证 SessionID 不会被 JSON 序列化导出
		validSC := &SessionContext{
			Version:   "1",
			Runner:    "codex",
			SessionID: "secret-session-id-123",
		}
		jsonBytes, err := json.Marshal(validSC)
		if err != nil {
			t.Fatalf("序列化失败: %v", err)
		}
		if strings.Contains(string(jsonBytes), "secret-session-id-123") || strings.Contains(string(jsonBytes), "session_id") {
			t.Fatalf("安全红线拦截：SessionContext 结构体 JSON 序列化泄漏了 session_id: %s", string(jsonBytes))
		}
	})

	t.Run("终端会话ID冲突与未知来源拦截 (R4-03)", func(t *testing.T) {
		// 1. live SID 与信封 sid 不一致（跨终端残留）
		envConflict := map[string]string{
			"AGATE_SESSION_ID":      "term-window-A",
			"AGATE_SESSION_CONTEXT": "v=1;runner=codex;src=terminal;sid=term-window-B;exp=1700001000",
		}
		getenvConflict := func(k string) string { return envConflict[k] }
		scConflict, _, warningsConflict := InspectSessionContext(getenvConflict, now)
		if scConflict != nil {
			t.Fatalf("跨终端窗口会话冲突的信封不得采信，实际: %+v", scConflict)
		}
		foundConflictWarn := false
		for _, w := range warningsConflict {
			if strings.Contains(w, "跨窗口/跨会话残留") {
				foundConflictWarn = true
				break
			}
		}
		if !foundConflictWarn {
			t.Fatalf("预期产生跨会话残留告警，实际: %v", warningsConflict)
		}

		// 2. src=unknown 或空来源
		envUnknownSrc := map[string]string{
			"AGATE_SESSION_CONTEXT": "v=1;runner=codex;src=unknown;exp=1700001000",
		}
		getenvUnknown := func(k string) string { return envUnknownSrc[k] }
		scUnknown, _, warningsUnknown := InspectSessionContext(getenvUnknown, now)
		if scUnknown != nil {
			t.Fatalf("缺失合规来源的信封不得采信，实际: %+v", scUnknown)
		}
		foundUnknownWarn := false
		for _, w := range warningsUnknown {
			if strings.Contains(w, "缺失合规来源特征") {
				foundUnknownWarn = true
				break
			}
		}
		if !foundUnknownWarn {
			t.Fatalf("预期产生缺失合规来源特征告警，实际: %v", warningsUnknown)
		}
	})
}
