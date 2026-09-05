package oauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	rdb "erent/internal/middleware/redis"
	"erent/internal/modules/oauth/oidc"

	"github.com/redis/go-redis/v9"
	"golang.org/x/oauth2"
)

var ErrInvalidOAuthState = errors.New("invalid oauth state")

// TokenExchanger is implemented by the upstream directory adapter.
type TokenExchanger interface {
	Exchange(ctx context.Context, code, verifier, provider string) (*oauth2.Token, error)
}

type OauthService struct {
	redisClient *redis.Client
	oidcAuth    *oidc.OIDCAuth
	exchanger   TokenExchanger
	provider    string
}

func NewOauthService(
	redisClient *redis.Client,
	oidcAuth *oidc.OIDCAuth,
	exchanger TokenExchanger,
	provider string,
) *OauthService {
	return &OauthService{
		redisClient: redisClient,
		oidcAuth:    oidcAuth,
		exchanger:   exchanger,
		provider:    provider,
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

// AuthCodeURL 生成授权跳转地址。
func (o *OauthService) AuthCodeURL(state string, verifier string) string {
	options := toAuthCodeOptions(o.oidcAuth.AuthURLParams)
	options = append([]oauth2.AuthCodeOption{oauth2.S256ChallengeOption(verifier)}, options...)
	return o.oidcAuth.OauthConfig.AuthCodeURL(
		state,
		options...,
	)
}

// Exchange 使用授权码兑换 OAuth2 Token。
func (o *OauthService) Exchange(ctx context.Context, code, verifier string) (*oauth2.Token, error) {
	if o.exchanger == nil {
		return nil, ErrUpstreamUnavailable
	}
	return o.exchanger.Exchange(ctx, code, verifier, o.provider)
}

func (o *OauthService) SaveToken(_ context.Context, _ *oauth2.Token) error {
	return nil
}
