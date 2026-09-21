package skill

// SkillConfig represents the configurable settings for one agent skill.
type SkillConfig struct {
	Name        string `json:"name"`         // unique skill name (e.g. "sql_executor")
	DisplayName string `json:"display_name"` // human-readable
	Description string `json:"description"`  // what the skill does
	Enabled     bool   `json:"enabled"`      // whether LLM can use this tool
	ConfigJSON  string `json:"config_json"`  // skill-specific JSON config
	// RequiresApproval marks destructive tools that must be confirmed by the
	// user before execution (SPEC-101). Stored as a first-class field (not
	// inside ConfigJSON).
	RequiresApproval bool `json:"requires_approval"`
}
