package payment

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/deeptrols/api/internal/domain"
	"github.com/deeptrols/api/internal/repository/paymentorder"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

type fakeSettings struct {
	data map[string]json.RawMessage
}

func (f *fakeSettings) All(ctx context.Context) (map[string]json.RawMessage, error) {
	return f.data, nil
}

func enabledSettings() *fakeSettings {
	return &fakeSettings{data: map[string]json.RawMessage{
		"payment_enabled":              json.RawMessage(`true`),
		"payment_compliance_confirmed": json.RawMessage(`true`),
		"pay_address":                  json.RawMessage(`"https://pay.example.com"`),
		"epay_id":                      json.RawMessage(`"1001"`),
		"epay_key":                     json.RawMessage(`"k123"`),
		"min_topup":                    json.RawMessage(`"1"`),
		"max_topup":                    json.RawMessage(`"1000000"`),
		"amount_options":               json.RawMessage(`[10,50,100]`),
	}}
}

type fakeGateway struct {
	payURL    string
	notify    *NotifyResult
	verifyErr error
}

func (g *fakeGateway) Name() string { return "epay" }
func (g *fakeGateway) CreateOrder(ctx context.Context, req CreateOrderRequest) (*CreateOrderResult, error) {
	if g.payURL == "" {
		return nil, errors.New("gateway down")
	}
	return &CreateOrderResult{PayURL: g.payURL}, nil
}
func (g *fakeGateway) VerifyNotify(ctx context.Context, params map[string]string) (*NotifyResult, error) {
	if g.verifyErr != nil {
		return nil, g.verifyErr
	}
	return g.notify, nil
}

type fakeOrders struct {
	byNo map[string]*paymentorder.Order
	byID map[uuid.UUID]*paymentorder.Order
}

func newFakeOrders() *fakeOrders {
	return &fakeOrders{byNo: map[string]*paymentorder.Order{}, byID: map[uuid.UUID]*paymentorder.Order{}}
}

func (f *fakeOrders) Create(ctx context.Context, o *paymentorder.Order) error {
	o.ID = uuid.New()
	f.byNo[o.OrderNo] = o
	f.byID[o.ID] = o
	return nil
}
func (f *fakeOrders) FindByOrderNo(ctx context.Context, no string) (*paymentorder.Order, error) {
	if o, ok := f.byNo[no]; ok {
		return o, nil
	}
	return nil, paymentorder.ErrNotFound
}
func (f *fakeOrders) FindByID(ctx context.Context, id uuid.UUID) (*paymentorder.Order, error) {
	if o, ok := f.byID[id]; ok {
		return o, nil
	}
	return nil, paymentorder.ErrNotFound
}
func (f *fakeOrders) ListByUser(ctx context.Context, u uuid.UUID, l, o int) ([]paymentorder.Order, error) {
	return nil, nil
}
func (f *fakeOrders) List(ctx context.Context, l, o int, s *string, u *uuid.UUID) ([]paymentorder.Order, error) {
	return nil, nil
}
func (f *fakeOrders) MarkPaid(ctx context.Context, id uuid.UUID, gtn string, raw []byte) (bool, error) {
	ord := f.byID[id]
	if ord.Status == paymentorder.StatusPending {
		ord.Status = paymentorder.StatusPaid
		gt := gtn
		ord.GatewayTradeNo = &gt
		return true, nil
	}
	return false, nil
}
func (f *fakeOrders) MarkClosed(ctx context.Context, id uuid.UUID) error {
	f.byID[id].Status = paymentorder.StatusClosed
	return nil
}

type fakeWallets struct {
	walletID   uuid.UUID
	credited   map[string]decimal.Decimal
	topupCount int
}

func (f *fakeWallets) FindByUser(ctx context.Context, u uuid.UUID, t *uuid.UUID) (*domain.Wallet, error) {
	return &domain.Wallet{ID: f.walletID}, nil
}
func (f *fakeWallets) TopUp(ctx context.Context, walletID uuid.UUID, amount decimal.Decimal, key string) (*domain.WalletTransaction, error) {
	if _, ok := f.credited[key]; !ok {
		f.credited[key] = amount
		f.topupCount++
	}
	return &domain.WalletTransaction{ID: uuid.New()}, nil
}

func newTestService(settings *fakeSettings, gw Gateway) (*Service, *fakeOrders, *fakeWallets) {
	orders := newFakeOrders()
	wallets := &fakeWallets{walletID: uuid.New(), credited: map[string]decimal.Decimal{}}
	s := NewService(orders, wallets, settings)
	s.newGateway = func(*paymentConfig) Gateway { return gw }
	s.now = func() time.Time { return time.Unix(1700000000, 0) }
	return s, orders, wallets
}

func TestInfoListsPayMethods(t *testing.T) {
	s, _, _ := newTestService(enabledSettings(), &fakeGateway{payURL: "u"})
	info, err := s.Info(context.Background())
	if err != nil {
		t.Fatalf("Info: %v", err)
	}
	if !info.Enabled || len(info.PayMethods) != 2 {
		t.Fatalf("expected 2 enabled methods, got %+v", info)
	}
}

func TestCreateOrderPersists(t *testing.T) {
	s, orders, _ := newTestService(enabledSettings(), &fakeGateway{payURL: "https://pay/u"})
	res, err := s.CreateOrder(context.Background(), uuid.New(), decimal.NewFromInt(50), "alipay")
	if err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}
	if res.PayURL != "https://pay/u" || res.OrderNo == "" {
		t.Fatalf("unexpected response: %+v", res)
	}
	ord, err := orders.FindByOrderNo(context.Background(), res.OrderNo)
	if err != nil {
		t.Fatalf("find order: %v", err)
	}
	if ord.Status != paymentorder.StatusPending || ord.PayURL == nil {
		t.Fatalf("order not persisted correctly: %+v", ord)
	}
}

func TestCreateSubscriptionOrderPersists(t *testing.T) {
	s, orders, _ := newTestService(enabledSettings(), &fakeGateway{payURL: "https://pay/u"})
	planID := uuid.New()
	res, err := s.CreateSubscriptionOrder(context.Background(), uuid.New(), planID, decimal.NewFromInt(50), "alipay")
	if err != nil {
		t.Fatalf("CreateSubscriptionOrder: %v", err)
	}
	if res.PayURL != "https://pay/u" {
		t.Fatalf("pay url = %q", res.PayURL)
	}
	o := orders.byNo[res.OrderNo]
	if o == nil {
		t.Fatal("order not persisted")
	}
	if o.Purpose != "subscription" || o.PlanID == nil || *o.PlanID != planID {
		t.Fatalf("purpose/plan = %q/%v", o.Purpose, o.PlanID)
	}
}

func TestCreditSubscriptionCallsActivator(t *testing.T) {
	s, orders, _ := newTestService(enabledSettings(), &fakeGateway{payURL: "u"})
	userID := uuid.New()
	planID := uuid.New()
	res, err := s.CreateSubscriptionOrder(context.Background(), userID, planID, decimal.NewFromInt(50), "alipay")
	if err != nil {
		t.Fatalf("CreateSubscriptionOrder: %v", err)
	}

	activated := false
	s.ActivateSubscription = func(ctx context.Context, uid, pid uuid.UUID) (time.Time, error) {
		activated = true
		if uid != userID || pid != planID {
			t.Fatalf("activator got %s/%s", uid, pid)
		}
		return time.Now(), nil
	}
	if err := s.credit(context.Background(), orders.byNo[res.OrderNo]); err != nil {
		t.Fatalf("credit: %v", err)
	}
	if !activated {
		t.Fatal("expected subscription activator to run")
	}
}

func TestCreditSubscriptionWithoutActivatorFails(t *testing.T) {
	s, orders, _ := newTestService(enabledSettings(), &fakeGateway{payURL: "u"})
	res, err := s.CreateSubscriptionOrder(context.Background(), uuid.New(), uuid.New(), decimal.NewFromInt(50), "alipay")
	if err != nil {
		t.Fatalf("CreateSubscriptionOrder: %v", err)
	}
	if err := s.credit(context.Background(), orders.byNo[res.OrderNo]); err == nil {
		t.Fatal("expected error without activator")
	}
}

func TestCreateOrderDisabled(t *testing.T) {
	s, _, _ := newTestService(&fakeSettings{data: map[string]json.RawMessage{"payment_enabled": json.RawMessage(`false`)}}, &fakeGateway{payURL: "u"})
	_, err := s.CreateOrder(context.Background(), uuid.New(), decimal.NewFromInt(50), "alipay")
	if !errors.Is(err, ErrPaymentDisabled) {
		t.Fatalf("expected ErrPaymentDisabled, got %v", err)
	}
}

func TestHandleNotifyCreditsIdempotently(t *testing.T) {
	settings := enabledSettings()
	s, orders, wallets := newTestService(settings, &fakeGateway{
		notify: &NotifyResult{OrderNo: "DTP1", GatewayTradeNo: "G1", Amount: decimal.NewFromInt(50), Success: true},
	})
	userID := uuid.New()
	// manually seed a pending order
	orders.byNo["DTP1"] = &paymentorder.Order{ID: uuid.New(), OrderNo: "DTP1", UserID: userID, Amount: decimal.NewFromInt(50), Currency: "CNY", Channel: "epay", PayMethod: "alipay", Status: paymentorder.StatusPending, ExpiresAt: time.Now().Add(time.Minute)}
	orders.byID[orders.byNo["DTP1"].ID] = orders.byNo["DTP1"]

	handled, err := s.HandleNotify(context.Background(), map[string]string{"out_trade_no": "DTP1", "money": "50.00", "trade_no": "G1", "trade_status": "TRADE_SUCCESS"})
	if err != nil {
		t.Fatalf("HandleNotify: %v", err)
	}
	if !handled {
		t.Fatal("expected handled=true on first notify")
	}
	if wallets.topupCount != 1 {
		t.Fatalf("expected exactly 1 credit, got %d", wallets.topupCount)
	}

	// replay -> no double credit
	handled2, err := s.HandleNotify(context.Background(), map[string]string{"out_trade_no": "DTP1"})
	if err != nil {
		t.Fatalf("replay HandleNotify: %v", err)
	}
	if handled2 {
		t.Fatal("expected handled=false on replay")
	}
	if wallets.topupCount != 1 {
		t.Fatalf("expected still 1 credit after replay, got %d", wallets.topupCount)
	}
}

func TestHandleNotifyAmountMismatch(t *testing.T) {
	s, orders, _ := newTestService(enabledSettings(), &fakeGateway{
		notify: &NotifyResult{OrderNo: "DTP2", GatewayTradeNo: "G2", Amount: decimal.NewFromInt(99), Success: true},
	})
	o := &paymentorder.Order{ID: uuid.New(), OrderNo: "DTP2", UserID: uuid.New(), Amount: decimal.NewFromInt(50), Currency: "CNY", Channel: "epay", PayMethod: "alipay", Status: paymentorder.StatusPending, ExpiresAt: time.Now().Add(time.Minute)}
	orders.byNo["DTP2"] = o
	orders.byID[o.ID] = o
	_, err := s.HandleNotify(context.Background(), map[string]string{"out_trade_no": "DTP2"})
	if !errors.Is(err, ErrAmountMismatch) {
		t.Fatalf("expected ErrAmountMismatch, got %v", err)
	}
}
