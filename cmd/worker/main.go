package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/deeptrols/api/internal/app"
	"github.com/deeptrols/api/internal/config"
	"github.com/deeptrols/api/internal/pkg/lease"
	"github.com/deeptrols/api/internal/worker/health_checker"
	"github.com/deeptrols/api/internal/worker/reconciliation"
	"github.com/deeptrols/api/internal/worker/subscriptions"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	application, err := app.NewApp(cfg)
	if err != nil {
		log.Fatalf("failed to create app: %v", err)
	}
	defer application.Shutdown()

	fmt.Println("Worker starting...")

	checker := health_checker.New(application.Pool)
	reconciler := reconciliation.New(application.Pool)
	subscriptionExpirer := subscriptions.New(application.Pool)
	subscriptionRenewer := subscriptions.NewRenewer(application.Pool, application.Wallets, application.Subscriptions)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go runHealthChecker(ctx, checker, application.Redis)
	go runReconciler(ctx, reconciler, application.Redis)
	go runBillingSync(ctx, application, application.Redis)
	go runSubscriptionExpirer(ctx, subscriptionExpirer, application.Redis)
	go runSubscriptionRenewer(ctx, subscriptionRenewer, application.Redis)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	fmt.Println("Worker shutting down...")
	cancel()
	time.Sleep(2 * time.Second)
}

// runSafely executes a worker cycle with panic recovery so a single bad run
// can never take down the whole worker process.
func runSafely(name string, fn func() error) {
	defer func() {
		if rc := recover(); rc != nil {
			log.Printf("worker %s panicked (recovered): %v", name, rc)
		}
	}()
	if err := fn(); err != nil {
		log.Printf("worker %s: %v", name, err)
	}
}

// withLease runs fn only when this instance holds the Redis lease for the
// given key (distributed leader election). Skipped cycles are logged. Without
// Redis the lease is always granted (single-instance mode).
func withLease(ctx context.Context, name string, redis *goredis.Client, key string, ttl time.Duration, fn func() error) error {
	held, err := lease.Acquire(ctx, redis, key, ttl)
	if err != nil {
		return fmt.Errorf("%s: lease acquire failed: %w", name, err)
	}
	if !held {
		log.Printf("worker %s: lease not acquired (another instance is leader); skipping cycle", name)
		return nil
	}
	return fn()
}

func runHealthChecker(ctx context.Context, c *health_checker.Checker, redis *goredis.Client) {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			runSafely("health_checker", func() error {
				return withLease(ctx, "health_checker", redis, "worker:lease:health_checker", 50*time.Second, func() error {
					if n, err := c.Run(ctx); err != nil {
						return err
					} else {
						log.Printf("health_checker: checked %d channels", n)
					}
					return nil
				})
			})
		}
	}
}

func runReconciler(ctx context.Context, r *reconciliation.Reconciler, redis *goredis.Client) {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			runSafely("reconciler", func() error {
				return withLease(ctx, "reconciler", redis, "worker:lease:reconciler", 5*time.Minute, func() error {
					return r.Run(ctx)
				})
			})
		}
	}
}

// runBillingSync periodically pulls due external billing connectors
// (OneAPI / NewAPI / Aliyun) into billing_records. Distributed leader election
// via the same Redis lease used by the other workers.
func runBillingSync(ctx context.Context, application *app.App, redis *goredis.Client) {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			runSafely("billing_sync", func() error {
				return withLease(ctx, "billing_sync", redis, "worker:lease:billing_sync", 50*time.Second, func() error {
					if application.BillingSync == nil {
						return nil
					}
					runs := application.BillingSync.RunDue(ctx, time.Now().UTC())
					log.Printf("billing_sync: ran %d due connectors", len(runs))
					return nil
				})
			})
		}
	}
}

// runSubscriptionExpirer sweeps expired subscriptions into the terminal state.
// It runs once at startup and then hourly, guarded by the Redis lease.
func runSubscriptionExpirer(ctx context.Context, e *subscriptions.Expirer, redis *goredis.Client) {
	runCycle := func() {
		runSafely("subscription_expirer", func() error {
			return withLease(ctx, "subscription_expirer", redis, "worker:lease:subscription_expirer", 5*time.Minute, func() error {
				n, err := e.Run(ctx)
				if err != nil {
					return err
				}
				if n > 0 {
					log.Printf("subscription_expirer: expired %d subscriptions", n)
				}
				return nil
			})
		})
	}
	runCycle()
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			runCycle()
		}
	}
}

// runSubscriptionRenewer auto-renews opted-in subscriptions from wallet
// balance shortly before expiry. Runs hourly with the Redis lease.
func runSubscriptionRenewer(ctx context.Context, r *subscriptions.Renewer, redis *goredis.Client) {
	runCycle := func() {
		runSafely("subscription_renewer", func() error {
			return withLease(ctx, "subscription_renewer", redis, "worker:lease:subscription_renewer", 5*time.Minute, func() error {
				n, err := r.Run(ctx)
				if err != nil {
					return err
				}
				if n > 0 {
					log.Printf("subscription_renewer: renewed %d subscriptions", n)
				}
				return nil
			})
		})
	}
	runCycle()
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			runCycle()
		}
	}
}
