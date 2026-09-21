package apikey

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ApikeyRepository struct {
	database *gorm.DB
}

func (r *ApikeyRepository) UpdateApikey(ctx context.Context, userid, id uint64, disabled *bool, expiresAt *time.Time, updateExpiry bool) error {
	updates := map[string]any{"updated_at": time.Now()}
	if disabled != nil {
		updates["disabled"] = *disabled
	}
	if updateExpiry {
		updates["expires_at"] = expiresAt
	}
	result := r.database.WithContext(ctx).Model(&ApikeyInfo{}).Where("id = ? AND user_id = ?", id, userid).Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrApikeyNotFound
	}
	return nil
}

func (r *ApikeyRepository) ReplaceApikeyAccounts(ctx context.Context, userid, id uint64, ids, externalIDs []uint64) error {
	if len(ids) == 0 && len(externalIDs) == 0 {
		return ErrInvalidAccounts
	}
	err := r.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Serialize replacements of the same key so concurrent requests cannot merge scopes.
		var key ApikeyInfo
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND user_id = ?", id, userid).First(&key).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrApikeyNotFound
			}
			return err
		}
		var count int64
		if err := tx.Table("oauth_infos").Where("user_id = ? AND id IN ?", userid, ids).Count(&count).Error; err != nil {
			return err
		}
		if count != int64(len(ids)) {
			return ErrInvalidAccounts
		}
		if err := tx.Where("api_key_id = ? AND user_id = ?", id, userid).Delete(&Apikeyaccounts{}).Error; err != nil {
			return err
		}
		accounts := toApikeyaccounts(userid, ids)
		for i := range accounts {
			accounts[i].ApiKeyID = id
		}
		if len(accounts) > 0 {
			if err := tx.Create(&accounts).Error; err != nil {
				return err
			}
		}
		if len(externalIDs) > 0 {
			var count int64
			if err := tx.Table("external_api_keys").Where("user_id = ? AND id IN ?", userid, externalIDs).Count(&count).Error; err != nil {
				return err
			}
			if count != int64(len(externalIDs)) {
				return ErrInvalidAccounts
			}
		}
		if err := tx.Where("api_key_id = ? AND user_id = ?", id, userid).Delete(&ApikeyExternalKey{}).Error; err != nil {
			return err
		}
		if len(externalIDs) > 0 {
			keys := make([]ApikeyExternalKey, 0, len(externalIDs))
			for _, externalID := range externalIDs {
				keys = append(keys, ApikeyExternalKey{UserID: userid, ApiKeyID: id, ExternalApiKeyID: externalID})
			}
			if err := tx.Create(&keys).Error; err != nil {
				return err
			}
		}
		return tx.Model(&ApikeyInfo{}).Where("id = ? AND user_id = ?", id, userid).Update("updated_at", time.Now()).Error
	})
	if errors.Is(err, gorm.ErrForeignKeyViolated) {
		return ErrInvalidAccounts
	}
	return err
}

func (r *ApikeyRepository) DeleteApikey(ctx context.Context, userid, id uint64) error {
	result := r.database.WithContext(ctx).Where("id = ? AND user_id = ?", id, userid).Delete(&ApikeyInfo{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrApikeyNotFound
	}
	return nil
}

func NewRepository(database *gorm.DB) *ApikeyRepository {
	return &ApikeyRepository{database: database}
}

func (r *ApikeyRepository) CountOwnedAccounts(ctx context.Context, userid uint64, ids []uint64) (int64, error) {
	var count int64
	err := r.database.WithContext(ctx).Table("oauth_infos").Where("user_id = ? AND id IN ?", userid, ids).Count(&count).Error
	return count, err
}

func (r *ApikeyRepository) CountOwnedExternalKeys(ctx context.Context, userid uint64, ids []uint64) (int64, error) {
	var count int64
	err := r.database.WithContext(ctx).Table("external_api_keys").Where("user_id = ? AND id IN ?", userid, ids).Count(&count).Error
	return count, err
}

func (r *ApikeyRepository) SaveApikey(ctx context.Context, model *ApikeyInfo, relatemodel *[]Apikeyaccounts, externalKeys []ApikeyExternalKey) error {
	if len(*relatemodel) == 0 && len(externalKeys) == 0 {
		return ErrInvalidAccounts
	}
	err := r.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(model).Error; err != nil {
			return err
		}
		for i := range *relatemodel {
			(*relatemodel)[i].ApiKeyID = model.ID
			(*relatemodel)[i].UserID = model.UserID
		}
		if len(*relatemodel) > 0 {
			if err := tx.Create(relatemodel).Error; err != nil {
				return err
			}
		}
		for i := range externalKeys {
			externalKeys[i].ApiKeyID = model.ID
			externalKeys[i].UserID = model.UserID
		}
		if len(externalKeys) > 0 {
			return tx.Create(&externalKeys).Error
		}
		return nil
	})
	if errors.Is(err, gorm.ErrForeignKeyViolated) {
		return ErrInvalidAccounts
	}
	return err
}

func (r *ApikeyRepository) GetApikeyByUserId(ctx context.Context, userid uint64) ([]ApikeyListItem, error) {
	items := make([]ApikeyListItem, 0)
	err := r.database.WithContext(ctx).Table("api_keys").
		Select("id", "key_prefix", "disabled", "expires_at").
		Where("user_id = ?", userid).Order("id DESC").Find(&items).Error
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return items, nil
	}
	ids := make([]uint64, 0, len(items))
	indexes := make(map[uint64]int, len(items))
	for i := range items {
		items[i].ExternalApiKeyIDs = make([]uint64, 0)
		items[i].Accounts = make([]ApikeyAccountItem, 0)
		ids = append(ids, items[i].ID)
		indexes[items[i].ID] = i
	}
	var accounts []struct {
		ApiKeyID          uint64
		ApikeyAccountItem `gorm:"embedded"`
	}
	err = r.database.WithContext(ctx).Table("api_key_accounts AS bindings").
		Select("bindings.api_key_id, accounts.id, accounts.email, accounts.type, accounts.disabled").
		Joins("JOIN oauth_infos AS accounts ON accounts.id = bindings.oauth_info_id AND accounts.user_id = bindings.user_id").
		Where("bindings.user_id = ? AND bindings.api_key_id IN ?", userid, ids).
		Order("bindings.api_key_id DESC, accounts.id ASC").Scan(&accounts).Error
	if err != nil {
		return nil, err
	}
	for _, account := range accounts {
		i := indexes[account.ApiKeyID]
		items[i].Accounts = append(items[i].Accounts, account.ApikeyAccountItem)
	}
	var externalKeys []ApikeyExternalKey
	if err := r.database.WithContext(ctx).
		Select("api_key_id", "external_api_key_id").
		Where("user_id = ? AND api_key_id IN ?", userid, ids).
		Order("api_key_id DESC, external_api_key_id ASC").Find(&externalKeys).Error; err != nil {
		return nil, err
	}
	for _, key := range externalKeys {
		i := indexes[key.ApiKeyID]
		items[i].ExternalApiKeyIDs = append(items[i].ExternalApiKeyIDs, key.ExternalApiKeyID)
	}
	return items, nil
}

func (r *ApikeyRepository) GetOauthByApikey(ctx context.Context, raw string) ([]ApikeyAccountItem, error) {
	hash := sha256.Sum256([]byte(raw))
	var key ApikeyInfo
	err := r.database.WithContext(ctx).Where("key_hash = ?", hex.EncodeToString(hash[:])).First(&key).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrInvalidApikey
	}
	if err != nil {
		return nil, err
	}
	if key.Disabled || (key.ExpiresAt != nil && !key.ExpiresAt.After(time.Now())) {
		return nil, ErrInvalidApikey
	}
	accounts := make([]ApikeyAccountItem, 0)
	err = r.database.WithContext(ctx).Table("api_key_accounts AS bindings").
		Select("accounts.id, accounts.email, accounts.type, accounts.disabled, accounts.access_token, accounts.expired").
		Joins("JOIN oauth_infos AS accounts ON accounts.id = bindings.oauth_info_id AND accounts.user_id = bindings.user_id").
		Where("bindings.api_key_id = ? AND bindings.user_id = ?", key.ID, key.UserID).
		Order("accounts.id ASC").Scan(&accounts).Error
	return accounts, err
}
