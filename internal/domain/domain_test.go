package domain

import "testing"

func TestValidRouteStrategy(t *testing.T) {
	cases := map[RouteStrategy]bool{
		RouteStrategyPriorityOnly: true,
		RouteStrategyCost:         true,
		RouteStrategyQuality:      true,
		RouteStrategy("random"):   false,
		RouteStrategy(""):         false,
	}
	for strategy, want := range cases {
		if got := ValidRouteStrategy(strategy); got != want {
			t.Errorf("ValidRouteStrategy(%q) = %v, want %v", strategy, got, want)
		}
	}
}

func TestTenant_AllowTraffic(t *testing.T) {
	cases := []struct {
		name   string
		status TenantStatus
		want   bool
	}{
		{"active allows traffic", TenantStatusActive, true},
		{"pending_review blocks", TenantStatusPendingReview, false},
		{"suspended blocks", TenantStatusSuspended, false},
		{"terminated blocks", TenantStatusTerminated, false},
		{"rejected blocks", TenantStatusRejected, false},
	}
	for _, c := range cases {
		if got := (Tenant{Status: c.status}).AllowTraffic(); got != c.want {
			t.Errorf("%s: AllowTraffic() = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestTenant_ValidTransitions(t *testing.T) {
	cases := []struct {
		from TenantStatus
		to   TenantStatus
		ok   bool
	}{
		{TenantStatusPendingReview, TenantStatusActive, true},
		{TenantStatusPendingReview, TenantStatusRejected, true},
		{TenantStatusPendingReview, TenantStatusSuspended, false},
		{TenantStatusActive, TenantStatusSuspended, true},
		{TenantStatusActive, TenantStatusTerminated, true},
		{TenantStatusActive, TenantStatusRejected, false},
		{TenantStatusSuspended, TenantStatusActive, true},
		{TenantStatusSuspended, TenantStatusTerminated, true},
		{TenantStatusTerminated, TenantStatusActive, false},
		{TenantStatusRejected, TenantStatusActive, false},
	}
	for _, c := range cases {
		got := (Tenant{Status: c.from}).ValidTransitions()
		ok := contains(got, c.to)
		if ok != c.ok {
			t.Errorf("ValidTransitions from %q: contains %q = %v, want %v (got %v)", c.from, c.to, ok, c.ok, got)
		}
	}
}

func contains[T comparable](items []T, target T) bool {
	for _, it := range items {
		if it == target {
			return true
		}
	}
	return false
}

func TestChannel_IsRoutable(t *testing.T) {
	cases := []struct {
		name string
		ch   Channel
		want bool
	}{
		{"active healthy 50", Channel{Status: ChannelStatusActive, HealthScore: 50, HealthStatus: HealthStatusHealthy}, true},
		{"active degraded 60", Channel{Status: ChannelStatusActive, HealthScore: 60, HealthStatus: HealthStatusDegraded}, true},
		{"active score below 50", Channel{Status: ChannelStatusActive, HealthScore: 49, HealthStatus: HealthStatusHealthy}, false},
		{"active unhealthy high score", Channel{Status: ChannelStatusActive, HealthScore: 90, HealthStatus: HealthStatusUnhealthy}, false},
		{"inactive healthy", Channel{Status: ChannelStatusInactive, HealthScore: 100, HealthStatus: HealthStatusHealthy}, false},
		{"pending_setup healthy", Channel{Status: ChannelStatusPendingSetup, HealthScore: 80, HealthStatus: HealthStatusHealthy}, false},
	}
	for _, c := range cases {
		if got := c.ch.IsRoutable(); got != c.want {
			t.Errorf("%s: IsRoutable() = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestModel_IsCallable(t *testing.T) {
	cases := []struct {
		name   string
		status ModelStatus
		want   bool
	}{
		{"active", ModelStatusActive, true},
		{"beta", ModelStatusBeta, true},
		{"deprecated", ModelStatusDeprecated, false},
		{"inactive", ModelStatusInactive, false},
	}
	for _, c := range cases {
		if got := (Model{Status: c.status}).IsCallable(); got != c.want {
			t.Errorf("%s: IsCallable() = %v, want %v", c.name, got, c.want)
		}
	}
}
