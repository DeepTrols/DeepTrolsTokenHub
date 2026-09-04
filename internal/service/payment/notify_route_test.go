package payment

// TH-P1-04: callback route channel resolver tests. The notify route channel
// is matched against the order's persisted channel BEFORE any verification
// or wallet settlement, so a payload posted to the wrong route can never
// credit a wallet, and historical orders settle through their original
// channel during cutover.

import (
	"context"
	"errors"
	"testing"

	"github.com/deeptrols/api/internal/repository/paymentorder"
	"github.com/shopspring/decimal"
)

// Unit: route-to-channel resolution is a closed set; anything else is
// rejected outright.
func TestChannelForRoute(t *testing.T) {
	for _, route := range []string{"epay", "alipay", "wechatpay"} {
		ch, err := channelForRoute(route)
		if err != nil || ch != route {
			t.Fatalf("route %q: channel = %q err = %v, want %q", route, ch, err, route)
		}
	}
	for _, route := range []string{"", "bitcoin", "Epay", "epay "} {
		if _, err := channelForRoute(route); !errors.Is(err, ErrInvalidChannel) {
			t.Fatalf("route %q: err = %v, want ErrInvalidChannel", route, err)
		}
	}
}

// AC-01: an epay order notified on the epay route reaches verification and
// settles exactly once.
func TestNotifyEpayRouteReachesVerification(t *testing.T) {
	s, orders, wallets := newTestService(enabledSettings(), &fakeGateway{
		notify: &NotifyResult{OrderNo: "DTPR1", GatewayTradeNo: "G1", Amount: decimal.NewFromInt(50), Success: true},
	})
	seedPendingOrder(orders, "DTPR1", decimal.NewFromInt(50))

	handled, err := s.HandleNotifyForChannel(context.Background(), "epay", map[string]string{"out_trade_no": "DTPR1"})
	if err != nil {
		t.Fatalf("HandleNotifyForChannel: %v", err)
	}
	if !handled {
		t.Fatal("expected handled=true on first notify")
	}
	if wallets.topupCount != 1 {
		t.Fatalf("expected exactly 1 credit, got %d", wallets.topupCount)
	}
}

// AC-02: an epay order payload posted to the alipay route is rejected
// before any wallet settlement; the order stays pending.
func TestNotifyRouteMismatchRejectedBeforeWallet(t *testing.T) {
	s, orders, wallets := newTestService(enabledSettings(), &fakeGateway{
		notify: &NotifyResult{OrderNo: "DTPR2", GatewayTradeNo: "G2", Amount: decimal.NewFromInt(50), Success: true},
	})
	o := seedPendingOrder(orders, "DTPR2", decimal.NewFromInt(50)) // channel epay

	handled, err := s.HandleNotifyForChannel(context.Background(), "alipay", map[string]string{"out_trade_no": "DTPR2"})
	if !errors.Is(err, ErrChannelMismatch) {
		t.Fatalf("error = %v, want ErrChannelMismatch", err)
	}
	if handled {
		t.Fatal("mismatched route must not settle")
	}
	if o.Status != paymentorder.StatusPending {
		t.Fatalf("order must stay pending on mismatch, got %s", o.Status)
	}
	if wallets.topupCount != 0 {
		t.Fatalf("mismatch must be rejected before any wallet call, got %d calls", wallets.topupCount)
	}
}

// AC-03: a route whose provider adapter has not landed returns the provider
// configuration error and leaves the order pending (historical orders keep
// settling through their original channel; a missing adapter fails closed).
// The real factory must run so alipay is genuinely not ready.
func TestNotifyRouteProviderNotReadyLeavesOrderPending(t *testing.T) {
	s, orders, wallets := newTestService(enabledSettings(), &fakeGateway{
		notify: &NotifyResult{OrderNo: "DTPR3", GatewayTradeNo: "G3", Amount: decimal.NewFromInt(50), Success: true},
	})
	s.newGateway = newGatewayForChannel
	o := seedPendingOrder(orders, "DTPR3", decimal.NewFromInt(50))
	o.Channel = "alipay" // order created on the alipay channel

	handled, err := s.HandleNotifyForChannel(context.Background(), "alipay", map[string]string{"out_trade_no": "DTPR3"})
	if !errors.Is(err, ErrChannelNotReady) {
		t.Fatalf("error = %v, want ErrChannelNotReady", err)
	}
	if handled {
		t.Fatal("not-ready provider must not settle")
	}
	if o.Status != paymentorder.StatusPending {
		t.Fatalf("order must stay pending, got %s", o.Status)
	}
	if wallets.topupCount != 0 {
		t.Fatalf("not-ready provider must not call the wallet, got %d calls", wallets.topupCount)
	}
}

// Unknown notify route: rejected before any order lookup or settlement.
func TestNotifyUnknownRouteRejected(t *testing.T) {
	s, orders, wallets := newTestService(enabledSettings(), &fakeGateway{payURL: "u"})
	seedPendingOrder(orders, "DTPR4", decimal.NewFromInt(50))

	_, err := s.HandleNotifyForChannel(context.Background(), "bitcoin", map[string]string{"out_trade_no": "DTPR4"})
	if !errors.Is(err, ErrInvalidChannel) {
		t.Fatalf("error = %v, want ErrInvalidChannel", err)
	}
	if wallets.topupCount != 0 {
		t.Fatalf("unknown route must not call the wallet, got %d calls", wallets.topupCount)
	}
}

// Failure injection: missing order number is rejected with the not-found
// class and settles nothing.
func TestNotifyMissingOrderNumber(t *testing.T) {
	s, _, wallets := newTestService(enabledSettings(), &fakeGateway{
		notify: &NotifyResult{OrderNo: "DTPRX", GatewayTradeNo: "GX", Amount: decimal.NewFromInt(50), Success: true},
	})
	handled, err := s.HandleNotifyForChannel(context.Background(), "epay", map[string]string{"foo": "bar"})
	if !errors.Is(err, ErrOrderNotFound) {
		t.Fatalf("error = %v, want ErrOrderNotFound", err)
	}
	if handled || wallets.topupCount != 0 {
		t.Fatalf("missing order number must settle nothing (handled=%v credits=%d)", handled, wallets.topupCount)
	}
}

// Regression: the legacy HandleNotify entry point stays epay-routed and
// idempotent after the resolver refactor.
func TestHandleNotifyLegacyEntryStillEpay(t *testing.T) {
	s, orders, wallets := newTestService(enabledSettings(), &fakeGateway{
		notify: &NotifyResult{OrderNo: "DTPR5", GatewayTradeNo: "G5", Amount: decimal.NewFromInt(50), Success: true},
	})
	seedPendingOrder(orders, "DTPR5", decimal.NewFromInt(50))

	handled, err := s.HandleNotify(context.Background(), map[string]string{"out_trade_no": "DTPR5"})
	if err != nil || !handled {
		t.Fatalf("legacy HandleNotify: handled=%v err=%v", handled, err)
	}
	handled2, err := s.HandleNotify(context.Background(), map[string]string{"out_trade_no": "DTPR5"})
	if err != nil || handled2 {
		t.Fatalf("replay: handled=%v err=%v", handled2, err)
	}
	if wallets.topupCount != 1 {
		t.Fatalf("expected exactly 1 credit across replays, got %d", wallets.topupCount)
	}
}
