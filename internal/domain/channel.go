package domain

import (
	"time"

	"github.com/google/uuid"
)

type Channel struct {
	ID             uuid.UUID
	Name           string
	ModelID        uuid.UUID
	TenantID       *uuid.UUID
	PoolType       PoolType
	HealthScore    int
	HealthStatus   HealthStatus
	Status         ChannelStatus
	Weight         int
	MaxConcurrency int
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type PoolType string

const (
	PoolTypeShared    PoolType = "shared"
	PoolTypeDedicated PoolType = "dedicated"
	PoolTypeMixed     PoolType = "mixed"
)

type HealthStatus string

const (
	HealthStatusHealthy   HealthStatus = "healthy"
	HealthStatusDegraded  HealthStatus = "degraded"
	HealthStatusUnhealthy HealthStatus = "unhealthy"
)

type ChannelStatus string

const (
	ChannelStatusActive       ChannelStatus = "active"
	ChannelStatusInactive     ChannelStatus = "inactive"
	ChannelStatusPendingSetup ChannelStatus = "pending_setup"
	ChannelStatusDisabled     ChannelStatus = "disabled"
)

func (c Channel) IsRoutable() bool {
	return c.Status == ChannelStatusActive &&
		c.HealthScore >= 50 &&
		c.HealthStatus != HealthStatusUnhealthy
}

type ChannelInstance struct {
	ID            uuid.UUID
	ChannelID     uuid.UUID
	InstanceType  string
	BaseURL       string
	ProviderRoute string
	CurrentLoad   int
	MaxLoad       int
	Config        map[string]any
	Status        InstanceStatus
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type InstanceStatus string

const (
	InstanceStatusActive   InstanceStatus = "active"
	InstanceStatusInactive InstanceStatus = "inactive"
	InstanceStatusPending  InstanceStatus = "pending"
)

type RoutePolicy struct {
	ID                  uuid.UUID
	Name                string
	TenantID            *uuid.UUID
	UserLevel           string
	ModelID             *uuid.UUID
	Priority            int
	CandidateChannelIDs []uuid.UUID
	FallbackPolicy      FallbackPolicy
	IsActive            bool
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

type FallbackPolicy string

const (
	FallbackDisabled      FallbackPolicy = "disabled"
	FallbackTenantDefault FallbackPolicy = "tenant_default"
	FallbackSharedAllowed FallbackPolicy = "shared_allowed"
	FallbackNextPolicy    FallbackPolicy = "next_policy"
)
