package apikey

import "time"

type ApikeyExternalKey struct {
	UserID           uint64 `gorm:"not null"`
	ApiKeyID         uint64 `gorm:"primaryKey;autoIncrement:false"`
	ExternalApiKeyID uint64 `gorm:"column:external_api_key_id;primaryKey;autoIncrement:false"`
	CreatedAt        time.Time
}

func (ApikeyExternalKey) TableName() string { return "api_key_external_keys" }
