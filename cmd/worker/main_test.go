package main

// TH-P05-11 acceptance tests for the leased worker cycle wrapper:
//
//	A — lease acquired: work executes exactly once, success recorded
//	B — lease competition: exactly one instance acquires and executes
//	C — Redis lease error: fail-closed, work never runs, error recorded
//	D — work failure: cycle failed, duration recorded, process continues
//	E — work panic: recovered, panic_recovered recorded, process survives
//	F — /metrics scrape: worker families visible, no sensitive leakage
//
// Redis: miniredis by default (hermetic); set TEST_REDIS_URL to run the same
// suite against a real Redis instance (used once as recorded evidence).

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/prometheus/client_golang/prometheus/testutil"
	goredis "github.com/redis/go-redis/v9"

	"github.com/deeptrols/api/internal/pkg/metrics"
)

// testLeaseRedis returns a live Redis client for lease tests.
func testLeaseRedis(t *testing.T) *goredis.Client {
	t.Helper()
	if url := os.Getenv("TEST_REDIS_URL"); url != "" {
		opts, err := goredis.ParseURL(url)
		if err != nil {
			t.Fatalf("parse TEST_REDIS_URL: %v", err)
		}
		c := goredis.NewClient(opts)
		t.Cleanup(func() { _ = c.Close() })
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := c.Ping(ctx).Err(); err != nil {
			t.Fatalf("TEST_REDIS_URL ping: %v", err)
		}
		return c
	}
	mr := miniredis.RunT(t)
	c := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = c.Close() })
	return c
}

// deadRedisClient returns a client whose every command fails (connection
// refused), simulating a Redis outage for the fail-closed test.
func deadRedisClient(t *testing.T) *goredis.Client {
	t.Helper()
	c := goredis.NewClient(&goredis.Options{
		Addr:        "127.0.0.1:1", // nothing listens here
		MaxRetries:  -1,            // fail immediately, no retry backoff
		DialTimeout: time.Second,
	})
	t.Cleanup(func() { _ = c.Close() })
	return c
}

// uniqueLeaseKey namespaces keys so a shared real Redis never sees collisions
// across runs.
func uniqueLeaseKey(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf("worker:lease:p0511:%d:%s", time.Now().UnixNano(), strings.ReplaceAll(t.Name(), "/", "-"))
}

func leaseCount(worker, outcome string) float64 {
	return testutil.ToFloat64(metrics.Default.WorkerLeaseTotal.
		WithLabelValues(metrics.SanitizeWorker(worker), metrics.SanitizeLeaseOutcome(outcome)))
}

func cycleCount(worker, outcome string) float64 {
	return testutil.ToFloat64(metrics.Default.WorkerCyclesTotal.
		WithLabelValues(metrics.SanitizeWorker(worker), metrics.SanitizeCycleOutcome(outcome)))
}

// durationObservations returns the histogram sample count for a worker.
func durationObservations(worker string) uint64 {
	fams, err := metrics.Default.Registry.Gather()
	if err != nil {
		panic(err)
	}
	for _, f := range fams {
		if f.GetName() != metrics.NameWorkerCycleDuration {
			continue
		}
		for _, m := range f.GetMetric() {
			for _, lp := range m.GetLabel() {
				if lp.GetName() == "worker" && lp.GetValue() == metrics.SanitizeWorker(worker) {
					return m.GetHistogram().GetSampleCount()
				}
			}
		}
	}
	return 0
}

// runLeasedCycle is the exact production composition used by all five
// workers: runSafely(name, func() error { return withLease(...) }).
func runLeasedCycle(ctx context.Context, name string, redis *goredis.Client, key string, ttl time.Duration, fn func() error) {
	runSafely(name, func() error {
		return withLease(ctx, name, redis, key, ttl, fn)
	})
}

// TestCycle_LeaseAcquired_WorkRunsOnce is AC Test A.
func TestCycle_LeaseAcquired_WorkRunsOnce(t *testing.T) {
	client := testLeaseRedis(t)
	key := uniqueLeaseKey(t)
	ctx := context.Background()

	var runs int32
	la := leaseCount("reconciler", metrics.LeaseOutcomeAcquired)
	ca := cycleCount("reconciler", metrics.WorkerOutcomeSuccess)
	dur := durationObservations("reconciler")

	runLeasedCycle(ctx, "reconciler", client, key, 30*time.Second, func() error {
		atomic.AddInt32(&runs, 1)
		return nil
	})

	if runs != 1 {
		t.Fatalf("work executions = %d, want exactly 1", runs)
	}
	if got := leaseCount("reconciler", metrics.LeaseOutcomeAcquired) - la; got != 1 {
		t.Errorf("lease acquired delta = %v, want 1", got)
	}
	if got := cycleCount("reconciler", metrics.WorkerOutcomeSuccess) - ca; got != 1 {
		t.Errorf("cycle success delta = %v, want 1", got)
	}
	if got := cycleCount("reconciler", metrics.WorkerOutcomeFailed); got < 0 {
		t.Errorf("cycle failed counter went backwards: %v", got)
	}
	if got := durationObservations("reconciler") - dur; got != 1 {
		t.Errorf("duration observations delta = %d, want 1", got)
	}
}

// TestCycle_LeaseCompetition_ExactlyOneExecutes is AC Test B (AC-01).
func TestCycle_LeaseCompetition_ExactlyOneExecutes(t *testing.T) {
	client := testLeaseRedis(t)
	key := uniqueLeaseKey(t)
	ctx := context.Background()

	var runs int32
	cycle := func() {
		runLeasedCycle(ctx, "reconciler", client, key, 30*time.Second, func() error {
			atomic.AddInt32(&runs, 1)
			return nil
		})
	}

	la := leaseCount("reconciler", metrics.LeaseOutcomeAcquired)
	ls := leaseCount("reconciler", metrics.LeaseOutcomeSkipped)
	cs := cycleCount("reconciler", metrics.WorkerOutcomeSuccess)
	ck := cycleCount("reconciler", metrics.WorkerOutcomeSkipped)

	// Two instances race for the same lease key at the same instant.
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			cycle()
		}()
	}
	close(start)
	wg.Wait()

	if runs != 1 {
		t.Fatalf("work executions = %d, want exactly 1 (no double execution)", runs)
	}
	if got := leaseCount("reconciler", metrics.LeaseOutcomeAcquired) - la; got != 1 {
		t.Errorf("lease acquired delta = %v, want exactly 1", got)
	}
	if got := leaseCount("reconciler", metrics.LeaseOutcomeSkipped) - ls; got != 1 {
		t.Errorf("lease skipped delta = %v, want exactly 1", got)
	}
	if got := cycleCount("reconciler", metrics.WorkerOutcomeSuccess) - cs; got != 1 {
		t.Errorf("cycle success delta = %v, want exactly 1", got)
	}
	if got := cycleCount("reconciler", metrics.WorkerOutcomeSkipped) - ck; got != 1 {
		t.Errorf("cycle skipped delta = %v, want exactly 1", got)
	}

	// Second round on the same key while the lease is still held: both
	// instances must skip; the work must never run twice.
	la2 := leaseCount("reconciler", metrics.LeaseOutcomeAcquired)
	ls2 := leaseCount("reconciler", metrics.LeaseOutcomeSkipped)
	for i := 0; i < 2; i++ {
		cycle()
	}
	if runs != 1 {
		t.Fatalf("work executions after held-lease round = %d, want still exactly 1", runs)
	}
	if got := leaseCount("reconciler", metrics.LeaseOutcomeAcquired) - la2; got != 0 {
		t.Errorf("held-lease round acquired delta = %v, want 0", got)
	}
	if got := leaseCount("reconciler", metrics.LeaseOutcomeSkipped) - ls2; got != 2 {
		t.Errorf("held-lease round skipped delta = %v, want 2", got)
	}
}

// TestCycle_LeaseError_FailClosed is AC Test C (AC-02).
func TestCycle_LeaseError_FailClosed(t *testing.T) {
	dead := deadRedisClient(t)
	ctx := context.Background()

	var runs int32
	work := func() error {
		atomic.AddInt32(&runs, 1)
		return nil
	}

	le := leaseCount("health_checker", metrics.LeaseOutcomeError)
	la := leaseCount("health_checker", metrics.LeaseOutcomeAcquired)
	cf := cycleCount("health_checker", metrics.WorkerOutcomeFailed)

	// Direct wrapper call: a lease error is returned and the work function
	// is never invoked (fail-closed).
	err := withLease(ctx, "health_checker", dead, uniqueLeaseKey(t), time.Second, work)
	if err == nil {
		t.Fatal("lease error produced nil error, want fail-closed error")
	}
	if errors.Is(err, errLeaseSkipped) {
		t.Fatal("lease error misclassified as lease skipped")
	}
	if !strings.Contains(err.Error(), "lease acquire failed") {
		t.Errorf("unexpected error shape: %v", err)
	}
	if runs != 0 {
		t.Fatalf("work executions = %d, want 0 (fail-closed)", runs)
	}
	if got := leaseCount("health_checker", metrics.LeaseOutcomeError) - le; got != 1 {
		t.Errorf("lease error delta = %v, want 1", got)
	}
	if got := leaseCount("health_checker", metrics.LeaseOutcomeAcquired) - la; got != 0 {
		t.Errorf("lease acquired delta = %v, want 0", got)
	}

	// Through the full production composition: the cycle is recorded as
	// failed and the process keeps running (next cycle below succeeds).
	le2 := leaseCount("health_checker", metrics.LeaseOutcomeError)
	runLeasedCycle(ctx, "health_checker", dead, uniqueLeaseKey(t), time.Second, work)
	if runs != 0 {
		t.Fatalf("work executions = %d, want still 0", runs)
	}
	if got := leaseCount("health_checker", metrics.LeaseOutcomeError) - le2; got != 1 {
		t.Errorf("lease error delta (full composition) = %v, want 1", got)
	}
	if got := cycleCount("health_checker", metrics.WorkerOutcomeFailed) - cf; got != 1 {
		t.Errorf("cycle failed delta = %v, want 1", got)
	}
}

// TestCycle_WorkFailure_CycleFailed_ProcessContinues is AC Test D.
func TestCycle_WorkFailure_CycleFailed_ProcessContinues(t *testing.T) {
	client := testLeaseRedis(t)
	ctx := context.Background()

	cf := cycleCount("billing_sync", metrics.WorkerOutcomeFailed)
	cs := cycleCount("billing_sync", metrics.WorkerOutcomeSuccess)
	dur := durationObservations("billing_sync")

	// Round 1: the work function returns an error.
	runLeasedCycle(ctx, "billing_sync", client, uniqueLeaseKey(t), 30*time.Second, func() error {
		return errors.New("connector unreachable")
	})
	if got := cycleCount("billing_sync", metrics.WorkerOutcomeFailed) - cf; got != 1 {
		t.Errorf("cycle failed delta = %v, want 1", got)
	}
	if got := durationObservations("billing_sync") - dur; got != 1 {
		t.Errorf("duration observations delta = %d, want 1 (failed cycle still timed)", got)
	}

	// Round 2: the process survived; the next cycle succeeds.
	runLeasedCycle(ctx, "billing_sync", client, uniqueLeaseKey(t), 30*time.Second, func() error {
		return nil
	})
	if got := cycleCount("billing_sync", metrics.WorkerOutcomeSuccess) - cs; got != 1 {
		t.Errorf("cycle success delta after failure = %v, want 1", got)
	}
}

// TestCycle_WorkPanic_Recovered_ProcessSurvives is AC Test E (AC-03).
func TestCycle_WorkPanic_Recovered_ProcessSurvives(t *testing.T) {
	client := testLeaseRedis(t)
	ctx := context.Background()

	const payload = "p0511 panic payload"
	cp := cycleCount("subscription_renewer", metrics.WorkerOutcomePanicRecovered)
	cf := cycleCount("subscription_renewer", metrics.WorkerOutcomeFailed)
	cs := cycleCount("subscription_renewer", metrics.WorkerOutcomeSuccess)

	// Without the recover boundary this would crash the whole test binary.
	runLeasedCycle(ctx, "subscription_renewer", client, uniqueLeaseKey(t), 30*time.Second, func() error {
		panic(payload)
	})

	if got := cycleCount("subscription_renewer", metrics.WorkerOutcomePanicRecovered) - cp; got != 1 {
		t.Errorf("panic_recovered delta = %v, want 1", got)
	}
	if got := cycleCount("subscription_renewer", metrics.WorkerOutcomeFailed) - cf; got != 0 {
		t.Errorf("cycle failed delta = %v, want 0 (panic is its own outcome)", got)
	}
	if got := cycleCount("subscription_renewer", metrics.WorkerOutcomeSuccess) - cs; got != 0 {
		t.Errorf("cycle success delta = %v, want 0", got)
	}

	// The next cycle still runs: the process (this test binary) survived.
	runLeasedCycle(ctx, "subscription_renewer", client, uniqueLeaseKey(t), 30*time.Second, func() error {
		return nil
	})
	if got := cycleCount("subscription_renewer", metrics.WorkerOutcomeSuccess) - cs; got != 1 {
		t.Errorf("cycle success delta after panic = %v, want 1", got)
	}

	// The panic payload must never reach the scrape surface.
	rec := httptest.NewRecorder()
	metrics.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if strings.Contains(rec.Body.String(), payload) {
		t.Error("metrics body leaked the panic payload")
	}
}

// TestCycle_NilRedis_SingleInstanceModeUnchanged pins the existing nil-Redis
// behavior: without Redis the lease is always granted and the work runs.
func TestCycle_NilRedis_SingleInstanceModeUnchanged(t *testing.T) {
	ctx := context.Background()
	var runs int32
	la := leaseCount("billing_sync", metrics.LeaseOutcomeAcquired)
	cs := cycleCount("billing_sync", metrics.WorkerOutcomeSuccess)

	runLeasedCycle(ctx, "billing_sync", nil, uniqueLeaseKey(t), time.Second, func() error {
		atomic.AddInt32(&runs, 1)
		return nil
	})

	if runs != 1 {
		t.Fatalf("nil-redis work executions = %d, want 1 (single-instance mode)", runs)
	}
	if got := leaseCount("billing_sync", metrics.LeaseOutcomeAcquired) - la; got != 1 {
		t.Errorf("nil-redis lease acquired delta = %v, want 1", got)
	}
	if got := cycleCount("billing_sync", metrics.WorkerOutcomeSuccess) - cs; got != 1 {
		t.Errorf("nil-redis cycle success delta = %v, want 1", got)
	}
}

// TestMetricsScrape_WorkerFamilies_NoLeakage is AC Test F.
func TestMetricsScrape_WorkerFamilies_NoLeakage(t *testing.T) {
	// Materialize all worker families regardless of what earlier tests ran.
	runLeasedCycle(context.Background(), "subscription_expirer", nil, uniqueLeaseKey(t), time.Second, func() error { return nil })

	rec := httptest.NewRecorder()
	metrics.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, name := range []string{
		metrics.NameWorkerCyclesTotal,
		metrics.NameWorkerCycleDuration,
		metrics.NameWorkerLeaseTotal,
	} {
		if !strings.Contains(body, name) {
			t.Errorf("metrics body missing worker family %s", name)
		}
	}
	// No raw lease keys, hostnames, ids or error text may ever be exposed.
	for _, leak := range []string{
		"worker:lease:", "p0511", "127.0.0.1", "localhost",
		"user_id", "request_id", "tenant_id", "order_no", "api_key", "Bearer", "@",
	} {
		if strings.Contains(body, leak) {
			t.Errorf("metrics body contains forbidden substring %q", leak)
		}
	}
}
