package guard

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

	// 注入写死的路径反例 (Windows 盘符路径 + Unix /home 路径)
	dirtyScript := filepath.Join(tempDir, "run.cmd")
	_ = os.WriteFile(dirtyScript, []byte("@echo off\nset PATH=C:\\Users\\Admin\\bin;%PATH%\n"), 0644)

	dirtyGo := filepath.Join(tempDir, "user_path.go")
	_ = os.WriteFile(dirtyGo, []byte("package main\nvar p = \"/home/admin/secret_data.txt\"\n"), 0644)

	res := RunPreflightAudit(tempDir)
	if !res.HasErrors() {
		t.Errorf("未能成功捕获硬编码个人绝对路径")
	}

	count := 0
	for _, v := range res.Violations {
		if v.Category == "机器绝对路径泄露" {
			count++
		}
	}
	if count < 2 {
		t.Errorf("未按预期同时捕获 Windows 与 Unix 机器绝对路径泄露，实际捕获 %d 处", count)
	}
}

func TestAuditHardcodedPathsWhitelistedSystemPaths(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "agate-guard-whitelist-*")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// 注入合法 POSIX 系统资源路径 (/dev/null, /tmp/..., /var/run/...)
	cleanCode := filepath.Join(tempDir, "system_call.go")
	content := `package main

import "os"

func run() {
	_, _ = os.OpenFile("/dev/null", os.O_WRONLY, 0)
	_, _ = os.OpenFile("/dev/zero", os.O_RDONLY, 0)
	_, _ = os.OpenFile("/dev/urandom", os.O_RDONLY, 0)
	sock := "/tmp/myapp.sock"
	pidFile := "/var/run/daemon.pid"
	_ = sock
	_ = pidFile
}
`
	_ = os.WriteFile(cleanCode, []byte(content), 0644)

	res := RunPreflightAudit(tempDir)
	for _, v := range res.Violations {
		if v.Category == "机器绝对路径泄露" {
			t.Errorf("合法的标准系统路径不应被误判拦截: %s:%d %s", v.File, v.LineNumber, v.Message)
		}
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

func TestInternalDirAudited(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "agate-guard-internal-*")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// 1. 在 internal/service/order.go 中写入违规代码 (debugger 断点)
	serviceDir := filepath.Join(tempDir, "internal", "service")
	if err := os.MkdirAll(serviceDir, 0755); err != nil {
		t.Fatalf("创建 internal/service 失败: %v", err)
	}
	dirtyCode := "package service\n\nfunc ProcessOrder() {\n\tdebugger\n}\n"
	if err := os.WriteFile(filepath.Join(serviceDir, "order.go"), []byte(dirtyCode), 0644); err != nil {
		t.Fatalf("写入测试代码失败: %v", err)
	}

	res := RunPreflightAudit(tempDir)
	if !res.HasErrors() {
		t.Errorf("internal/service/order.go 中的 debugger 违规代码必须被拦截，但未检测出错误！")
	}

	found := false
	for _, v := range res.Violations {
		if v.Category == "代码洁癖-调试断点残留" && strings.Contains(v.File, "internal/service/order.go") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("未能成功捕获 internal/service/order.go 的代码洁癖断点违规，实际违规: %+v", res.Violations)
	}
}

func TestRunStagedAudit(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "agate-guard-staged-*")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(tempDir)

	origWd, _ := os.Getwd()
	_ = os.Chdir(tempDir)
	defer os.Chdir(origWd)

	cmd := exec.Command("git", "init")
	if err := cmd.Run(); err != nil {
		t.Fatalf("git init 失败: %v", err)
	}

	// 1. 暂存区为空
	res := RunStagedAudit(tempDir, []string{})
	if res.HasErrors() {
		t.Errorf("空暂存区预期无错误，但捕获到: %v", res.Violations)
	}

	// 2. 工作区有违规文件，但未放入暂存区；暂存区只有干净文件
	cleanFile := "clean.go"
	_ = os.WriteFile(cleanFile, []byte("package main\n\nfunc Run() {}\n"), 0644)
	dirtyWorktreeFile := "dirty.go"
	_ = os.WriteFile(dirtyWorktreeFile, []byte("package main\n\nfunc Debug() {\n\tdebugger\n}\n"), 0644)

	// 只 git add cleanFile
	_ = exec.Command("git", "add", cleanFile).Run()

	res = RunStagedAudit(tempDir, []string{cleanFile})
	if res.HasErrors() {
		t.Errorf("暂存区仅有干净代码，不应误报工作区未暂存的 dirty.go，但捕获到: %v", res.Violations)
	}

	// 3. 将 dirty.go 加入暂存区，预期精准拦截
	_ = exec.Command("git", "add", dirtyWorktreeFile).Run()
	res = RunStagedAudit(tempDir, []string{cleanFile, dirtyWorktreeFile})
	if !res.HasErrors() {
		t.Errorf("dirty.go 加入暂存区后预期被拦截，但未检测出错误")
	}

	// 4. 将 TASK.md 私有文件加入暂存区，预期私有文件泄露拦截
	taskFile := "TASK.md"
	_ = os.WriteFile(taskFile, []byte("# My private task"), 0644)
	_ = exec.Command("git", "add", taskFile).Run()
	res = RunStagedAudit(tempDir, []string{taskFile})
	if !res.HasErrors() {
		t.Errorf("暂存区包含 TASK.md 预期被拦截，但未检测出错误")
	}
}

func TestLargeLineSourceAudited(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "agate-guard-largeline-*")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// 构造一行超过 70KB 的超长代码行（包含一个违规 debugger 断点）
	largePadding := strings.Repeat("/* padding */ ", 5000) // 约 70KB
	dirtyCode := "package main\n\nfunc BigLine() {\n\t" + largePadding + "\n\tdebugger\n}\n"

	filePath := filepath.Join(tempDir, "big.go")
	if err := os.WriteFile(filePath, []byte(dirtyCode), 0644); err != nil {
		t.Fatalf("写入超长大文件失败: %v", err)
	}

	res := RunPreflightAudit(tempDir)
	if !res.HasErrors() {
		t.Errorf("超长单行文件中的 debugger 断点未被成功捕获，存在 Scanner 截断失效缺陷！")
	}
}

func TestMaskSensitiveLine(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{
			input:    `db_password := "SuperSecret123"; debugger`,
			expected: `db_password := "******"; debugger`,
		},
		{
			input:    `apiKey: "sk-proj-999999999"`,
			expected: `apiKey: "******"`,
		},
		{
			input:    `ACCESS_TOKEN = 'ghp_xxxxxx'`,
			expected: `ACCESS_TOKEN = "******"`,
		},
		{
			input:    `auth_secret=mysecret123`,
			expected: `auth_secret = "******"`,
		},
		{
			input:    `normalVar := 42; // debugger`,
			expected: `normalVar := 42; // debugger`,
		},
		{
			input:    `token := "secret1"; password := "secret2"`,
			expected: `token := "******"; password := "******"`,
		},
	}

	for _, c := range cases {
		got := maskSensitiveLine(c.input)
		if got != c.expected {
			t.Errorf("maskSensitiveLine(%q) = %q, expected %q", c.input, got, c.expected)
		}
	}
}

func TestDocImageAllowedAndSourceImageBlocked(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "agate-guard-img-*")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(tempDir)

	origWd, _ := os.Getwd()
	_ = os.Chdir(tempDir)
	defer os.Chdir(origWd)

	if err := exec.Command("git", "init").Run(); err != nil {
		t.Fatalf("git init 失败: %v", err)
	}

	// 1. 在 docs/ 目录下写入一个 200KB 的合法架构图（>50KB，但 <2MB）
	_ = os.MkdirAll("docs", 0755)
	docImgData := make([]byte, 200*1024)
	docImgPath := filepath.Join("docs", "architecture.png")
	if err := os.WriteFile(docImgPath, docImgData, 0644); err != nil {
		t.Fatalf("写入文档图片失败: %v", err)
	}
	_ = exec.Command("git", "add", docImgPath).Run()

	// 2. 在 src/ 源码目录下写入一个 60KB 的图片（>50KB，属于误放源码）
	_ = os.MkdirAll("src", 0755)
	srcImgData := make([]byte, 60*1024)
	srcImgPath := filepath.Join("src", "huge_icon.png")
	if err := os.WriteFile(srcImgPath, srcImgData, 0644); err != nil {
		t.Fatalf("写入源码图片失败: %v", err)
	}

	// 仅检查 docs 目录图片时，不应被视为错误 (AG-010)
	resDoc := RunPreflightAudit(tempDir)
	hasDocError := false
	for _, v := range resDoc.Violations {
		if v.File == filepath.ToSlash(docImgPath) && v.Level == "ERROR" {
			hasDocError = true
		}
	}
	if hasDocError {
		t.Errorf("docs/ 目录下的 200KB 合法架构图预期不被 ERROR 拦截，但被拦截: %v", resDoc.Violations)
	}

	// 源码目录下的图片预期被 ERROR 拦截
	hasSrcError := false
	for _, v := range resDoc.Violations {
		if v.File == filepath.ToSlash(srcImgPath) && v.Level == "ERROR" {
			hasSrcError = true
		}
	}
	if !hasSrcError {
		t.Errorf("src/ 源码目录下的 60KB 图片预期被 ERROR 拦截，但未检测出违规")
	}
}

func TestGitContextBatchIgnoredPerformance(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "agate-guard-perf-*")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(tempDir)

	origWd, _ := os.Getwd()
	_ = os.Chdir(tempDir)
	defer os.Chdir(origWd)

	if err := exec.Command("git", "init").Run(); err != nil {
		t.Fatalf("git init 失败: %v", err)
	}

	// 写入 .gitignore 忽略 temp_build/
	_ = os.WriteFile(".gitignore", []byte("temp_build/\n*.log\n"), 0644)
	_ = os.MkdirAll("temp_build", 0755)
	_ = os.WriteFile(filepath.Join("temp_build", "artifact.bin"), []byte("bin"), 0644)
	_ = os.WriteFile("debug.log", []byte("log"), 0644)

	ctx := newGitContext(tempDir)
	if !ctx.isGitRepo {
		t.Fatalf("预期识别为 Git 仓库")
	}

	// 验证批量已加载 ignoredCache (AG-024)
	if !ctx.isIgnored("debug.log") {
		t.Errorf("debug.log 预期被识别为忽略")
	}
	if !ctx.isIgnored("temp_build/artifact.bin") {
		t.Errorf("temp_build/artifact.bin 预期被识别为忽略")
	}
}


