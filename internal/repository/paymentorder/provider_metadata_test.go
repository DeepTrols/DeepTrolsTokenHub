package paymentorder

// TH-P1-05 — provider query/retry/review metadata tests.
//
// AC-01: a newly created order row carries channel, method, order number,
// amount, expiry and pay URL.
// AC-02: migration adds nullable retry fields and a down migration removes
// them again (up/down round-trip).
// AC-03: rows that predate the metadata columns (or carry explicit NULLs)
// stay readable through find/list flows with nil metadata and no panic.

import (
	"context"
	"testing"
	"time"

	"github.com/deeptrols/api/internal/repository/testutil"
	"github.com/deeptrols/api/migrations"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
)

const (
	metadataUpMigration   = "000037_payment_order_provider_metadata.up.sql"
	metadataDownMigration = "000037_payment_order_provider_metadata.down.sql"
)

// paymentOrderColumns reports the payment_orders column names visible in the
// package's private test schema.
func paymentOrderColumns(t *testing.T, pool *pgxpool.Pool) []string {
	t.Helper()
	rows, err := pool.Query(context.Background(),
		`SELECT column_name FROM information_schema.columns
		 WHERE table_schema = current_schema() AND table_name = 'payment_orders'
		 ORDER BY column_name`)
	if err != nil {
		t.Fatalf("query columns: %v", err)
	}
	defer rows.Close()
	var cols []string
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			t.Fatalf("scan column: %v", err)
		}
		cols = append(cols, c)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
	return cols
}

func hasColumn(cols []string, name string) bool {
	for _, c := range cols {
		if c == name {
			return true
		}
	}
	return false
}

var metadataColumns = []string{"query_attempts", "last_query_at", "next_retry_at", "review_reason"}

// TestMigrationProviderMetadataUpDownRoundTrip covers AC-02: the harness
// applies every up migration during provisioning, so the columns must exist;
// the down migration must drop exactly those columns; re-applying the up
// migration restores them (and leaves the shared schema intact for the rest
// of the package).
func TestMigrationProviderMetadataUpDownRoundTrip(t *testing.T) {
	pool := testutil.SetupPool(t)
	if pool == nil {
		return // SetupPool skipped
	}
	ctx := context.Background()

	cols := paymentOrderColumns(t, pool)
	for _, want := range metadataColumns {
		if !hasColumn(cols, want) {
			t.Fatalf("expected column %q after up migration, columns: %v", want, cols)
		}
	}

	down, err := migrations.Files.ReadFile(metadataDownMigration)
	if err != nil {
		t.Fatalf("read down migration: %v", err)
	}
	if _, err := pool.Exec(ctx, string(down)); err != nil {
		t.Fatalf("apply down migration: %v", err)
	}
	cols = paymentOrderColumns(t, pool)
	for _, gone := range metadataColumns {
		if hasColumn(cols, gone) {
			t.Fatalf("down migration left column %q behind", gone)
		}
	}
	if !hasColumn(cols, "order_no") || !hasColumn(cols, "pay_url") {
		t.Fatalf("down migration damaged pre-existing columns: %v", cols)
	}

	up, err := migrations.Files.ReadFile(metadataUpMigration)
	if err != nil {
		t.Fatalf("read up migration: %v", err)
	}
	if _, err := pool.Exec(ctx, string(up)); err != nil {
		t.Fatalf("re-apply up migration: %v", err)
	}
	cols = paymentOrderColumns(t, pool)
	for _, want := range metadataColumns {
		if !hasColumn(cols, want) {
			t.Fatalf("expected column %q after re-applying up migration", want)
		}
	}
}

// seedMetadataUser inserts a user row and returns its id.
func seedMetadataUser(t *testing.T, pool *pgxpool.Pool) uuid.UUID {
	t.Helper()
	userID := uuid.New()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO users (id, email, password_hash) VALUES ($1,$2,$3)`,
		userID, userID.String()+"@t.com", "hash")
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return userID
}

// TestCreateNewOrderRowCarriesCoreFields covers AC-01: a newly created order
// persists channel, pay method, order number, amount, expiry and pay URL.
func TestCreateNewOrderRowCarriesCoreFields(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	if repo.pool == nil {
		return // SetupPool skipped
	}
	ctx := context.Background()
	testutil.TruncateTables(t, repo.pool, "payment_orders", "users")
	userID := seedMetadataUser(t, repo.pool)

	payURL := "https://pay.example/meta?sign=x"
	expires := time.Now().Add(time.Hour).Truncate(time.Second)
	o := &Order{
		ID: uuid.New(), OrderNo: "DTPMETA1", UserID: userID, Amount: decimal.NewFromInt(88),
		Currency: "CNY", Channel: "epay", PayMethod: "alipay", Status: StatusPending,
		PayURL: &payURL, ExpiresAt: expires,
	}
	if err := repo.Create(ctx, o); err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := repo.FindByOrderNo(ctx, "DTPMETA1")
	if err != nil {
		t.Fatalf("FindByOrderNo: %v", err)
	}
	if got.Channel != "epay" || got.PayMethod != "alipay" || got.OrderNo != "DTPMETA1" {
		t.Fatalf("core identity not persisted: %+v", got)
	}
	if got.Amount.String() != "88" {
		t.Fatalf("amount not persisted: %v", got.Amount)
	}
	if !got.ExpiresAt.Equal(expires) {
		t.Fatalf("expiry not persisted: %v != %v", got.ExpiresAt, expires)
	}
	if got.PayURL == nil || *got.PayURL != payURL {
		t.Fatalf("pay_url not persisted: %+v", got.PayURL)
	}
	// A brand-new order has never been queried or reviewed: metadata is nil,
	// never zero-valued garbage.
	if got.QueryAttempts != nil || got.LastQueryAt != nil || got.NextRetryAt != nil || got.ReviewReason != nil {
		t.Fatalf("new order must carry nil provider metadata: %+v", got)
	}
}

// seedLegacyOrder inserts a row using ONLY the pre-TH-P1-05 column set,
// simulating an order created before the metadata migration ran.
func seedLegacyOrder(t *testing.T, repo *PostgresRepository, orderNo string, userID uuid.UUID) {
	t.Helper()
	_, err := repo.pool.Exec(context.Background(),
		`INSERT INTO payment_orders
			(order_no, user_id, amount, currency, purpose, channel, pay_method, status, expires_at)
		 VALUES ($1,$2,$3,'CNY','topup','epay','alipay','pending', NOW() + interval '1 hour')`,
		orderNo, userID, decimal.NewFromInt(30))
	if err != nil {
		t.Fatalf("seed legacy order: %v", err)
	}
}

// TestLegacyRowWithoutMetadataRemainsReadable covers AC-03 (regression):
// rows created before the metadata migration read back with nil metadata
// through every find/list path, no panic, no error.
func TestLegacyRowWithoutMetadataRemainsReadable(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	if repo.pool == nil {
		return // SetupPool skipped
	}
	ctx := context.Background()
	testutil.TruncateTables(t, repo.pool, "payment_orders", "users")
	userID := seedMetadataUser(t, repo.pool)
	seedLegacyOrder(t, repo, "DTPLEGACY1", userID)

	byNo, err := repo.FindByOrderNo(ctx, "DTPLEGACY1")
	if err != nil {
		t.Fatalf("FindByOrderNo: %v", err)
	}
	assertNilMetadata(t, "FindByOrderNo", byNo)

	byID, err := repo.FindByID(ctx, byNo.ID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	assertNilMetadata(t, "FindByID", byID)

	userOrders, err := repo.ListByUser(ctx, userID, 10, 0)
	if err != nil {
		t.Fatalf("ListByUser: %v", err)
	}
	if len(userOrders) != 1 {
		t.Fatalf("ListByUser: expected 1 order, got %d", len(userOrders))
	}
	assertNilMetadata(t, "ListByUser", &userOrders[0])

	all, err := repo.List(ctx, 10, 0, nil, nil)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("List: expected 1 order, got %d", len(all))
	}
	assertNilMetadata(t, "List", &all[0])
}

// TestNullMetadataFailureInjection covers the AC-03 failure-injection path:
// explicit NULLs in every metadata column (not merely absent defaults) must
// scan into nil pointers without error.
func TestNullMetadataFailureInjection(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	if repo.pool == nil {
		return // SetupPool skipped
	}
	ctx := context.Background()
	testutil.TruncateTables(t, repo.pool, "payment_orders", "users")
	userID := seedMetadataUser(t, repo.pool)

	_, err := repo.pool.Exec(ctx,
		`INSERT INTO payment_orders
			(order_no, user_id, amount, currency, purpose, channel, pay_method, status, expires_at,
			 query_attempts, last_query_at, next_retry_at, review_reason)
		 VALUES ($1,$2,$3,'CNY','topup','epay','alipay','pending', NOW() + interval '1 hour',
			 NULL, NULL, NULL, NULL)`,
		"DTPNULLMETA1", userID, decimal.NewFromInt(12))
	if err != nil {
		t.Fatalf("insert null-metadata order: %v", err)
	}
	got, err := repo.FindByOrderNo(ctx, "DTPNULLMETA1")
	if err != nil {
		t.Fatalf("FindByOrderNo: %v", err)
	}
	assertNilMetadata(t, "null-injection", got)
}

func assertNilMetadata(t *testing.T, path string, o *Order) {
	t.Helper()
	if o.QueryAttempts != nil {
		t.Fatalf("%s: expected nil query_attempts, got %d", path, *o.QueryAttempts)
	}
	if o.LastQueryAt != nil {
		t.Fatalf("%s: expected nil last_query_at, got %v", path, *o.LastQueryAt)
	}
	if o.NextRetryAt != nil {
		t.Fatalf("%s: expected nil next_retry_at, got %v", path, *o.NextRetryAt)
	}
	if o.ReviewReason != nil {
		t.Fatalf("%s: expected nil review_reason, got %q", path, *o.ReviewReason)
	}
}

// TestRecordProviderQueryTracksRetryMetadata verifies the query/compensation
// write path: each provider query attempt increments the counter, stamps
// last_query_at and schedules (or clears) next_retry_at without touching
// status or amount.
func TestRecordProviderQueryTracksRetryMetadata(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	if repo.pool == nil {
		return // SetupPool skipped
	}
	ctx := context.Background()
	testutil.TruncateTables(t, repo.pool, "payment_orders", "users")
	userID := seedMetadataUser(t, repo.pool)

	o := &Order{
		ID: uuid.New(), OrderNo: "DTPRETRY1", UserID: userID, Amount: decimal.NewFromInt(20),
		Currency: "CNY", Channel: "epay", PayMethod: "alipay", Status: StatusPending,
		ExpiresAt: time.Now().Add(time.Hour),
	}
	if err := repo.Create(ctx, o); err != nil {
		t.Fatalf("Create: %v", err)
	}
	created, err := repo.FindByOrderNo(ctx, "DTPRETRY1")
	if err != nil {
		t.Fatalf("FindByOrderNo: %v", err)
	}

	next := time.Now().Add(5 * time.Minute).Truncate(time.Second)
	if err := repo.RecordProviderQuery(ctx, created.ID, &next); err != nil {
		t.Fatalf("RecordProviderQuery: %v", err)
	}
	got, err := repo.FindByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if got.QueryAttempts == nil || *got.QueryAttempts != 1 {
		t.Fatalf("expected query_attempts=1, got %+v", got.QueryAttempts)
	}
	if got.LastQueryAt == nil {
		t.Fatal("expected last_query_at to be stamped")
	}
	if got.NextRetryAt == nil || !got.NextRetryAt.Equal(next) {
		t.Fatalf("expected next_retry_at=%v, got %+v", next, got.NextRetryAt)
	}
	if got.Status != StatusPending || got.Amount.String() != "20" {
		t.Fatalf("retry metadata must not change status/amount: %+v", got)
	}

	if err := repo.RecordProviderQuery(ctx, created.ID, &next); err != nil {
		t.Fatalf("RecordProviderQuery 2: %v", err)
	}
	got, err = repo.FindByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("FindByID 2: %v", err)
	}
	if got.QueryAttempts == nil || *got.QueryAttempts != 2 {
		t.Fatalf("expected query_attempts=2, got %+v", got.QueryAttempts)
	}

	// nil clears the retry schedule (order no longer due for a query).
	if err := repo.RecordProviderQuery(ctx, created.ID, nil); err != nil {
		t.Fatalf("RecordProviderQuery clear: %v", err)
	}
	got, err = repo.FindByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("FindByID 3: %v", err)
	}
	if got.NextRetryAt != nil {
		t.Fatalf("expected next_retry_at cleared, got %v", *got.NextRetryAt)
	}
	if got.QueryAttempts == nil || *got.QueryAttempts != 3 {
		t.Fatalf("expected query_attempts=3, got %+v", got.QueryAttempts)
	}

	// Unknown order fails closed with ErrNotFound.
	if err := repo.RecordProviderQuery(ctx, uuid.New(), &next); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound for missing order, got %v", err)
	}
}

// TestSetReviewReasonFlagsOrder verifies the manual-review write path: the
// reason is stored for reconciliation/review tracking, empty clears it, and
// neither path mutates status.
func TestSetReviewReasonFlagsOrder(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	if repo.pool == nil {
		return // SetupPool skipped
	}
	ctx := context.Background()
	testutil.TruncateTables(t, repo.pool, "payment_orders", "users")
	userID := seedMetadataUser(t, repo.pool)

	o := &Order{
		ID: uuid.New(), OrderNo: "DTPREVIEW1", UserID: userID, Amount: decimal.NewFromInt(15),
		Currency: "CNY", Channel: "epay", PayMethod: "alipay", Status: StatusPending,
		ExpiresAt: time.Now().Add(time.Hour),
	}
	if err := repo.Create(ctx, o); err != nil {
		t.Fatalf("Create: %v", err)
	}
	created, err := repo.FindByOrderNo(ctx, "DTPREVIEW1")
	if err != nil {
		t.Fatalf("FindByOrderNo: %v", err)
	}

	if err := repo.SetReviewReason(ctx, created.ID, "amount_mismatch"); err != nil {
		t.Fatalf("SetReviewReason: %v", err)
	}
	got, err := repo.FindByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if got.ReviewReason == nil || *got.ReviewReason != "amount_mismatch" {
		t.Fatalf("expected review_reason=amount_mismatch, got %+v", got.ReviewReason)
	}
	if got.Status != StatusPending {
		t.Fatalf("review flag must not change status: %+v", got)
	}

	if err := repo.SetReviewReason(ctx, created.ID, ""); err != nil {
		t.Fatalf("SetReviewReason clear: %v", err)
	}
	got, err = repo.FindByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("FindByID 2: %v", err)
	}
	if got.ReviewReason != nil {
		t.Fatalf("expected review_reason cleared, got %q", *got.ReviewReason)
	}

	if err := repo.SetReviewReason(ctx, uuid.New(), "x"); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound for missing order, got %v", err)
	}
}
