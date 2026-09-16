package guard

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestCleanProjectPasses(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "agate-guard-clean-*")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// 创建标准干净代码
	cleanGo := filepath.Join(tempDir, "main.go")
	_ = os.WriteFile(cleanGo, []byte("package main\n\nfunc main() {}\n"), 0644)

	res := RunPreflightAudit(tempDir)
	if res.HasErrors() {
		t.Errorf("干净工程预期无错误，但捕获到: %v", res.Violations)
	}
}

func TestCatchHardcodedPaths(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "agate-guard-path-*")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// 注入写死的路径反例
	dirtyScript := filepath.Join(tempDir, "run.cmd")
	_ = os.WriteFile(dirtyScript, []byte("@echo off\nset PATH=C:\\Users\\Admin\\bin;%PATH%\n"), 0644)

	res := RunPreflightAudit(tempDir)
	if !res.HasErrors() {
		t.Errorf("未能成功捕获硬编码个人绝对路径")
	}

	found := false
	for _, v := range res.Violations {
		if v.Category == "机器绝对路径泄露" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("未按预期分类为'机器绝对路径泄露'")
	}
}

func TestCatchForbiddenPrivateFiles(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "agate-guard-private-*")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// 初始化 Git 仓库以测试版本库忽略门禁
	gitInit := exec.Command("git", "init")
	gitInit.Dir = tempDir
	_ = gitInit.Run()

	// 注入未隔离的私有文件反例
	_ = os.WriteFile(filepath.Join(tempDir, ".env"), []byte("SECRET_KEY=123456"), 0644)
	_ = os.WriteFile(filepath.Join(tempDir, "TASK.md"), []byte("# Task list"), 0644)

	res := RunPreflightAudit(tempDir)
	if !res.HasErrors() {
		t.Errorf("未能成功捕获 .env 与 TASK.md 私有文件")
	}

	count := 0
	for _, v := range res.Violations {
		if v.Category == "私有文件泄露" {
			count++
		}
	}
	if count != 2 {
		t.Errorf("预期捕获 2 项私有文件违规，实际捕获: %d", count)
	}
}

func TestCatchVendorPollution(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "agate-guard-vendor-*")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// 模拟 vendor 中混入了上游 .github 工作流
	workflowDir := filepath.Join(tempDir, "vendor", "github.com", "upstream", ".github", "workflows")
	_ = os.MkdirAll(workflowDir, 0755)
	_ = os.WriteFile(filepath.Join(workflowDir, "ci.yml"), []byte("name: CI"), 0644)

	res := RunPreflightAudit(tempDir)
	if !res.HasErrors() {
		t.Errorf("未能成功拦截 vendor 混入上游 .github 工作流")
	}

	found := false
	for _, v := range res.Violations {
		if v.Category == "Vendor依赖污染" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("未按预期分类为'Vendor依赖污染'")
	}
}

func TestCatchTemporaryFiles(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "agate-guard-temp-*")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(tempDir)

	gitInit := exec.Command("git", "init")
	gitInit.Dir = tempDir
	_ = gitInit.Run()

	// 注入临时备份与临时草稿文件
	_ = os.WriteFile(filepath.Join(tempDir, "app.go.bak"), []byte("package main"), 0644)
	_ = os.WriteFile(filepath.Join(tempDir, "scratch.tmp"), []byte("temporary data"), 0644)

	res := RunPreflightAudit(tempDir)
	if !res.HasErrors() {
		t.Errorf("未能成功捕获临时残留文件")
	}

	count := 0
	for _, v := range res.Violations {
		if v.Category == "代码洁癖-临时文件残留" {
			count++
		}
	}
	if count != 2 {
		t.Errorf("预期捕获 2 处临时文件违规，实际捕获: %d", count)
	}
}

func TestCatchContentHygiene(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "agate-guard-hygiene-*")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// 注入冲突标记、调试断点与伪代码占位符
	dirtyCode := `package main

func foo() {
<<<<<<< HEAD
	debugger
=======
	// ... 保持原有逻辑不变
>>>>>>> main
}
`
	_ = os.WriteFile(filepath.Join(tempDir, "app.go"), []byte(dirtyCode), 0644)

	res := RunPreflightAudit(tempDir)
	if !res.HasErrors() {
		t.Errorf("未能成功捕获代码内容洁癖违规")
	}

	categories := make(map[string]bool)
	for _, v := range res.Violations {
		categories[v.Category] = true
	}

	if !categories["代码洁癖-Git冲突残留"] {
		t.Errorf("未捕获 Git冲突残留")
	}
	if !categories["代码洁癖-调试断点残留"] {
		t.Errorf("未捕获 调试断点残留")
	}
	if !categories["代码洁癖-伪代码占位符"] {
		t.Errorf("未捕获 伪代码占位符")
	}
}

func TestNodeModulesAndReleaseIgnored(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "agate-guard-ignored-*")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// 模拟 node_modules 包含绝对路径示例
	nmDir := filepath.Join(tempDir, "node_modules", "@types", "node")
	_ = os.MkdirAll(nmDir, 0755)
	_ = os.WriteFile(filepath.Join(nmDir, "fs.d.ts"), []byte("// C:\\Users\\Admin\\AppData\\Local\\Temp\\foo\n"), 0644)

	// 模拟 release 构建产物目录中包含冲突分割线格式与绝对路径
	relDir := filepath.Join(tempDir, "release")
	_ = os.MkdirAll(relDir, 0755)
	_ = os.WriteFile(filepath.Join(relDir, "LICENSES.chromium.html"), []byte("=======\n"), 0644)
	_ = os.WriteFile(filepath.Join(relDir, "builder-debug.yml"), []byte("path: C:\\Users\\Admin\\AppData\n"), 0644)

	res := RunPreflightAudit(tempDir)
	if res.HasErrors() {
		t.Errorf("node_modules 与 release 目录应当被自动剪枝跳过，但捕获了错误: %v", res.Violations)
	}
}

func TestFalsePositiveRegexDefinition(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "agate-guard-fp-*")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// 测试：包含防御性正则表达式或普通文案中的“保持不变”，不能误判为伪代码
	validJS := `
const regex = /\/\/\s*\.{3,}\s*(?:保持不变|其余不变)/i;
const prompt = "请确保现有业务逻辑保持不变";
`
	_ = os.WriteFile(filepath.Join(tempDir, "rule.js"), []byte(validJS), 0644)

	res := RunPreflightAudit(tempDir)
	if res.HasErrors() {
		t.Errorf("正则表达式与普通字符串不应误判为伪代码占位符，但捕获了: %v", res.Violations)
	}
}

func TestGithubAndTestdataIgnored(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "agate-guard-gh-*")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// 模拟 .github 包含社交大图（>50KB）
	ghDir := filepath.Join(tempDir, ".github", "assets")
	_ = os.MkdirAll(ghDir, 0755)
	largeImg := make([]byte, 100*1024)
	_ = os.WriteFile(filepath.Join(ghDir, "social-preview.png"), largeImg, 0644)

	// 模拟 testdata 包含测试用压缩包
	tdDir := filepath.Join(tempDir, "testdata")
	_ = os.MkdirAll(tdDir, 0755)
	_ = os.WriteFile(filepath.Join(tdDir, "sample.tar.gz"), []byte("archive content"), 0644)

	res := RunPreflightAudit(tempDir)
	if res.HasErrors() {
		t.Errorf(".github 与 testdata 应当被跳过剪枝，但捕获到错误: %v", res.Violations)
	}
}

func TestNonGitRepoGracefulDegrade(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "agate-guard-nongit-*")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// 非 Git 目录放入 .env 与 .zip（未 git init）
	_ = os.WriteFile(filepath.Join(tempDir, ".env"), []byte("FOO=BAR"), 0644)
	_ = os.WriteFile(filepath.Join(tempDir, "archive.zip"), []byte("binary data"), 0644)

	res := RunPreflightAudit(tempDir)
	// 平滑降级原则：非 Git 目录下的文件级隔离只报 WARN，不报阻断级 ERROR
	if res.HasErrors() {
		t.Errorf("非 Git 裸目录预期平滑降级为 WARN，但触发了阻断级 ERROR: %v", res.Violations)
	}
}

func TestDebuggerVariableAndCommentNotBlocked(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "agate-guard-dbg-*")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(tempDir)

	code := `package main

// TODO: 接入 debugger 排查异常
func debugTest() {
	debugger := attach()
	_ = debugger
}
`
	_ = os.WriteFile(filepath.Join(tempDir, "app.go"), []byte(code), 0644)

	res := RunPreflightAudit(tempDir)
	if res.HasErrors() {
		t.Errorf("注释与变量名中的 debugger 不应被拦截，但捕获了: %v", res.Violations)
	}
}

func TestTestFilesHardcodedPathExempt(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "agate-guard-testpath-*")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// _test.go 中的路径测试样例应当被豁免
	testCode := `package main
import "testing"
func TestPath(t *testing.T) {
	p := "C:\\Users\\Admin\\AppData\\Local\\Temp\\fixture.txt"
	_ = p
}
`
	_ = os.WriteFile(filepath.Join(tempDir, "parser_test.go"), []byte(testCode), 0644)

	res := RunPreflightAudit(tempDir)
	if res.HasErrors() {
		t.Errorf("测试文件中的路径 fixture 应当被豁免，但捕获了: %v", res.Violations)
	}
}

func TestArchitectureMapsCompleteness(t *testing.T) {
	// Case 1: 缺少 contexts/context.md
	tempDir1, err := os.MkdirTemp("", "agate-guard-map1-*")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(tempDir1)

	_ = os.WriteFile(filepath.Join(tempDir1, "AGENTS.md"), []byte("# AGENTS"), 0644)
	res1 := RunPreflightAudit(tempDir1)
	if !res1.HasErrors() {
		t.Errorf("仅有 AGENTS.md 时预期报错缺失 contexts/context.md，但未报错")
	}

	// Case 2: 缺少 AGENTS.md
	tempDir2, err := os.MkdirTemp("", "agate-guard-map2-*")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(tempDir2)

	_ = os.MkdirAll(filepath.Join(tempDir2, "contexts"), 0755)
	_ = os.WriteFile(filepath.Join(tempDir2, "contexts", "context.md"), []byte("# Context"), 0644)
	res2 := RunPreflightAudit(tempDir2)
	if !res2.HasErrors() {
		t.Errorf("仅有 contexts/context.md 时预期报错缺失 AGENTS.md，但未报错")
	}

	// Case 3: 双地图齐备
	_ = os.WriteFile(filepath.Join(tempDir2, "AGENTS.md"), []byte("# AGENTS"), 0644)
	res3 := RunPreflightAudit(tempDir2)
	if res3.HasErrors() {
		t.Errorf("双地图齐备时预期通过，但报错: %v", res3.Violations)
	}
}


