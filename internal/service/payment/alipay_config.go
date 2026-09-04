package payment

// TH-P1-AL-01 — Alipay merchant configuration loading and validation.
//
// Configuration lives in the settings store under the keys listed below,
// with SEPARATE production and sandbox value sets selected by the
// alipay_sandbox flag:
//
//	alipay_sandbox               mode selector (bool)
//	alipay_app_id                production merchant app id
//	alipay_private_key           production merchant private key (SECRET)
//	alipay_gateway_url           production gateway endpoint (optional;
//	                             defaults to the official endpoint)
//	alipay_sandbox_app_id        sandbox merchant app id
//	alipay_sandbox_private_key   sandbox merchant private key (SECRET)
//	alipay_sandbox_gateway_url   sandbox gateway endpoint (optional;
//	                             defaults to the official sandbox endpoint)
//
// Security policy: private key values are secrets. Validate and every error
// built from it are redacted BY CONSTRUCTION — diagnostics name the setting
// KEY only and never carry the configured value, so they are safe to log,
// return to handlers, and surface in PaymentInfo.ChannelError. No code path
// in this file may log an AlipayConfig value.

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

// Alipay settings keys (documented in docs/DEPLOYMENT.md).
const (
	SettingAlipaySandbox           = "alipay_sandbox"
	SettingAlipayAppID             = "alipay_app_id"
	SettingAlipayPrivateKey        = "alipay_private_key"
	SettingAlipayGatewayURL        = "alipay_gateway_url"
	SettingAlipaySandboxAppID      = "alipay_sandbox_app_id"
	SettingAlipaySandboxPrivateKey = "alipay_sandbox_private_key"
	SettingAlipaySandboxGatewayURL = "alipay_sandbox_gateway_url"
)

// Official Alipay gateway endpoints used when no override is configured.
const (
	alipayProductionGatewayURL = "https://openapi.alipay.com/gateway.do"
	alipaySandboxGatewayURL    = "https://openapi-sandbox.dl.alipaydev.com/gateway.do"
)

// AlipayConfig holds both production and sandbox merchant configuration.
// The Sandbox flag selects which value set is effective. PrivateKey fields
// are secrets and must never be logged, serialized into API responses, or
// rendered into error text.
type AlipayConfig struct {
	Sandbox bool

	AppID      string
	PrivateKey string
	GatewayURL string

	SandboxAppID      string
	SandboxPrivateKey string
	SandboxGatewayURL string
}

// alipayEffective is the credential set selected by the Sandbox flag, after
// applying the default gateway endpoint.
type alipayEffective struct {
	Sandbox    bool
	AppID      string
	PrivateKey string
	GatewayURL string
	appIDKey   string // settings key names, used for redacted diagnostics
	keyKey     string
	urlKey     string
}

// effective selects the production or sandbox value set. Empty gateway URLs
// fall back to the official endpoint for the selected mode.
func (c AlipayConfig) effective() alipayEffective {
	if c.Sandbox {
		gw := c.SandboxGatewayURL
		if gw == "" {
			gw = alipaySandboxGatewayURL
		}
		return alipayEffective{
			Sandbox: true, AppID: c.SandboxAppID, PrivateKey: c.SandboxPrivateKey, GatewayURL: gw,
			appIDKey: SettingAlipaySandboxAppID, keyKey: SettingAlipaySandboxPrivateKey, urlKey: SettingAlipaySandboxGatewayURL,
		}
	}
	gw := c.GatewayURL
	if gw == "" {
		gw = alipayProductionGatewayURL
	}
	return alipayEffective{
		AppID: c.AppID, PrivateKey: c.PrivateKey, GatewayURL: gw,
		appIDKey: SettingAlipayAppID, keyKey: SettingAlipayPrivateKey, urlKey: SettingAlipayGatewayURL,
	}
}

// Validate checks the effective (sandbox-selected) credential set.
//
// The returned error is redacted by construction: it names the offending
// settings keys only, never the configured values, so callers may log it
// and surface it to operators without leaking merchant secrets (TH-P1-AL-01
// AC-03).
func (c AlipayConfig) Validate() error {
	eff := c.effective()
	var missing []string
	if eff.AppID == "" {
		missing = append(missing, eff.appIDKey)
	}
	if eff.PrivateKey == "" {
		missing = append(missing, eff.keyKey)
	}
	if len(missing) > 0 {
		return fmt.Errorf("alipay: missing required settings: %s", strings.Join(missing, ", "))
	}
	if err := validateAlipayGatewayURL(eff.GatewayURL); err != nil {
		return fmt.Errorf("alipay: setting %s is not a valid https URL (%s)", eff.urlKey, err)
	}
	return nil
}

// validateAlipayGatewayURL enforces an absolute https endpoint with a host.
// The error describes the shape requirement only; it never echoes the
// configured value.
func validateAlipayGatewayURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("unparseable URL")
	}
	if u.Scheme != "https" {
		return fmt.Errorf("scheme must be https")
	}
	if u.Host == "" {
		return fmt.Errorf("missing host")
	}
	return nil
}

// loadAlipayConfig reads the Alipay value sets from the settings store.
func loadAlipayConfig(all map[string]json.RawMessage) AlipayConfig {
	return AlipayConfig{
		Sandbox:           rawBool(all, SettingAlipaySandbox),
		AppID:             rawStr(all, SettingAlipayAppID),
		PrivateKey:        rawStr(all, SettingAlipayPrivateKey),
		GatewayURL:        rawStr(all, SettingAlipayGatewayURL),
		SandboxAppID:      rawStr(all, SettingAlipaySandboxAppID),
		SandboxPrivateKey: rawStr(all, SettingAlipaySandboxPrivateKey),
		SandboxGatewayURL: rawStr(all, SettingAlipaySandboxGatewayURL),
	}
}
