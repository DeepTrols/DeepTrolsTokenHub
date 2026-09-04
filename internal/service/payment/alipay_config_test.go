package payment

// TH-P1-AL-01 — Alipay configuration loading and startup validation tests.
//
// AC-01: payment_channel=alipay with missing required fields reports a
// config error through the payment info check (and fail-fast at order
// creation).
// AC-02: with all required fields present, payment info reports the Alipay
// method available.
// AC-03: validation failures never carry private key or certificate body in
// their errors or logs — errors name the offending setting key only.

import (
	"bytes"
	"context"
	"errors"
	"log"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

const alipayTestSecret = "MIIEvTESTPRIVATEKEYBODYSECRET"

func validAlipayConfig() AlipayConfig {
	return AlipayConfig{
		AppID:      "2088testapp",
		PrivateKey: alipayTestSecret,
		// GatewayURL empty -> production default endpoint.
	}
}

func alipaySettingsWithData(s *fakeSettings, kv map[string]string) *fakeSettings {
	for k, v := range kv {
		s.data[k] = []byte(`"` + v + `"`)
	}
	return s
}

// completeAlipaySettings returns payment_channel=alipay with every required
// Alipay field present (production values).
func completeAlipaySettings() *fakeSettings {
	return alipaySettingsWithData(settingsWithChannel(ChannelAlipay), map[string]string{
		"alipay_app_id":      "2088testapp",
		"alipay_private_key": alipayTestSecret,
	})
}

// TestAlipayConfigValidateTable is the unit validation table: required-field
// presence, https URL shape, and sandbox/production value separation.
func TestAlipayConfigValidateTable(t *testing.T) {
	cases := []struct {
		name    string
		cfg     AlipayConfig
		wantErr string // substring of the expected error; "" means valid
	}{
		{name: "production complete (default gateway)", cfg: validAlipayConfig()},
		{
			name: "production explicit gateway",
			cfg:  AlipayConfig{AppID: "a", PrivateKey: "k", GatewayURL: "https://openapi.example.com/gateway.do"},
		},
		{
			name:    "missing app id",
			cfg:     AlipayConfig{PrivateKey: "k"},
			wantErr: "alipay_app_id",
		},
		{
			name:    "missing private key",
			cfg:     AlipayConfig{AppID: "a"},
			wantErr: "alipay_private_key",
		},
		{
			name:    "missing both",
			cfg:     AlipayConfig{},
			wantErr: "alipay_app_id",
		},
		{
			name:    "malformed gateway url",
			cfg:     AlipayConfig{AppID: "a", PrivateKey: "k", GatewayURL: "notaurl"},
			wantErr: "alipay_gateway_url",
		},
		{
			name:    "non-https gateway url",
			cfg:     AlipayConfig{AppID: "a", PrivateKey: "k", GatewayURL: "http://openapi.example.com/gateway.do"},
			wantErr: "alipay_gateway_url",
		},
		{
			name: "sandbox complete with production empty",
			cfg: AlipayConfig{
				Sandbox:           true,
				SandboxAppID:      "sandbox-app",
				SandboxPrivateKey: "sandbox-key",
			},
		},
		{
			name: "sandbox mode selects sandbox values",
			cfg: AlipayConfig{
				Sandbox:    true,
				AppID:      "prod-app",
				PrivateKey: "prod-key",
				// sandbox fields missing -> must fail despite complete production set
			},
			wantErr: "alipay_sandbox_app_id",
		},
		{
			name: "sandbox explicit gateway",
			cfg: AlipayConfig{
				Sandbox:           true,
				SandboxAppID:      "sandbox-app",
				SandboxPrivateKey: "sandbox-key",
				SandboxGatewayURL: "https://openapi-sandbox.example.com/gateway.do",
			},
		},
	}
	for _, tc := range cases {
		err := tc.cfg.Validate()
		if tc.wantErr == "" {
			if err != nil {
				t.Errorf("%s: expected valid, got %v", tc.name, err)
			}
			continue
		}
		if err == nil {
			t.Errorf("%s: expected error containing %q, got nil", tc.name, tc.wantErr)
			continue
		}
		if !strings.Contains(err.Error(), tc.wantErr) {
			t.Errorf("%s: error %q does not mention %q", tc.name, err.Error(), tc.wantErr)
		}
	}
}

// TestAlipayConfigValidateRedactsSecrets covers AC-03 at the error level:
// validation errors name the offending SETTING KEY and never carry the
// configured secret value (or the raw URL).
func TestAlipayConfigValidateRedactsSecrets(t *testing.T) {
	// Missing app id while a private key IS configured: the key body must
	// not leak into the complaint about the app id.
	err := AlipayConfig{PrivateKey: alipayTestSecret}.Validate()
	if err == nil {
		t.Fatal("expected validation error for missing app id")
	}
	if strings.Contains(err.Error(), alipayTestSecret) {
		t.Fatalf("validation error leaks private key body: %q", err.Error())
	}

	// Malformed URL while key + app id are configured: neither secret nor
	// raw URL value may leak.
	badURL := "http://insecure.example/gateway.do"
	err = AlipayConfig{AppID: "a", PrivateKey: alipayTestSecret, GatewayURL: badURL}.Validate()
	if err == nil {
		t.Fatal("expected validation error for malformed URL")
	}
	if strings.Contains(err.Error(), alipayTestSecret) || strings.Contains(err.Error(), badURL) {
		t.Fatalf("validation error leaks configured values: %q", err.Error())
	}
}

// TestInfoAlipayChannelMissingAppIDReportsConfigError covers AC-01: the
// payment info check reports the configuration error and advertises no
// payable methods.
func TestInfoAlipayChannelMissingAppIDReportsConfigError(t *testing.T) {
	// Private key present but app id missing — the most likely operator
	// mistake when switching payment_channel to alipay.
	settings := alipaySettingsWithData(settingsWithChannel(ChannelAlipay), map[string]string{
		"alipay_private_key": alipayTestSecret,
	})
	s, _, _ := newTestService(settings, &fakeGateway{payURL: "u"})
	info, err := s.Info(context.Background())
	if err != nil {
		t.Fatalf("Info: %v", err)
	}
	if info.ChannelError == "" {
		t.Fatal("expected non-empty channel_error for incomplete alipay config")
	}
	if !strings.Contains(info.ChannelError, "alipay_app_id") {
		t.Fatalf("channel_error should name the missing setting, got %q", info.ChannelError)
	}
	if strings.Contains(info.ChannelError, alipayTestSecret) {
		t.Fatalf("channel_error leaks private key body: %q", info.ChannelError)
	}
	if len(info.PayMethods) != 0 {
		t.Fatalf("invalid config must not advertise pay methods, got %+v", info.PayMethods)
	}
	if info.Channel != ChannelAlipay {
		t.Fatalf("channel = %q, want alipay", info.Channel)
	}
}

// TestInfoAlipayChannelReadyReportsMethod covers AC-02: complete config lets
// the payment info check report the Alipay method available.
func TestInfoAlipayChannelReadyReportsMethod(t *testing.T) {
	s, _, _ := newTestService(completeAlipaySettings(), &fakeGateway{payURL: "u"})
	info, err := s.Info(context.Background())
	if err != nil {
		t.Fatalf("Info: %v", err)
	}
	if info.ChannelError != "" {
		t.Fatalf("complete config must not report channel_error, got %q", info.ChannelError)
	}
	if len(info.PayMethods) != 1 || info.PayMethods[0].Type != "alipay" {
		t.Fatalf("expected alipay method available, got %+v", info.PayMethods)
	}
}

// TestInfoAlipayReadyButPaymentDisabled: the global enabled/compliance gate
// still applies even with a valid channel config.
func TestInfoAlipayReadyButPaymentDisabled(t *testing.T) {
	settings := completeAlipaySettings()
	settings.data["payment_enabled"] = []byte(`false`)
	s, _, _ := newTestService(settings, &fakeGateway{payURL: "u"})
	info, err := s.Info(context.Background())
	if err != nil {
		t.Fatalf("Info: %v", err)
	}
	if info.ChannelError != "" {
		t.Fatalf("config itself is valid, got channel_error %q", info.ChannelError)
	}
	if len(info.PayMethods) != 0 {
		t.Fatalf("disabled payment must not advertise methods, got %+v", info.PayMethods)
	}
}

// TestInfoEpayChannelErrorEmpty is the epay regression: the epay channel
// keeps its existing behavior and reports no channel_error.
func TestInfoEpayChannelErrorEmpty(t *testing.T) {
	for _, ch := range []string{"epay", ""} {
		s, _, _ := newTestService(settingsWithChannel(ch), &fakeGateway{payURL: "u"})
		info, err := s.Info(context.Background())
		if err != nil {
			t.Fatalf("Info(%q): %v", ch, err)
		}
		if info.ChannelError != "" {
			t.Fatalf("epay channel must not report channel_error, got %q", info.ChannelError)
		}
		if len(info.PayMethods) != 2 {
			t.Fatalf("epay channel: expected 2 methods, got %+v", info.PayMethods)
		}
	}
}

// TestInfoAlipayMalformedURLReportsConfigError injects a malformed gateway
// URL through settings and verifies the info check reports it by key name.
func TestInfoAlipayMalformedURLReportsConfigError(t *testing.T) {
	settings := alipaySettingsWithData(completeAlipaySettings(), map[string]string{
		"alipay_gateway_url": "http://insecure.example/gateway.do",
	})
	s, _, _ := newTestService(settings, &fakeGateway{payURL: "u"})
	info, err := s.Info(context.Background())
	if err != nil {
		t.Fatalf("Info: %v", err)
	}
	if !strings.Contains(info.ChannelError, "alipay_gateway_url") {
		t.Fatalf("channel_error should name the malformed URL setting, got %q", info.ChannelError)
	}
	if len(info.PayMethods) != 0 {
		t.Fatalf("malformed config must not advertise pay methods, got %+v", info.PayMethods)
	}
}

// TestCreateOrderAlipayFailFastOnMissingConfig covers fail-fast order
// creation: alipay channel with incomplete config rejects BEFORE any gateway
// call, creates no order row, and logs nothing that contains the secret
// (AC-03 at the log site).
func TestCreateOrderAlipayFailFastOnMissingConfig(t *testing.T) {
	settings := alipaySettingsWithData(settingsWithChannel(ChannelAlipay), map[string]string{
		"alipay_private_key": alipayTestSecret,
	})
	s, orders, _ := newTestService(settings, &fakeGateway{payURL: "https://pay/u"})
	s.newGateway = newGatewayForChannel // real factory must run

	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	_, err := s.CreateOrder(context.Background(), uuid.New(), decimal.NewFromInt(50), "alipay")
	if !errors.Is(err, ErrChannelConfigInvalid) {
		t.Fatalf("error = %v, want ErrChannelConfigInvalid", err)
	}
	if len(orders.byNo) != 0 {
		t.Fatalf("fail-fast rejection must create no order rows, got %d", len(orders.byNo))
	}
	if strings.Contains(buf.String(), alipayTestSecret) {
		t.Fatalf("log output leaks private key body: %q", buf.String())
	}
}

// TestCreateOrderAlipayValidConfigStillNotReady guards the AL-01 scope line:
// even with fully valid config the alipay provider adapter has not landed
// yet (TH-P1-AL-02), so order creation still fails closed — now past the
// config gate, at the factory — and creates no row.
func TestCreateOrderAlipayValidConfigStillNotReady(t *testing.T) {
	s, orders, _ := newTestService(completeAlipaySettings(), &fakeGateway{payURL: "https://pay/u"})
	s.newGateway = newGatewayForChannel
	_, err := s.CreateOrder(context.Background(), uuid.New(), decimal.NewFromInt(50), "alipay")
	if !errors.Is(err, ErrChannelNotReady) {
		t.Fatalf("error = %v, want ErrChannelNotReady", err)
	}
	if len(orders.byNo) != 0 {
		t.Fatalf("not-ready channel must create no order rows, got %d", len(orders.byNo))
	}
}

// TestConfigLoadsAlipaySettingsFromSettingsStore verifies the settings-key
// mapping (including sandbox selection) end to end through Service.config.
func TestConfigLoadsAlipaySettingsFromSettingsStore(t *testing.T) {
	settings := alipaySettingsWithData(settingsWithChannel(ChannelAlipay), map[string]string{
		"alipay_app_id":              "prod-app",
		"alipay_private_key":         "prod-key",
		"alipay_sandbox":             "true",
		"alipay_sandbox_app_id":      "sandbox-app",
		"alipay_sandbox_private_key": "sandbox-key",
	})
	s, _, _ := newTestService(settings, &fakeGateway{payURL: "u"})
	cfg, err := s.config(context.Background())
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	if !cfg.Alipay.Sandbox {
		t.Fatal("alipay_sandbox=true must select sandbox mode")
	}
	if err := cfg.Alipay.Validate(); err != nil {
		t.Fatalf("sandbox-complete config must validate, got %v", err)
	}
	// Sandbox mode must validate against sandbox values only.
	cfg.Alipay.SandboxAppID = ""
	if err := cfg.Alipay.Validate(); err == nil || !strings.Contains(err.Error(), "alipay_sandbox_app_id") {
		t.Fatalf("expected sandbox app id error, got %v", err)
	}
}
