package channel

import (
	"context"
	"time"

	"github.com/deeptrols/api/internal/domain"
	"github.com/google/uuid"
)

type Repository interface {
	ListByModel(ctx context.Context, modelID uuid.UUID, tenantID *uuid.UUID) ([]domain.Channel, error)
	FindByID(ctx context.Context, id uuid.UUID) (*domain.Channel, error)
	ListInstances(ctx context.Context, channelID uuid.UUID) ([]domain.ChannelInstance, error)
	UpdateHealth(ctx context.Context, id uuid.UUID, score int, status domain.HealthStatus) error
	UpdateInstanceLoad(ctx context.Context, id uuid.UUID, load int) error
	EnterCooldown(ctx context.Context, instanceID uuid.UUID, until time.Time) error
	ClearCooldown(ctx context.Context, instanceID uuid.UUID) error
}
