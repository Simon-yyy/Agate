package cmd

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestExecuteWithArgsTriStateExitCodes(t *testing.T) {
	// 1. 成功执行 (Exit Code 0)
	outBuf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	code := ExecuteWithArgs([]string{"--version"}, outBuf, errBuf)
	if code != 0 {
		t.Fatalf("预期退出码 0 (成功)，实际得到: %d, errOut: %s", code, errBuf.String())
	}
	if !strings.Contains(outBuf.String(), "agate version") {
		t.Errorf("版本输出不符合预期: %s", outBuf.String())
	}

	// 2. 非法 Flag 参数用法错误 (Exit Code 2)
	outBuf.Reset()
	errBuf.Reset()
	code = ExecuteWithArgs([]string{"--non-existent-flag-xyz"}, outBuf, errBuf)
	if code != 2 {
		t.Fatalf("预期非法参数退出码为 2 (USAGE_ERR)，实际得到: %d, out: %s, err: %s", code, outBuf.String(), errBuf.String())
	}
	if !strings.Contains(errBuf.String(), "参数或用法错误") {
		t.Errorf("错误输出未包含用法错误提示: %s", errBuf.String())
	}

	// 3. 未知子命令用法错误 (Exit Code 2)
	outBuf.Reset()
	errBuf.Reset()
	code = ExecuteWithArgs([]string{"nonexistent-subcommand"}, outBuf, errBuf)
	if code != 2 {
		t.Fatalf("预期未知子命令退出码为 2 (USAGE_ERR)，实际得到: %d, err: %s", code, errBuf.String())
	}
	if !strings.Contains(errBuf.String(), "参数或用法错误") {
		t.Errorf("错误输出未包含用法错误提示: %s", errBuf.String())
	}

	// 4. 父命令的未知子命令同样必须视为用法错误，不能静默展示帮助并返回成功。
	outBuf.Reset()
	errBuf.Reset()
	code = ExecuteWithArgs([]string{"ci", "verfy"}, outBuf, errBuf)
	if code != 2 {
		t.Fatalf("父命令未知子命令预期退出码为 2，实际得到: %d, err: %s", code, errBuf.String())
	}

	// 5. 业务/门禁失败 (Exit Code 1)
	tempDir, err := os.MkdirTemp("", "agate-exit-test-*")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(tempDir)

	oldWd, _ := os.Getwd()
	_ = os.Chdir(tempDir)
	defer os.Chdir(oldWd)

	outBuf.Reset()
	errBuf.Reset()
	// 在无测试用例且非 git 仓库目录下执行严格自检，预期触发严格模式拦截 (Exit Code 1)
	code = ExecuteWithArgs([]string{"verify", "--strict"}, outBuf, errBuf)
	if code != 1 {
		t.Fatalf("预期业务失败退出码为 1 (FAIL)，实际得到: %d, out: %s, err: %s", code, outBuf.String(), errBuf.String())
	}
	if !strings.Contains(errBuf.String(), "执行失败") {
		t.Errorf("业务失败输出应包含执行失败提示: %s", errBuf.String())
	}
}
