package oauth

import (
	"context"
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrNotFound = errors.New("user not found")

type Repository struct {
	database *gorm.DB
}

func NewRepository(database *gorm.DB) *Repository {
	return &Repository{database: database}
}

func (r *Repository) SaveToken(ctx context.Context, model *OAuthInfo) error {
	return r.database.WithContext(ctx).Create(model).Error
}

// GetOwnedTokenForUpdate runs SELECT ... WHERE id = ? AND user_id = ? FOR UPDATE.
// tx must be the active transaction owned by the calling service.
func (r *Repository) GetOwnedTokenForUpdate(ctx context.Context, tx *gorm.DB, ownerID, id uint64) (*OAuthInfo, error) {
	var model OAuthInfo
	err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND user_id = ?", id, ownerID).First(&model).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrOAuthNotFound
	}
	if err != nil {
		return nil, err
	}
	return &model, nil
}

// UpdateToken writes only credential fields using the same transaction as the locked read.
func (r *Repository) UpdateToken(ctx context.Context, tx *gorm.DB, ownerID, id uint64, model *OAuthInfo) error {
	result := tx.WithContext(ctx).Model(&OAuthInfo{}).Where("id = ? AND user_id = ?", id, ownerID).Updates(map[string]any{
		"access_token": model.AccessToken, "refresh_token": model.RefreshToken,
		"id_token": model.IDToken, "expired": model.Expired, "last_refresh": model.LastRefresh,
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrOAuthNotFound
	}
	return nil
}

func (r *Repository) getUserOauth(ctx context.Context, userid uint64) ([]OAuthListItem, error) {
	items := make([]OAuthListItem, 0)
	err := r.database.WithContext(ctx).Model(&OAuthInfo{}).
		Select("id", "account_id", "email", "type", "disabled", "expired", "last_refresh", "created_at", "updated_at").
		Where("user_id = ?", userid).Order("id DESC").Find(&items).Error
	if err != nil {
		return nil, err
	}
	return items, nil
}

func (r *Repository) deleteUserOauth(ctx context.Context, id int64) error {
	err := r.database.WithContext(ctx).Delete(&OAuthInfo{}, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	return nil
}
