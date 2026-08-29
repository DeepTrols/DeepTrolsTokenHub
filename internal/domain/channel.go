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
	GroupName      string
	Weight         int
	MaxConcurrency int
	Strategy       RouteStrategy
	StickySession  bool
	FallbackOrder  int
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type RouteStrategy string

const (
	RouteStrategyPriorityOnly RouteStrategy = "priority_only"
	RouteStrategyCost         RouteStrategy = "cost"
	RouteStrategyQuality      RouteStrategy = "quality"
)

// ValidRouteStrategy reports whether s is a supported routing strategy.
func ValidRouteStrategy(s RouteStrategy) bool {
	switch s {
	case RouteStrategyPriorityOnly, RouteStrategyCost, RouteStrategyQuality:
		return true
	default:
		return false
	}
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
	ID               uuid.UUID
	ChannelID        uuid.UUID
	InstanceType     string
	BaseURL          string
	ProviderRoute    string
	CurrentLoad      int
	MaxLoad          int
	ConcurrencyLimit int
	CooldownUntil    *time.Time
	LastCheckedAt    *time.Time
	Config           map[string]any
	Status           InstanceStatus
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type InstanceStatus string

const (
	InstanceStatusActive   InstanceStatus = "active"
	InstanceStatusInactive InstanceStatus = "inactive"
	InstanceStatusPending  InstanceStatus = "pending"
)
