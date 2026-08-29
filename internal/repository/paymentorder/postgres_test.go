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
