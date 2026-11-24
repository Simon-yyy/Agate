package adapter

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"agate/pkg/agent"
	"agate/pkg/harness"
)

// NativeHookPlan 描述宿主原生 Hook 的文件落盘计划。
type NativeHookPlan struct {
	Target     agent.Kind
	Path       string
	Content    []byte
	Existing   bool
	Tracked    bool
	Skipped    bool
	Conflict   bool
	Reason     string
	BackupPath   string
	SourceDigest string
}

// isVerifiedNativeHookHost 判断目标是否已有核对过的原生适配器。
func isVerifiedNativeHookHost(target agent.Kind) bool {
	return agent.SupportsNativeHook(target)
}

type hookInvocation struct {
	Prefix   string
	Diverged bool
	Note     string
}

// resolveHookInvocation 决定 Hook 命令前缀。测试可替换。
var resolveHookInvocation = defaultHookInvocation

func desiredHookEvents(target agent.Kind, prefix string) (relPath string, events map[string]string, topLevelVersion bool, err error) {
	if strings.TrimSpace(prefix) == "" {
		prefix = "agate"
	}
	cmd := func(host, event string) string {
		return prefix + " prompt --hook --host " + host + " --event " + event
	}
	switch target {
	case agent.Cursor:
		return filepath.Join(".cursor", "hooks.json"), map[string]string{
			"beforeSubmitPrompt": cmd("cursor", "beforeSubmitPrompt"),
			"postToolUse":        cmd("cursor", "postToolUse"),
			"afterAgentResponse": cmd("cursor", "afterAgentResponse"),
			"preToolUse":         cmd("cursor", "preToolUse"),
		}, true, nil
	case agent.Antigravity:
		return filepath.Join(".agents", "hooks.json"), map[string]string{
			"PreInvocation":  cmd("antigravity", "PreInvocation"),
			"PostToolUse":    cmd("antigravity", "PostToolUse"),
			"PostInvocation": cmd("antigravity", "PostInvocation"),
			"PreToolUse":     cmd("antigravity", "PreToolUse"),
		}, true, nil
	case agent.Claude:
		return filepath.Join(".claude", "settings.json"), map[string]string{
			"UserPromptSubmit": cmd("claude", "UserPromptSubmit"),
			"PostToolUse":      cmd("claude", "PostToolUse"),
			"Stop":             cmd("claude", "Stop"),
			"PreToolUse":       cmd("claude", "PreToolUse"),
		}, false, nil
	case agent.Codex:
		return filepath.Join(".codex", "hooks.json"), map[string]string{
			"UserPromptSubmit": cmd("codex", "UserPromptSubmit"),
			"PostToolUse":      cmd("codex", "PostToolUse"),
			"Stop":             cmd("codex", "Stop"),
			"PreToolUse":       cmd("codex", "PreToolUse"),
		}, false, nil
	case agent.Windsurf:
		return filepath.Join(".windsurf", "hooks.json"), map[string]string{
			"pre_user_prompt":       cmd("windsurf", "pre_user_prompt"),
			"post_read_code":        cmd("windsurf", "post_read_code"),
			"post_cascade_response": cmd("windsurf", "post_cascade_response"),
			"pre_write_code":        cmd("windsurf", "pre_write_code"),
		}, false, nil
	default:
		return "", nil, false, fmt.Errorf("非受管宿主")
	}
}

func defaultHookInvocation() hookInvocation {
	exe, err := os.Executable()
	if err != nil {
		return hookInvocation{Prefix: "agate"}
	}
	base := strings.ToLower(filepath.Base(exe))
	if base != "agate" && base != "agate.exe" {
		return hookInvocation{Prefix: "agate"}
	}
	found, lookErr := exec.LookPath("agate")
	if lookErr != nil {
		return hookInvocation{
			Prefix:   quoteExecutable(exe),
			Diverged: true,
			Note:     "PATH 上没有 agate，隔离配置使用当前程序的绝对路径",
		}
	}
	same, sameErr := sameExecutable(exe, found)
	if sameErr == nil && same {
		return hookInvocation{Prefix: "agate"}
	}
	return hookInvocation{
		Prefix:   quoteExecutable(exe),
		Diverged: true,
		Note:     "PATH 上的 agate 指向另一份程序，隔离配置使用当前程序的绝对路径",
	}
}

func sameExecutable(left, right string) (bool, error) {
	leftInfo, err := os.Stat(left)
	if err != nil {
		return false, err
	}
	rightInfo, err := os.Stat(right)
	if err != nil {
		return false, err
	}
	return os.SameFile(leftInfo, rightInfo), nil
}

func quoteExecutable(path string) string {
	if strings.ContainsAny(path, " \t\"") {
		return `"` + strings.ReplaceAll(path, `"`, `\"`) + `"`
	}
	return path
}

func currentHookPrefix() string {
	inv := resolveHookInvocation()
	if inv.Diverged {
		return inv.Prefix
	}
	return "agate"
}

const agateManagedKey = "agateManaged"

func isExactAgateCommand(entry map[string]any, expectedCommand string) bool {
	return entryCommand(entry) == strings.TrimSpace(expectedCommand)
}

func isMarkedAgateEntry(entry map[string]any, expectedCommand string) bool {
	marked, _ := entry[agateManagedKey].(bool)
	return marked && isExactAgateCommand(entry, expectedCommand)
}

func entryCommand(entry map[string]any) string {
	cmd, _ := entry["command"].(string)
	return strings.TrimSpace(cmd)
}

func cloneEntry(src map[string]any) map[string]any {
	out := make(map[string]any, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

func newAgateEntry(command string, preserveFailClosed any) map[string]any {
	entry := map[string]any{"command": command, agateManagedKey: true}
	if preserveFailClosed != nil {
		entry["failClosed"] = preserveFailClosed
	}
	return entry
}

// mergeNativeHookConfig 按宿主结构合并已有 JSON：保留用户条目、未知字段、启停属性与顺序。
func mergeNativeHookConfig(existing []byte, desiredEvents map[string]string, withVersion bool) ([]byte, error) {
	var root map[string]any
	if len(bytes.TrimSpace(existing)) == 0 {
		root = map[string]any{}
	} else {
		dec := json.NewDecoder(bytes.NewReader(existing))
		dec.UseNumber()
		if err := dec.Decode(&root); err != nil {
			return nil, fmt.Errorf("非法 Hook JSON: %w", err)
		}
		if root == nil {
			root = map[string]any{}
		}
	}

	hooksVal, ok := root["hooks"]
	var hooks map[string]any
	if !ok || hooksVal == nil {
		hooks = map[string]any{}
		root["hooks"] = hooks
	} else {
		hooks, ok = hooksVal.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("hooks 字段类型冲突")
		}
	}

	for eventName, command := range desiredEvents {
		rawList, exists := hooks[eventName]
		var list []any
		if exists && rawList != nil {
			arr, ok := rawList.([]any)
			if !ok {
				return nil, fmt.Errorf("事件 %s 的 hooks 列表类型冲突", eventName)
			}
			list = arr
		}

		replaced := false
		var preservedFailClosed any
		newList := make([]any, 0, len(list)+1)
		agateCount := 0
		for _, item := range list {
			entry, ok := item.(map[string]any)
			if !ok {
				newList = append(newList, item)
				continue
			}
			if isExactAgateCommand(entry, command) {
				agateCount++
				if agateCount > 1 {
					return nil, fmt.Errorf("事件 %s 存在多条归属不明的 Agate Hook 条目", eventName)
				}
				if fc, ok := entry["failClosed"]; ok {
					preservedFailClosed = fc
				}
				// Cursor：保留已存在的 preToolUse.failClosed
				if eventName == "preToolUse" && preservedFailClosed == nil {
					preservedFailClosed = true
				}
				newList = append(newList, newAgateEntry(command, preservedFailClosed))
				replaced = true
				continue
			}
			newList = append(newList, cloneEntry(entry))
		}
		if !replaced {
			var fc any
			if eventName == "preToolUse" {
				fc = true
			}
			newList = append(newList, newAgateEntry(command, fc))
		}
		hooks[eventName] = newList
	}

	if withVersion {
		if _, ok := root["version"]; !ok {
			root["version"] = 1
		}
	}

	return json.MarshalIndent(root, "", "  ")
}

const antigravityHookGroup = "agate"

func isAntigravityToolEvent(name string) bool {
	return name == "PreToolUse" || name == "PostToolUse"
}

func newAntigravityCommand(command string) map[string]any {
	entry := newAgateEntry(command, nil)
	entry["type"] = "command"
	return entry
}

func mergeAntigravityHookConfig(existing []byte, desiredEvents map[string]string) ([]byte, error) {
	var root map[string]any
	if len(bytes.TrimSpace(existing)) == 0 {
		root = map[string]any{}
	} else {
		dec := json.NewDecoder(bytes.NewReader(existing))
		dec.UseNumber()
		if err := dec.Decode(&root); err != nil {
			return nil, fmt.Errorf("非法 Hook JSON: %w", err)
		}
		if root == nil {
			root = map[string]any{}
		}
	}
	rawGroup, ok := root[antigravityHookGroup]
	var group map[string]any
	if !ok || rawGroup == nil {
		group = map[string]any{}
		root[antigravityHookGroup] = group
	} else {
		group, ok = rawGroup.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("agate 分组类型冲突")
		}
	}
	for eventName, command := range desiredEvents {
		if isAntigravityToolEvent(eventName) {
			if err := mergeAntigravityToolEvent(group, eventName, command); err != nil {
				return nil, err
			}
			continue
		}
		if err := mergeAntigravityDirectEvent(group, eventName, command); err != nil {
			return nil, err
		}
	}
	return json.MarshalIndent(root, "", "  ")
}

func mergeAntigravityDirectEvent(group map[string]any, eventName, command string) error {
	var list []any
	if raw, ok := group[eventName]; ok && raw != nil {
		arr, ok := raw.([]any)
		if !ok {
			return fmt.Errorf("事件 %s 的配置类型冲突", eventName)
		}
		list = arr
	}
	rewritten, count, err := rewriteManagedCommands(list, eventName, command, func() map[string]any {
		return newAntigravityCommand(command)
	})
	if err != nil {
		return err
	}
	if count == 0 {
		rewritten = append(rewritten, newAntigravityCommand(command))
	}
	group[eventName] = rewritten
	return nil
}

func mergeAntigravityToolEvent(group map[string]any, eventName, command string) error {
	var groups []any
	if raw, ok := group[eventName]; ok && raw != nil {
		arr, ok := raw.([]any)
		if !ok {
			return fmt.Errorf("事件 %s 的配置类型冲突", eventName)
		}
		groups = arr
	}
	total := 0
	for i, item := range groups {
		bucket, ok := item.(map[string]any)
		if !ok {
			continue
		}
		hooks, ok := bucket["hooks"].([]any)
		if !ok {
			continue
		}
		rewritten, count, err := rewriteManagedCommands(hooks, eventName, command, func() map[string]any {
			return newAntigravityCommand(command)
		})
		if err != nil {
			return err
		}
		total += count
		if total > 1 {
			return fmt.Errorf("事件 %s 存在多条归属不明的 Agate Hook 条目", eventName)
		}
		bucket["hooks"] = rewritten
		groups[i] = bucket
	}
	if total == 0 {
		groups = append(groups, map[string]any{
			"matcher": "*",
			"hooks":   []any{newAntigravityCommand(command)},
		})
	}
	group[eventName] = groups
	return nil
}

func rewriteManagedCommands(list []any, eventName, command string, fresh func() map[string]any) ([]any, int, error) {
	count := 0
	newList := make([]any, 0, len(list))
	for _, item := range list {
		entry, ok := item.(map[string]any)
		if !ok {
			newList = append(newList, item)
			continue
		}
		if isExactAgateCommand(entry, command) {
			count++
			if count > 1 {
				return nil, count, fmt.Errorf("事件 %s 存在多条归属不明的 Agate Hook 条目", eventName)
			}
			created := fresh()
			if fc, ok := entry["failClosed"]; ok {
				created["failClosed"] = fc
			}
			newList = append(newList, created)
			continue
		}
		newList = append(newList, cloneEntry(entry))
	}
	return newList, count, nil
}

func antigravityConfigHasManagedEntry(root map[string]any, events map[string]string) (bool, error) {
	group, _ := root[antigravityHookGroup].(map[string]any)
	if group == nil {
		return false, nil
	}
	for eventName, expected := range events {
		raw, ok := group[eventName]
		if !ok || raw == nil {
			continue
		}
		if isAntigravityToolEvent(eventName) {
			groups, ok := raw.([]any)
			if !ok {
				continue
			}
			for _, item := range groups {
				bucket, ok := item.(map[string]any)
				if !ok {
					continue
				}
				hooks, ok := bucket["hooks"].([]any)
				if !ok {
					continue
				}
				if commandListHasManaged(hooks, expected) {
					return true, nil
				}
			}
			continue
		}
		arr, ok := raw.([]any)
		if ok && commandListHasManaged(arr, expected) {
			return true, nil
		}
	}
	return false, nil
}

func commandListHasManaged(list []any, expected string) bool {
	for _, item := range list {
		entry, ok := item.(map[string]any)
		if ok && isMarkedAgateEntry(entry, expected) {
			return true
		}
	}
	return false
}

func rollbackAntigravityHooks(rootObj map[string]any, events map[string]string) error {
	group, _ := rootObj[antigravityHookGroup].(map[string]any)
	if group == nil {
		return fmt.Errorf("当前文件缺少 agate 分组，拒绝回退")
	}
	for eventName, expected := range events {
		raw, ok := group[eventName]
		if !ok || raw == nil {
			continue
		}
		if isAntigravityToolEvent(eventName) {
			groups, ok := raw.([]any)
			if !ok {
				return fmt.Errorf("事件 %s 列表类型异常", eventName)
			}
			keptGroups := make([]any, 0, len(groups))
			for _, item := range groups {
				bucket, ok := item.(map[string]any)
				if !ok {
					keptGroups = append(keptGroups, item)
					continue
				}
				hooks, ok := bucket["hooks"].([]any)
				if !ok {
					keptGroups = append(keptGroups, bucket)
					continue
				}
				kept := stripMarkedCommands(hooks, expected)
				if len(kept) == 0 {
					continue
				}
				bucket["hooks"] = kept
				keptGroups = append(keptGroups, bucket)
			}
			if len(keptGroups) == 0 {
				delete(group, eventName)
			} else {
				group[eventName] = keptGroups
			}
			continue
		}
		arr, ok := raw.([]any)
		if !ok {
			return fmt.Errorf("事件 %s 列表类型异常", eventName)
		}
		kept := stripMarkedCommands(arr, expected)
		if len(kept) == 0 {
			delete(group, eventName)
		} else {
			group[eventName] = kept
		}
	}
	return nil
}

func stripMarkedCommands(list []any, expected string) []any {
	kept := make([]any, 0, len(list))
	for _, item := range list {
		entry, ok := item.(map[string]any)
		if ok && isMarkedAgateEntry(entry, expected) {
			continue
		}
		kept = append(kept, item)
	}
	return kept
}

// PlanNativeHooks 生成指定宿主的原生 Hook 装配计划。
func PlanNativeHooks(root string, target agent.Kind, rt agent.NativeRuntime, mode agent.NativeMode) (*NativeHookPlan, error) {
	return PlanNativeHooksWithOptions(root, target, rt, mode, false)
}

// PlanNativeHooksWithOptions 支持 Git 跟踪保护开关。
func PlanNativeHooksWithOptions(root string, target agent.Kind, rt agent.NativeRuntime, mode agent.NativeMode, allowTracked bool) (*NativeHookPlan, error) {
	if mode == agent.NativeModeOff {
		return &NativeHookPlan{Target: target, Skipped: true, Reason: "用户显式指定 --native-hooks=off"}, nil
	}

	verified := isVerifiedNativeHookHost(target)
	if !verified {
		reason := fmt.Sprintf("宿主 %s 协议未核对，保持阻塞（不生成可加载 Hook）", target)
		if mode == agent.NativeModeRequire {
			return nil, fmt.Errorf("%s (--native-hooks=require)", reason)
		}
		return &NativeHookPlan{Target: target, Skipped: true, Reason: reason, Conflict: false}, nil
	}

	if mode == agent.NativeModeAuto {
		if rt.Host != "" && rt.Host != target {
			return &NativeHookPlan{
				Target:  target,
				Skipped: true,
				Reason:  fmt.Sprintf("证据不足：检测宿主为 %s，跳过目标 %s", rt.Host, target),
			}, nil
		}
		if !rt.SupportsHook {
			return &NativeHookPlan{Target: target, Skipped: true, Reason: fmt.Sprintf("宿主 %s 尚未开启原生 Hook 自动装配", target)}, nil
		}
	}

	if mode == agent.NativeModeRequire {
		if rt.Host != target {
			return nil, fmt.Errorf("require 模式要求单一目标与检测宿主匹配: 目标=%s 宿主=%s", target, rt.Host)
		}
		if strings.TrimSpace(rt.ClientVer) == "" {
			return nil, fmt.Errorf("require 模式需要已验证的客户端版本，当前宿主 %s 版本未知", target)
		}
		if !rt.SupportsHook {
			return nil, fmt.Errorf("宿主 %s 尚未提供经验证的原生 Hook 适配支持 (--native-hooks=require)", target)
		}
	}

	if mode == agent.NativeModeAuto || mode == agent.NativeModeRequire {
		if !agent.NativeHookVersionAccepted(target, rt.ClientVer) {
			reason := fmt.Sprintf("宿主 %s 的版本 %q 尚未验收", target, strings.TrimSpace(rt.ClientVer))
			if mode == agent.NativeModeRequire {
				return nil, fmt.Errorf("%s (--native-hooks=require)", reason)
			}
			return &NativeHookPlan{Target: target, Skipped: true, Reason: reason}, nil
		}
	}

	inv := resolveHookInvocation()
	relPath, events, withVersion, err := desiredHookEvents(target, "agate")
	if err != nil {
		return &NativeHookPlan{Target: target, Skipped: true, Reason: err.Error()}, nil
	}

	fullPath := filepath.Join(root, relPath)
	info, statErr := os.Lstat(fullPath)
	existing := statErr == nil
	if existing && info.Mode()&os.ModeSymlink != 0 {
		return &NativeHookPlan{
			Target:   target,
			Path:     fullPath,
			Existing: true,
			Conflict: true,
			Skipped:  true,
			Reason:   "Hook 配置为符号链接，拒绝写入",
		}, nil
	}

	var existingBytes []byte
	if existing {
		existingBytes, err = os.ReadFile(fullPath)
		if err != nil {
			return nil, fmt.Errorf("读取已有 Hook 配置失败: %w", err)
		}
	}

	var merged []byte
	var mergeErr error
	if target == agent.Antigravity {
		merged, mergeErr = mergeAntigravityHookConfig(existingBytes, events)
	} else {
		merged, mergeErr = mergeNativeHookConfig(existingBytes, events, withVersion)
	}
	if mergeErr != nil {
		return &NativeHookPlan{
			Target:   target,
			Path:     fullPath,
			Existing: existing,
			Conflict: true,
			Skipped:  true,
			Reason:   mergeErr.Error(),
		}, nil
	}

	tracked, trackErr := CheckPathTracked(root, filepath.ToSlash(relPath))
	if trackErr != nil {
		return nil, trackErr
	}
	if inv.Diverged && tracked {
		return &NativeHookPlan{
			Target:   target,
			Path:     fullPath,
			Existing: existing,
			Tracked:  true,
			Skipped:  true,
			Reason:   "PATH 上的 agate 与当前程序不是同一文件，拒绝把本机绝对路径写入已跟踪的 Hook 配置",
		}, nil
	}
	if inv.Diverged {
		_, events, withVersion, err = desiredHookEvents(target, inv.Prefix)
		if err != nil {
			return &NativeHookPlan{Target: target, Skipped: true, Reason: err.Error()}, nil
		}
		var mergeErr error
		if target == agent.Antigravity {
			merged, mergeErr = mergeAntigravityHookConfig(existingBytes, events)
		} else {
			merged, mergeErr = mergeNativeHookConfig(existingBytes, events, withVersion)
		}
		if mergeErr != nil {
			return &NativeHookPlan{
				Target:   target,
				Path:     fullPath,
				Existing: existing,
				Conflict: true,
				Skipped:  true,
				Reason:   mergeErr.Error(),
			}, nil
		}
	}
	if tracked && !allowTracked {
		return &NativeHookPlan{
			Target:   target,
			Path:     fullPath,
			Content:  merged,
			Existing: existing,
			Tracked:  true,
			Conflict: true,
			Skipped:  true,
			Reason:   "Hook 配置已被 Git 跟踪，需 --allow-tracked 才可更新受管条目",
		}, nil
	}

	backupPath := fullPath + ".agate-backup"
	plan := &NativeHookPlan{
		Target:       target,
		Path:         fullPath,
		Content:      merged,
		Existing:     existing,
		Tracked:      tracked,
		BackupPath:   backupPath,
		SourceDigest: hookSourceDigest(existingBytes),
	}
	if inv.Diverged {
		plan.Reason = inv.Note
	}
	return plan, nil
}

func hookSourceDigest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// ExecuteNativeHookPlan 执行原生 Hook 计划落盘；冲突或跳过时不写字节。
func ExecuteNativeHookPlan(plan *NativeHookPlan) error {
	if plan == nil || plan.Skipped || plan.Conflict {
		return nil
	}
	parent := filepath.Dir(plan.Path)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return fmt.Errorf("创建 Hook 配置目录失败: %w", err)
	}
	current, readErr := os.ReadFile(plan.Path)
	if readErr != nil {
		if !os.IsNotExist(readErr) {
			return fmt.Errorf("复核 Hook 配置失败: %w", readErr)
		}
		current = nil
	}
	if hookSourceDigest(current) != plan.SourceDigest {
		return fmt.Errorf("Hook 配置在计划生成后已变化，拒绝写入")
	}
	if plan.Existing {
		if err := harness.WriteFileAtomic(plan.BackupPath, current, 0o644); err != nil {
			return fmt.Errorf("写入 Hook 备份失败: %w", err)
		}
	}
	return harness.WriteFileAtomic(plan.Path, plan.Content, 0o644)
}

// NativeHookStatus 描述单个宿主 Hook 的当前状态。
type NativeHookStatus struct {
	Host       agent.Kind `json:"host"`
	Path       string     `json:"path"`
	Exists     bool       `json:"exists"`
	Tracked    bool       `json:"tracked"`
	HasAgate   bool       `json:"has_agate"`
	BackupPath string     `json:"backup_path,omitempty"`
	Blocked    bool       `json:"blocked"`
	Reason     string     `json:"reason,omitempty"`
}

// hookFileHasManagedEntry 解析 Hook JSON，只认已标记且命令与受管命令完全一致的条目。
func hookFileHasManagedEntry(host agent.Kind, data []byte, events map[string]string) (bool, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return false, nil
	}
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		return false, fmt.Errorf("非法 JSON")
	}
	if host == agent.Antigravity {
		return antigravityConfigHasManagedEntry(root, events)
	}
	hooks, _ := root["hooks"].(map[string]any)
	if hooks == nil {
		return false, nil
	}
	for eventName, expected := range events {
		raw, ok := hooks[eventName]
		if !ok || raw == nil {
			continue
		}
		arr, ok := raw.([]any)
		if !ok {
			continue
		}
		for _, item := range arr {
			entry, ok := item.(map[string]any)
			if ok && isMarkedAgateEntry(entry, expected) {
				return true, nil
			}
		}
	}
	return false, nil
}

// ListNativeHookStatus 汇总已核对与阻塞宿主的 Hook 状态。
func ListNativeHookStatus(root string) ([]NativeHookStatus, error) {
	var out []NativeHookStatus
	for _, kind := range agent.All() {
		st := NativeHookStatus{Host: kind}
		if !isVerifiedNativeHookHost(kind) {
			st.Blocked = true
			st.Reason = "协议未核对"
			out = append(out, st)
			continue
		}
		rel, events, _, err := desiredHookEvents(kind, currentHookPrefix())
		if err != nil {
			st.Blocked = true
			st.Reason = err.Error()
			out = append(out, st)
			continue
		}
		st.Path = filepath.Join(root, rel)
		st.BackupPath = st.Path + ".agate-backup"
		if info, err := os.Lstat(st.Path); err == nil {
			st.Exists = true
			if info.Mode()&os.ModeSymlink != 0 {
				st.Blocked = true
				st.Reason = "符号链接"
			} else if data, err := os.ReadFile(st.Path); err == nil {
				has, parseErr := hookFileHasManagedEntry(kind, data, events)
				if parseErr != nil {
					st.Blocked = true
					st.Reason = parseErr.Error()
				} else {
					st.HasAgate = has
				}
			}
		}
		tracked, _ := CheckPathTracked(root, filepath.ToSlash(rel))
		st.Tracked = tracked
		out = append(out, st)
	}
	return out, nil
}

// RollbackNativeHooks 回退指定宿主的 Agate Hook 条目，保留用户后来添加的条目。
func RollbackNativeHooks(root string, host agent.Kind) error {
	if !isVerifiedNativeHookHost(host) {
		return fmt.Errorf("宿主 %s 协议未核对，无法回退", host)
	}
	rel, events, withVersion, err := desiredHookEvents(host, currentHookPrefix())
	if err != nil {
		return err
	}
	fullPath := filepath.Join(root, rel)
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return fmt.Errorf("读取当前 Hook 配置失败: %w", err)
	}

	var rootObj map[string]any
	if err := json.Unmarshal(data, &rootObj); err != nil {
		return fmt.Errorf("当前 Hook JSON 非法: %w", err)
	}
	if host == agent.Antigravity {
		if err := rollbackAntigravityHooks(rootObj, events); err != nil {
			return err
		}
		out, err := json.MarshalIndent(rootObj, "", "  ")
		if err != nil {
			return err
		}
		return harness.WriteFileAtomic(fullPath, out, 0o644)
	}
	hooks, _ := rootObj["hooks"].(map[string]any)
	if hooks == nil {
		return fmt.Errorf("当前文件缺少 hooks 字段，拒绝回退")
	}

	for eventName := range events {
		rawList, ok := hooks[eventName]
		if !ok {
			continue
		}
		arr, ok := rawList.([]any)
		if !ok {
			return fmt.Errorf("事件 %s 列表类型异常", eventName)
		}
		kept := make([]any, 0, len(arr))
		for _, item := range arr {
			entry, ok := item.(map[string]any)
			if !ok {
				kept = append(kept, item)
				continue
			}
			expected := events[eventName]
			if isMarkedAgateEntry(entry, expected) {
				continue
			}
			kept = append(kept, entry)
		}
		if len(kept) == 0 {
			delete(hooks, eventName)
		} else {
			hooks[eventName] = kept
		}
	}
	if withVersion {
		if _, ok := rootObj["version"]; !ok {
			rootObj["version"] = 1
		}
	}
	out, err := json.MarshalIndent(rootObj, "", "  ")
	if err != nil {
		return err
	}
	return harness.WriteFileAtomic(fullPath, out, 0o644)
}
