package payment

// TH-P1-AL-02 — official Alipay create-order client (alipay.trade.precreate,
// QR-code precreate flow).
//
// Scope: client call, request mapping, response parsing and error mapping
// for order creation. The request is RSA2-signed (SHA256withRSA) over the
// sorted non-empty parameters; the response envelope is parsed and only the
// success code 10000 with a non-empty qr_code yields a pay URL — every
// other outcome fails closed with a mapped error class.
//
// Error mapping:
//   - business failure / transport failure / malformed response ->
//     ErrAlipayProvider carrying the SANITIZED provider code/sub_code
//     (shape-clamped; raw message text never enters the error);
//   - context deadline / transport timeout -> ErrAlipayTimeout.
//
// Neither class ever produces order state or wallet effects: CreateOrder
// fails BEFORE the service persists the order row.
//
// Security policy: the merchant private key is a secret. It is parsed once
// at construction; parse errors are redacted by construction (no key
// material in error text) and no code path in this file logs key material.
//
// Out of scope until follow-up tasks, both fail closed:
//   - VerifyNotify: callback verification and settlement (TH-P1-AL-03/04);
//   - QueryOrder: the active query client (TH-P1-AL-05).

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
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/deeptrols/api/internal/pkg/metrics"
	"github.com/shopspring/decimal"
)

// Alipay protocol constants.
const (
	alipayMethodPrecreate      = "alipay.trade.precreate"
	alipayPrecreateResponseKey = "alipay_trade_precreate_response"
	alipayCodeSuccess          = "10000"
	alipayTimestampLayout      = "2006-01-02 15:04:05"
	alipaySignTypeRSA2         = "RSA2"
	// alipayMaxResponseBytes bounds response bodies read from the provider.
	alipayMaxResponseBytes = 1 << 20
)

// defaultAlipayClient is the transport used outside tests. The timeout is a
// backstop; callers normally bound the call via the request context.
var defaultAlipayClient = &http.Client{Timeout: 15 * time.Second}

// AlipayGateway is the official Alipay adapter. PrivateKey is parsed once
// at construction and never leaves the process.
type AlipayGateway struct {
	AppID      string
	PrivateKey *rsa.PrivateKey
	GatewayURL string
	NotifyURL  string
	// HTTPClient is injectable (integration tests substitute the TLS-test
	// client); nil selects defaultAlipayClient.
	HTTPClient *http.Client
}

func (g *AlipayGateway) Name() string { return ChannelAlipay }

// CreateOrder places a precreate order with Alipay and returns the QR pay
// URL (TH-P1-AL-02 AC-01). Every failure maps onto ErrAlipayProvider or
// ErrAlipayTimeout and carries one create-outcome metric observation.
func (g *AlipayGateway) CreateOrder(ctx context.Context, req CreateOrderRequest) (*CreateOrderResult, error) {
	start := time.Now()
	outcome := metrics.AlipayOutcomeSuccess
	defer func() { metrics.RecordAlipayCreateOrder(outcome, time.Since(start)) }()

	payURL, err := g.precreate(ctx, req)
	if err != nil {
		switch {
		case errors.Is(err, ErrAlipayTimeout):
			outcome = metrics.AlipayOutcomeTimeout
		case errors.Is(err, ErrAlipayProvider):
			outcome = metrics.AlipayOutcomeProviderError
		default:
			outcome = metrics.AlipayOutcomeError
		}
		return nil, err
	}
	return &CreateOrderResult{PayURL: payURL}, nil
}

func (g *AlipayGateway) precreate(ctx context.Context, req CreateOrderRequest) (string, error) {
	if g.PrivateKey == nil || g.AppID == "" || g.GatewayURL == "" {
		return "", ErrNotConfigured
	}
	params := buildAlipayParams(req, g.AppID, g.NotifyURL, time.Now())
	sign, err := alipaySignParams(params, g.PrivateKey)
	if err != nil {
		return "", err
	}
	params["sign"] = sign

	form := url.Values{}
	for k, v := range params {
		form.Set(k, v)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, g.GatewayURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("%w: request_build_failed", ErrAlipayProvider)
	}
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded;charset=utf-8")

	client := g.HTTPClient
	if client == nil {
		client = defaultAlipayClient
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		if isAlipayTimeout(ctx, err) {
			return "", fmt.Errorf("%w", ErrAlipayTimeout)
		}
		return "", fmt.Errorf("%w: transport", ErrAlipayProvider)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, alipayMaxResponseBytes))
	if err != nil {
		return "", fmt.Errorf("%w: response_read", ErrAlipayProvider)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%w: http_status=%d", ErrAlipayProvider, resp.StatusCode)
	}
	return parseAlipayPrecreateResponse(body)
}

// isAlipayTimeout classifies transport errors caused by an expired context
// or a client timeout.
func isAlipayTimeout(ctx context.Context, err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) && urlErr.Timeout() {
		return true
	}
	return false
}

// buildAlipayParams maps a channel-agnostic create request onto the Alipay
// common parameters plus the biz_content JSON (request mapper). Empty
// optional parameters are omitted so they never enter the signature.
func buildAlipayParams(req CreateOrderRequest, appID, notifyURL string, now time.Time) map[string]string {
	biz, _ := json.Marshal(map[string]string{
		"out_trade_no": req.OrderNo,
		"total_amount": formatAlipayAmount(req.Amount),
		"subject":      req.Subject,
	})
	params := map[string]string{
		"app_id":      appID,
		"method":      alipayMethodPrecreate,
		"format":      "JSON",
		"charset":     "utf-8",
		"sign_type":   alipaySignTypeRSA2,
		"timestamp":   now.Format(alipayTimestampLayout),
		"version":     "1.0",
		"biz_content": string(biz),
	}
	if notifyURL != "" {
		params["notify_url"] = notifyURL
	}
	return params
}

// formatAlipayAmount renders the CNY amount with exactly two decimals as
// required by the precreate total_amount field (Implementation Note).
func formatAlipayAmount(amount decimal.Decimal) string {
	return amount.StringFixed(2)
}

// alipaySignParams signs the request per the Alipay RSA2 rules: sorted
// k=v pairs joined by &, excluding the sign parameter itself and empty
// values, signed with SHA256withRSA and base64-encoded.
func alipaySignParams(params map[string]string, key *rsa.PrivateKey) (string, error) {
	keys := make([]string, 0, len(params))
	for k, v := range params {
		if k == "sign" || v == "" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+params[k])
	}
	digest := sha256.Sum256([]byte(strings.Join(parts, "&")))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		return "", fmt.Errorf("%w: signing_failed", ErrAlipayProvider)
	}
	return base64.StdEncoding.EncodeToString(sig), nil
}

// parseAlipayPrecreateResponse extracts the pay URL from the precreate
// envelope. Only code 10000 with a non-empty qr_code succeeds; every other
// shape fails closed with the sanitized provider code/sub_code kept in the
// error (Risk: provider identity must survive for debugging, sanitized).
func parseAlipayPrecreateResponse(body []byte) (string, error) {
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(body, &envelope); err != nil {
		return "", fmt.Errorf("%w: code=%s", ErrAlipayProvider, "invalid_json")
	}
	raw, ok := envelope[alipayPrecreateResponseKey]
	if !ok {
		return "", fmt.Errorf("%w: code=%s", ErrAlipayProvider, "missing_response_node")
	}
	var r struct {
		Code    string `json:"code"`
		SubCode string `json:"sub_code"`
		QRCode  string `json:"qr_code"`
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return "", fmt.Errorf("%w: code=%s", ErrAlipayProvider, "invalid_response_node")
	}
	if r.Code != alipayCodeSuccess {
		return "", fmt.Errorf("%w: code=%s sub_code=%s",
			ErrAlipayProvider, sanitizeAlipayCode(r.Code), sanitizeAlipayCode(r.SubCode))
	}
	if r.QRCode == "" {
		return "", fmt.Errorf("%w: code=%s sub_code=%s", ErrAlipayProvider, alipayCodeSuccess, "missing_qr_code")
	}
	return r.QRCode, nil
}

// sanitizeAlipayCode keeps a raw provider code/sub_code for diagnostics but
// clamps anything outside the bounded token shape, so provider text can
// never smuggle free-form content into errors, logs or labels.
func sanitizeAlipayCode(code string) string {
	if code == "" {
		return "unknown"
	}
	if len(code) > 64 {
		return "invalid"
	}
	for _, c := range code {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		case c == '_' || c == '.' || c == '-':
		default:
			return "invalid"
		}
	}
	return code
}

// parseAlipayPrivateKey accepts the merchant private key in the shapes the
// Alipay console issues: PEM ("PRIVATE KEY" PKCS#8 or "RSA PRIVATE KEY"
// PKCS#1) or the raw base64 key body without headers (the shape stored in
// the settings store). Errors are redacted by construction: they never
// carry key material.
func parseAlipayPrivateKey(raw string) (*rsa.PrivateKey, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, errors.New("alipay: private key is empty")
	}
	if block, _ := pem.Decode([]byte(trimmed)); block != nil {
		return parseAlipayKeyDER(block.Bytes)
	}
	der, err := base64.StdEncoding.DecodeString(trimmed)
	if err != nil {
		return nil, errors.New("alipay: private key is not valid PEM or base64")
	}
	return parseAlipayKeyDER(der)
}

func parseAlipayKeyDER(der []byte) (*rsa.PrivateKey, error) {
	if key, err := x509.ParsePKCS8PrivateKey(der); err == nil {
		if rsaKey, ok := key.(*rsa.PrivateKey); ok {
			return rsaKey, nil
		}
		return nil, errors.New("alipay: private key is not RSA")
	}
	if key, err := x509.ParsePKCS1PrivateKey(der); err == nil {
		return key, nil
	}
	return nil, errors.New("alipay: private key could not be parsed")
}

// VerifyNotify fails closed until TH-P1-AL-03 lands callback verification:
// an Alipay callback must never settle an order before its signature path
// exists.
func (g *AlipayGateway) VerifyNotify(ctx context.Context, params map[string]string) (*NotifyResult, error) {
	return nil, fmt.Errorf("%w: alipay notify verification lands in TH-P1-AL-03", ErrChannelNotReady)
}

// QueryOrder fails closed until TH-P1-AL-05 lands the query client
// (same sentinel contract as the epay adapter).
func (g *AlipayGateway) QueryOrder(ctx context.Context, orderNo string) (*QueryOrderResult, error) {
	return nil, fmt.Errorf("%w: alipay", ErrQueryUnsupported)
}
