package oauth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
	"time"

	"erent/internal/dto/request"
	"erent/internal/dto/response"
	jwtservice "erent/internal/middleware/jwt"
	"erent/internal/modules/oauth/oidc"

	"github.com/gin-gonic/gin"
	"golang.org/x/oauth2"
)

var AuthenticationRequired = errors.New("authentication required")

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
	userID, ok := c.Get(jwtservice.UserIDContextKey)
	ownerID, valid := userID.(uint64)
	if !ok || !valid || ownerID == 0 {
		response.Error(c, http.StatusUnauthorized, 40100, "authentication required")
		return
	}
	provider := c.Query("provider")
	if _, err := h.service.authFor(provider); err != nil {
		if errors.Is(err, ErrProviderUnavailable) {
			response.Error(c, http.StatusBadRequest, 400, "invalid_provider")
		} else {
			response.Error(c, http.StatusServiceUnavailable, 503, "oauth service unavailable")
		}
		return
	}
	state, err := randomValue()
	if err != nil {
		response.Error(c, 500, 500, "生成 state 失败")
		return
	}

	// PKCE code_verifier。
	verifier := oauth2.GenerateVerifier()
	nonce, err := randomValue()
	if err != nil {
		response.Error(c, http.StatusInternalServerError, 500, "generate nonce failed")
		return
	}

	authURL, err := h.service.AuthCodeURL(provider, state, verifier, nonce)
	if err != nil {
		response.Error(c, http.StatusServiceUnavailable, 503, "oauth service unavailable")
		return
	}
	if err := h.service.StoreFlow(state, oidc.LoginFlow{
		Provider:  provider,
		Verifier:  verifier,
		UserID:    ownerID,
		Nonce:     nonce,
		ExpiresAt: time.Now().Add(5 * time.Minute),
	}, c.Request.Context()); err != nil {
		response.Error(c, 500, 500, "保存登录状态失败")
		return
	}

	if strings.Contains(c.GetHeader("Accept"), "application/json") {
		c.Header("Cache-Control", "no-store")
		response.Success(c, gin.H{"url": authURL})
		return
	}
	c.Redirect(302, authURL)
}

func (h *OauthHandler) Callback(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Header("Referrer-Policy", "no-referrer")
	// 处理 OAuth2 回调逻辑
	ctx := c.Request.Context()
	state := c.Query("state")
	if state == "" {
		response.Error(c, http.StatusBadRequest, 400, "invalid_state")
		return
	}

	flow, err := h.service.PopFlow(state, ctx)
	if errors.Is(err, ErrInvalidOAuthState) || (err == nil && (time.Now().After(flow.ExpiresAt) || flow.UserID == 0 || flow.Nonce == "" || flow.Provider == "")) {
		response.Error(c, http.StatusBadRequest, 400, "invalid_state")
		return
	}
	if err != nil {
		response.Error(c, http.StatusInternalServerError, 500, "load login state failed")
		return
	}

	if providerError := c.Query("error"); providerError != "" {
		response.Error(c, 400, 400, "provider denied")
		return
	}

	code := c.Query("code")
	if code == "" {
		response.Error(c, 400, 400, "missing code")
		return
	}

	oauthToken, err := h.service.Exchange(
		ctx,
		code,
		flow.Verifier,
		flow.Provider,
		"browser",
	)
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidExchange), errors.Is(err, ErrExchangeRejected):
			response.Error(c, http.StatusBadRequest, 400, "token exchange rejected")
		case errors.Is(err, ErrProviderUnavailable), errors.Is(err, ErrUpstreamUnavailable):
			response.Error(c, http.StatusServiceUnavailable, 503, "oauth service unavailable")
		case errors.Is(err, ErrExchangeTimeout):
			response.Error(c, http.StatusGatewayTimeout, 504, "token exchange timed out")
		default:
			response.Error(c, http.StatusBadGateway, 502, "token exchange failed")
		}
		return
	}

	if err := h.service.SaveToken(ctx, oauthToken, flow, true); err != nil {
		switch {
		case errors.Is(err, ErrInvalidIDToken):
			response.Error(c, http.StatusBadGateway, 502, "invalid provider identity")
		case errors.Is(err, ErrInvalidOAuthState):
			response.Error(c, http.StatusBadRequest, 400, "invalid_state")
		case errors.Is(err, ErrUpstreamUnavailable), errors.Is(err, ErrProviderUnavailable):
			response.Error(c, http.StatusServiceUnavailable, 503, "oauth service unavailable")
		default:
			response.Error(c, http.StatusInternalServerError, 500, "save token failed")
		}
		return
	}
	if strings.Contains(c.GetHeader("Accept"), "text/html") && !strings.Contains(c.GetHeader("Accept"), "application/json") {
		c.Redirect(http.StatusSeeOther, "/authorized-accounts")
		return
	}
	response.SuccessWithStatus(c, http.StatusOK, nil, "oauth credentials saved")
}

func (h *OauthHandler) LoginDeviceFlow(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	ctx := c.Request.Context()
	ownerID, err := getUserid(c)
	if err != nil {
		response.Error(c, 401, 40100, "authentication required")
		return
	}
	provider := c.Query("provider")
	startedAt := time.Now()
	usercodeinfo, err := h.service.GetDeviceFlowCode(ctx, provider)
	if err != nil {
		deviceFlowErrorResponse(c, err)
		return
	}
	flow := deviceLoginFlow{Provider: provider, UserID: ownerID, DeviceAuthID: usercodeinfo.DeviceAuthID, UserCode: usercodeinfo.UserCode, Interval: usercodeinfo.Interval, ExpiresAt: startedAt.Add(15 * time.Minute)}
	if err := h.service.storeDeviceFlow(ctx, flow); err != nil {
		response.Error(c, 500, 500, "save device login state failed")
		return
	}
	response.Success(c, gin.H{
		"device_auth_id":   usercodeinfo.DeviceAuthID,
		"user_code":        usercodeinfo.UserCode,
		"interval":         usercodeinfo.Interval,
		"verification_url": usercodeinfo.VerificationURL,
	})
}

func deviceFlowErrorResponse(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrInvalidDeviceFlow), errors.Is(err, ErrDeviceFlowRejected):
		response.Error(c, 400, 400, "device authorization rejected")
	case errors.Is(err, ErrDeviceFlowTimeout), errors.Is(err, context.DeadlineExceeded):
		response.Error(c, 504, 504, "device authorization timed out")
	case errors.Is(err, context.Canceled):
		response.Error(c, 408, 408, "device authorization canceled")
	case errors.Is(err, ErrProviderUnavailable), errors.Is(err, ErrUpstreamUnavailable):
		response.Error(c, 503, 503, "oauth service unavailable")
	default:
		response.Error(c, 502, 502, "device authorization failed")
	}
}

func (h *OauthHandler) CallbackDeviceFlow(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	ctx := c.Request.Context()
	provider := c.Query("provider")
	ownerID, err := getUserid(c)
	if err != nil {
		response.Error(c, http.StatusUnauthorized, 40100, "authentication required")
		return
	}
	var req request.OAuthPollRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, 400, "invalid poll request")
		return
	}
	flow, err := h.service.popDeviceFlow(ctx, req.DeviceAuthID, provider, ownerID)
	if err != nil {
		if errors.Is(err, ErrInvalidOAuthState) {
			response.Error(c, 400, 400, "invalid_state")
		} else {
			response.Error(c, 500, 500, "load device login state failed")
		}
		return
	}
	ctx, cancel := context.WithDeadline(ctx, flow.ExpiresAt)
	defer cancel()
	verifyinfo, err := h.service.Poll(ctx, flow.DeviceAuthID, flow.UserCode, flow.Interval, flow.Provider)
	if err != nil {
		deviceFlowErrorResponse(c, err)
		return
	}

	oauthToken, err := h.service.Exchange(
		ctx,
		verifyinfo.Code,
		verifyinfo.CodeVerifier,
		provider,
		"device",
	)
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidExchange), errors.Is(err, ErrExchangeRejected):
			response.Error(c, http.StatusBadRequest, 400, "token exchange rejected")
		case errors.Is(err, ErrProviderUnavailable), errors.Is(err, ErrUpstreamUnavailable):
			response.Error(c, http.StatusServiceUnavailable, 503, "oauth service unavailable")
		case errors.Is(err, ErrExchangeTimeout):
			response.Error(c, http.StatusGatewayTimeout, 504, "token exchange timed out")
		default:
			response.Error(c, http.StatusBadGateway, 502, "token exchange failed")
		}
		return
	}

	if err := h.service.SaveToken(ctx, oauthToken, oidc.LoginFlow{Provider: provider, UserID: ownerID}, false); err != nil {
		switch {
		case errors.Is(err, ErrInvalidIDToken):
			response.Error(c, http.StatusBadGateway, 502, "invalid provider identity")
		case errors.Is(err, ErrInvalidOAuthState):
			response.Error(c, http.StatusBadRequest, 400, "invalid_state")
		case errors.Is(err, ErrUpstreamUnavailable), errors.Is(err, ErrProviderUnavailable):
			response.Error(c, http.StatusServiceUnavailable, 503, "oauth service unavailable")
		default:
			response.Error(c, http.StatusInternalServerError, 500, "save token failed")
		}
		return
	}
	if strings.Contains(c.GetHeader("Accept"), "text/html") && !strings.Contains(c.GetHeader("Accept"), "application/json") {
		c.Redirect(http.StatusSeeOther, "/authorized-accounts")
		return
	}
	response.SuccessWithStatus(c, http.StatusOK, nil, "oauth credentials saved")
}

func getUserid(c *gin.Context) (uint64, error) {
	userID, ok := c.Get(jwtservice.UserIDContextKey)
	ownerID, valid := userID.(uint64)
	if !ok || !valid || ownerID == 0 {
		return 0, AuthenticationRequired
	}
	return ownerID, nil
}

func (h *OauthHandler) RefreshToken(c *gin.Context) {
	ownerID, err := getUserid(c)
	if err != nil {
		response.Error(c, http.StatusUnauthorized, 40100, "authentication required")
		return
	}
	var req request.OAuthRefreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, 400, "invalid refresh request")
		return
	}
	if err := h.service.RefreshToken(c.Request.Context(), ownerID, req.ID); err != nil {
		switch {
		case errors.Is(err, ErrOAuthNotFound):
			response.Error(c, 404, 404, "oauth credential not found")
		case errors.Is(err, ErrInvalidRefresh):
			response.Error(c, 400, 400, "invalid refresh request")
		case errors.Is(err, ErrRefreshRejected):
			response.Error(c, 400, 400, "refresh token rejected; reauthorize account")
		case errors.Is(err, ErrProviderUnavailable), errors.Is(err, ErrUpstreamUnavailable):
			response.Error(c, 503, 503, "oauth service unavailable")
		case errors.Is(err, ErrRefreshTimeout), errors.Is(err, context.DeadlineExceeded):
			response.Error(c, 504, 504, "token refresh timed out")
		case errors.Is(err, context.Canceled):
			response.Error(c, 408, 408, "token refresh canceled")
		case errors.Is(err, ErrRefreshFailed), errors.Is(err, ErrInvalidIDToken):
			response.Error(c, 502, 502, "invalid provider refresh response")
		default:
			response.Error(c, 500, 500, "save refreshed credentials failed")
		}
		return
	}
	c.Header("Cache-Control", "no-store")
	response.Success(c, gin.H{"message": "refresh success"})
}

func (h *OauthHandler) OauthList(c *gin.Context) {
	ctx := c.Request.Context()
	ownerID, err := getUserid(c)
	if err != nil {
		response.Error(c, http.StatusUnauthorized, 40100, "authentication required")
		return
	}
	oauthlist, err := h.service.getUserOauth(ctx, ownerID)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, 500, "load oauth list failed")
		return
	}
	c.Header("Cache-Control", "no-store")
	response.Success(c, gin.H{"oauthlist": oauthlist})
}

func (h *OauthHandler) Delete(c *gin.Context) {
	ctx := c.Request.Context()
	ownerID, err := getUserid(c)
	if err != nil {
		response.Error(c, http.StatusUnauthorized, 40100, "authentication required")
		return
	}
	var req request.OAuthRefreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, 400, "invalid delete request")
		return
	}
	err = h.service.deleteUserOauth(ctx, req.ID, ownerID)
	if err != nil {
		if errors.Is(err, ErrOAuthNotFound) {
			response.Error(c, http.StatusNotFound, 404, "oauth credential not found")
		} else {
			response.Error(c, http.StatusInternalServerError, 500, "delete oauth failed")
		}
		return
	}
	response.Success(c, nil)
}
