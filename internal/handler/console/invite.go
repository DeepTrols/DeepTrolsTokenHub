package console

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/deeptrols/api/internal/app"
	"github.com/deeptrols/api/internal/pkg/jwtutil"
	"github.com/deeptrols/api/internal/service/setting"
	"github.com/shopspring/decimal"
)

// generateInviteCode builds a unique 8-hex invite code (DTP-prefixed).
func generateInviteCode() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return "DTP" + hex.EncodeToString(b)
}

// inviteRewardSetting reads the configured CNY reward for each side of an
// invite (default 10).
func inviteRewardSetting(a *app.App, r *http.Request) decimal.Decimal {
	if a == nil || a.Settings == nil {
		return decimal.NewFromInt(10)
	}
	all, err := a.Settings.All(r.Context())
	if err != nil {
		return decimal.NewFromInt(10)
	}
	if v, ok := all[setting.KeyInviteReward]; ok {
		var s string
		if json.Unmarshal(v, &s) == nil {
			if d, derr := decimal.NewFromString(s); derr == nil {
				return d
			}
		}
	}
	return decimal.NewFromInt(10)
}

// consoleStringSetting reads a string system setting with a fallback.
func consoleStringSetting(a *app.App, r *http.Request, key, fallback string) string {
	if a == nil || a.Settings == nil {
		return fallback
	}
	all, err := a.Settings.All(r.Context())
	if err != nil {
		return fallback
	}
	if v, ok := all[key]; ok {
		var s string
		if json.Unmarshal(v, &s) == nil && s != "" {
			return s
		}
	}
	return fallback
}

// HandleInviteInfo returns the authenticated user's invite code, referral
// count and shareable register link.
func HandleInviteInfo(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, err := jwtutil.UserIDFromContext(r.Context())
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Not authenticated"})
			return
		}
		var code string
		_ = a.Pool.QueryRow(r.Context(),
			`SELECT invite_code FROM users WHERE id = $1`, userID).Scan(&code)
		if code == "" {
			code = generateInviteCode()
			_, _ = a.Pool.Exec(r.Context(),
				`UPDATE users SET invite_code = $2 WHERE id = $1`, userID, code)
		}
		var invited int
		_ = a.Pool.QueryRow(r.Context(),
			`SELECT COUNT(*) FROM users WHERE invited_by = $1`, userID).Scan(&invited)

		scheme := "http"
		if a.Config != nil && a.Config.Cookie.Secure {
			scheme = "https"
		}
		link := scheme + "://" + r.Host + "/register?invite=" + strings.TrimSpace(code)
		writeJSON(w, http.StatusOK, map[string]any{
			"invite_code":   code,
			"invited_count": invited,
			"invite_link":   link,
		})
	}
}
