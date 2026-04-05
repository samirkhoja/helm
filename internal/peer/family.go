package peer

import "strings"

const (
	FamilyClaude   = "claude"
	FamilyCodex    = "codex"
	FamilyGemini   = "gemini"
	FamilyCopilot  = "copilot"
	FamilyCursor   = "cursor"
	FamilyKiro     = "kiro"
	FamilyAider    = "aider"
	FamilyOpenCode = "opencode"
	FamilyGeneric  = "generic"
)

func NormalizeFamily(value string) string {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case FamilyClaude:
		return FamilyClaude
	case FamilyCodex:
		return FamilyCodex
	case FamilyGemini:
		return FamilyGemini
	case FamilyCopilot:
		return FamilyCopilot
	case FamilyCursor:
		return FamilyCursor
	case FamilyKiro:
		return FamilyKiro
	case FamilyAider:
		return FamilyAider
	case FamilyOpenCode:
		return FamilyOpenCode
	default:
		return FamilyGeneric
	}
}

func SupportsSkillMarkdown(family string) bool {
	switch NormalizeFamily(family) {
	case FamilyClaude, FamilyCodex, FamilyCopilot, FamilyKiro, FamilyOpenCode:
		return true
	default:
		return false
	}
}
