package paymentorder

import (
	"context"
	"testing"
	"time"

	"github.com/deeptrols/api/internal/repository/testutil"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

func TestCreateMarkPaid(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	if repo.pool == nil {
		return // SetupPool skipped
	}
	ctx := context.Background()
	testutil.TruncateTables(t, repo.pool, "payment_orders", "users")

	userID := uuid.New()
	_, err := repo.pool.Exec(ctx, `INSERT INTO users (id, email, password_hash) VALUES ($1,$2,$3)`,
		userID, userID.String()+"@t.com", "hash")
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	payURL := "https://pay/u"
	o := &Order{
		ID: uuid.New(), OrderNo: "DTPTEST1", UserID: userID, Amount: decimal.NewFromInt(50),
		Currency: "CNY", Channel: "epay", PayMethod: "alipay", Status: StatusPending,
		PayURL: &payURL, ExpiresAt: time.Now().Add(time.Minute),
	}
	if err := repo.Create(ctx, o); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.FindByOrderNo(ctx, "DTPTEST1")
	if err != nil {
		t.Fatalf("FindByOrderNo: %v", err)
	}
	if got.Status != StatusPending || got.Amount.String() != "50" {
		t.Fatalf("unexpected order: %+v", got)
	}

	applied, err := repo.MarkPaid(ctx, got.ID, "GATEWAY1", []byte(`{"ok":true}`))
	if err != nil {
		t.Fatalf("MarkPaid: %v", err)
	}
	if !applied {
		t.Fatal("expected MarkPaid to apply")
	}
	// second MarkPaid is a no-op
	applied2, err := repo.MarkPaid(ctx, got.ID, "GATEWAY1", []byte(`{}`))
	if err != nil {
		t.Fatalf("MarkPaid2: %v", err)
	}
	if applied2 {
		t.Fatal("expected second MarkPaid to be no-op")
	}

	paid, err := repo.FindByID(ctx, got.ID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if paid.Status != StatusPaid || paid.GatewayTradeNo == nil || *paid.GatewayTradeNo != "GATEWAY1" {
		t.Fatalf("order not marked paid: %+v", paid)
	}
}

// TestCreatePersistsPayURL verifies TH-P05-10 AC-01 at the persistence layer:
// Create must write pay_url so pending orders stay actionable after refresh
// (find/list must return the exact stored URL).
func TestCreatePersistsPayURL_FindAndListReturnURL(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	if repo.pool == nil {
		return // SetupPool skipped
	}
	ctx := context.Background()
	testutil.TruncateTables(t, repo.pool, "payment_orders", "users")

	userID := uuid.New()
	_, err := repo.pool.Exec(ctx, `INSERT INTO users (id, email, password_hash) VALUES ($1,$2,$3)`,
		userID, userID.String()+"@t.com", "hash")
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	payURL := "https://pay.example/o1?sign=abc"
	o := &Order{
		ID: uuid.New(), OrderNo: "DTPURL1", UserID: userID, Amount: decimal.NewFromInt(10),
		Currency: "CNY", Channel: "epay", PayMethod: "alipay", Status: StatusPending,
		PayURL: &payURL, ExpiresAt: time.Now().Add(time.Minute),
	}
	if err := repo.Create(ctx, o); err != nil {
		t.Fatalf("Create: %v", err)
	}

	byNo, err := repo.FindByOrderNo(ctx, "DTPURL1")
	if err != nil {
		t.Fatalf("FindByOrderNo: %v", err)
	}
	if byNo.PayURL == nil || *byNo.PayURL != payURL {
		t.Fatalf("pay_url not persisted via FindByOrderNo: got %+v", byNo.PayURL)
	}
	byID, err := repo.FindByID(ctx, byNo.ID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if byID.PayURL == nil || *byID.PayURL != payURL {
		t.Fatalf("pay_url not persisted via FindByID: got %+v", byID.PayURL)
	}
	userOrders, err := repo.ListByUser(ctx, userID, 10, 0)
	if err != nil {
		t.Fatalf("ListByUser: %v", err)
	}
	if len(userOrders) != 1 || userOrders[0].PayURL == nil || *userOrders[0].PayURL != payURL {
		t.Fatalf("pay_url not persisted via ListByUser: got %+v", userOrders)
	}
	all, err := repo.List(ctx, 10, 0, nil, nil)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 1 || all[0].PayURL == nil || *all[0].PayURL != payURL {
		t.Fatalf("pay_url not persisted via List: got %+v", all)
	}
}

// TestCreateWithoutPayURL_ReadsBackNull covers TH-P05-10 AC-03 at the
// persistence layer: rows without pay_url (legacy / URL-less channels) read
// back as nil without error.
func TestCreateWithoutPayURL_ReadsBackNull(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	if repo.pool == nil {
		return // SetupPool skipped
	}
	ctx := context.Background()
	testutil.TruncateTables(t, repo.pool, "payment_orders", "users")

	userID := uuid.New()
	_, err := repo.pool.Exec(ctx, `INSERT INTO users (id, email, password_hash) VALUES ($1,$2,$3)`,
		userID, userID.String()+"@t.com", "hash")
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	o := &Order{
		ID: uuid.New(), OrderNo: "DTPNULL1", UserID: userID, Amount: decimal.NewFromInt(5),
		Currency: "CNY", Channel: "epay", PayMethod: "alipay", Status: StatusPending,
		ExpiresAt: time.Now().Add(time.Minute),
	}
	if err := repo.Create(ctx, o); err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := repo.FindByOrderNo(ctx, "DTPNULL1")
	if err != nil {
		t.Fatalf("FindByOrderNo: %v", err)
	}
	if got.PayURL != nil {
		t.Fatalf("expected nil pay_url, got %q", *got.PayURL)
	}
}
