package payment

import (
	"context"
	"errors"

	"github.com/shopspring/decimal"
)

// Gateway abstracts a payment channel (epay first, official adapters later).
type Gateway interface {
	Name() string
	CreateOrder(ctx context.Context, req CreateOrderRequest) (*CreateOrderResult, error)
	VerifyNotify(ctx context.Context, params map[string]string) (*NotifyResult, error)
}

// CreateOrderRequest is the channel-agnostic order creation request.
type CreateOrderRequest struct {
	OrderNo   string
	Amount    decimal.Decimal
	PayMethod string // alipay / wxpay
	Subject   string
	NotifyURL string
	ReturnURL string
}

// CreateOrderResult carries the payment URL the client should open/scan.
type CreateOrderResult struct {
	PayURL string
}

// NotifyResult is the verified, normalized gateway callback payload.
type NotifyResult struct {
	OrderNo        string
	GatewayTradeNo string
	Amount         decimal.Decimal
	PayMethod      string
	Success        bool
}

var (
	ErrPaymentDisabled = errors.New("payment: online topup disabled")
	ErrNotConfigured   = errors.New("payment: gateway not configured")
	ErrInvalidMethod   = errors.New("payment: unsupported pay method")
	ErrAmountRange     = errors.New("payment: amount out of range")
	ErrOrderNotFound   = errors.New("payment: order not found")
	ErrAmountMismatch  = errors.New("payment: notify amount mismatch")
	ErrOrderExpired    = errors.New("payment: order expired")
)
