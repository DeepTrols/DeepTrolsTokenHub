package payment

// TH-P1-03: payment channel factory tests. The configured payment_channel
// value selects the gateway implementation; unknown values must never route
// paid traffic to the wrong gateway or create an order row.

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

func settingsWithChannel(channel string) *fakeSettings {
	s := enabledSettings()
	s.data["payment_channel"] = json.RawMessage(`"` + channel + `"`)
	return s
}

// AC-01: payment_channel=epay (and the empty default) select the epay
// gateway carrying the configured credentials.
func TestFactorySelectsEpay(t *testing.T) {
	for _, ch := range []string{"epay", ""} {
		cfg := &paymentConfig{Channel: ch, PayAddress: "https://pay.example.com", EpayID: "1001", EpayKey: "k123", CallbackBase: "https://cb.example"}
		gw, err := newGatewayForChannel(cfg)
		if err != nil {
			t.Fatalf("channel %q: %v", ch, err)
		}
		epay, ok := gw.(*EpayGateway)
		if !ok || epay.Name() != "epay" {
			t.Fatalf("channel %q: expected *EpayGateway, got %T", ch, gw)
		}
		if epay.PayAddress != cfg.PayAddress || epay.PartnerID != cfg.EpayID || epay.Key != cfg.EpayKey {
			t.Fatalf("channel %q: credentials not carried to gateway", ch)
		}
		if epay.NotifyURL != "https://cb.example/api/payment/notify/epay" {
			t.Fatalf("channel %q: notify url = %q", ch, epay.NotifyURL)
		}
	}
}

// AC-02: payment_channel=alipay before the Alipay provider lands returns a
// channel configuration error, never a gateway.
func TestFactoryAlipayNotReadyIsConfigError(t *testing.T) {
	cfg := &paymentConfig{Channel: "alipay"}
	gw, err := newGatewayForChannel(cfg)
	if gw != nil {
		t.Fatalf("expected nil gateway for unimplemented channel, got %T", gw)
	}
	if !errors.Is(err, ErrChannelNotReady) {
		t.Fatalf("error = %v, want ErrChannelNotReady", err)
	}
}

func TestFactoryWeChatPayNotReadyIsConfigError(t *testing.T) {
	cfg := &paymentConfig{Channel: "wechatpay"}
	gw, err := newGatewayForChannel(cfg)
	if gw != nil || !errors.Is(err, ErrChannelNotReady) {
		t.Fatalf("gateway=%T err=%v, want nil + ErrChannelNotReady", gw, err)
	}
}

// AC-03 (factory level): unknown channel values are rejected outright.
func TestFactoryUnknownChannelInvalid(t *testing.T) {
	for _, ch := range []string{"bitcoin", "Epay", "epay "} {
		cfg := &paymentConfig{Channel: ch}
		gw, err := newGatewayForChannel(cfg)
		if gw != nil || !errors.Is(err, ErrInvalidChannel) {
			t.Fatalf("channel %q: gateway=%T err=%v, want nil + ErrInvalidChannel", ch, gw, err)
		}
	}
}

// AC-03 (service level): order creation with an unknown channel fails and
// creates NO payment_orders row. The real factory must run here — the fake
// gateway injection would mask channel selection.
func TestCreateOrderUnknownChannelCreatesNoRow(t *testing.T) {
	s, orders, _ := newTestService(settingsWithChannel("bitcoin"), &fakeGateway{payURL: "https://pay/u"})
	s.newGateway = newGatewayForChannel
	_, err := s.CreateOrder(context.Background(), uuid.New(), decimal.NewFromInt(50), "alipay")
	if !errors.Is(err, ErrInvalidChannel) {
		t.Fatalf("error = %v, want ErrInvalidChannel", err)
	}
	if len(orders.byNo) != 0 {
		t.Fatalf("unknown channel must create no order rows, got %d", len(orders.byNo))
	}
}

// AC-02 (service level): order creation on the not-ready alipay channel
// returns the configuration error and creates no row. Real factory restored
// for the same reason as above.
func TestCreateOrderAlipayChannelNotReady(t *testing.T) {
	s, orders, _ := newTestService(settingsWithChannel("alipay"), &fakeGateway{payURL: "https://pay/u"})
	s.newGateway = newGatewayForChannel
	_, err := s.CreateOrder(context.Background(), uuid.New(), decimal.NewFromInt(50), "alipay")
	if !errors.Is(err, ErrChannelNotReady) {
		t.Fatalf("error = %v, want ErrChannelNotReady", err)
	}
	if len(orders.byNo) != 0 {
		t.Fatalf("not-ready channel must create no order rows, got %d", len(orders.byNo))
	}
}

// Regression: explicit epay channel records the channel on the order row
// and the response; epay order creation stays green.
func TestCreateOrderRecordsEpayChannel(t *testing.T) {
	s, orders, _ := newTestService(settingsWithChannel("epay"), &fakeGateway{payURL: "https://pay/u"})
	res, err := s.CreateOrder(context.Background(), uuid.New(), decimal.NewFromInt(50), "alipay")
	if err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}
	if res.Channel != "epay" {
		t.Fatalf("response channel = %q, want epay", res.Channel)
	}
	ord := orders.byNo[res.OrderNo]
	if ord == nil || ord.Channel != "epay" {
		t.Fatalf("order row channel = %+v, want epay", ord)
	}
}

// The not-ready channels must not expose pay methods either (methods only
// exist for the implemented epay channel today).
func TestInfoMethodsOnlyForImplementedChannel(t *testing.T) {
	for _, ch := range []string{"alipay", "wechatpay", "bitcoin"} {
		s, _, _ := newTestService(settingsWithChannel(ch), &fakeGateway{payURL: "u"})
		info, err := s.Info(context.Background())
		if err != nil {
			t.Fatalf("Info(%q): %v", ch, err)
		}
		if len(info.PayMethods) != 0 {
			t.Fatalf("channel %q: expected no pay methods, got %+v", ch, info.PayMethods)
		}
	}
	s, _, _ := newTestService(settingsWithChannel("epay"), &fakeGateway{payURL: "u"})
	info, err := s.Info(context.Background())
	if err != nil || len(info.PayMethods) != 2 {
		t.Fatalf("epay channel: expected 2 methods, got %+v (err %v)", info, err)
	}
}
