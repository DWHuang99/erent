package apikey

func init() {
	// Keep these models aligned with the supported chat routes.
	for _, model := range []string{
		"gpt-6-astra", "gpt-5.6-sol", "gpt-5.6-terra",
		"gpt-5.6-luna", "gpt-5.5", "gpt-5.3-codex-spark",
	} {
		Register(model, "codex")
	}
}
