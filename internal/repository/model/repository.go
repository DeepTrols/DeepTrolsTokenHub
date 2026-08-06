package model

import (
	"context"

	"github.com/deeptrols/api/internal/domain"
	"github.com/google/uuid"
)

type Repository interface {
	ListActive(ctx context.Context) ([]domain.Model, error)
	FindByID(ctx context.Context, id uuid.UUID) (*domain.Model, error)
	FindByCode(ctx context.Context, code string) (*domain.Model, error)
	ListByTenant(ctx context.Context, tenantID *uuid.UUID) ([]TenantModelView, error)
	GetTenantModel(ctx context.Context, tenantID uuid.UUID, modelCode string) (*domain.TenantModel, error)
}

type PricingRepository interface {
	FindByModel(ctx context.Context, modelID uuid.UUID, tenantID *uuid.UUID) ([]domain.ModelPricing, error)
}

type TenantModelView struct {
	Model         domain.Model
	TenantModel   *domain.TenantModel
	PlatformPrice []domain.ModelPricing
	TenantPrice   []domain.ModelPricing
}
