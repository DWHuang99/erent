package chat

import (
	"erent/internal/modules/apikey"
	"erent/internal/modules/translator"

	"github.com/gin-gonic/gin"
)

func RegisterChatRoutes(api *gin.RouterGroup, handler *ChatHandler) {
	api = api.Group("", apikey.ApikeyFilter())
	api.POST("/v1/messages", func(c *gin.Context) { handler.Chat(c, c.Writer, translator.FormatClaude) })
	api.POST("/v1/response", func(c *gin.Context) { handler.Chat(c, c.Writer, translator.FormatOpenAIResponse) })
	api.POST("/chat/completions", func(c *gin.Context) { handler.Chat(c, c.Writer, translator.FormatOpenAI) })
}
