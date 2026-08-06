package apikey

import (
	"context"

	"github.com/deeptrols/api/internal/domain"
	"github.com/google/uuid"
)

// Repository defines the API key data access interface.
type Repository interface {
	FindByHash(ctx context.Context, keyHash string) (*domain.APIKey, error)
	FindByID(ctx context.Context, id uuid.UUID) (*domain.APIKey, error)
	ListByUser(ctx context.Context, userID uuid.UUID, tenantID *uuid.UUID) ([]domain.APIKey, error)
	Create(ctx context.Context, key *domain.APIKey) error
	Update(ctx context.Context, key *domain.APIKey) error
	UpdateLastUsed(ctx context.Context, id uuid.UUID) error
	GetSpend(ctx context.Context, keyID uuid.UUID, periodType string) (*domain.APIKeySpend, error)
	UpdateSpend(ctx context.Context, spend *domain.APIKeySpend) error
}

// APIKeySpend is defined here to avoid circular imports.
type APIKeySpend = domain.APIKeySpend
