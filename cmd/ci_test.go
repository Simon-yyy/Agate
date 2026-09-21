package cmd

import (
	"os"
	"testing"
)

func TestCIVerifyForcesStrictReportMode(t *testing.T) {
	tempDir, cleanup := initTestGitRepo(t)
	defer cleanup()
	writeVerifyFixture(t, tempDir, true)

	rootCmd.SetArgs([]string{"ci", "verify"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("ci verify 执行失败: %v", err)
	}
	if _, err := os.Stat(".ai-memory/reviews"); err != nil {
		t.Fatalf("ci verify 未创建审查报告目录: %v", err)
	}
}
