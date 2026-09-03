package metrics

import (
	"context"
	"log"
	"time"
)

// DependencyProbeTimeout bounds a single reachability probe so a wedged
// dependency cannot stall the watchdog loop (matches the /readyz timeout).
const DependencyProbeTimeout = 2 * time.Second

// DependencyProbe is one reachability check exported as app_dependency_up.
// Name must be an allowlisted dependency label (see SanitizeDependency);
// Ping returns nil when the dependency is reachable.
type DependencyProbe struct {
	Name string
	Ping func(ctx context.Context) error
}

// StartDependencyWatchdog launches a background goroutine that probes every
// dependency on the given interval and exports the result via
// app_dependency_up{dependency} (TH-P05-05 alert baseline: TokenHubDatabaseUnavailable /
// TokenHubRedisUnavailable alert on the gauge, so the watchdog is the only
// new instrumentation those alerts need).
//
// The watchdog is observability-only: probe outcomes never affect serving or
// worker scheduling, probe faults are swallowed (safely), and a panic stops
// only the watchdog, never the process. The first probe runs immediately so a
// fresh process reports a truthful value before the first alert window
// elapses.
func StartDependencyWatchdog(interval time.Duration, probes ...DependencyProbe) {
	if interval <= 0 || len(probes) == 0 {
		return
	}
	checkAll := func() {
		for _, p := range probes {
			probe := p
			safely(func() {
				ctx, cancel := context.WithTimeout(context.Background(), DependencyProbeTimeout)
				defer cancel()
				SetDependencyUp(probe.Name, probe.Ping(ctx) == nil)
			})
		}
	}
	go func() {
		defer func() {
			if rc := recover(); rc != nil {
				log.Printf("metrics: dependency watchdog stopped after panic: %v", rc)
			}
		}()
		checkAll()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			checkAll()
		}
	}()
}
