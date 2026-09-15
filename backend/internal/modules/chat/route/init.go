package route

import "erent/internal/modules/translator"

func init() {
	// Codex 使用 ChatGPT 登录时支持的模型（2026-09-12）。
	// 来源：https://learn.chatgpt.com/docs/models
	for _, model := range []string{
		"gpt-6-astra",
		"gpt-5.6-sol",
		"gpt-5.6-terra",
		"gpt-5.6-luna",
		"gpt-5.5",
		"gpt-5.3-codex-spark",
	} {
		for _, clientFormat := range []translator.Format{
			translator.FormatClaude,
			translator.FormatOpenAIResponse,
			translator.FormatOpenAI,
		} {
			Register(model, AuthModeAccessToken, clientFormat, Route{
				Endpoint:       "https://chatgpt.com/backend-api/codex/responses",
				UpstreamFormat: translator.FormatCodex,
				AuthHeader:     "Authorization",
				AuthPrefix:     "Bearer ",
				ForceStream:    true})
		}
	}
}
