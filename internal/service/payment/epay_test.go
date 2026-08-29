package payment

import (
	"context"
	"testing"

	"github.com/Calcium-Ion/go-epay/epay"
)

func TestEpayGatewayVerifySignature(t *testing.T) {
	gw := &EpayGateway{
		PayAddress: "https://pay.example.com",
		PartnerID:  "1001",
		Key:        "k123",
		NotifyURL:  "https://host/api/payment/notify/epay",
		ReturnURL:  "https://host/recharge",
	}
	params := map[string]string{
		"pid":          "1001",
		"type":         "alipay",
		"out_trade_no": "DTP88",
		"money":        "50.00",
		"trade_status": epay.StatusTradeSuccess,
		"trade_no":     "GATEWAY88",
		"sign_type":    "MD5",
	}
	signed := epay.GenerateParams(params, "k123")
	res, err := gw.VerifyNotify(context.Background(), signed)
	if err != nil {
		t.Fatalf("VerifyNotify: %v", err)
	}
	if !res.Success || res.OrderNo != "DTP88" || res.Amount.String() != "50" || res.GatewayTradeNo != "GATEWAY88" {
		t.Fatalf("unexpected verify result: %+v", res)
	}
}

func TestEpayGatewayVerifyRejectsBadSignature(t *testing.T) {
	gw := &EpayGateway{
		PayAddress: "https://pay.example.com",
		PartnerID:  "1001",
		Key:        "k123",
		NotifyURL:  "https://host/api/payment/notify/epay",
		ReturnURL:  "https://host/recharge",
	}
	params := map[string]string{
		"pid":          "1001",
		"type":         "alipay",
		"out_trade_no": "DTP88",
		"money":        "50.00",
		"trade_status": epay.StatusTradeSuccess,
		"trade_no":     "GATEWAY88",
		"sign":         "bad-sign",
	}
	if _, err := gw.VerifyNotify(context.Background(), params); err == nil {
		t.Fatal("expected signature verification error")
	}
}
