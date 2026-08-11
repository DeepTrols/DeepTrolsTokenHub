package domain

import (
	"time"

	"github.com/google/uuid"
)

type UserType string

const (
	UserTypePersonal   UserType = "personal"
	UserTypeEnterprise UserType = "enterprise"
)

type User struct {
	ID           uuid.UUID
	Email        string
	PasswordHash string
	DisplayName  string
	Role         string
	UserType     UserType
	Status       UserStatus
	Phone        string
	AvatarURL    string
	TOTPSecret   string
	TOTPEnabled  bool
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// IsEnterprise returns true when the user belongs to an enterprise tenant.
func (u User) IsEnterprise() bool {
	return u.UserType == UserTypeEnterprise
}

type UserStatus string

const (
	UserStatusActive  UserStatus = "active"
	UserStatusBanned  UserStatus = "banned"
	UserStatusDeleted UserStatus = "deleted"
)

func (u User) IsActive() bool {
	return u.Status == UserStatusActive
}

func NewUser(email, passwordHash, displayName string) User {
	now := time.Now().UTC()
	return User{
		ID:           uuid.New(),
		Email:        email,
		PasswordHash: passwordHash,
		DisplayName:  displayName,
		UserType:     UserTypePersonal,
		Status:       UserStatusActive,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
}
