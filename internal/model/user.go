package model

import "time"

const RoleUser = "user"

type User struct {
	ID           uint64 `gorm:"primaryKey"`
	Email        string
	PasswordHash string `json:"-"`
	DisplayName  string
	Role         string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}
