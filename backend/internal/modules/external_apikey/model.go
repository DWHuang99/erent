package externalapikey

import (
	"erent/internal/dto/request"
	"time"
)

// ExternalApikey stores recoverable credentials encrypted at rest.
type ExternalApikey struct {
	ID         uint64                       `gorm:"primaryKey" json:"id"`
	UserID     uint64                       `gorm:"not null;index:external_key_lookup,priority:1" json:"-"`
	KeyHash    string                       `gorm:"size:64;not null;index:external_key_lookup,priority:2" json:"-"`
	Ciphertext string                       `gorm:"type:text;not null" json:"-"`
	Endpoint   string                       `gorm:"type:text;not null" json:"endpoint"`
	Suffix     request.ExternalApikeySuffix `gorm:"embedded;embeddedPrefix:suffix_" json:"suffix"`
	CreatedAt  time.Time                    `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt  time.Time                    `gorm:"not null;default:CURRENT_TIMESTAMP" json:"updated_at"`
}

func (ExternalApikey) TableName() string { return "external_api_keys" }

// ListItem exposes metadata without credential storage fields.
type ListItem struct {
	ID        uint64                       `json:"id"`
	Endpoint  string                       `json:"endpoint"`
	Suffix    request.ExternalApikeySuffix `json:"suffix"`
	CreatedAt time.Time                    `json:"created_at"`
	UpdatedAt time.Time                    `json:"updated_at"`
}
