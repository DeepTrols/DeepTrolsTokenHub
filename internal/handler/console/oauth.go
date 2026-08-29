package console

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/deeptrols/api/internal/app"
	"github.com/deeptrols/api/internal/domain"
	"github.com/deeptrols/api/internal/repository/user"
	"github.com/deeptrols/api/internal/service/setting"
	"github.com/google/uuid"
)

// oauthStateTTL mirrors new-api's 10-minute OAuth flow window.
const oauthStateTTL = 10 * time.Minute

type oauthSettings struct {
	Enabled      bool
	ClientID     string
	ClientSecret string
	RedirectBase string
	TokenURL     string
	APIURL       string
}

// HandleOAuthAuthorize starts a GitHub OAuth login: it redirects to GitHub's
// authorize endpoint with an HMAC-signed state (CSRF + expiry, no storage).
func HandleOAuthAuthorize(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cfg, err := resolveOAuthSettings(a, r)
		if err != nil || !cfg.Enabled || cfg.ClientID == "" {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "OAuth is not enabled"})
			return
		}
		state, err := buildOAuthState(cfg.ClientSecret, time.Now().Add(oauthStateTTL).Unix())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to start OAuth"})
			return
		}
		q := url.Values{}
		q.Set("client_id", cfg.ClientID)
		q.Set("redirect_uri", oauthCallbackURI(r, cfg, a))
		q.Set("scope", "read:user user:email")
		q.Set("state", state)
		http.Redirect(w, r, "https://github.com/login/oauth/authorize?"+q.Encode(), http.StatusFound)
	}
}

// HandleOAuthCallback completes the GitHub OAuth flow: validates state,
// exchanges the code for a token, resolves the GitHub user, find-or-creates a
// platform account and issues the JWT cookie before redirecting to the console.
func HandleOAuthCallback(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cfg, err := resolveOAuthSettings(a, r)
		fail := func(reason string) {
			http.Redirect(w, r, oauthLoginURL(r, cfg, a, "error", reason), http.StatusFound)
		}
		if err != nil || !cfg.Enabled || cfg.ClientID == "" || cfg.ClientSecret == "" {
			fail("oauth_disabled")
			return
		}
		state := r.URL.Query().Get("state")
		code := r.URL.Query().Get("code")
		if !verifyOAuthState(cfg.ClientSecret, state) {
			fail("invalid_state")
			return
		}
		if code == "" {
			fail("missing_code")
			return
		}
		token, err := exchangeGitHubToken(r.Context(), a, cfg, code, oauthCallbackURI(r, cfg, a))
		if err != nil {
			fail("token_exchange_failed")
			return
		}
		gh, err := fetchGitHubUser(r.Context(), a, token, cfg.APIURL)
		if err != nil {
			fail("userinfo_failed")
			return
		}
		if gh.Email == "" {
			fail("email_required")
			return
		}
		name := gh.Name
		if name == "" {
			name = gh.Login
		}
		u, _, err := findOrCreateOAuthUser(r.Context(), a, gh.Email, name)
		if err != nil {
			fail("account_failed")
			return
		}
		jwtToken, _, err := generateLoginJWT(a, u)
		if err != nil {
			fail("login_failed")
			return
		}
		completeConsoleLogin(a, w, r, u.ID, jwtToken)
		http.Redirect(w, r, oauthLoginURL(r, cfg, a, "oauth", "success"), http.StatusFound)
	}
}

// githubUser is the subset of the GitHub /user + /user/emails payloads.
type githubUser struct {
	Login string
	Name  string
	Email string
}

func exchangeGitHubToken(ctx context.Context, a *app.App, cfg oauthSettings, code, redirectURI string) (string, error) {
	form := url.Values{}
	form.Set("client_id", cfg.ClientID)
	form.Set("client_secret", cfg.ClientSecret)
	form.Set("code", code)
	form.Set("redirect_uri", redirectURI)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := oauthHTTPClient(a).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var out struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", err
	}
	if out.Error != "" || out.AccessToken == "" {
		return "", fmt.Errorf("github token exchange failed: %s", out.Error)
	}
	return out.AccessToken, nil
}

func fetchGitHubUser(ctx context.Context, a *app.App, token, apiURL string) (*githubUser, error) {
	userReq, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL+"/user", nil)
	if err != nil {
		return nil, err
	}
	userReq.Header.Set("Authorization", "Bearer "+token)
	userReq.Header.Set("Accept", "application/vnd.github+json")
	userResp, err := oauthHTTPClient(a).Do(userReq)
	if err != nil {
		return nil, err
	}
	defer userResp.Body.Close()
	if userResp.StatusCode >= 400 {
		return nil, fmt.Errorf("github user fetch failed: %d", userResp.StatusCode)
	}
	var out struct {
		Login string `json:"login"`
		Name  string `json:"name"`
		Email string `json:"email"`
	}
	if err := json.NewDecoder(userResp.Body).Decode(&out); err != nil {
		return nil, err
	}
	gh := &githubUser{Login: out.Login, Name: out.Name, Email: out.Email}

	// GitHub often leaves `email` null; the emails endpoint (user:email scope)
	// returns the primary verified address.
	if gh.Email == "" {
		emailsReq, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL+"/user/emails", nil)
		if err != nil {
			return nil, err
		}
		emailsReq.Header.Set("Authorization", "Bearer "+token)
		emailsReq.Header.Set("Accept", "application/vnd.github+json")
		emailsResp, err := oauthHTTPClient(a).Do(emailsReq)
		if err == nil {
			defer emailsResp.Body.Close()
			var emails []struct {
				Email    string `json:"email"`
				Primary  bool   `json:"primary"`
				Verified bool   `json:"verified"`
			}
			if json.NewDecoder(emailsResp.Body).Decode(&emails) == nil {
				for _, e := range emails {
					if e.Primary && e.Verified && e.Email != "" {
						gh.Email = e.Email
						break
					}
				}
			}
		}
	}
	return gh, nil
}

func findOrCreateOAuthUser(ctx context.Context, a *app.App, email, displayName string) (*domain.User, bool, error) {
	existing, err := a.Users.FindByEmail(ctx, email)
	if err == nil {
		return existing, false, nil
	}
	if !errors.Is(err, user.ErrNotFound) {
		return nil, false, err
	}
	now := time.Now().UTC()
	u := &domain.User{
		ID:           uuid.New(),
		Email:        email,
		PasswordHash: "",
		DisplayName:  displayName,
		Role:         "user",
		UserType:     domain.UserTypePersonal,
		Status:       domain.UserStatusActive,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := a.Users.Create(ctx, u); err != nil {
		return nil, false, err
	}
	bonus := "0"
	if a.Config != nil && a.Config.FakePayment {
		bonus = "1000"
	}
	if a.Pool != nil {
		_, _ = a.Pool.Exec(ctx,
			`INSERT INTO wallets (id, user_id, balance, frozen, currency, version, created_at, updated_at)
			 VALUES ($1, $2, $3, '0', 'CNY', 0, $4, $4)`,
			uuid.New(), u.ID, bonus, now)
	}
	return u, true, nil
}

// --- state signing (CSRF + expiry, no server-side storage) ---

func buildOAuthState(secret string, expiresAt int64) (string, error) {
	payload := strconv.FormatInt(expiresAt, 10)
	mac := hmac.New(sha256.New, []byte(secret))
	if _, err := mac.Write([]byte(payload)); err != nil {
		return "", err
	}
	sig := hex.EncodeToString(mac.Sum(nil))
	return base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." + sig, nil
}

func verifyOAuthState(secret, state string) bool {
	parts := strings.SplitN(state, ".", 2)
	if len(parts) != 2 {
		return false
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return false
	}
	expiresAt, err := strconv.ParseInt(string(raw), 10, 64)
	if err != nil || time.Now().Unix() > expiresAt {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(raw)
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(parts[1]))
}

// --- settings + URL helpers ---

func resolveOAuthSettings(a *app.App, r *http.Request) (oauthSettings, error) {
	if a == nil || a.Settings == nil {
		return oauthSettings{}, fmt.Errorf("settings unavailable")
	}
	all, err := a.Settings.All(r.Context())
	if err != nil {
		return oauthSettings{}, err
	}
	return oauthSettings{
		Enabled:      rawSettingBool(all, setting.KeyOAuthGithubEnabled),
		ClientID:     rawSettingString(all, setting.KeyOAuthGithubClientID),
		ClientSecret: rawSettingString(all, setting.KeyOAuthGithubSecret),
		RedirectBase: rawSettingString(all, setting.KeyOAuthRedirectBase),
		TokenURL:     defaultString(rawSettingString(all, setting.KeyOAuthGithubTokenURL), "https://github.com/login/oauth/access_token"),
		APIURL:       defaultString(rawSettingString(all, setting.KeyOAuthGithubAPIURL), "https://api.github.com"),
	}, nil
}

func defaultString(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

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

func rawSettingString(m map[string]json.RawMessage, key string) string {
	v, ok := m[key]
	if !ok {
		return ""
	}
	var s string
	if json.Unmarshal(v, &s) == nil {
		return s
	}
	return ""
}

func oauthHTTPClient(a *app.App) *http.Client {
	if a != nil && a.HttpClient != nil {
		return a.HttpClient
	}
	return http.DefaultClient
}

func oauthScheme(a *app.App) string {
	if a != nil && a.Config != nil && a.Config.Cookie.Secure {
		return "https"
	}
	return "http"
}

func oauthBase(r *http.Request, cfg oauthSettings, a *app.App) string {
	if cfg.RedirectBase != "" {
		return strings.TrimRight(cfg.RedirectBase, "/")
	}
	return oauthScheme(a) + "://" + r.Host
}

func oauthCallbackURI(r *http.Request, cfg oauthSettings, a *app.App) string {
	return oauthBase(r, cfg, a) + "/api/oauth/github/callback"
}

func oauthLoginURL(r *http.Request, cfg oauthSettings, a *app.App, key, value string) string {
	return oauthBase(r, cfg, a) + "/login?" + key + "=" + url.QueryEscape(value)
}

// --- WeChat QR login (new-api wechat oauth parity) ---

type wechatSettings struct {
	Enabled      bool
	AppID        string
	Secret       string
	TokenURL     string
	UserURL      string
	RedirectBase string
}

func resolveWechatSettings(a *app.App, r *http.Request) (wechatSettings, error) {
	if a == nil || a.Settings == nil {
		return wechatSettings{}, fmt.Errorf("settings unavailable")
	}
	all, err := a.Settings.All(r.Context())
	if err != nil {
		return wechatSettings{}, err
	}
	return wechatSettings{
		Enabled: rawSettingBool(all, setting.KeyOAuthWechatEnabled),
		AppID:   rawSettingString(all, setting.KeyOAuthWechatAppID),
		Secret:  rawSettingString(all, setting.KeyOAuthWechatSecret),
		TokenURL: defaultString(rawSettingString(all, setting.KeyOAuthWechatTokenURL),
			"https://api.weixin.qq.com/sns/oauth2/access_token"),
		UserURL: defaultString(rawSettingString(all, setting.KeyOAuthWechatUserURL),
			"https://api.weixin.qq.com/sns/userinfo"),
		RedirectBase: rawSettingString(all, setting.KeyOAuthRedirectBase),
	}, nil
}

// HandleWechatAuthorize redirects to the WeChat QR connect page (snsapi_login).
func HandleWechatAuthorize(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cfg, err := resolveWechatSettings(a, r)
		if err != nil || !cfg.Enabled || cfg.AppID == "" {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "OAuth is not enabled"})
			return
		}
		state, err := buildOAuthState(cfg.Secret, time.Now().Add(oauthStateTTL).Unix())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to start OAuth"})
			return
		}
		q := url.Values{}
		q.Set("appid", cfg.AppID)
		q.Set("redirect_uri", oauthBase(r, oauthSettings{RedirectBase: cfg.RedirectBase}, a)+"/api/oauth/wechat/callback")
		q.Set("response_type", "code")
		q.Set("scope", "snsapi_login")
		q.Set("state", state)
		http.Redirect(w, r, "https://open.weixin.qq.com/connect/qrconnect?"+q.Encode()+"#wechat_redirect", http.StatusFound)
	}
}

// HandleWechatCallback completes the WeChat QR login: exchanges the code for
// access_token + openid, fetches the profile and find-or-creates an account
// with a synthetic `wechat_{openid}@oauth.local` email (WeChat does not
// provide email addresses).
func HandleWechatCallback(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cfg, err := resolveWechatSettings(a, r)
		fail := func(reason string) {
			http.Redirect(w, r, oauthLoginURL(r, oauthSettings{RedirectBase: cfg.RedirectBase}, a, "error", reason), http.StatusFound)
		}
		if err != nil || !cfg.Enabled || cfg.AppID == "" || cfg.Secret == "" {
			fail("oauth_disabled")
			return
		}
		state := r.URL.Query().Get("state")
		code := r.URL.Query().Get("code")
		if !verifyOAuthState(cfg.Secret, state) {
			fail("invalid_state")
			return
		}
		if code == "" {
			fail("missing_code")
			return
		}
		accessToken, openid, err := exchangeWechatToken(r.Context(), a, cfg, code)
		if err != nil {
			fail("token_exchange_failed")
			return
		}
		nickname, err := fetchWechatUser(r.Context(), a, cfg, accessToken, openid)
		if err != nil {
			fail("userinfo_failed")
			return
		}
		email := "wechat_" + openid + "@oauth.local"
		if nickname == "" {
			nickname = "微信用户"
		}
		u, _, err := findOrCreateOAuthUser(r.Context(), a, email, nickname)
		if err != nil {
			fail("account_failed")
			return
		}
		jwtToken, _, err := generateLoginJWT(a, u)
		if err != nil {
			fail("login_failed")
			return
		}
		completeConsoleLogin(a, w, r, u.ID, jwtToken)
		http.Redirect(w, r, oauthLoginURL(r, oauthSettings{RedirectBase: cfg.RedirectBase}, a, "oauth", "success"), http.StatusFound)
	}
}

func exchangeWechatToken(ctx context.Context, a *app.App, cfg wechatSettings, code string) (string, string, error) {
	q := url.Values{}
	q.Set("appid", cfg.AppID)
	q.Set("secret", cfg.Secret)
	q.Set("code", code)
	q.Set("grant_type", "authorization_code")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cfg.TokenURL+"?"+q.Encode(), nil)
	if err != nil {
		return "", "", err
	}
	resp, err := oauthHTTPClient(a).Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	var out struct {
		AccessToken string `json:"access_token"`
		OpenID      string `json:"openid"`
		Errcode     int    `json:"errcode"`
		Errmsg      string `json:"errmsg"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", "", err
	}
	if out.Errcode != 0 || out.AccessToken == "" || out.OpenID == "" {
		return "", "", fmt.Errorf("wechat token exchange failed: %s", out.Errmsg)
	}
	return out.AccessToken, out.OpenID, nil
}

func fetchWechatUser(ctx context.Context, a *app.App, cfg wechatSettings, accessToken, openid string) (string, error) {
	q := url.Values{}
	q.Set("access_token", accessToken)
	q.Set("openid", openid)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cfg.UserURL+"?"+q.Encode(), nil)
	if err != nil {
		return "", err
	}
	resp, err := oauthHTTPClient(a).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var out struct {
		Nickname string `json:"nickname"`
		Errcode  int    `json:"errcode"`
		Errmsg   string `json:"errmsg"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	if out.Errcode != 0 {
		return "", fmt.Errorf("wechat userinfo failed: %s", out.Errmsg)
	}
	return out.Nickname, nil
}

// --- Google OAuth login (new-api oauth parity) ---

type googleSettings struct {
	Enabled      bool
	ClientID     string
	ClientSecret string
	AuthorizeURL string
	TokenURL     string
	UserURL      string
	RedirectBase string
}

func resolveGoogleSettings(a *app.App, r *http.Request) (googleSettings, error) {
	if a == nil || a.Settings == nil {
		return googleSettings{}, fmt.Errorf("settings unavailable")
	}
	all, err := a.Settings.All(r.Context())
	if err != nil {
		return googleSettings{}, err
	}
	return googleSettings{
		Enabled:      rawSettingBool(all, setting.KeyOAuthGoogleEnabled),
		ClientID:     rawSettingString(all, setting.KeyOAuthGoogleClientID),
		ClientSecret: rawSettingString(all, setting.KeyOAuthGoogleSecret),
		AuthorizeURL: defaultString(rawSettingString(all, setting.KeyOAuthGoogleAuthorizeURL),
			"https://accounts.google.com/o/oauth2/v2/auth"),
		TokenURL: defaultString(rawSettingString(all, setting.KeyOAuthGoogleTokenURL),
			"https://oauth2.googleapis.com/token"),
		UserURL: defaultString(rawSettingString(all, setting.KeyOAuthGoogleUserURL),
			"https://www.googleapis.com/oauth2/v2/userinfo"),
		RedirectBase: rawSettingString(all, setting.KeyOAuthRedirectBase),
	}, nil
}

// HandleGoogleAuthorize redirects to Google's OAuth consent screen.
func HandleGoogleAuthorize(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cfg, err := resolveGoogleSettings(a, r)
		if err != nil || !cfg.Enabled || cfg.ClientID == "" {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "OAuth is not enabled"})
			return
		}
		state, err := buildOAuthState(cfg.ClientSecret, time.Now().Add(oauthStateTTL).Unix())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to start OAuth"})
			return
		}
		q := url.Values{}
		q.Set("client_id", cfg.ClientID)
		q.Set("redirect_uri", oauthBase(r, oauthSettings{RedirectBase: cfg.RedirectBase}, a)+"/api/oauth/google/callback")
		q.Set("response_type", "code")
		q.Set("scope", "openid email profile")
		q.Set("state", state)
		http.Redirect(w, r, cfg.AuthorizeURL+"?"+q.Encode(), http.StatusFound)
	}
}

// HandleGoogleCallback completes the Google OAuth flow: validates state,
// exchanges the code for an access token, resolves the Google profile and
// find-or-creates a platform account with the verified email.
func HandleGoogleCallback(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cfg, err := resolveGoogleSettings(a, r)
		fail := func(reason string) {
			http.Redirect(w, r, oauthLoginURL(r, oauthSettings{RedirectBase: cfg.RedirectBase}, a, "error", reason), http.StatusFound)
		}
		if err != nil || !cfg.Enabled || cfg.ClientID == "" || cfg.ClientSecret == "" {
			fail("oauth_disabled")
			return
		}
		state := r.URL.Query().Get("state")
		code := r.URL.Query().Get("code")
		if !verifyOAuthState(cfg.ClientSecret, state) {
			fail("invalid_state")
			return
		}
		if code == "" {
			fail("missing_code")
			return
		}
		redirectURI := oauthBase(r, oauthSettings{RedirectBase: cfg.RedirectBase}, a) + "/api/oauth/google/callback"
		token, err := exchangeGoogleToken(r.Context(), a, cfg, code, redirectURI)
		if err != nil {
			fail("token_exchange_failed")
			return
		}
		profile, err := fetchGoogleUser(r.Context(), a, cfg, token)
		if err != nil {
			fail("userinfo_failed")
			return
		}
		if profile.Email == "" {
			fail("email_required")
			return
		}
		name := profile.Name
		if name == "" {
			name = strings.Split(profile.Email, "@")[0]
		}
		u, _, err := findOrCreateOAuthUser(r.Context(), a, profile.Email, name)
		if err != nil {
			fail("account_failed")
			return
		}
		jwtToken, _, err := generateLoginJWT(a, u)
		if err != nil {
			fail("login_failed")
			return
		}
		completeConsoleLogin(a, w, r, u.ID, jwtToken)
		http.Redirect(w, r, oauthLoginURL(r, oauthSettings{RedirectBase: cfg.RedirectBase}, a, "oauth", "success"), http.StatusFound)
	}
}

// googleUser is the subset of the Google userinfo (OAuth2 v2) payload.
type googleUser struct {
	Name  string
	Email string
}

func exchangeGoogleToken(ctx context.Context, a *app.App, cfg googleSettings, code, redirectURI string) (string, error) {
	form := url.Values{}
	form.Set("client_id", cfg.ClientID)
	form.Set("client_secret", cfg.ClientSecret)
	form.Set("code", code)
	form.Set("redirect_uri", redirectURI)
	form.Set("grant_type", "authorization_code")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := oauthHTTPClient(a).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var out struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", err
	}
	if out.Error != "" || out.AccessToken == "" {
		return "", fmt.Errorf("google token exchange failed: %s", out.Error)
	}
	return out.AccessToken, nil
}

func fetchGoogleUser(ctx context.Context, a *app.App, cfg googleSettings, token string) (*googleUser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cfg.UserURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	resp, err := oauthHTTPClient(a).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("google userinfo fetch failed: %d", resp.StatusCode)
	}
	var out struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return &googleUser{Name: out.Name, Email: out.Email}, nil
}
