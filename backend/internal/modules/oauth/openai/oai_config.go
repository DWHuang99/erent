package openai

func OaiAuthURLParams() map[string]string {
	return map[string]string{
		"prompt":                     "login",
		"id_token_add_organizations": "true",
		"codex_cli_simplified_flow":  "true",
	}
}

func OaiScopes() []string {
	return []string{}
}
