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
