package payment

import (
	"context"
	"fmt"
	"net/url"

	"github.com/Calcium-Ion/go-epay/epay"
	"github.com/shopspring/decimal"
)

// EpayGateway talks to a 易支付 (epay) aggregator via Calcium-Ion/go-epay.
type EpayGateway struct {
	PayAddress string
	PartnerID  string
	Key        string
	NotifyURL  string
	ReturnURL  string
}

func (g *EpayGateway) Name() string { return "epay" }

func (g *EpayGateway) client() (*epay.Client, error) {
	if g.PayAddress == "" || g.PartnerID == "" || g.Key == "" {
		return nil, ErrNotConfigured
	}
	return epay.NewClient(&epay.Config{PartnerID: g.PartnerID, Key: g.Key}, g.PayAddress)
}

func (g *EpayGateway) CreateOrder(ctx context.Context, req CreateOrderRequest) (*CreateOrderResult, error) {
	c, err := g.client()
	if err != nil {
		return nil, err
	}
	notify, _ := url.Parse(g.NotifyURL)
	ret, _ := url.Parse(g.ReturnURL)
	uri, _, err := c.Purchase(&epay.PurchaseArgs{
		Type:           req.PayMethod,
		ServiceTradeNo: req.OrderNo,
		Name:           req.Subject,
		Money:          req.Amount.StringFixed(2),
		Device:         epay.PC,
		NotifyUrl:      notify,
		ReturnUrl:      ret,
	})
	if err != nil {
		return nil, fmt.Errorf("payment epay purchase: %w", err)
	}
	return &CreateOrderResult{PayURL: uri}, nil
}

// QueryOrder is a placeholder: the bundled epay library (Calcium-Ion/go-epay
// v0.0.4) exposes only Purchase and Verify, so no active query path exists
// yet. The compensation-worker tasks add the real epay query; until then the
// sentinel keeps the contract explicit instead of silently succeeding.
func (g *EpayGateway) QueryOrder(ctx context.Context, orderNo string) (*QueryOrderResult, error) {
	return nil, fmt.Errorf("%w: epay", ErrQueryUnsupported)
}

func (g *EpayGateway) VerifyNotify(ctx context.Context, params map[string]string) (*NotifyResult, error) {
	c, err := g.client()
	if err != nil {
		return nil, err
	}
	res, err := c.Verify(params)
	if err != nil {
		return nil, fmt.Errorf("payment epay verify: %w", err)
	}
	if !res.VerifyStatus {
		return nil, fmt.Errorf("payment: epay signature verification failed")
	}
	amt, err := decimal.NewFromString(res.Money)
	if err != nil {
		return nil, fmt.Errorf("payment: bad notify amount %q", res.Money)
	}
	return &NotifyResult{
		OrderNo:        res.ServiceTradeNo,
		GatewayTradeNo: res.TradeNo,
		Amount:         amt,
		PayMethod:      res.Type,
		Success:        res.TradeStatus == epay.StatusTradeSuccess,
	}, nil
}
