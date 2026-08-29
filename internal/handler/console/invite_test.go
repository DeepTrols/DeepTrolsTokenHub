package console

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/deeptrols/api/internal/app"
	"github.com/deeptrols/api/internal/domain"
	"github.com/google/uuid"
)

func seedInviter(t *testing.T, a *app.App, email string) *domain.User {
	t.Helper()
	u := seedUserForConsoleTest(t, a, email, "pass12345", "Inviter")
	if _, err := a.Pool.Exec(context.Background(),
		`INSERT INTO wallets (id, user_id, balance, frozen, currency, version, created_at, updated_at)
		 VALUES ($1, $2, '0', '0', 'CNY', 0, NOW(), NOW())`, uuid.New(), u.ID); err != nil {
		t.Fatalf("seed inviter wallet: %v", err)
	}
	if _, err := a.Pool.Exec(context.Background(),
		`UPDATE users SET invite_code = 'DTPTEST01' WHERE id = $1`, u.ID); err != nil {
		t.Fatalf("set invite code: %v", err)
	}
	return u
}

func registerWithInvite(t *testing.T, a *app.App, email, code string) *httptest.ResponseRecorder {
	t.Helper()
	body := `{"email":"` + email + `","password":"password123","name":"Invitee","invite_code":"` + code + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/console/auth/register", strings.NewReader(body))
	w := httptest.NewRecorder()
	HandleRegister(a).ServeHTTP(w, req)
	return w
}

func TestRegister_WithInviteCodeCreditsBoth(t *testing.T) {
	a := appForConsoleTest(t)
	inviter := seedInviter(t, a, "inviter@example.com")

	w := registerWithInvite(t, a, "invitee@example.com", "DTPTEST01")
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d; body = %s", w.Code, w.Body.String())
	}

	// New user is linked to the inviter and received the reward on top of the
	// signup bonus (FakePayment=true → 1000 + 10).
	var invitedBy *uuid.UUID
	var inviteeID uuid.UUID
	if err := a.Pool.QueryRow(context.Background(),
		`SELECT id, invited_by FROM users WHERE email = $1`, "invitee@example.com").Scan(&inviteeID, &invitedBy); err != nil {
		t.Fatalf("invitee query: %v", err)
	}
	if invitedBy == nil || *invitedBy != inviter.ID {
		t.Fatalf("invited_by = %v, want %s", invitedBy, inviter.ID)
	}
	var inviteeBalance string
	_ = a.Pool.QueryRow(context.Background(),
		`SELECT balance::text FROM wallets WHERE user_id = $1`, inviteeID).Scan(&inviteeBalance)
	if inviteeBalance != "1010.000000" {
		t.Fatalf("invitee balance = %s, want 1010", inviteeBalance)
	}
	var inviterBalance string
	_ = a.Pool.QueryRow(context.Background(),
		`SELECT balance::text FROM wallets WHERE user_id = $1`, inviter.ID).Scan(&inviterBalance)
	if inviterBalance != "10.000000" {
		t.Fatalf("inviter balance = %s, want 10", inviterBalance)
	}
}

func TestRegister_InvalidInviteCodeRejected(t *testing.T) {
	a := appForConsoleTest(t)
	w := registerWithInvite(t, a, "bad-invite@example.com", "DTPNOPE123")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", w.Code, w.Body.String())
	}
}

func TestInviteInfo(t *testing.T) {
	a := appForConsoleTest(t)
	inviter := seedInviter(t, a, "invite-info@example.com")

	req := httptest.NewRequest(http.MethodGet, "/api/console/invite", nil)
	req = setUserInWalletContext(req, inviter.ID.String())
	w := httptest.NewRecorder()
	HandleInviteInfo(a).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; body = %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "DTPTEST01") {
		t.Fatalf("missing invite code: %s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "/register?invite=DTPTEST01") {
		t.Fatalf("missing invite link: %s", w.Body.String())
	}
}
