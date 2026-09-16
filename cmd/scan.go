package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"agate/pkg/harness"

	"github.com/spf13/cobra"
)

var flagWriteScan bool

type ProjectTopology struct {
	TechStack []string
	PortInfo  map[string]string
	Services  []string
}

var scanCmd = &cobra.Command{
	Use:   "scan",
	Short: "扫描当前工程配置，自动提取技术栈与端口拓扑",
	Long: `智能扫描当前代码库中的描述文件与配置文件：
- 识别技术栈（Maven/Spring Boot、npm/前端框架、Go、Python等）；
- 检索端口配置（server.port、vite port、.env 等）；
- 输出架构拓扑报告，支持通过 --write 自动丰富 AGENTS.md 与 contexts/context.md。`,
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("=== [agate scan] 正在深度扫描工程技术拓扑 ===")

		topo := scanProject()

		fmt.Println("\n【识别到的技术栈】")
		if len(topo.TechStack) == 0 {
			fmt.Println("  - 未识别到标准技术栈描述文件（通用环境）")
		} else {
			for _, tech := range topo.TechStack {
				fmt.Printf("  - %s\n", tech)
			}
		}

		fmt.Println("\n【识别到的端口矩阵】")
		if len(topo.PortInfo) == 0 {
			fmt.Println("  - 未检测到显式端口配置（默认端口或纯库函数项目）")
		} else {
			for file, port := range topo.PortInfo {
				fmt.Printf("  - %-25s -> %s\n", file, port)
			}
		}

		if flagWriteScan {
			if err := appendTopologyToAgents(topo); err != nil {
				return err
			}
			if err := appendTopologyToContext(topo); err != nil {
				return err
			}
			return nil
		}

		fmt.Println("\n💡 提示：运行 `agate scan --write` 可将上述扫描结果自动合并回填至 AGENTS.md 与 contexts/context.md")
		return nil
	},
}

func scanProject() *ProjectTopology {
	topo := &ProjectTopology{
		PortInfo: make(map[string]string),
	}

	// 1. 扫描构建与依赖文件
	if _, err := os.Stat("pom.xml"); err == nil {
		topo.TechStack = append(topo.TechStack, "Java / Maven / Spring Boot 体系")
	}
	if _, err := os.Stat("package.json"); err == nil {
		topo.TechStack = append(topo.TechStack, "Node.js / 前端包管理体系")
	}
	if _, err := os.Stat("go.mod"); err == nil {
		topo.TechStack = append(topo.TechStack, "Go 1.21+ / 单二进制 CLI 体系")
	}
	if _, err := os.Stat("requirements.txt"); err == nil {
		topo.TechStack = append(topo.TechStack, "Python 数据处理 / 脚本体系")
	}

	// 2. 扫描端口定义 (正则快速匹配常见配置)
	portRegex := regexp.MustCompile(`(?i)(?:server\.port|port)\s*[:=]\s*(\d+)`)

	configCandidates := []string{
		"application.yml", "application.yaml", "application.properties",
		"vite.config.ts", "vite.config.js", ".env", ".env.local",
	}

	for _, cand := range configCandidates {
		if _, err := os.Stat(cand); err == nil {
			if port := extractPortFromFile(cand, portRegex); port != "" {
				topo.PortInfo[cand] = "端口 " + port
			}
		}
	}

	// 递归浅层检索一级子目录配置
	entries, _ := os.ReadDir(".")
	for _, entry := range entries {
		if entry.IsDir() && !strings.HasPrefix(entry.Name(), ".") && entry.Name() != "node_modules" && entry.Name() != "vendor" {
			for _, cand := range configCandidates {
				subPath := filepath.Join(entry.Name(), cand)
				if _, err := os.Stat(subPath); err == nil {
					if port := extractPortFromFile(subPath, portRegex); port != "" {
						topo.PortInfo[subPath] = "端口 " + port
					}
				}
			}
		}
	}

	return topo
}

func extractPortFromFile(path string, re *regexp.Regexp) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}
		if matches := re.FindStringSubmatch(line); len(matches) > 1 {
			return matches[1]
		}
	}
	return ""
}

func appendTopologyToAgents(topo *ProjectTopology) error {
	agentsFile := "AGENTS.md"
	if _, err := os.Stat(agentsFile); os.IsNotExist(err) {
		return fmt.Errorf("未找到 AGENTS.md，请先执行 agate init")
	}

	content, err := os.ReadFile(agentsFile)
	if err != nil {
		return err
	}

	var sb strings.Builder
	sb.WriteString("\n\n<!-- agate scan autogen start -->\n")
	sb.WriteString("## 自动扫描推导拓扑\n")
	if len(topo.TechStack) > 0 {
		sb.WriteString("### 侦测技术栈:\n")
		for _, t := range topo.TechStack {
			sb.WriteString(fmt.Sprintf("- %s\n", t))
		}
	}
	if len(topo.PortInfo) > 0 {
		sb.WriteString("### 侦测端口矩阵:\n")
		for f, p := range topo.PortInfo {
			sb.WriteString(fmt.Sprintf("- `%s`: %s\n", f, p))
		}
	}
	sb.WriteString("<!-- agate scan autogen end -->\n")

	// 若已存在标记则替换，否则追加
	original := string(content)
	startTag := "<!-- agate scan autogen start -->"
	endTag := "<!-- agate scan autogen end -->"

	var finalContent string
	if startIdx := strings.Index(original, startTag); startIdx != -1 {
		if endIdx := strings.Index(original, endTag); endIdx != -1 {
			finalContent = original[:startIdx] + strings.TrimSpace(sb.String()) + original[endIdx+len(endTag):]
		} else {
			finalContent = original + sb.String()
		}
	} else {
		finalContent = original + sb.String()
	}

	if err := os.WriteFile(agentsFile, []byte(finalContent), 0644); err != nil {
		return fmt.Errorf("写入 AGENTS.md 失败: %w", err)
	}

	fmt.Println("  \033[92m[+] 扫描结果已成功合并回填至 AGENTS.md\033[0m")
	return nil
}

func appendTopologyToContext(topo *ProjectTopology) error {
	contextFile := filepath.Join("contexts", "context.md")
	if _, err := os.Stat(contextFile); os.IsNotExist(err) {
		if _, err := harness.EnsureContext(); err != nil {
			return fmt.Errorf("初始化 contexts/context.md 失败: %w", err)
		}
	}

	content, err := os.ReadFile(contextFile)
	if err != nil {
		return err
	}

	var sb strings.Builder
	sb.WriteString("\n\n<!-- agate scan autogen start -->\n")
	sb.WriteString("## 自动推导运行时与端口契约\n")
	if len(topo.TechStack) > 0 {
		sb.WriteString("### 侦测技术栈与运行环境:\n")
		for _, t := range topo.TechStack {
			sb.WriteString(fmt.Sprintf("- %s\n", t))
		}
	}
	if len(topo.PortInfo) > 0 {
		sb.WriteString("### 侦测服务与端口契约:\n")
		for f, p := range topo.PortInfo {
			sb.WriteString(fmt.Sprintf("- `%s`: %s\n", f, p))
		}
	}
	sb.WriteString("<!-- agate scan autogen end -->\n")

	original := string(content)
	startTag := "<!-- agate scan autogen start -->"
	endTag := "<!-- agate scan autogen end -->"

	var finalContent string
	if startIdx := strings.Index(original, startTag); startIdx != -1 {
		if endIdx := strings.Index(original, endTag); endIdx != -1 {
			finalContent = original[:startIdx] + strings.TrimSpace(sb.String()) + original[endIdx+len(endTag):]
		} else {
			finalContent = original + sb.String()
		}
	} else {
		finalContent = original + sb.String()
	}

	if err := os.WriteFile(contextFile, []byte(finalContent), 0644); err != nil {
		return fmt.Errorf("写入 contexts/context.md 失败: %w", err)
	}

	fmt.Println("  \033[92m[+] 扫描结果已成功合并回填至 contexts/context.md\033[0m")
	return nil
}

func init() {
	scanCmd.Flags().BoolVarP(&flagWriteScan, "write", "w", false, "将扫描出的技术拓扑自动回填至 AGENTS.md 与 contexts/context.md")
	rootCmd.AddCommand(scanCmd)
}
