package payment

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/deeptrols/api/internal/domain"
	"github.com/deeptrols/api/internal/repository/paymentorder"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

const orderTTL = 30 * time.Minute

// settingReader is the slice of the settings service the payment service needs.
type settingReader interface {
	All(ctx context.Context) (map[string]json.RawMessage, error)
}

// walletRepo is the wallet slice the payment service needs for idempotent credit.
type walletRepo interface {
	FindByUser(ctx context.Context, userID uuid.UUID, tenantID *uuid.UUID) (*domain.Wallet, error)
	TopUp(ctx context.Context, walletID uuid.UUID, amount decimal.Decimal, idempotencyKey string) (*domain.WalletTransaction, error)
}

// Service creates payment orders and settles them idempotently via wallet.
type Service struct {
	orders     paymentorder.Repository
	wallets    walletRepo
	settings   settingReader
	newGateway func(cfg *paymentConfig) Gateway
	now        func() time.Time
	// ActivateSubscription settles a paid subscription order (nil = disabled).
	ActivateSubscription func(ctx context.Context, userID, planID uuid.UUID) (time.Time, error)
}

// NewService creates a payment Service.
func NewService(orders paymentorder.Repository, wallets walletRepo, settings settingReader) *Service {
	return &Service{
		orders:   orders,
		wallets:  wallets,
		settings: settings,
		newGateway: func(cfg *paymentConfig) Gateway {
			return &EpayGateway{
				PayAddress: cfg.PayAddress,
				PartnerID:  cfg.EpayID,
				Key:        cfg.EpayKey,
				NotifyURL:  cfg.CallbackBase + "/api/payment/notify/epay",
				ReturnURL:  cfg.CallbackBase + "/recharge",
			}
		},
		now: time.Now,
	}
}

type paymentConfig struct {
	Enabled       bool
	Compliance    bool
	PayAddress    string
	EpayID        string
	EpayKey       string
	MinTopup      decimal.Decimal
	MaxTopup      decimal.Decimal
	AmountOptions []string
	CallbackBase  string
	Channel       string
}

func (s *Service) config(ctx context.Context) (*paymentConfig, error) {
	all, err := s.settings.All(ctx)
	if err != nil {
		return nil, err
	}
	return &paymentConfig{
		Enabled:       rawBool(all, "payment_enabled"),
		Compliance:    rawBool(all, "payment_compliance_confirmed"),
		PayAddress:    rawStr(all, "pay_address"),
		EpayID:        rawStr(all, "epay_id"),
		EpayKey:       rawStr(all, "epay_key"),
		MinTopup:      rawDecimal(all, "min_topup", decimal.NewFromInt(1)),
		MaxTopup:      rawDecimal(all, "max_topup", decimal.NewFromInt(1000000)),
		AmountOptions: rawArray(all, "amount_options"),
		CallbackBase:  rawStr(all, "callback_base_url"),
		Channel:       rawStr(all, "payment_channel"),
	}, nil
}

func (s *Service) gateway(cfg *paymentConfig) Gateway {
	if s.newGateway != nil {
		return s.newGateway(cfg)
	}
	return &EpayGateway{
		PayAddress: cfg.PayAddress,
		PartnerID:  cfg.EpayID,
		Key:        cfg.EpayKey,
		NotifyURL:  cfg.CallbackBase + "/api/payment/notify/epay",
		ReturnURL:  cfg.CallbackBase + "/recharge",
	}
}

// PayMethod is a user-facing payment method entry.
type PayMethod struct {
	Name  string `json:"name"`
	Type  string `json:"type"`
	Color string `json:"color"`
}

// PaymentInfo is returned to the client on /payment/methods.
type PaymentInfo struct {
	Enabled       bool        `json:"enabled"`
	Compliance    bool        `json:"payment_compliance_confirmed"`
	PayMethods    []PayMethod `json:"pay_methods"`
	MinTopup      string      `json:"min_topup"`
	MaxTopup      string      `json:"max_topup"`
	AmountOptions []string    `json:"amount_options"`
	Channel       string      `json:"channel"`
}

// Info returns the currently available payment methods and amount bounds.
func (s *Service) Info(ctx context.Context) (*PaymentInfo, error) {
	cfg, err := s.config(ctx)
	if err != nil {
		return nil, err
	}
	info := &PaymentInfo{
		Enabled:       cfg.Enabled && cfg.Compliance,
		Compliance:    cfg.Compliance,
		MinTopup:      cfg.MinTopup.StringFixed(2),
		MaxTopup:      cfg.MaxTopup.StringFixed(2),
		AmountOptions: cfg.AmountOptions,
		Channel:       cfg.Channel,
		PayMethods:    []PayMethod{},
	}
	if info.Enabled && cfg.PayAddress != "" && cfg.EpayID != "" && cfg.EpayKey != "" {
		info.PayMethods = []PayMethod{
			{Name: "支付宝", Type: "alipay", Color: "#1677FF"},
			{Name: "微信支付", Type: "wxpay", Color: "#07C160"},
		}
	}
	return info, nil
}

// CreateOrderResponse is returned after placing a recharge order.
type CreateOrderResponse struct {
	OrderNo   string `json:"order_no"`
	Amount    string `json:"amount"`
	Currency  string `json:"currency"`
	Channel   string `json:"channel"`
	PayMethod string `json:"pay_method"`
	PayURL    string `json:"pay_url"`
}

// CreateOrder validates the request, calls the gateway and records the order.
func (s *Service) CreateOrder(ctx context.Context, userID uuid.UUID, amount decimal.Decimal, payMethod string) (*CreateOrderResponse, error) {
	cfg, err := s.config(ctx)
	if err != nil {
		return nil, err
	}
	if !cfg.Enabled || !cfg.Compliance {
		return nil, ErrPaymentDisabled
	}
	if payMethod != "alipay" && payMethod != "wxpay" {
		return nil, ErrInvalidMethod
	}
	if cfg.PayAddress == "" || cfg.EpayID == "" || cfg.EpayKey == "" {
		return nil, ErrNotConfigured
	}
	if amount.LessThan(cfg.MinTopup) || amount.GreaterThan(cfg.MaxTopup) {
		return nil, ErrAmountRange
	}

	orderNo := genOrderNo()
	res, err := s.gateway(cfg).CreateOrder(ctx, CreateOrderRequest{
		OrderNo:   orderNo,
		Amount:    amount,
		PayMethod: payMethod,
		Subject:   "智曜TokenHub 平台充值 " + amount.StringFixed(2) + " 元",
		NotifyURL: cfg.CallbackBase + "/api/payment/notify/epay",
		ReturnURL: cfg.CallbackBase + "/recharge",
	})
	if err != nil {
		return nil, err
	}

	o := &paymentorder.Order{
		ID:        uuid.New(),
		OrderNo:   orderNo,
		UserID:    userID,
		Amount:    amount,
		Currency:  "CNY",
		Channel:   "epay",
		PayMethod: payMethod,
		Status:    paymentorder.StatusPending,
		PayURL:    &res.PayURL,
		ExpiresAt: s.now().Add(orderTTL),
	}
	if err := s.orders.Create(ctx, o); err != nil {
		return nil, err
	}
	return &CreateOrderResponse{
		OrderNo:   orderNo,
		Amount:    amount.StringFixed(2),
		Currency:  "CNY",
		Channel:   "epay",
		PayMethod: payMethod,
		PayURL:    res.PayURL,
	}, nil
}

// CreateSubscriptionOrder creates a payment order for a subscription plan. The
// credit path (HandleNotify / AdminComplete) activates the plan instead of
// crediting the wallet when ActivateSubscription is wired.
func (s *Service) CreateSubscriptionOrder(ctx context.Context, userID, planID uuid.UUID, amount decimal.Decimal, payMethod string) (*CreateOrderResponse, error) {
	cfg, err := s.config(ctx)
	if err != nil {
		return nil, err
	}
	if !cfg.Enabled || !cfg.Compliance {
		return nil, ErrPaymentDisabled
	}
	if payMethod != "alipay" && payMethod != "wxpay" {
		return nil, ErrInvalidMethod
	}
	if cfg.PayAddress == "" || cfg.EpayID == "" || cfg.EpayKey == "" {
		return nil, ErrNotConfigured
	}

	orderNo := genOrderNo()
	res, err := s.gateway(cfg).CreateOrder(ctx, CreateOrderRequest{
		OrderNo:   orderNo,
		Amount:    amount,
		PayMethod: payMethod,
		Subject:   "智曜TokenHub 订阅 " + amount.StringFixed(2) + " 元",
		NotifyURL: cfg.CallbackBase + "/api/payment/notify/epay",
		ReturnURL: cfg.CallbackBase + "/subscriptions",
	})
	if err != nil {
		return nil, err
	}
	o := &paymentorder.Order{
		ID:        uuid.New(),
		OrderNo:   orderNo,
		UserID:    userID,
		Amount:    amount,
		Currency:  "CNY",
		Purpose:   "subscription",
		PlanID:    &planID,
		Channel:   "epay",
		PayMethod: payMethod,
		Status:    paymentorder.StatusPending,
		PayURL:    &res.PayURL,
		ExpiresAt: s.now().Add(orderTTL),
	}
	if err := s.orders.Create(ctx, o); err != nil {
		return nil, err
	}
	return &CreateOrderResponse{
		OrderNo:   orderNo,
		Amount:    amount.StringFixed(2),
		Currency:  "CNY",
		Channel:   "epay",
		PayMethod: payMethod,
		PayURL:    res.PayURL,
	}, nil
}

// HandleNotify verifies a gateway callback and settles the order idempotently.
// It returns true when the order was settled on this call, false on a replay.
func (s *Service) HandleNotify(ctx context.Context, params map[string]string) (bool, error) {
	cfg, err := s.config(ctx)
	if err != nil {
		return false, err
	}
	notify, err := s.gateway(cfg).VerifyNotify(ctx, params)
	if err != nil {
		return false, err
	}
	if !notify.Success {
		return false, nil
	}
	order, err := s.orders.FindByOrderNo(ctx, notify.OrderNo)
	if err != nil {
		return false, err
	}
	if order.Status != paymentorder.StatusPending {
		return false, nil // already settled
	}
	if !notify.Amount.Equal(order.Amount) {
		return false, ErrAmountMismatch
	}
	if s.now().After(order.ExpiresAt) {
		_ = s.orders.MarkClosed(ctx, order.ID)
		return false, ErrOrderExpired
	}
	if err := s.credit(ctx, order); err != nil {
		return false, err
	}
	raw, _ := json.Marshal(params)
	applied, err := s.orders.MarkPaid(ctx, order.ID, notify.GatewayTradeNo, raw)
	if err != nil {
		return false, err
	}
	return applied, nil
}

// AdminComplete manually credits a pending/closed order (callback loss fallback).
func (s *Service) AdminComplete(ctx context.Context, orderID uuid.UUID) error {
	order, err := s.orders.FindByID(ctx, orderID)
	if err != nil {
		return err
	}
	if order.Status == paymentorder.StatusPaid {
		return nil
	}
	if err := s.credit(ctx, order); err != nil {
		return err
	}
	raw, _ := json.Marshal(map[string]string{"manual": "admin_complete"})
	_, err = s.orders.MarkPaid(ctx, order.ID, "manual", raw)
	return err
}

// credit credits the wallet idempotently by order_no. The unique constraint on
// wallet_transactions.idempotency_key guarantees exactly one credit.
func (s *Service) credit(ctx context.Context, order *paymentorder.Order) error {
	if order.Purpose == "subscription" {
		if s.ActivateSubscription == nil || order.PlanID == nil {
			return fmt.Errorf("payment credit: subscription activator not configured")
		}
		if _, err := s.ActivateSubscription(ctx, order.UserID, *order.PlanID); err != nil {
			return fmt.Errorf("payment credit: activate subscription: %w", err)
		}
		return nil
	}
	wal, err := s.wallets.FindByUser(ctx, order.UserID, nil)
	if err != nil {
		return fmt.Errorf("payment credit: find wallet: %w", err)
	}
	if _, err := s.wallets.TopUp(ctx, wal.ID, order.Amount, order.OrderNo); err != nil {
		return fmt.Errorf("payment credit: topup: %w", err)
	}
	return nil
}

func genOrderNo() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return fmt.Sprintf("DTP%s%s", time.Now().Format("20060102150405"), hex.EncodeToString(b))
}

// ---- settings raw helpers ----
func rawStr(m map[string]json.RawMessage, key string) string {
	if v, ok := m[key]; ok {
		var s string
		if json.Unmarshal(v, &s) == nil {
			return s
		}
	}
	return ""
}

func rawBool(m map[string]json.RawMessage, key string) bool {
	if v, ok := m[key]; ok {
		var b bool
		if json.Unmarshal(v, &b) == nil {
			return b
		}
		// tolerate JSON-string booleans ("true"/"false") written by the admin API.
		var s string
		if json.Unmarshal(v, &s) == nil {
			if parsed, err := strconv.ParseBool(s); err == nil {
				return parsed
			}
		}
	}
	return false
}

func rawDecimal(m map[string]json.RawMessage, key string, def decimal.Decimal) decimal.Decimal {
	s := rawStr(m, key)
	if s == "" {
		if v, ok := m[key]; ok {
			// JSON number (e.g. default amount_options style)
			if d, err := decimal.NewFromString(string(v)); err == nil {
				return d
			}
		}
		return def
	}
	if d, err := decimal.NewFromString(s); err == nil {
		return d
	}
	return def
}

func rawArray(m map[string]json.RawMessage, key string) []string {
	if v, ok := m[key]; ok {
		var arr []any
		if json.Unmarshal(v, &arr) == nil {
			out := make([]string, 0, len(arr))
			for _, e := range arr {
				out = append(out, fmt.Sprintf("%v", e))
			}
			return out
		}
		// tolerate JSON-string encoded arrays.
		var s string
		if json.Unmarshal(v, &s) == nil {
			var arr2 []any
			if json.Unmarshal([]byte(s), &arr2) == nil {
				out := make([]string, 0, len(arr2))
				for _, e := range arr2 {
					out = append(out, fmt.Sprintf("%v", e))
				}
				return out
			}
		}
	}
	return nil
}
