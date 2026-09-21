package apikey

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"time"
)

var ErrInvalidAccounts = errors.New("select valid accounts owned by the current user")
var ErrApikeyNotFound = errors.New("api key not found")

type ApikeyService struct {
	repository *ApikeyRepository
}

func NewService(repository *ApikeyRepository) *ApikeyService {
	return &ApikeyService{repository: repository}
}

func (a *ApikeyService) CreatApikey(ctx context.Context, userid uint64, oauthinfos, externalKeyIDs []uint64) (string, error) {
	if userid == 0 || userid > 1<<63-1 || (len(oauthinfos) == 0 && len(externalKeyIDs) == 0) {
		return "", ErrInvalidAccounts
	}
	// Deduplicate before checking ownership and writing associations.
	ids := make([]uint64, 0, len(oauthinfos))
	seen := make(map[uint64]bool, len(oauthinfos))
	for _, id := range oauthinfos {
		if id == 0 || id > 1<<63-1 {
			return "", ErrInvalidAccounts
		}
		if !seen[id] {
			ids = append(ids, id)
			seen[id] = true
		}
	}
	if len(ids) > 0 {
		if err := a.checkOauth(ctx, userid, ids); err != nil {
			return "", err
		}
	}
	externalIDs := make([]uint64, 0, len(externalKeyIDs))
	externalSeen := make(map[uint64]bool, len(externalKeyIDs))
	for _, id := range externalKeyIDs {
		if id == 0 || id > 1<<63-1 {
			return "", ErrInvalidAccounts
		}
		if !externalSeen[id] {
			externalIDs = append(externalIDs, id)
			externalSeen[id] = true
		}
	}
	if len(externalIDs) > 0 {
		count, err := a.repository.CountOwnedExternalKeys(ctx, userid, externalIDs)
		if err != nil {
			return "", err
		}
		if count != int64(len(externalIDs)) {
			return "", ErrInvalidAccounts
		}
	}

	apikeyraw, apikeyhash, err := generateApikey()
	if err != nil {
		return "", err
	}

	ApikeyInfo := toApikeyinfo(apikeyhash, userid)
	ApikeyInfo.KeyPrefix = apikeyraw[:12]
	Apikeyaccounts := toApikeyaccounts(userid, ids)
	externalKeys := make([]ApikeyExternalKey, 0, len(externalIDs))
	for _, id := range externalIDs {
		externalKeys = append(externalKeys, ApikeyExternalKey{UserID: userid, ExternalApiKeyID: id})
	}
	if err := a.repository.SaveApikey(ctx, &ApikeyInfo, &Apikeyaccounts, externalKeys); err != nil {
		return "", err
	}

	return apikeyraw, nil
}

func (a *ApikeyService) checkOauth(ctx context.Context, userid uint64, oauthinfos []uint64) error {
	if userid == 0 || userid > 1<<63-1 || len(oauthinfos) == 0 {
		return ErrInvalidAccounts
	}
	count, err := a.repository.CountOwnedAccounts(ctx, userid, oauthinfos)
	if err != nil {
		return err
	}
	if count != int64(len(oauthinfos)) {
		return ErrInvalidAccounts
	}
	return nil
}

func generateApikey() (string, string, error) {
	var secret [32]byte
	if _, err := rand.Read(secret[:]); err != nil {
		return "", "", err
	}
	raw := "sk-" + base64.RawURLEncoding.EncodeToString(secret[:])
	hash := sha256.Sum256([]byte(raw))
	return raw, hex.EncodeToString(hash[:]), nil
}

func toApikeyinfo(keyhash string, userid uint64) ApikeyInfo {
	return ApikeyInfo{KeyHash: keyhash, UserID: userid}
}

func toApikeyaccounts(userid uint64, oauthinfos []uint64) []Apikeyaccounts {
	accounts := make([]Apikeyaccounts, 0, len(oauthinfos))
	for _, id := range oauthinfos {
		accounts = append(accounts, Apikeyaccounts{UserID: userid, OAuthInfoID: id})
	}
	return accounts
}

func (a *ApikeyService) UserApikeyList(ctx context.Context, userid uint64) ([]ApikeyListItem, error) {
	return a.repository.GetApikeyByUserId(ctx, userid)
}

func (a *ApikeyService) UpdateApikey(ctx context.Context, userid, id uint64, disabled *bool, expiresAt *time.Time, updateExpiry bool) error {
	return a.repository.UpdateApikey(ctx, userid, id, disabled, expiresAt, updateExpiry)
}

func (a *ApikeyService) ReplaceApikeyAccounts(ctx context.Context, userid, id uint64, oauthinfos, externalKeyIDs []uint64) error {
	ids := make([]uint64, 0, len(oauthinfos))
	seen := make(map[uint64]bool, len(oauthinfos))
	for _, accountID := range oauthinfos {
		if accountID == 0 || accountID > 1<<63-1 {
			return ErrInvalidAccounts
		}
		if !seen[accountID] {
			ids = append(ids, accountID)
			seen[accountID] = true
		}
	}
	if len(ids) == 0 && len(externalKeyIDs) == 0 {
		return ErrInvalidAccounts
	}
	externalIDs := make([]uint64, 0, len(externalKeyIDs))
	externalSeen := make(map[uint64]bool, len(externalKeyIDs))
	for _, keyID := range externalKeyIDs {
		if keyID == 0 || keyID > 1<<63-1 {
			return ErrInvalidAccounts
		}
		if !externalSeen[keyID] {
			externalIDs = append(externalIDs, keyID)
			externalSeen[keyID] = true
		}
	}
	return a.repository.ReplaceApikeyAccounts(ctx, userid, id, ids, externalIDs)
}

func (a *ApikeyService) DeleteApikey(ctx context.Context, userid, id uint64) error {
	return a.repository.DeleteApikey(ctx, userid, id)
}
