package apikey

import "time"

type ApikeyInfo struct {
	ID        uint64 `gorm:"primaryKey"`
	UserID    uint64 `gorm:"not null;index"`
	KeyHash   string `gorm:"size:64;not null;uniqueIndex" json:"-"`
	KeyPrefix string `gorm:"size:20;not null"`
	Disabled  bool   `gorm:"not null;default:false"`
	ExpiresAt *time.Time
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (ApikeyInfo) TableName() string { return "api_keys" }

type Apikeyaccounts struct {
	UserID      uint64 `gorm:"not null"`
	ApiKeyID    uint64 `gorm:"primaryKey;autoIncrement:false"`
	OAuthInfoID uint64 `gorm:"column:oauth_info_id;primaryKey;autoIncrement:false"`
	CreatedAt   time.Time
}

func (Apikeyaccounts) TableName() string { return "api_key_accounts" }

// ApikeyListItem exposes key metadata and bound accounts without credentials.
type ApikeyListItem struct {
	ExternalApiKeyIDs []uint64            `json:"external_api_key_ids" gorm:"-"`
	ID                uint64              `json:"id"`
	KeyPrefix         string              `json:"key_prefix"`
	Disabled          bool                `json:"disabled"`
	ExpiresAt         *time.Time          `json:"expires_at"`
	Accounts          []ApikeyAccountItem `json:"accounts" gorm:"-"`
}

type ApikeyAccountItem struct {
	AccessToken string     `json:"-"`
	Expired     *time.Time `json:"-"`
	ID          uint64     `json:"id"`
	Email       string     `json:"email"`
	Type        string     `json:"type"`
	Disabled    bool       `json:"disabled"`
}
