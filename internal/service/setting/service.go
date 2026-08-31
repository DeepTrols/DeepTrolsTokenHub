package setting

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	settingrepo "github.com/deeptrols/api/internal/repository/setting"
	"github.com/shopspring/decimal"
)

// Supported system_settings keys. Keep this set in sync with the plan doc.
const (
	KeySiteName         = "site_name"
	KeyLogoURL          = "logo_url"
	KeyFaviconURL       = "favicon_url"
	KeyFooterText       = "footer_text"
	KeyNotice           = "notice"
	KeyAbout            = "about"
	KeyHomePageContent  = "home_page_content"
	KeyServerAddress    = "server_address"
	KeyUserAgreement    = "legal.user_agreement"
	KeyPrivacyPolicy    = "legal.privacy_policy"
	KeyContactEmail     = "contact_email"
	KeyHeaderNavModules = "header_nav_modules"
	KeySidebarModules   = "sidebar_modules"
	// Payment (B) configuration keys.
	KeyPaymentEnabled    = "payment_enabled"
	KeyPaymentCompliance = "payment_compliance_confirmed"
	KeyPayAddress        = "pay_address"
	KeyEpayID            = "epay_id"
	KeyEpayKey           = "epay_key"
	KeyMinTopup          = "min_topup"
	KeyMaxTopup          = "max_topup"
	KeyAmountOptions     = "amount_options"
	KeyCallbackBaseURL   = "callback_base_url"
	// Official-channel (M1-2) config keys.
	KeyPaymentChannel   = "payment_channel"
	KeyWechatAppID      = "wechat_app_id"
	KeyWechatMchID      = "wechat_mch_id"
	KeyWechatAPIV3Key   = "wechat_api_v3_key"
	KeyWechatCertSerial = "wechat_cert_serial_no"
	KeyWechatPrivateKey = "wechat_private_key"
	KeyAlipayAppID      = "alipay_app_id"
	KeyAlipayPrivateKey = "alipay_private_key"
	KeyAlipayPublicKey  = "alipay_public_key"
	KeyRegisterEnabled  = "register_enabled"
	// Operations / request-limit config keys (system management partitions).
	KeyChannelAutoDisableThreshold = "channel_auto_disable_threshold"
	KeyReconciliationIntervalHours = "reconciliation_interval_hours"
	KeyGatewayRateLimitRPM         = "gateway_rate_limit_rpm"
	KeyLoginRateLimitRPM           = "login_rate_limit_rpm"
	// Daily sign-in: enabled + min/max reward range.
	KeyCheckinEnabled  = "checkin.enabled"
	KeyCheckinMinQuota = "checkin.min_quota"
	KeyCheckinMaxQuota = "checkin.max_quota"
	// OAuth login (GitHub first).
	KeyOAuthGithubEnabled  = "oauth_github_enabled"
	KeyOAuthGithubClientID = "oauth_github_client_id"
	KeyOAuthGithubSecret   = "oauth_github_client_secret"
	KeyOAuthRedirectBase   = "oauth_redirect_base_url"
	KeyOAuthGithubTokenURL = "oauth_github_token_url"
	KeyOAuthGithubAPIURL   = "oauth_github_api_url"
	// WeChat QR login.
	KeyOAuthWechatEnabled  = "oauth_wechat_enabled"
	KeyOAuthWechatAppID    = "oauth_wechat_appid"
	KeyOAuthWechatSecret   = "oauth_wechat_secret"
	KeyOAuthWechatTokenURL = "oauth_wechat_token_url"
	KeyOAuthWechatUserURL  = "oauth_wechat_userinfo_url"
	// Google OAuth login.
	KeyOAuthGoogleEnabled      = "oauth_google_enabled"
	KeyOAuthGoogleClientID     = "oauth_google_client_id"
	KeyOAuthGoogleSecret       = "oauth_google_client_secret"
	KeyOAuthGoogleAuthorizeURL = "oauth_google_authorize_url"
	KeyOAuthGoogleTokenURL     = "oauth_google_token_url"
	KeyOAuthGoogleUserURL      = "oauth_google_userinfo_url"
	// Invite reward: CNY credited to both sides.
	KeyInviteReward = "invite_reward"
	// Model catalog settings.
	KeyModelsPublicVisible   = "models_public_visible"
	KeyNewModelDefaultStatus = "new_model_default_status"
	// User group pricing ratios: JSON array of
	// {"name": "...", "ratio": "0.8"} applied to sell prices per API-key group.
	KeyUserGroups = "user_groups"
	// Volume discount tiers (volume-based discount + monthly counter): JSON
	// array of {"min_tokens": N, "ratio": "0.95"} matched against the user's
	// cumulative token usage in the current GMT+8 month.
	KeyDiscountTiers = "discount_tiers"
)

// defaults are the JSON-encoded default values applied when a key is absent.
var defaults = map[string]json.RawMessage{
	KeySiteName:                    json.RawMessage(`"DeepTrols"`),
	KeyLogoURL:                     json.RawMessage(`""`),
	KeyFaviconURL:                  json.RawMessage(`""`),
	KeyFooterText:                  json.RawMessage(`""`),
	KeyNotice:                      json.RawMessage(`""`),
	KeyAbout:                       json.RawMessage(`""`),
	KeyHomePageContent:             json.RawMessage(`""`),
	KeyServerAddress:               json.RawMessage(`""`),
	KeyUserAgreement:               json.RawMessage(`""`),
	KeyPrivacyPolicy:               json.RawMessage(`""`),
	KeyContactEmail:                json.RawMessage(`""`),
	KeyHeaderNavModules:            json.RawMessage(`[]`),
	KeySidebarModules:              json.RawMessage(`[]`),
	KeyPaymentEnabled:              json.RawMessage(`false`),
	KeyPaymentCompliance:           json.RawMessage(`false`),
	KeyPayAddress:                  json.RawMessage(`""`),
	KeyEpayID:                      json.RawMessage(`""`),
	KeyEpayKey:                     json.RawMessage(`""`),
	KeyMinTopup:                    json.RawMessage(`"1"`),
	KeyMaxTopup:                    json.RawMessage(`"1000000"`),
	KeyAmountOptions:               json.RawMessage(`[10,50,100,200,500]`),
	KeyCallbackBaseURL:             json.RawMessage(`""`),
	KeyPaymentChannel:              json.RawMessage(`"epay"`),
	KeyWechatAppID:                 json.RawMessage(`""`),
	KeyWechatMchID:                 json.RawMessage(`""`),
	KeyWechatAPIV3Key:              json.RawMessage(`""`),
	KeyWechatCertSerial:            json.RawMessage(`""`),
	KeyWechatPrivateKey:            json.RawMessage(`""`),
	KeyAlipayAppID:                 json.RawMessage(`""`),
	KeyAlipayPrivateKey:            json.RawMessage(`""`),
	KeyAlipayPublicKey:             json.RawMessage(`""`),
	KeyRegisterEnabled:             json.RawMessage(`true`),
	KeyChannelAutoDisableThreshold: json.RawMessage(`"0"`),
	KeyReconciliationIntervalHours: json.RawMessage(`"24"`),
	KeyGatewayRateLimitRPM:         json.RawMessage(`"100"`),
	KeyLoginRateLimitRPM:           json.RawMessage(`"5"`),
	KeyCheckinEnabled:              json.RawMessage(`true`),
	KeyCheckinMinQuota:             json.RawMessage(`"1"`),
	KeyCheckinMaxQuota:             json.RawMessage(`"5"`),
	KeyOAuthGithubEnabled:          json.RawMessage(`false`),
	KeyOAuthGithubClientID:         json.RawMessage(`""`),
	KeyOAuthGithubSecret:           json.RawMessage(`""`),
	KeyOAuthRedirectBase:           json.RawMessage(`""`),
	KeyOAuthGithubTokenURL:         json.RawMessage(`"https://github.com/login/oauth/access_token"`),
	KeyOAuthGithubAPIURL:           json.RawMessage(`"https://api.github.com"`),
	KeyOAuthWechatEnabled:          json.RawMessage(`false`),
	KeyOAuthWechatAppID:            json.RawMessage(`""`),
	KeyOAuthWechatSecret:           json.RawMessage(`""`),
	KeyOAuthWechatTokenURL:         json.RawMessage(`"https://api.weixin.qq.com/sns/oauth2/access_token"`),
	KeyOAuthWechatUserURL:          json.RawMessage(`"https://api.weixin.qq.com/sns/userinfo"`),
	KeyOAuthGoogleEnabled:          json.RawMessage(`false`),
	KeyOAuthGoogleClientID:         json.RawMessage(`""`),
	KeyOAuthGoogleSecret:           json.RawMessage(`""`),
	KeyOAuthGoogleAuthorizeURL:     json.RawMessage(`"https://accounts.google.com/o/oauth2/v2/auth"`),
	KeyOAuthGoogleTokenURL:         json.RawMessage(`"https://oauth2.googleapis.com/token"`),
	KeyOAuthGoogleUserURL:          json.RawMessage(`"https://www.googleapis.com/oauth2/v2/userinfo"`),
	KeyInviteReward:                json.RawMessage(`"10"`),
	KeyModelsPublicVisible:         json.RawMessage(`true`),
	KeyNewModelDefaultStatus:       json.RawMessage(`"active"`),
	KeyUserGroups:                  json.RawMessage(`[]`),
	KeyDiscountTiers:               json.RawMessage(`[]`),
}

// CheckinConfig is the daily sign-in reward policy (mirrors new-api's
// checkin_setting options). Amounts are CNY decimals.
type CheckinConfig struct {
	Enabled  bool
	MinQuota decimal.Decimal
	MaxQuota decimal.Decimal
}

// publicKeys are the keys exposed on the unauthenticated /api/public/site.
var publicKeys = []string{
	KeySiteName, KeyLogoURL, KeyFaviconURL, KeyFooterText, KeyNotice,
	KeyAbout, KeyHomePageContent, KeyServerAddress, KeyContactEmail,
	KeyUserAgreement, KeyPrivacyPolicy,
}

// PublicSite is the unauthenticated, non-sensitive brand payload.
type PublicSite struct {
	SiteName        string `json:"site_name"`
	LogoURL         string `json:"logo_url"`
	FaviconURL      string `json:"favicon_url"`
	FooterText      string `json:"footer_text"`
	Notice          string `json:"notice"`
	About           string `json:"about"`
	HomePageContent string `json:"home_page_content"`
	ServerAddress   string `json:"server_address"`
	ContactEmail    string `json:"contact_email"`
	Legal           struct {
		UserAgreement string `json:"user_agreement"`
		PrivacyPolicy string `json:"privacy_policy"`
	} `json:"legal"`
	OAuthProviders []string `json:"oauth_providers"`
}

// Service resolves and persists runtime settings.
type Service struct {
	repo settingrepo.Repository
}

// NewService creates a Service backed by the given repository.
func NewService(repo settingrepo.Repository) *Service {
	return &Service{repo: repo}
}

// merge overlays persisted entries on top of defaults.
func merge(base map[string]json.RawMessage, entries []settingrepo.Entry) map[string]json.RawMessage {
	out := make(map[string]json.RawMessage, len(base)+len(entries))
	for k, v := range base {
		out[k] = v
	}
	for _, e := range entries {
		out[e.Key] = json.RawMessage(e.Value)
	}
	return out
}

// All returns every known key merged with defaults (admin-facing).
func (s *Service) All(ctx context.Context) (map[string]json.RawMessage, error) {
	entries, err := s.repo.All(ctx)
	if err != nil {
		return nil, err
	}
	return merge(defaults, entries), nil
}

// PublicSite returns the public brand payload merged with defaults.
func (s *Service) PublicSite(ctx context.Context) (*PublicSite, error) {
	all, err := s.All(ctx)
	if err != nil {
		return nil, err
	}
	site := &PublicSite{}
	read := func(key string) string {
		if v, ok := all[key]; ok {
			var s string
			if json.Unmarshal(v, &s) == nil {
				return s
			}
		}
		return ""
	}
	site.SiteName = read(KeySiteName)
	site.LogoURL = read(KeyLogoURL)
	site.FaviconURL = read(KeyFaviconURL)
	site.FooterText = read(KeyFooterText)
	site.Notice = read(KeyNotice)
	site.About = read(KeyAbout)
	site.HomePageContent = read(KeyHomePageContent)
	site.ServerAddress = read(KeyServerAddress)
	site.ContactEmail = read(KeyContactEmail)
	site.Legal.UserAgreement = read(KeyUserAgreement)
	site.Legal.PrivacyPolicy = read(KeyPrivacyPolicy)
	site.OAuthProviders = []string{}
	if rawSettingBool(all, KeyOAuthGithubEnabled) {
		site.OAuthProviders = append(site.OAuthProviders, "github")
	}
	if rawSettingBool(all, KeyOAuthWechatEnabled) {
		site.OAuthProviders = append(site.OAuthProviders, "wechat")
	}
	if rawSettingBool(all, KeyOAuthGoogleEnabled) {
		site.OAuthProviders = append(site.OAuthProviders, "google")
	}
	return site, nil
}

// rawSettingBool tolerates native JSON booleans and the JSON-string form
// written by the admin settings API.
func rawSettingBool(m map[string]json.RawMessage, key string) bool {
	v, ok := m[key]
	if !ok {
		return false
	}
	var b bool
	if json.Unmarshal(v, &b) == nil {
		return b
	}
	var s string
	if json.Unmarshal(v, &s) == nil {
		if p, err := strconv.ParseBool(s); err == nil {
			return p
		}
	}
	return false
}

// Update validates keys against the known set and upserts values.
func (s *Service) Update(ctx context.Context, kv map[string]string) error {
	entries := make([]settingrepo.Entry, 0, len(kv))
	for k, v := range kv {
		if _, ok := defaults[k]; !ok {
			return fmt.Errorf("setting: unknown key %q", k)
		}
		b, err := json.Marshal(v)
		if err != nil {
			return fmt.Errorf("setting: marshal %q: %w", k, err)
		}
		entries = append(entries, settingrepo.Entry{Key: k, Value: b})
	}
	return s.repo.Upsert(ctx, entries)
}

// CheckinConfig resolves the daily sign-in policy from system_settings,
// falling back to the built-in defaults when the keys are absent.
func (s *Service) CheckinConfig(ctx context.Context) (CheckinConfig, error) {
	all, err := s.All(ctx)
	if err != nil {
		return CheckinConfig{}, err
	}
	cfg := CheckinConfig{Enabled: true, MinQuota: decimal.NewFromInt(1), MaxQuota: decimal.NewFromInt(5)}
	if v, ok := all[KeyCheckinEnabled]; ok {
		// Accept both native JSON booleans (true) and the JSON-string form
		// ("true") written by the admin settings API.
		var b bool
		if json.Unmarshal(v, &b) == nil {
			cfg.Enabled = b
		} else {
			var s string
			if json.Unmarshal(v, &s) == nil {
				if parsed, perr := strconv.ParseBool(s); perr == nil {
					cfg.Enabled = parsed
				}
			}
		}
	}
	if v, ok := all[KeyCheckinMinQuota]; ok {
		var s string
		if json.Unmarshal(v, &s) == nil {
			if d, derr := decimal.NewFromString(s); derr == nil {
				cfg.MinQuota = d
			}
		}
	}
	if v, ok := all[KeyCheckinMaxQuota]; ok {
		var s string
		if json.Unmarshal(v, &s) == nil {
			if d, derr := decimal.NewFromString(s); derr == nil {
				cfg.MaxQuota = d
			}
		}
	}
	if cfg.MaxQuota.LessThan(cfg.MinQuota) {
		cfg.MaxQuota = cfg.MinQuota
	}
	return cfg, nil
}
