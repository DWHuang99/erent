package oauth

import "time"

// OAuthInfo stores upstream account credentials with AES-GCM encrypted token fields.
// Use a separate DTO for API responses.
type OAuthInfo struct {
	ID           uint64 `gorm:"primaryKey"`
	UserID       uint64 `gorm:"not null;uniqueIndex:oauth_infos_owner_account_idx,priority:1"`
	AccessToken  string `gorm:"type:text;not null;default:''" json:"-"`
	AccountID    string `gorm:"type:text;not null;default:'';uniqueIndex:oauth_infos_owner_account_idx,priority:3"`
	Disabled     bool   `gorm:"not null;default:false"`
	Email        string `gorm:"type:text;not null;default:''"`
	Expired      *time.Time
	IDToken      string `gorm:"column:id_token;type:text;not null;default:''" json:"-"`
	LastRefresh  *time.Time
	RefreshToken string    `gorm:"type:text;not null;default:''" json:"-"`
	Type         string    `gorm:"type:text;not null;default:codex;uniqueIndex:oauth_infos_owner_account_idx,priority:2"`
	CreatedAt    time.Time `gorm:"not null;default:CURRENT_TIMESTAMP"`
	UpdatedAt    time.Time `gorm:"not null;default:CURRENT_TIMESTAMP"`
}

func (OAuthInfo) TableName() string {
	return "oauth_infos"
}

// OAuthListItem exposes account metadata without stored credentials or owner IDs.
type OAuthListItem struct {
	ID          uint64     `json:"id"`
	AccountID   string     `json:"accountId"`
	Email       string     `json:"email"`
	Type        string     `json:"type"`
	Disabled    bool       `json:"disabled"`
	Expired     *time.Time `json:"expired"`
	LastRefresh *time.Time `json:"lastRefresh"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
}
