package console

import (
	"crypto/sha256"
	"encoding/hex"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/deeptrols/api/internal/app"
	"github.com/deeptrols/api/internal/handler/middleware"
	"github.com/deeptrols/api/internal/pkg/jwtutil"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// revokedSessionTTLBuffer extends the Redis denylist slightly past token
// expiry so a revoked token is rejected even with clock skew.
const revokedSessionTTLBuffer = 5 * time.Minute

// sessionTokenHash returns the SHA-256 hex of a JWT used as the session key.
func sessionTokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// recordAuthSession persists a login session (best-effort: a transient DB
// failure must never fail the login itself).
func recordAuthSession(a *app.App, r *http.Request, userID uuid.UUID, token string) {
	if a == nil || a.Pool == nil {
		return
	}
	expires := time.Now().UTC().Add(time.Duration(a.Config.JWT.ExpiryHours) * time.Hour)
	_, err := a.Pool.Exec(r.Context(),
		`INSERT INTO auth_sessions (id, user_id, token_hash, ip_address, user_agent, created_at, expires_at)
		 VALUES ($1, $2, $3, $4, $5, NOW(), $6)`,
		uuid.New(), userID, sessionTokenHash(token), clientIP(r), truncateUA(r.UserAgent()), expires)
	if err != nil {
		log.Printf("console: record session: %v", err)
	}
}

type sessionResponse struct {
	ID         string  `json:"id"`
	IPAddress  string  `json:"ip_address"`
	UserAgent  string  `json:"user_agent"`
	CreatedAt  string  `json:"created_at"`
	ExpiresAt  string  `json:"expires_at"`
	LastSeenAt *string `json:"last_seen_at"`
	Current    bool    `json:"current"`
}

// HandleListSessions returns the authenticated user's login sessions with the
// current one flagged.
func HandleListSessions(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, err := jwtutil.UserIDFromContext(r.Context())
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Not authenticated"})
			return
		}
		currentHash, _ := r.Context().Value(middleware.SessionHashCtxKey).(string)
		rows, err := a.Pool.Query(r.Context(),
			`SELECT id, ip_address, user_agent, created_at, expires_at, last_seen_at, token_hash
			 FROM auth_sessions WHERE user_id = $1 AND revoked_at IS NULL
			 ORDER BY created_at DESC`, userID)
		if err != nil {
			log.Printf("console: list sessions: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to list sessions"})
			return
		}
		defer rows.Close()

		sessions := make([]sessionResponse, 0)
		for rows.Next() {
			var s sessionResponse
			var created, expires time.Time
			var lastSeen *time.Time
			var hash string
			if err := rows.Scan(&s.ID, &s.IPAddress, &s.UserAgent, &created, &expires, &lastSeen, &hash); err != nil {
				log.Printf("console: scan session: %v", err)
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to load sessions"})
				return
			}
			s.CreatedAt = created.Format(time.RFC3339)
			s.ExpiresAt = expires.Format(time.RFC3339)
			if lastSeen != nil {
				v := lastSeen.Format(time.RFC3339)
				s.LastSeenAt = &v
			}
			s.Current = currentHash != "" && hash == currentHash
			sessions = append(sessions, s)
		}
		if err := rows.Err(); err != nil {
			log.Printf("console: sessions rows: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to load sessions"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": sessions, "total": len(sessions)})
	}
}

// HandleRevokeSession revokes one of the user's sessions by id.
func HandleRevokeSession(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, err := jwtutil.UserIDFromContext(r.Context())
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Not authenticated"})
			return
		}
		sessionID, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid session id"})
			return
		}
		if !revokeSessionByID(a, r, userID, sessionID) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "Session not found"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}

// HandleRevokeOtherSessions revokes every session except the current one.
func HandleRevokeOtherSessions(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, err := jwtutil.UserIDFromContext(r.Context())
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Not authenticated"})
			return
		}
		currentHash, _ := r.Context().Value(middleware.SessionHashCtxKey).(string)
		rows, err := a.Pool.Query(r.Context(),
			`SELECT id, token_hash, expires_at FROM auth_sessions
			 WHERE user_id = $1 AND revoked_at IS NULL`, userID)
		if err != nil {
			log.Printf("console: list sessions for revoke: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to revoke sessions"})
			return
		}
		var targets []struct {
			ID        uuid.UUID
			Hash      string
			ExpiresAt time.Time
		}
		for rows.Next() {
			var t struct {
				ID        uuid.UUID
				Hash      string
				ExpiresAt time.Time
			}
			if err := rows.Scan(&t.ID, &t.Hash, &t.ExpiresAt); err != nil {
				rows.Close()
				log.Printf("console: scan revoke: %v", err)
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to revoke sessions"})
				return
			}
			if currentHash != "" && t.Hash == currentHash {
				continue
			}
			targets = append(targets, t)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			log.Printf("console: revoke rows: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to revoke sessions"})
			return
		}
		for _, t := range targets {
			revokeSessionRow(a, r, t.ID, t.Hash, t.ExpiresAt)
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "revoked": len(targets)})
	}
}

// revokeSessionByID marks a session revoked and denylists its token hash.
func revokeSessionByID(a *app.App, r *http.Request, userID uuid.UUID, sessionID uuid.UUID) bool {
	var hash string
	var expires time.Time
	err := a.Pool.QueryRow(r.Context(),
		`SELECT token_hash, expires_at FROM auth_sessions
		 WHERE id = $1 AND user_id = $2 AND revoked_at IS NULL`,
		sessionID, userID).Scan(&hash, &expires)
	if err != nil {
		return false
	}
	revokeSessionRow(a, r, sessionID, hash, expires)
	return true
}

// revokeSessionRow marks revoked_at and adds the hash to the Redis denylist.
func revokeSessionRow(a *app.App, r *http.Request, sessionID uuid.UUID, hash string, expiresAt time.Time) {
	_, _ = a.Pool.Exec(r.Context(),
		`UPDATE auth_sessions SET revoked_at = NOW() WHERE id = $1`, sessionID)
	if a.Redis != nil {
		ttl := time.Until(expiresAt) + revokedSessionTTLBuffer
		if ttl <= 0 {
			ttl = revokedSessionTTLBuffer
		}
		_ = a.Redis.Set(r.Context(), "deeptrols:auth:revoked:"+hash, "1", ttl).Err()
	}
}

// revokeSessionToken revokes the session belonging to a raw JWT (logout path).
func revokeSessionToken(a *app.App, r *http.Request, token string) {
	if token == "" {
		return
	}
	hash := sessionTokenHash(token)
	if a.Pool != nil {
		_, _ = a.Pool.Exec(r.Context(),
			`UPDATE auth_sessions SET revoked_at = NOW()
			 WHERE token_hash = $1 AND revoked_at IS NULL`, hash)
	}
	if a.Redis != nil {
		_ = a.Redis.Set(r.Context(), "deeptrols:auth:revoked:"+hash, "1", revokedSessionTTLBuffer).Err()
	}
}

func clientIP(r *http.Request) string {
	if ip := r.Header.Get("X-Forwarded-For"); ip != "" {
		if comma := strings.IndexByte(ip, ','); comma >= 0 {
			ip = ip[:comma]
		}
		return strings.TrimSpace(ip)
	}
	return strings.Split(r.RemoteAddr, ":")[0]
}

func truncateUA(ua string) string {
	if len(ua) > 512 {
		return ua[:512]
	}
	return ua
}
