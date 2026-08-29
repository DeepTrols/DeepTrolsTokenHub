package console

import (
	"crypto/rand"
	"math/big"
	"net/http"
	"time"

	"github.com/deeptrols/api/internal/app"
	"github.com/deeptrols/api/internal/pkg/jwtutil"
	"github.com/deeptrols/api/internal/service/setting"
	"github.com/shopspring/decimal"
)

// defaultCheckinConfig mirrors the setting service defaults and is used when
// the App has no Settings service wired (unit tests / misconfiguration).
func defaultCheckinConfig() setting.CheckinConfig {
	return setting.CheckinConfig{
		Enabled:  true,
		MinQuota: decimal.NewFromInt(1),
		MaxQuota: decimal.NewFromInt(5),
	}
}

// resolveCheckinConfig returns the admin-configured sign-in policy, falling
// back to defaults when the settings service is unavailable.
func resolveCheckinConfig(a *app.App, r *http.Request) setting.CheckinConfig {
	if a == nil || a.Settings == nil {
		return defaultCheckinConfig()
	}
	cfg, err := a.Settings.CheckinConfig(r.Context())
	if err != nil {
		return defaultCheckinConfig()
	}
	return cfg
}

// HandleCheckIn credits the authenticated user a small daily bonus (once/day).
func HandleCheckIn(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, err := jwtutil.UserIDFromContext(r.Context())
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Not authenticated"})
			return
		}
		cfg := resolveCheckinConfig(a, r)
		if !cfg.Enabled {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "签到功能未启用"})
			return
		}
		today := time.Now().UTC().Format("2006-01-02")
		var exists bool
		if err := a.Pool.QueryRow(r.Context(),
			`SELECT EXISTS(SELECT 1 FROM checkins WHERE user_id = $1 AND checkin_date = $2)`,
			userID, today).Scan(&exists); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to check check-in"})
			return
		}
		if exists {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "Already checked in today"})
			return
		}
		amount := randomCheckinReward(cfg)
		wallet, err := a.Wallets.FindByUser(r.Context(), userID, nil)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Wallet not found"})
			return
		}
		idem := "checkin:" + userID.String() + ":" + today
		if _, err := a.Wallets.TopUp(r.Context(), wallet.ID, amount, idem); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to credit wallet"})
			return
		}
		if _, err := a.Pool.Exec(r.Context(),
			`INSERT INTO checkins (user_id, checkin_date, amount) VALUES ($1, $2, $3)`,
			userID, today, amount); err != nil {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "Already checked in today"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "amount": amount.String()})
	}
}

// HandleCheckInStatus reports whether the user has checked in today + total days.
func HandleCheckInStatus(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, err := jwtutil.UserIDFromContext(r.Context())
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Not authenticated"})
			return
		}
		cfg := resolveCheckinConfig(a, r)
		today := time.Now().UTC().Format("2006-01-02")
		var checkedIn, total int
		_ = a.Pool.QueryRow(r.Context(),
			`SELECT EXISTS(SELECT 1 FROM checkins WHERE user_id=$1 AND checkin_date=$2)::int`, userID, today).Scan(&checkedIn)
		_ = a.Pool.QueryRow(r.Context(),
			`SELECT COUNT(*) FROM checkins WHERE user_id=$1`, userID).Scan(&total)
		writeJSON(w, http.StatusOK, map[string]any{
			"enabled":          cfg.Enabled,
			"min_quota":        cfg.MinQuota.String(),
			"max_quota":        cfg.MaxQuota.String(),
			"checked_in_today": checkedIn == 1,
			"total_days":       total,
		})
	}
}

// randomCheckinReward picks an integer amount uniformly in [min, max]. When
// the configured range is empty or inverted it returns min (clamped earlier
// by the settings service; this guards direct callers).
func randomCheckinReward(cfg setting.CheckinConfig) decimal.Decimal {
	lo := cfg.MinQuota.IntPart()
	hi := cfg.MaxQuota.IntPart()
	if hi <= lo {
		return cfg.MinQuota
	}
	span := hi - lo + 1
	n, err := rand.Int(rand.Reader, big.NewInt(span))
	if err != nil {
		return cfg.MinQuota
	}
	return decimal.NewFromInt(lo + n.Int64())
}
