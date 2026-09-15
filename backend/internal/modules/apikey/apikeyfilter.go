package apikey

import (
	"bytes"
	"encoding/json"
	"erent/internal/dto/response"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

func ApikeyFilter() gin.HandlerFunc {
	return func(c *gin.Context) {
		parts := strings.Fields(c.GetHeader("Authorization"))
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			response.Error(c, http.StatusUnauthorized, 40100, "missing or invalid authorization header")
			c.Abort()
			return
		}
		apikey := parts[1]
		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			response.Error(c, http.StatusBadRequest, 40000, "invalid request body")
			c.Abort()
			return
		}
		c.Request.Body = io.NopCloser(bytes.NewReader(body))
		var request struct {
			Model string `json:"model"`
		}
		if err := json.Unmarshal(body, &request); err != nil || strings.TrimSpace(request.Model) == "" {
			response.Error(c, http.StatusBadRequest, 40000, "invalid chat request")
			c.Abort()
			return
		}
		accounts, err := CheckOauth(c.Request.Context(), apikey, request.Model)
		if err != nil {
			code, message := http.StatusInternalServerError, "api key authentication failed"
			switch {
			case errors.Is(err, ErrInvalidApikey):
				code, message = http.StatusUnauthorized, "invalid api key"
			case errors.Is(err, ErrUnsupportedModel):
				code, message = http.StatusBadRequest, "unsupported model"
			case errors.Is(err, ErrOauthForbidden):
				code, message = http.StatusForbidden, "no authorized OAuth account for model"
			}
			response.Error(c, code, code*100, message)
			c.Abort()
			return
		}

		c.Set("apikey", apikey)
		c.Set("apikey_accounts", accounts)
	}
}
