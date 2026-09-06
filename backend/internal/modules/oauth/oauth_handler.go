package oauth

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net/http"
	"time"

	"erent/internal/modules/oauth/oidc"

	"github.com/gin-gonic/gin"
	"golang.org/x/oauth2"
)

type OauthHandler struct {
	service *OauthService
}

func NewOauthHandler(service *OauthService) *OauthHandler {
	return &OauthHandler{service: service}
}

func randomValue() (string, error) {
	data := make([]byte, 32)

	if _, err := rand.Read(data); err != nil {
		return "", err
	}

	return base64.RawURLEncoding.EncodeToString(data), nil
}

func (h *OauthHandler) Login(c *gin.Context) {
	state, err := randomValue()
	if err != nil {
		c.JSON(500, gin.H{"error": "生成 state 失败"})
		return
	}

	// PKCE code_verifier。
	verifier := oauth2.GenerateVerifier()

	if err := h.service.StoreFlow(state, oidc.LoginFlow{
		Verifier:  verifier,
		ExpiresAt: time.Now().Add(5 * time.Minute),
	}, c.Request.Context()); err != nil {
		c.JSON(500, gin.H{"error": "保存登录状态失败"})
		return
	}

	authURL := h.service.AuthCodeURL(
		state,
		verifier,
	)

	c.Redirect(302, authURL)
}

func (h *OauthHandler) Callback(c *gin.Context) {
	// 处理 OAuth2 回调逻辑
	ctx := c.Request.Context()
	state := c.Query("state")
	if state == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_state"})
		return
	}

	flow, err := h.service.PopFlow(state, ctx)
	if errors.Is(err, ErrInvalidOAuthState) || (err == nil && time.Now().After(flow.ExpiresAt)) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_state"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "load login state failed"})
		return
	}

	if providerError := c.Query("error"); providerError != "" {
		c.JSON(400, gin.H{"error": "provider denied"})
		return
	}

	code := c.Query("code")
	if code == "" {
		c.JSON(400, gin.H{"error": "missing code"})
		return
	}

	oauthToken, err := h.service.Exchange(
		ctx,
		code,
		flow.Verifier,
	)
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidExchange), errors.Is(err, ErrExchangeRejected):
			c.JSON(http.StatusBadRequest, gin.H{"error": "token exchange rejected"})
		case errors.Is(err, ErrProviderUnavailable), errors.Is(err, ErrUpstreamUnavailable):
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "oauth service unavailable"})
		case errors.Is(err, ErrExchangeTimeout):
			c.JSON(http.StatusGatewayTimeout, gin.H{"error": "token exchange timed out"})
		default:
			c.JSON(http.StatusBadGateway, gin.H{"error": "token exchange failed"})
		}
		return
	}

	if err := h.service.SaveToken(ctx, oauthToken); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "save token failed"})
	}
}
