package payment

// TH-P1-AL-02 — Alipay CreateOrder client tests.
//
// AC-01: valid sandbox config + amount 0.01 returns a non-empty pay URL and
// a local order number (order stays pending).
// AC-02: a provider business failure maps to the provider error class and
// creates no paid order state.
// AC-03: a context timeout maps to the timeout error class and creates no
// wallet transaction.
//
// Unit covers the request mapper, amount formatter, RSA2 signer and key
// parser; integration runs the real factory and the real gateway against an
// httptest TLS server (its https URL also satisfies the AL-01 gateway URL
// validation). Callback verification and settlement stay out of scope
// (TH-P1-AL-03/04): both paths must fail closed until those tasks land.

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/deeptrols/api/internal/repository/paymentorder"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// generateAlipayTestKey returns a fresh RSA key plus its PKCS#8 body in the
// two shapes the parser must accept: PEM and raw base64 (the raw form is
// what the settings store carries, since JSON strings cannot embed newlines).
func generateAlipayTestKey(t *testing.T) (*rsa.PrivateKey, string, string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RSA key: %v", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshal PKCS8: %v", err)
	}
	pemBody := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
	return key, string(pemBody), base64.StdEncoding.EncodeToString(der)
}

// alipaySandboxSettings builds a complete sandbox configuration (AC-01
// shape) pointing at the given gateway URL with min_topup lowered to 0.01.
func alipaySandboxSettings(gatewayURL, keyBody string) *fakeSettings {
	return alipaySettingsWithData(settingsWithChannel(ChannelAlipay), map[string]string{
		"alipay_sandbox":             "true",
		"alipay_sandbox_app_id":      "2088sandboxapp",
		"alipay_sandbox_private_key": keyBody,
		"alipay_sandbox_gateway_url": gatewayURL,
		"min_topup":                  "0.01",
		"callback_base_url":          "https://cb.example",
	})
}

// newAlipaySandboxService wires the service through the REAL factory and a
// real AlipayGateway, substituting only the HTTP transport so the gateway
// trusts the test TLS certificate.
func newAlipaySandboxService(t *testing.T, settings *fakeSettings, srv *httptest.Server) (*Service, *fakeOrders, *fakeWallets) {
	t.Helper()
	s, orders, wallets := newTestService(settings, &fakeGateway{payURL: "unused"})
	s.newGateway = func(cfg *paymentConfig) (Gateway, error) {
		gw, err := newGatewayForChannel(cfg)
		if err != nil {
			return nil, err
		}
		ag, ok := gw.(*AlipayGateway)
		if !ok {
			return nil, fmt.Errorf("factory returned %T for the alipay channel, want *AlipayGateway", gw)
		}
		ag.HTTPClient = srv.Client()
		return ag, nil
	}
	return s, orders, wallets
}

// alipayFormSignContent rebuilds the Alipay sign content from a submitted
// form: sorted k=v pairs joined by &, excluding the sign parameter and empty
// values (the same rule the signer applies).
func alipayFormSignContent(form url.Values) string {
	keys := make([]string, 0, len(form))
	for k, vs := range form {
		if k == "sign" || len(vs) == 0 || vs[0] == "" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+form.Get(k))
	}
	return strings.Join(parts, "&")
}

// newAlipaySuccessServer mocks the sandbox gateway: it verifies the RSA2
// signature over the submitted parameters, checks the precreate protocol and
// answers with a qr_code. sigChecked flips to 1 once a request passed
// signature verification.
func newAlipaySuccessServer(t *testing.T, pub *rsa.PublicKey) (*httptest.Server, *int32) {
	t.Helper()
	var sigChecked int32
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		sig, err := base64.StdEncoding.DecodeString(r.PostForm.Get("sign"))
		digest := sha256.Sum256([]byte(alipayFormSignContent(r.PostForm)))
		if err != nil || rsa.VerifyPKCS1v15(pub, crypto.SHA256, digest[:], sig) != nil {
			http.Error(w, "invalid signature", http.StatusBadRequest)
			return
		}
		if r.PostForm.Get("method") != "alipay.trade.precreate" || r.PostForm.Get("sign_type") != "RSA2" {
			http.Error(w, "unexpected protocol", http.StatusBadRequest)
			return
		}
		atomic.StoreInt32(&sigChecked, 1)
		var biz struct {
			OutTradeNo  string `json:"out_trade_no"`
			TotalAmount string `json:"total_amount"`
		}
		_ = json.Unmarshal([]byte(r.PostForm.Get("biz_content")), &biz)
		if biz.TotalAmount != "0.01" {
			http.Error(w, "unexpected amount", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json;charset=utf-8")
		fmt.Fprintf(w, `{"alipay_trade_precreate_response":{"code":"10000","msg":"Success","out_trade_no":"%s","qr_code":"https://qr.alipay.example/bax01234"}}`, biz.OutTradeNo)
	}))
	return srv, &sigChecked
}

// TestFormatAlipayAmount pins the two-decimal CNY rendering required by the
// precreate total_amount field (Implementation Note).
func TestFormatAlipayAmount(t *testing.T) {
	cases := map[string]string{
		"0.01":  "0.01",
		"10":    "10.00",
		"1.5":   "1.50",
		"1.234": "1.23",
		"1.239": "1.24",
		"100":   "100.00",
	}
	for in, want := range cases {
		if got := formatAlipayAmount(decimal.RequireFromString(in)); got != want {
			t.Errorf("formatAlipayAmount(%s) = %q, want %q", in, got, want)
		}
	}
}

// TestParseAlipayPrivateKeyShapes covers every key shape the Alipay console
// issues, and proves parse errors never carry key material.
func TestParseAlipayPrivateKeyShapes(t *testing.T) {
	key, pemBody, rawB64 := generateAlipayTestKey(t)

	pkcs8PEM, err := parseAlipayPrivateKey(pemBody)
	if err != nil || pkcs8PEM.N.Cmp(key.N) != 0 {
		t.Fatalf("PKCS8 PEM parse: err=%v", err)
	}
	raw, err := parseAlipayPrivateKey(rawB64)
	if err != nil || raw.N.Cmp(key.N) != 0 {
		t.Fatalf("raw base64 PKCS8 parse: err=%v", err)
	}

	// PKCS#1 PEM must also parse.
	pkcs1 := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	if _, err := parseAlipayPrivateKey(string(pkcs1)); err != nil {
		t.Fatalf("PKCS1 PEM parse: %v", err)
	}

	// Failure injection: garbage input fails with a redacted error.
	garbage := "NOT-A-KEY-BODY-abc123"
	_, err = parseAlipayPrivateKey(garbage)
	if err == nil {
		t.Fatal("expected parse error for garbage key")
	}
	if strings.Contains(err.Error(), garbage) {
		t.Fatalf("parse error leaks key material: %q", err.Error())
	}
	if _, err := parseAlipayPrivateKey(""); err == nil {
		t.Fatal("expected parse error for empty key")
	}
}

// TestAlipaySignParamsVerifiable: the signature is a valid RSA2/SHA256
// signature of the sorted sign content, and empty values plus any prior sign
// parameter are excluded from it.
func TestAlipaySignParamsVerifiable(t *testing.T) {
	key, _, _ := generateAlipayTestKey(t)
	params := map[string]string{
		"app_id": "2088sandboxapp",
		"method": "alipay.trade.precreate",
		"empty":  "",
		"sign":   "STALE-SIGNATURE-MUST-BE-EXCLUDED",
	}
	sig, err := alipaySignParams(params, key)
	if err != nil {
		t.Fatalf("alipaySignParams: %v", err)
	}
	sigBytes, err := base64.StdEncoding.DecodeString(sig)
	if err != nil {
		t.Fatalf("signature is not base64: %v", err)
	}
	wantContent := "app_id=2088sandboxapp&method=alipay.trade.precreate"
	digest := sha256.Sum256([]byte(wantContent))
	if err := rsa.VerifyPKCS1v15(&key.PublicKey, crypto.SHA256, digest[:], sigBytes); err != nil {
		t.Fatalf("signature does not verify over %q: %v", wantContent, err)
	}
}

// TestBuildAlipayParamsMapsRequest pins the request mapping: the common
// protocol parameters and the biz_content JSON fields.
func TestBuildAlipayParamsMapsRequest(t *testing.T) {
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	req := CreateOrderRequest{
		OrderNo:   "DTPALMAP1",
		Amount:    decimal.RequireFromString("0.01"),
		Subject:   "智曜TokenHub 平台充值 0.01 元",
		NotifyURL: "https://cb.example/api/payment/notify/alipay",
	}
	params := buildAlipayParams(req, "2088sandboxapp", req.NotifyURL, now)

	checks := map[string]string{
		"app_id":     "2088sandboxapp",
		"method":     "alipay.trade.precreate",
		"format":     "JSON",
		"charset":    "utf-8",
		"sign_type":  "RSA2",
		"version":    "1.0",
		"timestamp":  "2026-09-04 12:00:00",
		"notify_url": req.NotifyURL,
	}
	for k, want := range checks {
		if params[k] != want {
			t.Errorf("param %s = %q, want %q", k, params[k], want)
		}
	}
	var biz struct {
		OutTradeNo  string `json:"out_trade_no"`
		TotalAmount string `json:"total_amount"`
		Subject     string `json:"subject"`
	}
	if err := json.Unmarshal([]byte(params["biz_content"]), &biz); err != nil {
		t.Fatalf("biz_content is not JSON: %v", err)
	}
	if biz.OutTradeNo != "DTPALMAP1" || biz.TotalAmount != "0.01" || biz.Subject != req.Subject {
		t.Fatalf("biz_content mapped wrong: %+v", biz)
	}
}

// TestCreateOrderAlipaySandboxReturnsPayURL covers AC-01 end to end through
// Service.CreateOrder with the real factory and a signature-verifying mock
// sandbox gateway, plus the regression: the local order remains pending and
// no wallet transaction happens at create time.
func TestCreateOrderAlipaySandboxReturnsPayURL(t *testing.T) {
	key, _, keyB64 := generateAlipayTestKey(t)
	srv, sigChecked := newAlipaySuccessServer(t, &key.PublicKey)
	defer srv.Close()

	s, orders, wallets := newAlipaySandboxService(t, alipaySandboxSettings(srv.URL, keyB64), srv)
	res, err := s.CreateOrder(context.Background(), uuid.New(), decimal.RequireFromString("0.01"), "alipay")
	if err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}
	if res.PayURL == "" {
		t.Fatal("AC-01: pay URL must be non-empty")
	}
	if res.PayURL != "https://qr.alipay.example/bax01234" {
		t.Fatalf("pay URL = %q, want the provider qr_code", res.PayURL)
	}
	if res.OrderNo == "" {
		t.Fatal("AC-01: local order number must be non-empty")
	}
	if res.Channel != ChannelAlipay {
		t.Fatalf("response channel = %q, want alipay", res.Channel)
	}
	if atomic.LoadInt32(sigChecked) != 1 {
		t.Fatal("the mock gateway never saw a valid RSA2 signature")
	}
	// Regression: local order remains pending after create.
	ord := orders.byNo[res.OrderNo]
	if ord == nil {
		t.Fatal("order row not persisted")
	}
	if ord.Status != paymentorder.StatusPending {
		t.Fatalf("order status = %s, want pending after create", ord.Status)
	}
	if ord.Channel != ChannelAlipay || ord.PayURL == nil || *ord.PayURL != res.PayURL {
		t.Fatalf("order row channel/pay URL wrong: %+v", ord)
	}
	if wallets.topupCount != 0 {
		t.Fatalf("order create must not credit the wallet, got %d credits", wallets.topupCount)
	}
}

// TestCreateOrderAlipayProviderErrorFailsClosed covers AC-02: a provider
// business failure (40004) maps to the provider error class carrying the
// sanitized provider code/sub_code, and creates no order state at all.
func TestCreateOrderAlipayProviderErrorFailsClosed(t *testing.T) {
	_, _, keyB64 := generateAlipayTestKey(t)
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json;charset=utf-8")
		fmt.Fprint(w, `{"alipay_trade_precreate_response":{"code":"40004","msg":"Business Failed","sub_code":"ACQ.INVALID_PARAMETER","sub_msg":"invalid parameter"}}`)
	}))
	defer srv.Close()

	s, orders, wallets := newAlipaySandboxService(t, alipaySandboxSettings(srv.URL, keyB64), srv)
	_, err := s.CreateOrder(context.Background(), uuid.New(), decimal.RequireFromString("0.01"), "alipay")
	if !errors.Is(err, ErrAlipayProvider) {
		t.Fatalf("error = %v, want ErrAlipayProvider", err)
	}
	if !strings.Contains(err.Error(), "40004") || !strings.Contains(err.Error(), "ACQ.INVALID_PARAMETER") {
		t.Fatalf("provider error must keep the sanitized provider code/sub_code, got %q", err.Error())
	}
	if len(orders.byNo) != 0 {
		t.Fatalf("provider failure must create no order rows, got %d", len(orders.byNo))
	}
	if wallets.topupCount != 0 {
		t.Fatalf("provider failure must not touch the wallet, got %d credits", wallets.topupCount)
	}
}

// TestCreateOrderAlipayTimeoutFailsClosed covers AC-03: a context deadline
// maps to the timeout error class and produces no wallet transaction and no
// order row.
func TestCreateOrderAlipayTimeoutFailsClosed(t *testing.T) {
	_, _, keyB64 := generateAlipayTestKey(t)
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
	}))
	defer srv.Close()

	s, orders, wallets := newAlipaySandboxService(t, alipaySandboxSettings(srv.URL, keyB64), srv)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := s.CreateOrder(ctx, uuid.New(), decimal.RequireFromString("0.01"), "alipay")
	if !errors.Is(err, ErrAlipayTimeout) {
		t.Fatalf("error = %v, want ErrAlipayTimeout", err)
	}
	if len(orders.byNo) != 0 {
		t.Fatalf("timeout must create no order rows, got %d", len(orders.byNo))
	}
	if wallets.topupCount != 0 {
		t.Fatalf("timeout must create no wallet transaction, got %d credits", wallets.topupCount)
	}
}

// TestAlipayGatewayNotifyAndQueryFailClosed guards the AL-02 scope lines:
// callback verification (TH-P1-AL-03) and the query client (TH-P1-AL-05)
// are not implemented yet and must fail closed instead of pretending to
// succeed.
func TestAlipayGatewayNotifyAndQueryFailClosed(t *testing.T) {
	_, _, keyB64 := generateAlipayTestKey(t)
	cfg := &paymentConfig{
		Channel:      ChannelAlipay,
		CallbackBase: "https://cb.example",
		Alipay: AlipayConfig{
			Sandbox:           true,
			SandboxAppID:      "2088sandboxapp",
			SandboxPrivateKey: keyB64,
		},
	}
	gw, err := newGatewayForChannel(cfg)
	if err != nil {
		t.Fatalf("factory: %v", err)
	}
	ag, ok := gw.(*AlipayGateway)
	if !ok {
		t.Fatalf("factory returned %T, want *AlipayGateway", gw)
	}
	if ag.Name() != ChannelAlipay {
		t.Fatalf("gateway name = %q, want alipay", ag.Name())
	}
	if _, err := ag.VerifyNotify(context.Background(), map[string]string{"out_trade_no": "DTPX"}); err == nil {
		t.Fatal("VerifyNotify must fail closed until TH-P1-AL-03")
	}
	if _, err := ag.QueryOrder(context.Background(), "DTPX"); !errors.Is(err, ErrQueryUnsupported) {
		t.Fatalf("QueryOrder error = %v, want ErrQueryUnsupported until TH-P1-AL-05", err)
	}
}

// TestNotifyAlipayRouteFailsClosedUntilCallbackTask: with a complete alipay
// config the alipay notify route now constructs the gateway, but callback
// verification still fails closed (TH-P1-AL-03) — the order stays pending
// and the wallet is never touched.
func TestNotifyAlipayRouteFailsClosedUntilCallbackTask(t *testing.T) {
	_, _, keyB64 := generateAlipayTestKey(t)
	s, orders, wallets := newTestService(alipaySandboxSettings("https://gateway.example/gateway.do", keyB64), &fakeGateway{
		notify: &NotifyResult{OrderNo: "DTPALN1", GatewayTradeNo: "G1", Amount: decimal.NewFromInt(1), Success: true},
	})
	s.newGateway = newGatewayForChannel // real factory must run
	o := seedPendingOrder(orders, "DTPALN1", decimal.NewFromInt(1))
	o.Channel = ChannelAlipay

	handled, err := s.HandleNotifyForChannel(context.Background(), ChannelAlipay, map[string]string{"out_trade_no": "DTPALN1"})
	if err == nil {
		t.Fatal("alipay notify must fail closed until TH-P1-AL-03")
	}
	if handled {
		t.Fatal("failed-closed notify must not settle")
	}
	if o.Status != paymentorder.StatusPending {
		t.Fatalf("order must stay pending, got %s", o.Status)
	}
	if wallets.topupCount != 0 {
		t.Fatalf("failed-closed notify must not call the wallet, got %d credits", wallets.topupCount)
	}
}
