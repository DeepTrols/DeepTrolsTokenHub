package usage

import (
	"context"

	"github.com/deeptrols/api/internal/domain"
	"github.com/google/uuid"
)

type Repository interface {
	CreateUsageLog(ctx context.Context, log *domain.UsageLog) error
	CreateChargeLines(ctx context.Context, lines []domain.ChargeLine) error
	CreateProviderEvidence(ctx context.Context, evidence *domain.ProviderEvidence) error
	FindByRequestID(ctx context.Context, requestID string) (*domain.UsageLog, error)
	ListByUser(ctx context.Context, userID uuid.UUID, filter UsageFilter) ([]domain.UsageLog, int, error)
	ListByAPIKey(ctx context.Context, apiKeyID uuid.UUID, filter UsageFilter) ([]domain.UsageLog, int, error)
}

type UsageFilter struct {
	ModelCode string
	Status    string
	RequestID string
	From      string
	To        string
	Limit     int
	Offset    int
}
