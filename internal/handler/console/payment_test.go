package console

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/deeptrols/api/internal/repository/paymentorder"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

func pendingOrderWithURL(payURL string) paymentorder.Order {
	return paymentorder.Order{
		ID: uuid.New(), OrderNo: "DTPDTO1", UserID: uuid.New(),
		Amount: decimal.NewFromInt(10), Currency: "CNY", Channel: "epay",
		PayMethod: "alipay", Status: paymentorder.StatusPending,
		PayURL: &payURL, ExpiresAt: time.Now().Add(time.Minute),
	}
}

// TestToOrderDTO_PendingShowsPayURL verifies TH-P05-10 AC-01 at the mapping
// layer: while the order is pending, the list DTO exposes the stored pay URL
// so a page refresh keeps the payment action available.
func TestToOrderDTO_PendingShowsPayURL(t *testing.T) {
	dto := toOrderDTO(pendingOrderWithURL("https://pay.example/o1"))
	if dto.PayURL != "https://pay.example/o1" {
		t.Fatalf("pending order must expose pay_url, got %q", dto.PayURL)
	}
}

// TestToOrderDTO_PaidHidesPayURL verifies TH-P05-10 AC-02: once the order is
// paid the list DTO must not return a usable payment URL.
func TestToOrderDTO_PaidHidesPayURL(t *testing.T) {
	o := pendingOrderWithURL("https://pay.example/o1")
	o.Status = paymentorder.StatusPaid
	now := time.Now()
	o.PaidAt = &now
	if dto := toOrderDTO(o); dto.PayURL != "" {
		t.Fatalf("paid order must not expose pay_url, got %q", dto.PayURL)
	}
}

// TestToOrderDTO_ClosedAndRefundedHidePayURL: non-pending terminal states
// never expose an actionable URL.
func TestToOrderDTO_ClosedAndRefundedHidePayURL(t *testing.T) {
	for _, status := range []string{paymentorder.StatusClosed, paymentorder.StatusRefunded} {
		o := pendingOrderWithURL("https://pay.example/o1")
		o.Status = status
		if dto := toOrderDTO(o); dto.PayURL != "" {
			t.Fatalf("status %s must not expose pay_url, got %q", status, dto.PayURL)
		}
	}
}

// TestToOrderDTO_NullPayURL_OmitsField verifies TH-P05-10 AC-03: legacy rows
// with NULL pay_url still marshal to valid JSON and omit the field.
func TestToOrderDTO_NullPayURL_OmitsField(t *testing.T) {
	o := pendingOrderWithURL("x")
	o.PayURL = nil
	dto := toOrderDTO(o)
	if dto.PayURL != "" {
		t.Fatalf("nil pay_url must map to empty, got %q", dto.PayURL)
	}
	b, err := json.Marshal(dto)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back map[string]any
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	if _, present := back["pay_url"]; present {
		t.Fatalf("pay_url must be omitted for nil rows, got %s", b)
	}
	if back["status"] != "pending" {
		t.Fatalf("status missing from DTO JSON: %s", b)
	}
}
