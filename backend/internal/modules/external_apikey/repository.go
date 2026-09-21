package externalapikey

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"gorm.io/gorm"
)

type Repository struct{ db *gorm.DB }

func NewRepo(db *gorm.DB) *Repository { return &Repository{db: db} }

func keyHash(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}

func (r *Repository) Add(ctx context.Context, key *ExternalApikey) error {
	return r.db.WithContext(ctx).Create(key).Error
}

func (r *Repository) GetUsersApiKey(ctx context.Context, userID uint64) ([]ExternalApikey, error) {
	items := make([]ExternalApikey, 0)
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).Order("id ASC").Find(&items).Error
	return items, err
}

// QueryByApiKey matches a raw upstream key within its owner's records.
// Multiple endpoints may use the same key, so all matching records are returned.
func (r *Repository) QueryByApiKey(ctx context.Context, userID uint64, raw string) ([]ExternalApikey, error) {
	items := make([]ExternalApikey, 0)
	err := r.db.WithContext(ctx).Where("user_id = ? AND key_hash = ?", userID, keyHash(raw)).Order("id ASC").Find(&items).Error
	return items, err
}

func (r *Repository) Update(ctx context.Context, userID, id uint64, key *ExternalApikey) error {
	result := r.db.WithContext(ctx).Model(&ExternalApikey{}).Where("user_id = ? AND id = ?", userID, id).Updates(map[string]any{
		"key_hash": key.KeyHash, "ciphertext": key.Ciphertext, "endpoint": key.Endpoint,
		"suffix_chat_completions": key.Suffix.ChatCompletions, "suffix_responses": key.Suffix.Responses,
		"suffix_messages": key.Suffix.Messages,
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *Repository) Delete(ctx context.Context, userID, id uint64) error {
	result := r.db.WithContext(ctx).Where("user_id = ? AND id = ?", userID, id).Delete(&ExternalApikey{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
