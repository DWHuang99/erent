package apikey

import (
	"context"
	"errors"
)

var (
	ErrInvalidApikey    = errors.New("invalid api key")
	ErrUnsupportedModel = errors.New("unsupported model")
	ErrOauthForbidden   = errors.New("no authorized OAuth account for model")
	ErrCheckUnavailable = errors.New("api key checker is not initialized")
)

type Apikeycheck struct {
	repository *ApikeyRepository
}

func NewCheck() *Apikeycheck {
	return &Apikeycheck{}
}

func (c *Apikeycheck) inject(repo *ApikeyRepository) {
	c.repository = repo
}

func (c *Apikeycheck) checkOauth(ctx context.Context, apikey string, model string) ([]ApikeyAccountItem, error) {
	if c.repository == nil || c.repository.database == nil {
		return nil, ErrCheckUnavailable
	}
	accounts, err := c.repository.GetOauthByApikey(ctx, apikey)
	if err != nil {
		return nil, err
	}
	needOauth := GetNeedOauth(model)
	if needOauth == "" {
		return nil, ErrUnsupportedModel
	}
	allowed := make([]ApikeyAccountItem, 0, len(accounts))
	for _, account := range accounts {
		if !account.Disabled && account.Type == needOauth {
			allowed = append(allowed, account)
		}
	}
	if len(allowed) == 0 {
		return nil, ErrOauthForbidden
	}
	return allowed, nil
}

var defaultcheck = NewCheck()

func ApikeycheckInject(repo *ApikeyRepository) {
	defaultcheck.inject(repo)
}

func CheckOauth(ctx context.Context, apikey string, model string) ([]ApikeyAccountItem, error) {
	return defaultcheck.checkOauth(ctx, apikey, model)
}
