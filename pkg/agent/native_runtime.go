package agent

import (
	"fmt"
	"os"
	"runtime"
	"strings"
)

// NativeMode 定义原生 Hook 的装配模式。
type NativeMode string

const (
	NativeModeAuto    NativeMode = "auto"
	NativeModeOff     NativeMode = "off"
	NativeModeRequire NativeMode = "require"
)

// NativeRuntime 包含检测到的宿主客户端环境与原生 Hook 兼容状态。
type NativeRuntime struct {
	Host         Kind   `json:"host"`
	ClientVer    string `json:"client_ver,omitempty"`
	Platform     string `json:"platform"`
	WorkDir      string `json:"work_dir"`
	SupportsHook bool   `json:"supports_hook"`
}

// verifiedNativeHookHosts 仅包含已核对协议的宿主；八个新目标默认不在此列。
var verifiedNativeHookHosts = map[Kind]struct{}{
	Cursor:      {},
	Antigravity: {},
	Claude:      {},
	Codex:       {},
	Windsurf:    {},
}

// SupportsNativeHook 返回目标是否具备已核对的原生 Hook 协议。
func SupportsNativeHook(kind Kind) bool {
	_, ok := verifiedNativeHookHosts[kind]
	return ok
}

// acceptedNativeHookVersions 收录仍按版本号登记的宿主。
// Cursor 不在此列：产品版本从已匹配的可执行文件读取，非空即视为已验收。
// 空集表示该宿主尚无已验收版本：auto 跳过，require 拒绝。
var acceptedNativeHookVersions = map[Kind]map[string]struct{}{}

// NativeHookVersionAccepted 判断版本是否已验收。空版本返回 false。
func NativeHookVersionAccepted(kind Kind, version string) bool {
	version = strings.TrimSpace(version)
	if version == "" {
		return false
	}
	if kind == Cursor {
		return true
	}
	set := acceptedNativeHookVersions[kind]
	_, ok := set[version]
	return ok
}

// TestingAcceptNativeHookVersion 供测试登记一个已验收版本，返回恢复函数。
func TestingAcceptNativeHookVersion(kind Kind, version string) func() {
	version = strings.TrimSpace(version)
	if acceptedNativeHookVersions[kind] == nil {
		acceptedNativeHookVersions[kind] = map[string]struct{}{}
	}
	acceptedNativeHookVersions[kind][version] = struct{}{}
	return func() {
		delete(acceptedNativeHookVersions[kind], version)
	}
}

// ParseNativeMode 只接受 auto、off、require。空字符串按 auto。
func ParseNativeMode(raw string) (NativeMode, error) {
	switch NativeMode(strings.ToLower(strings.TrimSpace(raw))) {
	case "":
		return NativeModeAuto, nil
	case NativeModeAuto, NativeModeOff, NativeModeRequire:
		return NativeMode(strings.ToLower(strings.TrimSpace(raw))), nil
	default:
		return "", fmt.Errorf("非法 --native-hooks 值 %q，只接受 auto、off、require", raw)
	}
}

// readExecutableProductVersion 读取可执行文件的产品版本。测试可替换。
var readExecutableProductVersion = defaultReadExecutableProductVersion

func cursorExecutable(ctx DetectionContext) string {
	for _, ev := range ctx.Evidences {
		if ev.Source == EvidenceSourceProcess && ev.Matched && ev.TargetRole == "editor" &&
			ev.TargetAgent == string(Cursor) && strings.TrimSpace(ev.Executable) != "" {
			return ev.Executable
		}
	}
	return ""
}

func clientVersionFor(kind Kind, executable string, getenv func(string) string) string {
	if getenv == nil {
		getenv = os.Getenv
	}
	switch kind {
	case Cursor:
		// 环境变量只作线索。没有已匹配可执行文件时不读取版本。
		executable = strings.TrimSpace(executable)
		if executable == "" {
			return ""
		}
		return strings.TrimSpace(readExecutableProductVersion(executable))
	case Claude:
		return firstNonEmpty(getenv("CLAUDE_CODE_VERSION"), getenv("CLAUDE_VERSION"))
	case Codex:
		return strings.TrimSpace(getenv("CODEX_VERSION"))
	case Windsurf:
		return firstNonEmpty(getenv("WINDSURF_VERSION"), getenv("CASCADE_VERSION"))
	case Antigravity:
		return firstNonEmpty(getenv("ANTIGRAVITY_VERSION"), getenv("GEMINI_VERSION"))
	default:
		return ""
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// DetectNativeRuntime 基于当前已仲裁的上下文构建 NativeRuntime 实例。
func DetectNativeRuntime(ctx DetectionContext, workDir string) NativeRuntime {
	return DetectNativeRuntimeEnv(ctx, workDir, os.Getenv)
}

// DetectNativeRuntimeEnv 允许测试注入 getenv。
func DetectNativeRuntimeEnv(ctx DetectionContext, workDir string, getenv func(string) string) NativeRuntime {
	target, _ := ctx.EffectiveTarget()
	rt := NativeRuntime{
		Host:      Kind(target),
		Platform:  runtime.GOOS,
		WorkDir:   workDir,
		ClientVer: clientVersionFor(Kind(target), cursorExecutable(ctx), getenv),
	}
	if SupportsNativeHook(Kind(target)) {
		rt.SupportsHook = true
	}
	return rt
}
