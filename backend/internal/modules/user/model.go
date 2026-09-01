package user

import "time"

// User is the GORM model for local username/password authentication.
type User struct {
	ID           uint64 `gorm:"primaryKey"`
	Username     string `gorm:"size:50;uniqueIndex;not null"`
	PasswordHash string `gorm:"size:255;not null"`
	RoleCode     string `gorm:"size:50;not null;default:user"`
	IsActive     bool   `gorm:"not null;default:true"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type UserAuth struct {
	ID           uint64
	Username     string
	PasswordHash string
	RoleCode     string
	IsActive     bool
}

type CurrentUser struct {
	ID          uint64
	Username    string
	RoleCode    string
	RoleName    string
	Roles       []string
	Permissions []string
	IsActive    bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
}
