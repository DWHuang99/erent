package apikey

import (
	"context"
	"erent/internal/security"
	"errors"
	"strings"
	"time"
)

var ErrCredentialUnavailable = errors.New("OAuth credential unavailable")

type Credential struct {
	Mode   string
	Secret string `json:"-"`
}

type CredentialGetter struct {
	encryptionKey []byte
}

func NewCredentialGetter() *CredentialGetter {
	return &CredentialGetter{}
}

func (g *CredentialGetter) inject(encryptionKey []byte) {
	g.encryptionKey = append([]byte(nil), encryptionKey...)
}

// getCredential receives the accounts authorized by ApikeyFilter, in repository order.
func (g *CredentialGetter) getCredential(ctx context.Context, accounts []ApikeyAccountItem) (Credential, error) {
	if err := ctx.Err(); err != nil {
		return Credential{}, err
	}
	if len(accounts) == 0 {
		return Credential{}, ErrOauthForbidden
	}
	// For now, always select the first authorized account. No fallback or rotation.
	account := accounts[0]
	if account.Disabled || account.Type != "codex" {
		return Credential{}, ErrOauthForbidden
	}
	if account.Expired != nil && !account.Expired.After(time.Now()) {
		return Credential{}, ErrCredentialUnavailable
	}
	plain, err := security.Decrypt(g.encryptionKey, []byte(account.AccessToken))
	if err != nil || strings.TrimSpace(string(plain)) == "" {
		return Credential{}, ErrCredentialUnavailable
	}
	return Credential{Mode: "access_token", Secret: string(plain)}, nil
}

var defaultCredentialGetter = NewCredentialGetter()

func CredentialInject(encryptionKey []byte) {
	defaultCredentialGetter.inject(encryptionKey)
}

func GetCredential(ctx context.Context, accounts []ApikeyAccountItem) (Credential, error) {
	return defaultCredentialGetter.getCredential(ctx, accounts)
}
