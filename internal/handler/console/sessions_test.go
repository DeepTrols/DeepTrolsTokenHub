package console

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deeptrols/api/internal/handler/middleware"
	"github.com/deeptrols/api/internal/repository/membership"
	"github.com/google/uuid"
)

func TestLoginCreatesSessionAndRevokeFlow(t *testing.T) {
	a := appForWalletTest(t)
	a.Memberships = membership.NewPostgresRepository(a.Pool)
	seedUser := seedUserForWalletTest(t, a, "sess-flow@example.com", "pass12345", "Sess Flow")

	body, _ := json.Marshal(map[string]string{"email": "sess-flow@example.com", "password": "pass12345"})
	req := httptest.NewRequest(http.MethodPost, "/api/console/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = setUserInModelsContext(req, seedUser.ID.String())
	w := httptest.NewRecorder()
	HandleLogin(a).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("login status = %d; body = %s", w.Code, w.Body.String())
	}
	var login struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &login); err != nil {
		t.Fatalf("unmarshal login: %v", err)
	}
	if login.Token == "" {
		t.Fatal("login token empty")
	}

	// A session row was recorded.
	var sessionID string
	var hash string
	if err := a.Pool.QueryRow(context.Background(),
		`SELECT id::text, token_hash FROM auth_sessions WHERE user_id = $1`, seedUser.ID).Scan(&sessionID, &hash); err != nil {
		t.Fatalf("session row missing: %v", err)
	}
	if hash != sessionTokenHash(login.Token) {
		t.Fatalf("session hash mismatch: got %s want %s", hash, sessionTokenHash(login.Token))
	}

	// List: current session flagged.
	req = httptest.NewRequest(http.MethodGet, "/api/console/sessions", nil)
	req = setUserInModelsContext(req, seedUser.ID.String())
	req = req.WithContext(context.WithValue(req.Context(), middleware.SessionHashCtxKey, hash))
	w = httptest.NewRecorder()
	HandleListSessions(a).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("list status = %d; body = %s", w.Code, w.Body.String())
	}
	var list struct {
		Data []sessionResponse `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil {
		t.Fatalf("unmarshal list: %v", err)
	}
	if len(list.Data) != 1 || !list.Data[0].Current {
		t.Fatalf("sessions = %+v, want 1 current session", list.Data)
	}

	// Revoke by id.
	req = httptest.NewRequest(http.MethodDelete, "/api/console/sessions/"+sessionID, nil)
	req = setUserInModelsContext(req, seedUser.ID.String())
	req = chiRouteCtx(req, "id", sessionID)
	w = httptest.NewRecorder()
	HandleRevokeSession(a).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("revoke status = %d; body = %s", w.Code, w.Body.String())
	}
	var revoked bool
	if err := a.Pool.QueryRow(context.Background(),
		`SELECT revoked_at IS NOT NULL FROM auth_sessions WHERE id = $1`, sessionID).Scan(&revoked); err != nil {
		t.Fatalf("query revoked: %v", err)
	}
	if !revoked {
		t.Fatal("session row not marked revoked")
	}

	// List no longer shows it.
	req = httptest.NewRequest(http.MethodGet, "/api/console/sessions", nil)
	req = setUserInModelsContext(req, seedUser.ID.String())
	w = httptest.NewRecorder()
	HandleListSessions(a).ServeHTTP(w, req)
	_ = json.Unmarshal(w.Body.Bytes(), &list)
	if len(list.Data) != 0 {
		t.Fatalf("sessions after revoke = %+v, want 0", list.Data)
	}
}

func TestHandleRevokeOtherSessions_KeepsCurrent(t *testing.T) {
	a := appForWalletTest(t)
	seedUser := seedUserForWalletTest(t, a, "sess-others@example.com", "pass12345", "Sess Others")

	currentHash := "current-hash-abc"
	otherHash := "other-hash-def"
	insertSession := func(hash string) string {
		id := uuid.New()
		if _, err := a.Pool.Exec(context.Background(),
			`INSERT INTO auth_sessions (id, user_id, token_hash, expires_at)
			 VALUES ($1, $2, $3, NOW() + INTERVAL '1 hour')`,
			id, seedUser.ID, hash); err != nil {
			t.Fatalf("insert session: %v", err)
		}
		return id.String()
	}
	otherID := insertSession(otherHash)
	insertSession(currentHash)

	req := httptest.NewRequest(http.MethodDelete, "/api/console/sessions", nil)
	req = setUserInModelsContext(req, seedUser.ID.String())
	req = req.WithContext(context.WithValue(req.Context(), middleware.SessionHashCtxKey, currentHash))
	w := httptest.NewRecorder()
	HandleRevokeOtherSessions(a).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; body = %s", w.Code, w.Body.String())
	}

	var revoked bool
	if err := a.Pool.QueryRow(context.Background(),
		`SELECT revoked_at IS NOT NULL FROM auth_sessions WHERE id = $1`, otherID).Scan(&revoked); err != nil {
		t.Fatalf("query other: %v", err)
	}
	if !revoked {
		t.Fatal("other session not revoked")
	}
	var activeCount int
	if err := a.Pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM auth_sessions WHERE user_id = $1 AND revoked_at IS NULL`,
		seedUser.ID).Scan(&activeCount); err != nil {
		t.Fatalf("count active: %v", err)
	}
	if activeCount != 1 {
		t.Fatalf("active sessions = %d, want 1 (current kept)", activeCount)
	}
}

func TestHandleRevokeSession_InvalidAndNotFound(t *testing.T) {
	a := appForWalletTest(t)
	seedUser := seedUserForWalletTest(t, a, "sess-bad@example.com", "pass12345", "Sess Bad")

	req := httptest.NewRequest(http.MethodDelete, "/api/console/sessions/not-a-uuid", nil)
	req = setUserInModelsContext(req, seedUser.ID.String())
	req = chiRouteCtx(req, "id", "not-a-uuid")
	w := httptest.NewRecorder()
	HandleRevokeSession(a).ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("bad id status = %d, want 400", w.Code)
	}

	req = httptest.NewRequest(http.MethodDelete, "/api/console/sessions/"+uuid.New().String(), nil)
	req = setUserInModelsContext(req, seedUser.ID.String())
	req = chiRouteCtx(req, "id", uuid.New().String())
	w = httptest.NewRecorder()
	HandleRevokeSession(a).ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("missing session status = %d, want 404", w.Code)
	}
}
