package console

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/deeptrols/api/internal/app"
	"github.com/deeptrols/api/internal/config"
	"github.com/deeptrols/api/internal/repository/membership"
	settingrepo "github.com/deeptrols/api/internal/repository/setting"
	"github.com/deeptrols/api/internal/repository/testutil"
	"github.com/deeptrols/api/internal/repository/user"
	"github.com/deeptrols/api/internal/repository/wallet"
	settingsvc "github.com/deeptrols/api/internal/service/setting"
)

type fakeOAuthSettingRepo struct {
	entries []settingrepo.Entry
}

func (f *fakeOAuthSettingRepo) All(ctx context.Context) ([]settingrepo.Entry, error) {
	return f.entries, nil
}

func (f *fakeOAuthSettingRepo) Get(ctx context.Context, keys ...string) ([]settingrepo.Entry, error) {
	return f.entries, nil
}

func (f *fakeOAuthSettingRepo) Upsert(ctx context.Context, entries []settingrepo.Entry) error {
	f.entries = append(f.entries, entries...)
	return nil
}

func oauthEntries(enabled bool, clientID, secret, tokenURL, apiURL string) []settingrepo.Entry {
	return []settingrepo.Entry{
		{Key: "oauth_github_enabled", Value: json.RawMessage(fmt.Sprintf(`%t`, enabled))},
		{Key: "oauth_github_client_id", Value: json.RawMessage(`"` + clientID + `"`)},
		{Key: "oauth_github_client_secret", Value: json.RawMessage(`"` + secret + `"`)},
		{Key: "oauth_github_token_url", Value: json.RawMessage(`"` + tokenURL + `"`)},
		{Key: "oauth_github_api_url", Value: json.RawMessage(`"` + apiURL + `"`)},
	}
}

func oauthApp(t *testing.T, entries []settingrepo.Entry, withDB bool) *app.App {
	t.Helper()
	cfg := &config.Config{
		JWT:    config.JWTConfig{Secret: "oauth-test-jwt-secret-32-bytes!", ExpiryHours: 24},
		Cookie: config.CookieConfig{Name: "dt_session", Secure: false, MaxAgeSeconds: 86400, SameSite: "lax"},
	}
	a := &app.App{
		Config:   cfg,
		Settings: settingsvc.NewService(&fakeOAuthSettingRepo{entries: entries}),
	}
	if withDB {
		pool := testutil.SetupPool(t)
		testutil.TruncateAll(t, pool)
		a.Pool = pool
		a.Users = user.NewPostgresRepository(pool)
		a.Memberships = membership.NewPostgresRepository(pool)
		// The OAuth callback provisions the new user's wallet through the
		// ledgered path (B2 fix); production always wires a.Wallets, so the
		// test app must too or wallet assertions see a nil repository.
		a.Wallets = wallet.NewPostgresRepository(pool)
	}
	return a
}

func TestBuildAndVerifyOAuthState(t *testing.T) {
	state, err := buildOAuthState("secret", time.Now().Add(time.Hour).Unix())
	if err != nil {
		t.Fatalf("buildOAuthState: %v", err)
	}
	if !verifyOAuthState("secret", state) {
		t.Fatal("expected state to verify")
	}
	if verifyOAuthState("other-secret", state) {
		t.Fatal("state must not verify with a different secret")
	}
	if verifyOAuthState("secret", "garbage") {
		t.Fatal("garbage state must not verify")
	}
}

func TestVerifyOAuthState_Expired(t *testing.T) {
	state, _ := buildOAuthState("secret", time.Now().Add(-time.Minute).Unix())
	if verifyOAuthState("secret", state) {
		t.Fatal("expired state must not verify")
	}
}

func TestHandleOAuthAuthorize_Disabled(t *testing.T) {
	a := oauthApp(t, oauthEntries(false, "id", "secret", "https://t", "https://a"), false)
	req := httptest.NewRequest(http.MethodGet, "/api/oauth/github/authorize", nil)
	w := httptest.NewRecorder()
	HandleOAuthAuthorize(a).ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestHandleOAuthAuthorize_RedirectsToGitHub(t *testing.T) {
	a := oauthApp(t, oauthEntries(true, "client-123", "secret", "https://t", "https://a"), false)
	req := httptest.NewRequest(http.MethodGet, "/api/oauth/github/authorize", nil)
	req.Host = "console.example.com"
	w := httptest.NewRecorder()
	HandleOAuthAuthorize(a).ServeHTTP(w, req)
	if w.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusFound)
	}
	loc := w.Header().Get("Location")
	if !strings.HasPrefix(loc, "https://github.com/login/oauth/authorize?") {
		t.Fatalf("location = %q", loc)
	}
	if !strings.Contains(loc, "client_id=client-123") {
		t.Fatalf("missing client_id: %q", loc)
	}
	if !strings.Contains(loc, "redirect_uri=http%3A%2F%2Fconsole.example.com%2Fapi%2Foauth%2Fgithub%2Fcallback") {
		t.Fatalf("missing redirect_uri: %q", loc)
	}
	if !strings.Contains(loc, "state=") {
		t.Fatalf("missing state: %q", loc)
	}
}

func TestHandleOAuthCallback_CreatesUserAndSetsCookie(t *testing.T) {
	// Mock GitHub token + user + emails endpoints.
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"gho_test","token_type":"bearer"}`))
	}))
	defer tokenServer.Close()
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/user/emails"):
			_, _ = w.Write([]byte(`[{"email":"dev@example.com","primary":true,"verified":true}]`))
		default:
			_, _ = w.Write([]byte(`{"login":"dev","name":"Dev User","email":null}`))
		}
	}))
	defer apiServer.Close()

	a := oauthApp(t, oauthEntries(true, "client-123", "secret", tokenServer.URL, apiServer.URL), true)

	state, err := buildOAuthState("secret", time.Now().Add(time.Minute).Unix())
	if err != nil {
		t.Fatalf("state: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/oauth/github/callback?code=abc&state="+state, nil)
	req.Host = "console.example.com"
	w := httptest.NewRecorder()
	HandleOAuthCallback(a).ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("status = %d, want redirect; body = %s", w.Code, w.Body.String())
	}
	loc := w.Header().Get("Location")
	if !strings.Contains(loc, "oauth=success") {
		t.Fatalf("location = %q", loc)
	}
	if cookie := w.Header().Get("Set-Cookie"); !strings.Contains(cookie, "dt_session=") {
		t.Fatalf("expected auth cookie, got %q", cookie)
	}

	// User was created with the GitHub email; wallet exists.
	u, err := a.Users.FindByEmail(context.Background(), "dev@example.com")
	if err != nil {
		t.Fatalf("FindByEmail: %v", err)
	}
	if u.DisplayName != "Dev User" {
		t.Fatalf("display name = %q", u.DisplayName)
	}
	var walletCount int
	if err := a.Pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM wallets WHERE user_id=$1`, u.ID).Scan(&walletCount); err != nil {
		t.Fatalf("wallet query: %v", err)
	}
	if walletCount != 1 {
		t.Fatalf("expected 1 wallet, got %d", walletCount)
	}
}

func TestHandleOAuthCallback_ExistingUserLogsIn(t *testing.T) {
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"access_token":"gho_test"}`))
	}))
	defer tokenServer.Close()
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"login":"dev","name":"Dev","email":"existing@example.com"}`))
	}))
	defer apiServer.Close()

	a := oauthApp(t, oauthEntries(true, "client-123", "secret", tokenServer.URL, apiServer.URL), true)
	seedUserForWalletTest(t, a, "existing@example.com", "pass12345", "Existing")

	state, _ := buildOAuthState("secret", time.Now().Add(time.Minute).Unix())
	req := httptest.NewRequest(http.MethodGet, "/api/oauth/github/callback?code=abc&state="+state, nil)
	req.Host = "console.example.com"
	w := httptest.NewRecorder()
	HandleOAuthCallback(a).ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("status = %d; body = %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Header().Get("Set-Cookie"), "dt_session=") {
		t.Fatalf("expected auth cookie")
	}
	// No duplicate account.
	if _, err := a.Users.FindByEmail(context.Background(), "existing@example.com"); err != nil {
		t.Fatalf("existing user should remain: %v", err)
	}
}

func TestHandleOAuthCallback_InvalidStateRedirectsError(t *testing.T) {
	a := oauthApp(t, oauthEntries(true, "id", "secret", "https://t", "https://a"), false)
	req := httptest.NewRequest(http.MethodGet, "/api/oauth/github/callback?code=abc&state=bad", nil)
	req.Host = "console.example.com"
	w := httptest.NewRecorder()
	HandleOAuthCallback(a).ServeHTTP(w, req)
	if w.Code != http.StatusFound {
		t.Fatalf("status = %d, want redirect", w.Code)
	}
	if !strings.Contains(w.Header().Get("Location"), "error=invalid_state") {
		t.Fatalf("location = %q", w.Header().Get("Location"))
	}
}

func wechatEntries(enabled bool, appid, secret, tokenURL, userURL string) []settingrepo.Entry {
	return []settingrepo.Entry{
		{Key: "oauth_wechat_enabled", Value: json.RawMessage(fmt.Sprintf(`%t`, enabled))},
		{Key: "oauth_wechat_appid", Value: json.RawMessage(`"` + appid + `"`)},
		{Key: "oauth_wechat_secret", Value: json.RawMessage(`"` + secret + `"`)},
		{Key: "oauth_wechat_token_url", Value: json.RawMessage(`"` + tokenURL + `"`)},
		{Key: "oauth_wechat_userinfo_url", Value: json.RawMessage(`"` + userURL + `"`)},
	}
}

func TestHandleWechatAuthorize_Disabled(t *testing.T) {
	a := oauthApp(t, wechatEntries(false, "wxid", "secret", "https://t", "https://u"), false)
	req := httptest.NewRequest(http.MethodGet, "/api/oauth/wechat/authorize", nil)
	w := httptest.NewRecorder()
	HandleWechatAuthorize(a).ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestHandleWechatAuthorize_RedirectsToQR(t *testing.T) {
	a := oauthApp(t, wechatEntries(true, "wxid123", "secret", "https://t", "https://u"), false)
	req := httptest.NewRequest(http.MethodGet, "/api/oauth/wechat/authorize", nil)
	req.Host = "console.example.com"
	w := httptest.NewRecorder()
	HandleWechatAuthorize(a).ServeHTTP(w, req)
	if w.Code != http.StatusFound {
		t.Fatalf("status = %d", w.Code)
	}
	loc := w.Header().Get("Location")
	if !strings.HasPrefix(loc, "https://open.weixin.qq.com/connect/qrconnect?") {
		t.Fatalf("location = %q", loc)
	}
	if !strings.Contains(loc, "appid=wxid123") || !strings.Contains(loc, "scope=snsapi_login") || !strings.Contains(loc, "state=") {
		t.Fatalf("location missing params: %q", loc)
	}
}

func TestHandleWechatCallback_CreatesSyntheticUser(t *testing.T) {
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"wx_token","openid":"OPENID_123","expires_in":7200}`))
	}))
	defer tokenServer.Close()
	userServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"openid":"OPENID_123","nickname":"微信用户小王"}`))
	}))
	defer userServer.Close()

	a := oauthApp(t, wechatEntries(true, "wxid123", "secret", tokenServer.URL, userServer.URL), true)
	state, err := buildOAuthState("secret", time.Now().Add(time.Minute).Unix())
	if err != nil {
		t.Fatalf("state: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/oauth/wechat/callback?code=abc&state="+state, nil)
	req.Host = "console.example.com"
	w := httptest.NewRecorder()
	HandleWechatCallback(a).ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("status = %d; body = %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Header().Get("Set-Cookie"), "dt_session=") {
		t.Fatalf("expected auth cookie")
	}
	u, err := a.Users.FindByEmail(context.Background(), "wechat_OPENID_123@oauth.local")
	if err != nil {
		t.Fatalf("synthetic user not created: %v", err)
	}
	if u.DisplayName != "微信用户小王" {
		t.Fatalf("display name = %q", u.DisplayName)
	}
}

func googleEntries(enabled bool, clientID, secret, authorizeURL, tokenURL, userURL string) []settingrepo.Entry {
	return []settingrepo.Entry{
		{Key: "oauth_google_enabled", Value: json.RawMessage(fmt.Sprintf(`%t`, enabled))},
		{Key: "oauth_google_client_id", Value: json.RawMessage(`"` + clientID + `"`)},
		{Key: "oauth_google_client_secret", Value: json.RawMessage(`"` + secret + `"`)},
		{Key: "oauth_google_authorize_url", Value: json.RawMessage(`"` + authorizeURL + `"`)},
		{Key: "oauth_google_token_url", Value: json.RawMessage(`"` + tokenURL + `"`)},
		{Key: "oauth_google_userinfo_url", Value: json.RawMessage(`"` + userURL + `"`)},
	}
}

func TestHandleGoogleAuthorize_Disabled(t *testing.T) {
	a := oauthApp(t, googleEntries(false, "gid", "secret", "https://auth", "https://t", "https://u"), false)
	req := httptest.NewRequest(http.MethodGet, "/api/oauth/google/authorize", nil)
	w := httptest.NewRecorder()
	HandleGoogleAuthorize(a).ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestHandleGoogleAuthorize_RedirectsToGoogle(t *testing.T) {
	a := oauthApp(t, googleEntries(true, "gid123", "secret", "https://accounts.google.com/o/oauth2/v2/auth", "https://t", "https://u"), false)
	req := httptest.NewRequest(http.MethodGet, "/api/oauth/google/authorize", nil)
	req.Host = "console.example.com"
	w := httptest.NewRecorder()
	HandleGoogleAuthorize(a).ServeHTTP(w, req)
	if w.Code != http.StatusFound {
		t.Fatalf("status = %d", w.Code)
	}
	loc := w.Header().Get("Location")
	if !strings.HasPrefix(loc, "https://accounts.google.com/o/oauth2/v2/auth?") {
		t.Fatalf("location = %q", loc)
	}
	if !strings.Contains(loc, "client_id=gid123") || !strings.Contains(loc, "response_type=code") {
		t.Fatalf("location missing params: %q", loc)
	}
	if !strings.Contains(loc, "redirect_uri=http%3A%2F%2Fconsole.example.com%2Fapi%2Foauth%2Fgoogle%2Fcallback") {
		t.Fatalf("missing google redirect_uri: %q", loc)
	}
	if !strings.Contains(loc, "state=") {
		t.Fatalf("missing state: %q", loc)
	}
}

func TestHandleGoogleCallback_CreatesUserAndSetsCookie(t *testing.T) {
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"ya29.google_test","token_type":"Bearer","expires_in":3600}`))
	}))
	defer tokenServer.Close()
	userServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"111","email":"google.user@example.com","verified_email":true,"name":"Google User","picture":"https://example.com/p.png"}`))
	}))
	defer userServer.Close()

	a := oauthApp(t, googleEntries(true, "gid123", "secret", "https://auth", tokenServer.URL, userServer.URL), true)
	state, err := buildOAuthState("secret", time.Now().Add(time.Minute).Unix())
	if err != nil {
		t.Fatalf("state: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/oauth/google/callback?code=abc&state="+state, nil)
	req.Host = "console.example.com"
	w := httptest.NewRecorder()
	HandleGoogleCallback(a).ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("status = %d; body = %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Header().Get("Location"), "oauth=success") {
		t.Fatalf("location = %q", w.Header().Get("Location"))
	}
	if !strings.Contains(w.Header().Get("Set-Cookie"), "dt_session=") {
		t.Fatalf("expected auth cookie")
	}
	u, err := a.Users.FindByEmail(context.Background(), "google.user@example.com")
	if err != nil {
		t.Fatalf("google user not created: %v", err)
	}
	if u.DisplayName != "Google User" {
		t.Fatalf("display name = %q", u.DisplayName)
	}
}

func TestHandleGoogleCallback_InvalidStateRedirectsError(t *testing.T) {
	a := oauthApp(t, googleEntries(true, "gid", "secret", "https://auth", "https://t", "https://u"), false)
	req := httptest.NewRequest(http.MethodGet, "/api/oauth/google/callback?code=abc&state=bad", nil)
	req.Host = "console.example.com"
	w := httptest.NewRecorder()
	HandleGoogleCallback(a).ServeHTTP(w, req)
	if w.Code != http.StatusFound {
		t.Fatalf("status = %d, want redirect", w.Code)
	}
	if !strings.Contains(w.Header().Get("Location"), "error=invalid_state") {
		t.Fatalf("location = %q", w.Header().Get("Location"))
	}
}
