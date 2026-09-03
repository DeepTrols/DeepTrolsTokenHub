package metrics

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

func dependencyUp(dependency string) float64 {
	return testutil.ToFloat64(Default.DependencyUp.WithLabelValues(SanitizeDependency(dependency)))
}

// TestWatchdog_ExportsReachability proves the watchdog exports truthful
// up/down transitions for the dependency gauge and that a failing probe
// never disturbs the healthy one.
func TestWatchdog_ExportsReachability(t *testing.T) {
	var dbHealthy atomic.Bool
	dbHealthy.Store(true)
	var probes atomic.Int64

	StartDependencyWatchdog(5*time.Millisecond,
		DependencyProbe{
			Name: DependencyDatabase,
			Ping: func(context.Context) error {
				probes.Add(1)
				if dbHealthy.Load() {
					return nil
				}
				return errors.New("dial tcp: connection refused")
			},
		},
		DependencyProbe{
			Name: DependencyRedis,
			Ping: func(context.Context) error { return nil },
		},
	)

	// Wait for at least one full probe round (first round runs immediately).
	deadline := time.Now().Add(2 * time.Second)
	for probes.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if probes.Load() == 0 {
		t.Fatal("watchdog never probed")
	}
	waitFor := func(dependency string, want float64) {
		t.Helper()
		deadline := time.Now().Add(2 * time.Second)
		for dependencyUp(dependency) != want && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}
		if got := dependencyUp(dependency); got != want {
			t.Fatalf("app_dependency_up{dependency=%q} = %v, want %v", dependency, got, want)
		}
	}
	waitFor(DependencyDatabase, 1)
	waitFor(DependencyRedis, 1)

	// Database fails -> gauge drops while Redis stays up.
	dbHealthy.Store(false)
	waitFor(DependencyDatabase, 0)
	if got := dependencyUp(DependencyRedis); got != 1 {
		t.Fatalf("redis gauge disturbed by database failure: %v", got)
	}

	// Database recovers -> gauge rises again (alert recovery condition).
	dbHealthy.Store(true)
	waitFor(DependencyDatabase, 1)
}

// TestSanitizeDependency pins the bounded dependency label space: the
// allowlist passes, everything else (DSN fragments, hostnames, arbitrary
// strings) is clamped to "other".
func TestSanitizeDependency(t *testing.T) {
	for _, in := range []string{DependencyDatabase, DependencyRedis} {
		if got := SanitizeDependency(in); got != in {
			t.Errorf("SanitizeDependency(%q) = %q, want %q", in, got, in)
		}
	}
	bad := []string{
		"",
		"postgres://deeptrols:secret@10.0.0.5:5432/prod", // DSN with credentials
		"redis-1.prod.internal:6379",                     // host:port
		"user_id=123",
		"database\ninjected_label",
	}
	for _, in := range bad {
		if got := SanitizeDependency(in); got != dependencyOther {
			t.Errorf("SanitizeDependency(%q) = %q, want %q", in, got, dependencyOther)
		}
	}
}
