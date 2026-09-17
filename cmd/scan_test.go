package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScanDeterministicOrdering(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "agate-scan-test-*")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(tempDir)

	origWd, _ := os.Getwd()
	_ = os.Chdir(tempDir)
	defer os.Chdir(origWd)

	// 模拟多个配置文件与端口
	_ = os.WriteFile("application.yml", []byte("server:\n  port: 8080\n"), 0644)
	_ = os.WriteFile("vite.config.ts", []byte("export default { server: { port: 3000 } }\n"), 0644)
	_ = os.WriteFile(".env", []byte("PORT=5000\n"), 0644)

	// 创建初始 AGENTS.md 与 contexts/context.md
	_ = os.WriteFile("AGENTS.md", []byte("# Project\n"), 0644)
	_ = os.MkdirAll("contexts", 0755)
	_ = os.WriteFile(filepath.Join("contexts", "context.md"), []byte("# Context\n"), 0644)

	topo := scanProject()
	if len(topo.PortInfo) < 3 {
		t.Fatalf("预期扫描到至少 3 个端口配置，实际: %d", len(topo.PortInfo))
	}

	// 第一次回填
	if err := appendTopologyToAgents(topo); err != nil {
		t.Fatalf("第一次写入 AGENTS.md 失败: %v", err)
	}
	if err := appendTopologyToContext(topo); err != nil {
		t.Fatalf("第一次写入 context.md 失败: %v", err)
	}

	content1, _ := os.ReadFile("AGENTS.md")
	context1, _ := os.ReadFile(filepath.Join("contexts", "context.md"))

	// 验证端口项在文件中为字母序升序排序 (AG-019)
	str1 := string(content1)
	idxEnv := strings.Index(str1, "`.env`")
	idxApp := strings.Index(str1, "`application.yml`")
	idxVite := strings.Index(str1, "`vite.config.ts`")

	if !(idxEnv < idxApp && idxApp < idxVite) {
		t.Errorf("端口配置未按字母序稳定排序: .env(%d), application.yml(%d), vite.config.ts(%d)", idxEnv, idxApp, idxVite)
	}

	// 重复执行 5 次，验证 100% 幂等无任何噪点
	for i := 0; i < 5; i++ {
		if err := appendTopologyToAgents(topo); err != nil {
			t.Fatalf("重复第 %d 次写入 AGENTS.md 失败: %v", i+1, err)
		}
		if err := appendTopologyToContext(topo); err != nil {
			t.Fatalf("重复第 %d 次写入 context.md 失败: %v", i+1, err)
		}

		contentN, _ := os.ReadFile("AGENTS.md")
		contextN, _ := os.ReadFile(filepath.Join("contexts", "context.md"))

		if string(contentN) != string(content1) {
			t.Errorf("第 %d 次写入产生非幂等差异!\nExpected:\n%s\nGot:\n%s", i+1, string(content1), string(contentN))
		}
		if string(contextN) != string(context1) {
			t.Errorf("第 %d 次写入 context.md 产生非幂等差异!", i+1)
		}
	}
}
