package oauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	rdb "erent/internal/middleware/redis"
	"erent/internal/modules/oauth/oidc"
	"erent/internal/security"

	oidcgo "github.com/coreos/go-oidc/v3/oidc"
	"github.com/redis/go-redis/v9"
	"golang.org/x/oauth2"
	"gorm.io/gorm"
)

var ErrInvalidOAuthState = errors.New("invalid oauth state")

var ErrInvalidIDToken = errors.New("invalid ID token")

type IDTokenClaims struct {
	Email string `json:"email"`
	Auth  struct {
		AccountID string `json:"chatgpt_account_id"`
	} `json:"https://api.openai.com/auth"`
}

// TokenExchanger is implemented by the upstream directory adapter.
type TokenExchanger interface {
	Exchange(ctx context.Context, code, verifier, provider string) (*oauth2.Token, error)
	RefreshToken(ctx context.Context, refreshtoken string, provider string) (*oauth2.Token, error)
}

type OauthService struct {
	redisClient *redis.Client
	// Provider configurations are initialized at startup and read-only during requests.
	oidcAuth      map[string]*oidc.OIDCAuth
	exchanger     TokenExchanger
	repository    *Repository
	encryptionKey []byte
}

func NewOauthService(
	redisClient *redis.Client,
	oidcAuth map[string]*oidc.OIDCAuth,
	exchanger TokenExchanger,
	repository *Repository,
	encryptionKey []byte,
) *OauthService {
	return &OauthService{
		redisClient:   redisClient,
		oidcAuth:      oidcAuth,
		exchanger:     exchanger,
		repository:    repository,
		encryptionKey: append([]byte(nil), encryptionKey...),
	}
}

// StoreFlow 保存一次登录流程的状态，供 Callback 阶段校验。
func (o *OauthService) StoreFlow(state string, flow oidc.LoginFlow, ctx context.Context) error {
	data, err := json.Marshal(flow)
	if err != nil {
		return err
	}
	return rdb.SetState(o.redisClient, ctx, state, data, 5*time.Minute)
}

// PopFlow 读取并删除对应 state 的流程数据，确保 state 只能使用一次。
func (o *OauthService) PopFlow(state string, ctx context.Context) (oidc.LoginFlow, error) {
	data, err := rdb.DeleteState(o.redisClient, ctx, state) // 删除 Redis 中的 state
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return oidc.LoginFlow{}, ErrInvalidOAuthState
		}
		return oidc.LoginFlow{}, fmt.Errorf("delete OAuth state: %w", err)
	}
	var flow oidc.LoginFlow
	if err := json.Unmarshal([]byte(data), &flow); err != nil {
		return oidc.LoginFlow{}, fmt.Errorf("decode OAuth state: %w", err)
	}
	return flow, nil
}

func toAuthCodeOptions(params map[string]string) []oauth2.AuthCodeOption {
	options := make([]oauth2.AuthCodeOption, 0, len(params))

	for key, value := range params {
		options = append(options, oauth2.SetAuthURLParam(key, value))
	}

	return options
}

func (o *OauthService) authFor(provider string) (*oidc.OIDCAuth, error) {
	auth, ok := o.oidcAuth[provider]
	if !ok {
		return nil, ErrProviderUnavailable
	}
	if auth == nil || auth.OauthConfig == nil {
		return nil, ErrUpstreamUnavailable
	}
	return auth, nil
}

// AuthCodeURL 生成对应 provider 的授权跳转地址。
func (o *OauthService) AuthCodeURL(provider, state, verifier, nonce string) (string, error) {
	auth, err := o.authFor(provider)
	if err != nil {
		return "", err
	}
	options := toAuthCodeOptions(auth.AuthURLParams)
	options = append([]oauth2.AuthCodeOption{oauth2.S256ChallengeOption(verifier)}, options...)
	options = append(options, oidcgo.Nonce(nonce))
	return auth.OauthConfig.AuthCodeURL(
		state,
		options...,
	), nil
}

// Exchange 使用授权码兑换 OAuth2 Token。
func (o *OauthService) Exchange(ctx context.Context, code, verifier, provider string) (*oauth2.Token, error) {
	if _, err := o.authFor(provider); err != nil {
		return nil, err
	}
	if o.exchanger == nil {
		return nil, ErrUpstreamUnavailable
	}
	return o.exchanger.Exchange(ctx, code, verifier, provider)
}

func (o *OauthService) VerifyIDToken(ctx context.Context, rawIDToken, provider string) (*oidcgo.IDToken, error) {
	auth, err := o.authFor(provider)
	if err != nil {
		return nil, err
	}
	if auth.Verifier == nil {
		return nil, ErrUpstreamUnavailable
	}
	return auth.Verifier.Verify(ctx, rawIDToken)
}

// RefreshToken locks the owned database row across refresh and persistence.
// PostgreSQL row locks serialize callers across API instances without a lease timeout.
func (o *OauthService) RefreshToken(ctx context.Context, ownerID, id uint64) error {
	if ownerID == 0 || id == 0 {
		return ErrInvalidRefresh
	}
	if o.repository == nil || o.exchanger == nil {
		return ErrUpstreamUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	return o.repository.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		model, err := o.repository.GetOwnedTokenForUpdate(ctx, tx, ownerID, id)
		if err != nil {
			return err
		}
		if err := o.refreshTokenCredentials(ctx, model); err != nil {
			return err
		}
		return o.repository.UpdateToken(ctx, tx, ownerID, id, model)
	})
}

// refreshTokenCredentials refreshes and encrypts credentials without database writes.
func (o *OauthService) refreshTokenCredentials(ctx context.Context, model *OAuthInfo) error {
	provider := ""
	switch model.Type {
	case "codex":
		provider = "oai"
	default:
		return ErrProviderUnavailable
	}
	if _, err := o.authFor(provider); err != nil {
		return err
	}
	plain, err := security.Decrypt(o.encryptionKey, []byte(model.RefreshToken))
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(plain)) == "" {
		return ErrRefreshRejected
	}
	token, err := o.exchanger.RefreshToken(ctx, string(plain), provider)
	if err != nil {
		return err
	}
	if token == nil || strings.TrimSpace(token.AccessToken) == "" || (!token.Expiry.IsZero() && !token.Expiry.After(time.Now())) {
		return ErrRefreshFailed
	}
	// Refresh responses may omit id_token. Preserve the existing verified identity then.
	if value := token.Extra("id_token"); value != nil && value != "" {
		raw, ok := value.(string)
		if !ok {
			return ErrInvalidIDToken
		}
		verified, err := o.VerifyIDToken(ctx, raw, provider)
		if err != nil {
			return ErrInvalidIDToken
		}
		var claims IDTokenClaims
		if err := verified.Claims(&claims); err != nil || claims.Auth.AccountID != model.AccountID {
			return ErrInvalidIDToken
		}
		encrypted, err := security.Encrypt(o.encryptionKey, []byte(raw))
		if err != nil {
			return err
		}
		model.IDToken = string(encrypted)
	}
	access, err := security.Encrypt(o.encryptionKey, []byte(token.AccessToken))
	if err != nil {
		return err
	}
	model.AccessToken = string(access)
	if token.RefreshToken != "" {
		refresh, err := security.Encrypt(o.encryptionKey, []byte(token.RefreshToken))
		if err != nil {
			return err
		}
		model.RefreshToken = string(refresh)
	}
	model.Expired = nil
	if !token.Expiry.IsZero() {
		expiry := token.Expiry.UTC()
		model.Expired = &expiry
	}
	now := time.Now().UTC()
	model.LastRefresh = &now
	return nil
}

// SaveToken verifies provider identity before persisting credentials for the state owner.
func (o *OauthService) SaveToken(ctx context.Context, token *oauth2.Token, flow oidc.LoginFlow) error {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if flow.UserID == 0 || flow.Nonce == "" || flow.Provider == "" {
		return ErrInvalidOAuthState
	}
	if token == nil || strings.TrimSpace(token.AccessToken) == "" || (!token.Expiry.IsZero() && !token.Expiry.After(time.Now())) {
		return ErrInvalidIDToken
	}
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok || rawIDToken == "" {
		return ErrInvalidIDToken
	}
	idToken, err := o.VerifyIDToken(ctx, rawIDToken, flow.Provider)
	if errors.Is(err, ErrUpstreamUnavailable) || errors.Is(err, ErrProviderUnavailable) {
		return err
	}
	if err != nil || idToken.Nonce != flow.Nonce {
		return ErrInvalidIDToken
	}
	var claims IDTokenClaims
	if err := idToken.Claims(&claims); err != nil || strings.TrimSpace(claims.Auth.AccountID) == "" {
		return ErrInvalidIDToken
	}
	if o.repository == nil || flow.Provider != "oai" {
		return ErrProviderUnavailable
	}
	model, err := o.toOauthInfo(0,
		flow.UserID,
		claims.Auth.AccountID,
		claims.Email,
		token.AccessToken,
		token.RefreshToken,
		rawIDToken, &token.Expiry)
	if err != nil {
		return err
	}
	return o.repository.SaveToken(ctx, &model)
}

func (o *OauthService) getUserOauth(ctx context.Context, userid uint64) ([]OAuthListItem, error) {
	if o.repository == nil {
		return nil, ErrUpstreamUnavailable
	}
	return o.repository.getUserOauth(ctx, userid)
}

func (o *OauthService) deleteUserOauth(ctx context.Context, id int64) error {
	return o.repository.deleteUserOauth(ctx, id)
}

func (o *OauthService) toOauthInfo(id, userid uint64, accountid, email, accesstoken, refreshtoken, idtoken string, expired *time.Time) (OAuthInfo, error) {
	accessToken, err := security.Encrypt(o.encryptionKey, []byte(accesstoken))
	if err != nil {
		return OAuthInfo{}, err
	}
	refreshToken, err := security.Encrypt(o.encryptionKey, []byte(refreshtoken))
	if err != nil {
		return OAuthInfo{}, err
	}
	idTokenEncrypted, err := security.Encrypt(o.encryptionKey, []byte(idtoken))
	if err != nil {
		return OAuthInfo{}, err
	}
	now := time.Now().UTC()
	model := OAuthInfo{
		ID: id, UserID: userid, AccountID: accountid, Email: email,
		AccessToken: string(accessToken), RefreshToken: string(refreshToken), IDToken: string(idTokenEncrypted),
		Type: "codex", LastRefresh: &now,
	}
	if expired != nil && !expired.IsZero() {
		expiry := expired.UTC()
		model.Expired = &expiry
	}
	return model, nil
}
