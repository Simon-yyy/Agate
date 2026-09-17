<!DOCTYPE html>
<html lang="zh-CN">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Agate 协同审查与物证报告 - {{.ProjectName}}</title>
  <style>
    :root {
      --bg: #0d1117;
      --card-bg: #161b22;
      --card-border: #30363d;
      --text: #c9d1d9;
      --text-muted: #8b949e;
      --heading: #f0f6fc;
      --pass: #238636;
      --pass-bg: rgba(46, 160, 67, 0.15);
      --pass-border: #2ea043;
      --fail: #da3633;
      --fail-bg: rgba(248, 81, 73, 0.15);
      --fail-border: #f85149;
      --warn: #d29922;
      --warn-bg: rgba(210, 153, 34, 0.15);
      --warn-border: #bb8009;
      --code-bg: #030407;
      --diff-add-bg: rgba(46, 160, 67, 0.2);
      --diff-add-text: #7ee787;
      --diff-del-bg: rgba(248, 81, 73, 0.2);
      --diff-del-text: #ffa198;
      --diff-hunk-text: #79c0ff;
      --accent: #58a6ff;
    }

    @media (prefers-color-scheme: light) {
      :root {
        --bg: #f6f8fa;
        --card-bg: #ffffff;
        --card-border: #d0d7de;
        --text: #24292f;
        --text-muted: #57606a;
        --heading: #1f2328;
        --pass: #1a7f37;
        --pass-bg: #dafbe1;
        --pass-border: #4ac26b;
        --fail: #cf222e;
        --fail-bg: #ffebe9;
        --fail-border: #ff8182;
        --warn: #9a6700;
        --warn-bg: #fff8c5;
        --warn-border: #d4a72c;
        --code-bg: #f6f8fa;
        --diff-add-bg: #e6ffec;
        --diff-add-text: #1a7f37;
        --diff-del-bg: #ffebe9;
        --diff-del-text: #cf222e;
        --diff-hunk-text: #0969da;
        --accent: #0969da;
      }
    }

    * { box-sizing: border-box; margin: 0; padding: 0; }
    body {
      font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", "Noto Sans", Helvetica, Arial, sans-serif;
      background-color: var(--bg);
      color: var(--text);
      line-height: 1.6;
      padding: 24px 16px;
    }

    .container {
      max-width: 1080px;
      margin: 0 auto;
    }

    /* 顶栏与状态条 */
    .header {
      display: flex;
      flex-wrap: wrap;
      justify-content: space-between;
      align-items: center;
      gap: 16px;
      padding-bottom: 20px;
      margin-bottom: 24px;
      border-bottom: 1px solid var(--card-border);
    }

    .title-group {
      display: flex;
      align-items: center;
      gap: 12px;
    }

    .agate-logo {
      display: inline-flex;
      align-items: center;
      justify-content: center;
      width: 36px;
      height: 36px;
      background: linear-gradient(135deg, #58a6ff 0%, #bc8cff 100%);
      color: #fff;
      font-weight: 900;
      border-radius: 8px;
      font-size: 18px;
      letter-spacing: -0.5px;
    }

    h1 {
      font-size: 22px;
      color: var(--heading);
      font-weight: 700;
    }

    .meta-badge {
      display: inline-flex;
      align-items: center;
      padding: 6px 16px;
      border-radius: 20px;
      font-weight: 700;
      font-size: 14px;
      text-transform: uppercase;
      letter-spacing: 0.5px;
    }
    .meta-badge.pass {
      background: var(--pass-bg);
      color: var(--pass);
      border: 1px solid var(--pass-border);
    }
    .meta-badge.fail {
      background: var(--fail-bg);
      color: var(--fail);
      border: 1px solid var(--fail-border);
    }

    /* 元数据矩阵 */
    .meta-grid {
      display: grid;
      grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
      gap: 12px;
      margin-bottom: 24px;
    }
    .meta-item {
      background: var(--card-bg);
      border: 1px solid var(--card-border);
      border-radius: 8px;
      padding: 12px 16px;
    }
    .meta-label {
      font-size: 12px;
      color: var(--text-muted);
      margin-bottom: 4px;
      text-transform: uppercase;
    }
    .meta-value {
      font-size: 14px;
      font-weight: 600;
      color: var(--heading);
      font-family: ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace;
    }

    /* 折叠区块卡片 */
    details.section-card {
      background: var(--card-bg);
      border: 1px solid var(--card-border);
      border-radius: 8px;
      margin-bottom: 16px;
      overflow: hidden;
    }
    details.section-card[open] {
      box-shadow: 0 4px 12px rgba(0,0,0,0.1);
    }
    summary.section-header {
      padding: 14px 18px;
      font-size: 15px;
      font-weight: 600;
      color: var(--heading);
      cursor: pointer;
      user-select: none;
      display: flex;
      align-items: center;
      justify-content: space-between;
      list-style: none;
      background: var(--card-bg);
      border-bottom: 1px solid transparent;
      transition: background 0.15s;
    }
    summary.section-header::-webkit-details-marker { display: none; }
    summary.section-header:hover {
      background: rgba(255,255,255,0.02);
    }
    details[open] summary.section-header {
      border-bottom-color: var(--card-border);
    }
    .section-title-wrap {
      display: flex;
      align-items: center;
      gap: 10px;
    }
    .badge-count {
      font-size: 12px;
      padding: 2px 8px;
      border-radius: 12px;
      background: rgba(255,255,255,0.08);
      color: var(--text-muted);
    }
    .section-content {
      padding: 16px 18px;
    }

    /* 代码与日志黑框 */
    pre.code-block, pre.console-block {
      background: var(--code-bg);
      border: 1px solid var(--card-border);
      border-radius: 6px;
      padding: 14px;
      font-family: ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace;
      font-size: 13px;
      overflow-x: auto;
      white-space: pre-wrap;
      word-break: break-all;
    }
    pre.console-block {
      max-height: 400px;
      overflow-y: auto;
      color: #7ee787;
    }

    /* 审计清单列表 */
    .audit-grid {
      display: flex;
      flex-direction: column;
      gap: 10px;
    }
    .violation-card {
      padding: 12px 16px;
      border-radius: 6px;
      font-size: 13px;
      border: 1px solid var(--card-border);
    }
    .violation-card.ERROR {
      background: var(--fail-bg);
      border-color: var(--fail-border);
    }
    .violation-card.WARN {
      background: var(--warn-bg);
      border-color: var(--warn-border);
    }
    .violation-title {
      font-weight: 700;
      margin-bottom: 6px;
      display: flex;
      align-items: center;
      gap: 8px;
    }
    .violation-snippet {
      background: var(--code-bg);
      padding: 6px 10px;
      border-radius: 4px;
      margin: 6px 0;
      font-family: monospace;
    }

    /* Diff 高亮 */
    .diff-container {
      background: var(--code-bg);
      border: 1px solid var(--card-border);
      border-radius: 6px;
      font-family: ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace;
      font-size: 12px;
      overflow-x: auto;
      max-height: 600px;
    }
    .diff-line {
      display: flex;
      padding: 1px 12px;
      min-width: 100%;
      white-space: pre;
    }
    .diff-line.add { background: var(--diff-add-bg); color: var(--diff-add-text); }
    .diff-line.del { background: var(--diff-del-bg); color: var(--diff-del-text); }
    .diff-line.hunk { background: rgba(88, 166, 255, 0.1); color: var(--diff-hunk-text); font-weight: 600; }
    .diff-line.file { background: rgba(255, 255, 255, 0.05); font-weight: bold; padding-top: 6px; padding-bottom: 6px; border-top: 1px solid var(--card-border); }

    /* 页脚 */
    footer {
      text-align: center;
      padding: 24px 0 12px;
      font-size: 12px;
      color: var(--text-muted);
      border-top: 1px solid var(--card-border);
      margin-top: 32px;
    }
  </style>
</head>
<body>
  <div class="container">
    <!-- 页头 -->
    <header class="header">
      <div class="title-group">
        <div class="agate-logo">A</div>
        <div>
          <h1>Agate 协同审查与物证报告</h1>
          <div style="font-size: 13px; color: var(--text-muted);">工程: {{.ProjectName}}</div>
        </div>
      </div>
      <div>
        {{if eq .Status "PASS"}}
        <span class="meta-badge pass">✔ PASS 准予交付</span>
        {{else}}
        <span class="meta-badge fail">✖ FAIL 阻断拦截</span>
        {{end}}
      </div>
    </header>

    <!-- 元数据矩阵 -->
    <div class="meta-grid">
      <div class="meta-item">
        <div class="meta-label">Git 分支</div>
        <div class="meta-value">{{if .GitBranch}}{{.GitBranch}}{{else}}HEAD{{end}}</div>
      </div>
      <div class="meta-item">
        <div class="meta-label">当前 Commit</div>
        <div class="meta-value">{{if .GitCommit}}{{.GitCommit}}{{else}}Uncommitted{{end}}</div>
      </div>
      <div class="meta-item">
        <div class="meta-label">审查生成时间</div>
        <div class="meta-value">{{.GeneratedAt}}</div>
      </div>
      <div class="meta-item">
        <div class="meta-label">门禁判定耗时</div>
        <div class="meta-value">{{.TotalDuration}}</div>
      </div>
    </div>

    <!-- 1. 任务目标与方案看板 -->
    <details class="section-card" open>
      <summary class="section-header">
        <div class="section-title-wrap">
          <span>📋 阶段任务目标与方案看板 (TASK.md)</span>
        </div>
        <span class="badge-count">工作台</span>
      </summary>
      <div class="section-content">
        {{if .TaskContent}}
        <pre class="code-block">{{.TaskContent}}</pre>
        {{else}}
        <div style="color: var(--text-muted); font-size: 13px;">（未检测到 TASK.md 或文件为空）</div>
        {{end}}
      </div>
    </details>

    <!-- 2. Phase 0 安全红线与代码洁癖审计 -->
    <details class="section-card" open>
      <summary class="section-header">
        <div class="section-title-wrap">
          <span>🛡️ Phase 0 静态安全红线与代码洁癖审计</span>
        </div>
        {{if .AuditResult.HasErrors}}
        <span class="meta-badge fail" style="font-size: 11px; padding: 2px 8px;">存在阻断项</span>
        {{else if .AuditResult.Violations}}
        <span class="meta-badge" style="font-size: 11px; padding: 2px 8px; background: var(--warn-bg); color: var(--warn);">有警示项</span>
        {{else}}
        <span class="meta-badge pass" style="font-size: 11px; padding: 2px 8px;">7 大卡口全绿</span>
        {{end}}
      </summary>
      <div class="section-content">
        {{if .AuditResult.Violations}}
        <div class="audit-grid">
          {{range .AuditResult.Violations}}
          <div class="violation-card {{.Level}}">
            <div class="violation-title">
              <span>[{{.Level}}] {{.Category}}</span>
              <span style="font-size: 12px; opacity: 0.8;">{{.File}}:{{.LineNumber}}</span>
            </div>
            <div>{{.Message}}</div>
            {{if .LineContent}}
            <div class="violation-snippet">{{.LineContent}}</div>
            {{end}}
            {{if .Suggestion}}
            <div style="font-size: 12px; margin-top: 4px; color: var(--text-muted);">建议: {{.Suggestion}}</div>
            {{end}}
          </div>
          {{end}}
        </div>
        {{else}}
        <div style="color: var(--pass); font-weight: 600; font-size: 14px; display: flex; align-items: center; gap: 8px;">
          <span>✔</span>
          <span>7 大红线全量通过：未检测到密钥泄露、大文件残留、机器绝对路径、调试断点、未完工占位符或架构断链。</span>
        </div>
        {{end}}
      </div>
    </details>

    <!-- 3. 测试套件与动态自检执行物证 -->
    <details class="section-card" open>
      <summary class="section-header">
        <div class="section-title-wrap">
          <span>🧪 动态自检与测试套件执行物证</span>
        </div>
        {{if .TestPassed}}
        <span class="meta-badge pass" style="font-size: 11px; padding: 2px 8px;">测试通过</span>
        {{else}}
        <span class="meta-badge fail" style="font-size: 11px; padding: 2px 8px;">测试未通过/未执行</span>
        {{end}}
      </summary>
      <div class="section-content">
        {{if .TestOutput}}
        <pre class="console-block">{{.TestOutput}}</pre>
        {{else}}
        <div style="color: var(--text-muted); font-size: 13px;">（未捕获到测试套件控制台输出或跳过了动态测试）</div>
        {{end}}
      </div>
    </details>

    <!-- 4. 代码变更差异对比器 -->
    <details class="section-card" open>
      <summary class="section-header">
        <div class="section-title-wrap">
          <span>🔍 代码变更差异对比 (Git Diff)</span>
        </div>
        <span class="badge-count">{{if .IsStaged}}暂存区 (Staged){{else}}工作区 (Working Tree){{end}}</span>
      </summary>
      <div class="section-content">
        {{if .GitDiffLines}}
        <div class="diff-container">
          {{range .GitDiffLines}}
          <div class="diff-line {{.Type}}">{{.Content}}</div>
          {{end}}
        </div>
        {{else}}
        <div style="color: var(--text-muted); font-size: 13px;">（无本地改动代码 Diff，工作区与暂存区干净）</div>
        {{end}}
      </div>
    </details>

    <!-- 5. 架构与技术契约 -->
    <details class="section-card">
      <summary class="section-header">
        <div class="section-title-wrap">
          <span>📐 架构技术契约基线 (contexts/context.md)</span>
        </div>
        <span class="badge-count">约束基线</span>
      </summary>
      <div class="section-content">
        {{if .ContextContent}}
        <pre class="code-block">{{.ContextContent}}</pre>
        {{else}}
        <div style="color: var(--text-muted); font-size: 13px;">（未检测到 contexts/context.md）</div>
        {{end}}
      </div>
    </details>

    <!-- 页脚 -->
    <footer>
      Agate (Agent Gate) 架构安全护栏体系 • 离线自包含审查凭证 • 生成即归档
    </footer>
  </div>
</body>
</html>
