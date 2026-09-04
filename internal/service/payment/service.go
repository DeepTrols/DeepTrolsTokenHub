package payment

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/deeptrols/api/internal/domain"
	"github.com/deeptrols/api/internal/pkg/metrics"
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
	newGateway func(cfg *paymentConfig) (Gateway, error)
	now        func() time.Time
	// ActivateSubscription settles a paid subscription order (nil = disabled).
	ActivateSubscription func(ctx context.Context, userID, planID uuid.UUID) (time.Time, error)
}

// NewService creates a payment Service.
func NewService(orders paymentorder.Repository, wallets walletRepo, settings settingReader) *Service {
	return &Service{
		orders:     orders,
		wallets:    wallets,
		settings:   settings,
		newGateway: newGatewayForChannel,
		now:        time.Now,
	}
}

// normalizeChannel resolves the configured payment_channel value; only an
// empty setting falls back to the default epay channel.
func normalizeChannel(raw string) string {
	if raw == "" {
		return ChannelEpay
	}
	return raw
}

// newGatewayForChannel selects the gateway implementation for the configured
// payment channel (TH-P1-03). epay is the only fully implemented channel
// today; alipay and wechatpay fail closed with ErrChannelNotReady until
// their provider tasks land concrete implementations, and any unknown value
// fails with ErrInvalidChannel so it can never route paid traffic to the
// wrong gateway. Logs the selected channel / error class, never credentials.
func newGatewayForChannel(cfg *paymentConfig) (Gateway, error) {
	channel := normalizeChannel(cfg.Channel)
	switch channel {
	case ChannelEpay:
		return &EpayGateway{
			PayAddress: cfg.PayAddress,
			PartnerID:  cfg.EpayID,
			Key:        cfg.EpayKey,
			NotifyURL:  cfg.CallbackBase + "/api/payment/notify/" + ChannelEpay,
			ReturnURL:  cfg.CallbackBase + "/recharge",
		}, nil
	case ChannelAlipay, ChannelWeChatPay:
		return nil, fmt.Errorf("%w: %s (provider adapter not implemented yet)", ErrChannelNotReady, channel)
	default:
		return nil, fmt.Errorf("%w: %q", ErrInvalidChannel, cfg.Channel)
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

func (s *Service) gateway(cfg *paymentConfig) (Gateway, error) {
	if s.newGateway != nil {
		return s.newGateway(cfg)
	}
	return newGatewayForChannel(cfg)
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
		Channel:       normalizeChannel(cfg.Channel), // effective channel
		PayMethods:    []PayMethod{},
	}
	// Pay methods exist only for the implemented epay channel; not-ready or
	// unknown channels must not advertise payable methods.
	if normalizeChannel(cfg.Channel) == ChannelEpay &&
		info.Enabled && cfg.PayAddress != "" && cfg.EpayID != "" && cfg.EpayKey != "" {
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
	channel := normalizeChannel(cfg.Channel)
	gw, err := s.gateway(cfg)
	if err != nil {
		log.Printf("payment: channel %q rejected: %v", cfg.Channel, err)
		return nil, err
	}
	if amount.LessThan(cfg.MinTopup) || amount.GreaterThan(cfg.MaxTopup) {
		return nil, ErrAmountRange
	}

	orderNo := genOrderNo()
	res, err := gw.CreateOrder(ctx, CreateOrderRequest{
		OrderNo:   orderNo,
		Amount:    amount,
		PayMethod: payMethod,
		Subject:   "智曜TokenHub 平台充值 " + amount.StringFixed(2) + " 元",
		NotifyURL: cfg.CallbackBase + "/api/payment/notify/" + channel,
		ReturnURL: cfg.CallbackBase + "/recharge",
	})
	if err != nil {
		return nil, err
	}
	log.Printf("payment: order %s created via channel %q", orderNo, channel)

	o := &paymentorder.Order{
		ID:        uuid.New(),
		OrderNo:   orderNo,
		UserID:    userID,
		Amount:    amount,
		Currency:  "CNY",
		Channel:   channel,
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
		Channel:   channel,
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
	channel := normalizeChannel(cfg.Channel)
	gw, err := s.gateway(cfg)
	if err != nil {
		log.Printf("payment: channel %q rejected: %v", cfg.Channel, err)
		return nil, err
	}

	orderNo := genOrderNo()
	res, err := gw.CreateOrder(ctx, CreateOrderRequest{
		OrderNo:   orderNo,
		Amount:    amount,
		PayMethod: payMethod,
		Subject:   "智曜TokenHub 订阅 " + amount.StringFixed(2) + " 元",
		NotifyURL: cfg.CallbackBase + "/api/payment/notify/" + channel,
		ReturnURL: cfg.CallbackBase + "/subscriptions",
	})
	if err != nil {
		return nil, err
	}
	log.Printf("payment: subscription order %s created via channel %q", orderNo, channel)
	o := &paymentorder.Order{
		ID:        uuid.New(),
		OrderNo:   orderNo,
		UserID:    userID,
		Amount:    amount,
		Currency:  "CNY",
		Purpose:   "subscription",
		PlanID:    &planID,
		Channel:   channel,
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
		Channel:   channel,
		PayMethod: payMethod,
		PayURL:    res.PayURL,
	}, nil
}

// HandleNotify verifies a gateway callback and settles the order idempotently.
// It returns true when the order was settled on this call, false on a replay.
// Legacy entry point for the epay notify route; resolves through the epay
// channel resolver (TH-P1-04).
func (s *Service) HandleNotify(ctx context.Context, params map[string]string) (bool, error) {
	return s.HandleNotifyForChannel(ctx, ChannelEpay, params)
}

// channelForRoute resolves a notify route segment onto the closed channel
// set (TH-P1-04). Anything outside the set is rejected outright so an
// unknown route can never reach verification or settlement.
func channelForRoute(route string) (string, error) {
	switch route {
	case ChannelEpay, ChannelAlipay, ChannelWeChatPay:
		return route, nil
	default:
		return "", fmt.Errorf("%w: %q", ErrInvalidChannel, route)
	}
}

// gatewayForRoute builds the gateway for the resolved callback channel
// (never the current global setting alone), so historical orders settle
// through their original channel during cutover.
func (s *Service) gatewayForRoute(cfg *paymentConfig, routeChannel string) (Gateway, error) {
	routed := *cfg
	routed.Channel = routeChannel
	return s.gateway(&routed)
}

// HandleNotifyForChannel verifies a gateway callback posted to the
// per-channel notify route and settles the order idempotently (TH-P1-04).
// The route channel is matched against the order's persisted channel BEFORE
// any signature verification or wallet settlement, so a payload posted to
// the wrong route can never credit a wallet. It returns true when the order
// was settled on this call, false on a replay.
func (s *Service) HandleNotifyForChannel(ctx context.Context, routeChannel string, params map[string]string) (bool, error) {
	route, err := channelForRoute(routeChannel)
	if err != nil {
		return false, err
	}
	orderNo := params["out_trade_no"]
	if orderNo == "" {
		return false, ErrOrderNotFound
	}
	order, err := s.orders.FindByOrderNo(ctx, orderNo)
	if err != nil {
		return false, err
	}
	orderChannel := order.Channel
	if orderChannel == "" {
		orderChannel = ChannelEpay // legacy rows predate channel tracking
	}
	if orderChannel != route {
		metrics.IncPaymentNotifyRouteMismatch(route, orderChannel)
		log.Printf("payment: notify route %q rejected: order channel is %q", route, orderChannel)
		return false, ErrChannelMismatch
	}
	cfg, err := s.config(ctx)
	if err != nil {
		return false, err
	}
	gw, err := s.gatewayForRoute(cfg, route)
	if err != nil {
		log.Printf("payment: notify route %q has no provider: %v", route, err)
		return false, err
	}
	notify, err := gw.VerifyNotify(ctx, params)
	if err != nil {
		return false, err
	}
	if !notify.Success {
		return false, nil
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
