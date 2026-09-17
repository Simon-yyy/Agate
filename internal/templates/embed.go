package templates

import _ "embed"

//go:embed SKILL.md
var DefaultSkill []byte

//go:embed ignore.tpl
var DefaultIgnore []byte

//go:embed agents.tpl
var DefaultAgentsTpl []byte

//go:embed context.tpl
var DefaultContextTpl []byte

//go:embed task.tpl
var DefaultTaskTpl []byte

//go:embed memory.tpl
var DefaultMemoryTpl []byte

// 便捷别名引用
var (
	SkillMD    = DefaultSkill
	IgnoreTpl  = DefaultIgnore
	AgentsTpl  = DefaultAgentsTpl
	ContextTpl = DefaultContextTpl
	TaskTpl    = DefaultTaskTpl
	MemoryTpl  = DefaultMemoryTpl
)
