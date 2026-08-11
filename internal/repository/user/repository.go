package user

import (
	"context"
	"fmt"

	"github.com/deeptrols/api/internal/domain"
	"github.com/google/uuid"
)

// ErrNotFound is returned when a user is not found.
var ErrNotFound = fmt.Errorf("user not found")

// Repository defines the user data access interface.
type Repository interface {
	FindByEmail(ctx context.Context, email string) (*domain.User, error)
	FindByID(ctx context.Context, id uuid.UUID) (*domain.User, error)
	Create(ctx context.Context, user *domain.User) error
	List(ctx context.Context, limit, offset int) ([]domain.User, error)
	UpdateStatus(ctx context.Context, id uuid.UUID, status domain.UserStatus) error
	UpdateRole(ctx context.Context, id uuid.UUID, role string) error
	UpdateProfile(ctx context.Context, id uuid.UUID, displayName, phone, avatarURL string) error
	UpdatePassword(ctx context.Context, id uuid.UUID, passwordHash string) error
	Count(ctx context.Context) (int, error)
}
