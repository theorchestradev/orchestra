package core

// Kilo is built on OpenCode and shares its session plugin, which reports status and
// the conversation ID. Its default permissions already ask before risky commands.
var kiloAgent = &agentSpec{
	ID: "kilo", Name: "Kilo", Executable: "kilo", SkillKey: "kilo",
	Command:      func(l launch) string { return l.command("kilo", "--session", "--prompt") },
	Prepare:      sessionPlugin("KILO_CONFIG_CONTENT"),
	ModelFlag:    "--model",
	ModelCommand: []string{"kilo", "models"},
	ParseModels:  parseProviderModels,
}
