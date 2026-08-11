package jwtutil

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestGenerateAndParseToken_Roundtrip(t *testing.T) {
	// Arrange
	userID := uuid.New()
	email := "test@example.com"
	name := "Test User"
	secret := "my-super-secret-key-at-least-32-bytes!"
	expiryHours := 24

	// Act
	token, err := GenerateToken(userID, email, name, "", "personal", "", "", secret, expiryHours)
	if err != nil {
		t.Fatalf("GenerateToken: unexpected error: %v", err)
	}
	if token == "" {
		t.Fatal("GenerateToken: returned empty token")
	}

	claims, err := ParseToken(token, secret)
	if err != nil {
		t.Fatalf("ParseToken: unexpected error: %v", err)
	}

	// Assert
	if claims.Subject != userID.String() {
		t.Errorf("Subject = %s, want %s", claims.Subject, userID.String())
	}
	if claims.Email != email {
		t.Errorf("Email = %s, want %s", claims.Email, email)
	}
	if claims.Name != name {
		t.Errorf("Name = %s, want %s", claims.Name, name)
	}
	if claims.Role != "" {
		t.Errorf("Role = %s, want empty", claims.Role)
	}

	// Verify expiry is set reasonably
	if claims.ExpiresAt == nil {
		t.Fatal("ExpiresAt is nil")
	}
	expectedExpiry := time.Now().Add(time.Duration(expiryHours) * time.Hour)
	diff := expectedExpiry.Sub(claims.ExpiresAt.Time)
	if diff < -5*time.Second || diff > 5*time.Second {
		t.Errorf("ExpiresAt diff too large: %v", diff)
	}

	// Verify issued at is set
	if claims.IssuedAt == nil {
		t.Fatal("IssuedAt is nil")
	}
}

func TestGenerateToken_UserIDStoredInSubject(t *testing.T) {
	// Arrange
	userID := uuid.MustParse("f47ac10b-58cc-4372-a567-0e02b2c3d479")
	secret := "another-32-byte-secret-key-here!"
	expiryHours := 1

	// Act
	token, err := GenerateToken(userID, "u@test.com", "U", "", "personal", "", "", secret, expiryHours)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}

	claims, err := ParseToken(token, secret)
	if err != nil {
		t.Fatalf("ParseToken: %v", err)
	}

	// Assert: Subject is the UUID string
	if claims.Subject != "f47ac10b-58cc-4372-a567-0e02b2c3d479" {
		t.Errorf("Subject = %s, want UUID string", claims.Subject)
	}
}

func TestParseToken_ExpiredToken(t *testing.T) {
	// Arrange
	userID := uuid.New()
	secret := "expired-token-test-secret-key-plz"
	// Generate a token with 0 expiry hours (already expired or about to)
	token, err := GenerateToken(userID, "exp@test.com", "Exp", "", "personal", "", "", secret, 0)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}

	// Wait a brief moment to ensure the token is expired
	time.Sleep(100 * time.Millisecond)

	// Act
	_, err = ParseToken(token, secret)

	// Assert: token should be expired
	if err == nil {
		t.Fatal("expected error for expired token")
	}
	if !strings.Contains(err.Error(), "expired") && !strings.Contains(err.Error(), "token") {
		t.Errorf("expected expired-related error, got: %v", err)
	}
}

func TestParseToken_WrongSecret(t *testing.T) {
	// Arrange
	userID := uuid.New()
	token, err := GenerateToken(userID, "w@test.com", "W", "", "personal", "", "", "correct-secret-key-at-least-32!!!", 24)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}

	// Act: parse with wrong secret
	_, err = ParseToken(token, "wrong-secret-key-at-least-32!!!!!")

	// Assert
	if err == nil {
		t.Fatal("expected error for wrong secret")
	}
}

func TestParseToken_MalformedToken(t *testing.T) {
	// Arrange
	secret := "some-secret-key-at-least-32-bytes"

	// Act & Assert: completely invalid tokens
	malformedCases := []string{
		"",
		"not-a-jwt",
		"header.payload",
		"header.payload.signature",
	}
	for _, tc := range malformedCases {
		_, err := ParseToken(tc, secret)
		if err == nil {
			t.Errorf("expected error for malformed token %q", tc)
		}
	}
}

func TestParseToken_TamperedToken(t *testing.T) {
	// Arrange
	userID := uuid.New()
	secret := "tamper-test-secret-key-enough-len"
	token, err := GenerateToken(userID, "t@test.com", "T", "", "personal", "", "", secret, 24)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}

	// Tamper with the payload by appending a character
	tamperedToken := token + "x"

	// Act
	_, err = ParseToken(tamperedToken, secret)

	// Assert
	if err == nil {
		t.Fatal("expected error for tampered token")
	}
}

func TestGenerateToken_EmptySecret(t *testing.T) {
	// Arrange
	userID := uuid.New()

	// Act
	token, err := GenerateToken(userID, "e@test.com", "E", "", "personal", "", "", "", 24)

	// Assert: GenerateToken itself should not error (signing happens),
	// but we should be able to parse it back with the same empty secret
	if err != nil {
		t.Fatalf("GenerateToken with empty secret: %v", err)
	}

	claims, err := ParseToken(token, "")
	if err != nil {
		t.Fatalf("ParseToken with empty secret should succeed: %v", err)
	}
	if claims.Email != "e@test.com" {
		t.Errorf("Email = %s, want e@test.com", claims.Email)
	}
}

func TestGenerateToken_SpecialCharactersInName(t *testing.T) {
	// Arrange
	userID := uuid.New()
	secret := "special-chars-secret-key-32-bytes"
	name := "用户 Admin 🚀"

	// Act
	token, err := GenerateToken(userID, "special@test.com", name, "", "personal", "", "", secret, 24)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}

	claims, err := ParseToken(token, secret)
	if err != nil {
		t.Fatalf("ParseToken: %v", err)
	}

	// Assert
	if claims.Name != name {
		t.Errorf("Name = %s, want %s", claims.Name, name)
	}
}

func TestParseToken_EmptyToken(t *testing.T) {
	// Arrange
	secret := "some-secret"

	// Act
	_, err := ParseToken("", secret)

	// Assert
	if err == nil {
		t.Fatal("expected error for empty token")
	}
}

func TestGenerateToken_NegativeExpiry(t *testing.T) {
	// Arrange
	userID := uuid.New()
	secret := "negative-expiry-key-32-bytes!!!!!"

	// Act: Generate with negative expiry
	token, err := GenerateToken(userID, "neg@test.com", "Neg", "", "personal", "", "", secret, -1)
	if err != nil {
		t.Fatalf("GenerateToken with negative expiry: %v", err)
	}

	// Wait and then parse -- it should be expired
	time.Sleep(100 * time.Millisecond)
	_, err = ParseToken(token, secret)

	// Assert
	if err == nil {
		t.Fatal("expected error for token with negative expiry")
	}
}

func TestUserIDFromContext(t *testing.T) {
	// Arrange
	userID := uuid.New()

	// Act: store userID as string in context with the standard key
	ctx := context.WithValue(context.Background(), CtxUserIDKey, userID.String())

	// Assert
	result, err := UserIDFromContext(ctx)
	if err != nil {
		t.Fatalf("UserIDFromContext: unexpected error: %v", err)
	}
	if result != userID {
		t.Errorf("UserIDFromContext = %s, want %s", result, userID)
	}
}

func TestUserIDFromContext_MissingValue(t *testing.T) {
	// Arrange
	ctx := context.Background()

	// Act
	_, err := UserIDFromContext(ctx)

	// Assert
	if err == nil {
		t.Fatal("expected error for missing user ID in context")
	}
}

func TestUserIDFromContext_InvalidUUID(t *testing.T) {
	// Arrange
	ctx := context.WithValue(context.Background(), CtxUserIDKey, "not-a-uuid")

	// Act
	_, err := UserIDFromContext(ctx)

	// Assert
	if err == nil {
		t.Fatal("expected error for invalid UUID in context")
	}
}

func TestEmailFromContext(t *testing.T) {
	// Arrange
	email := "user@example.com"
	ctx := context.WithValue(context.Background(), CtxEmailKey, email)

	// Act
	result, err := EmailFromContext(ctx)
	if err != nil {
		t.Fatalf("EmailFromContext: unexpected error: %v", err)
	}

	// Assert
	if result != email {
		t.Errorf("EmailFromContext = %s, want %s", result, email)
	}
}

func TestEmailFromContext_MissingValue(t *testing.T) {
	// Arrange
	ctx := context.Background()

	// Act
	_, err := EmailFromContext(ctx)

	// Assert
	if err == nil {
		t.Fatal("expected error for missing email in context")
	}
}

func TestEmailFromContext_EmptyString(t *testing.T) {
	// Arrange
	ctx := context.WithValue(context.Background(), CtxEmailKey, "")

	// Act
	result, err := EmailFromContext(ctx)

	// Assert
	if err != nil {
		t.Fatalf("EmailFromContext: unexpected error: %v", err)
	}
	if result != "" {
		t.Errorf("EmailFromContext = %q, want empty", result)
	}
}

func TestGenerateToken_LongExpiry(t *testing.T) {
	// Arrange
	userID := uuid.New()
	secret := "long-expiry-test-key-32-bytes!!!"
	expiryHours := 720 // 30 days

	// Act
	token, err := GenerateToken(userID, "long@test.com", "Long Expiry", "", "personal", "", "", secret, expiryHours)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}

	claims, err := ParseToken(token, secret)
	if err != nil {
		t.Fatalf("ParseToken: %v", err)
	}

	// Assert: expiry should be roughly 30 days in the future
	expectedExpiry := time.Now().Add(time.Duration(expiryHours) * time.Hour)
	diff := expectedExpiry.Sub(claims.ExpiresAt.Time)
	if diff < -10*time.Second || diff > 10*time.Second {
		t.Errorf("ExpiresAt diff too large: %v", diff)
	}
}

func TestParseToken_NilContext(t *testing.T) {
	// UserIDFromContext and EmailFromContext should handle nil context gracefully
	// Arrange: don't create a context

	// Act
	_, err := UserIDFromContext(nil)

	// Assert: should return error, not panic
	if err == nil {
		t.Fatal("expected error for UserIDFromContext(nil)")
	}

	_, err = EmailFromContext(nil)
	if err == nil {
		t.Fatal("expected error for EmailFromContext(nil)")
	}
}

// --- Role-aware token tests ---

func TestGenerateToken_WithRole(t *testing.T) {
	// Arrange
	userID := uuid.New()
	email := "admin@example.com"
	name := "Admin User"
	role := "admin"
	secret := "role-test-secret-key-32-bytes!!"
	expiryHours := 24

	// Act
	token, err := GenerateToken(userID, email, name, role, "personal", "", "", secret, expiryHours)
	if err != nil {
		t.Fatalf("GenerateToken with role: %v", err)
	}

	claims, err := ParseToken(token, secret)
	if err != nil {
		t.Fatalf("ParseToken: %v", err)
	}

	// Assert
	if claims.Role != role {
		t.Errorf("Role = %q, want %q", claims.Role, role)
	}
	if claims.Subject != userID.String() {
		t.Errorf("Subject = %s, want %s", claims.Subject, userID.String())
	}
	if claims.Email != email {
		t.Errorf("Email = %s, want %s", claims.Email, email)
	}
}

func TestGenerateToken_EmptyRole(t *testing.T) {
	// Arrange: empty role should produce token with empty role in claims
	userID := uuid.New()
	secret := "empty-role-test-key-32-bytes!!!"

	// Act
	token, err := GenerateToken(userID, "user@test.com", "User", "", "personal", "", "", secret, 24)
	if err != nil {
		t.Fatalf("GenerateToken with empty role: %v", err)
	}

	claims, err := ParseToken(token, secret)
	if err != nil {
		t.Fatalf("ParseToken: %v", err)
	}

	// Assert
	if claims.Role != "" {
		t.Errorf("Role = %q, want empty", claims.Role)
	}
}

func TestGenerateToken_RoleUser(t *testing.T) {
	// Arrange: role="user" for normal users
	userID := uuid.New()
	role := "user"
	secret := "user-role-test-key-min-32-byte!!"

	// Act
	token, err := GenerateToken(userID, "normal@user.com", "Normal User", role, "personal", "", "", secret, 24)
	if err != nil {
		t.Fatalf("GenerateToken with user role: %v", err)
	}

	claims, err := ParseToken(token, secret)
	if err != nil {
		t.Fatalf("ParseToken: %v", err)
	}

	// Assert
	if claims.Role != "user" {
		t.Errorf("Role = %q, want %q", claims.Role, "user")
	}
}

func TestRoleFromContext(t *testing.T) {
	// Arrange
	expectedRole := "admin"
	ctx := context.WithValue(context.Background(), CtxRoleKey, expectedRole)

	// Act
	result, err := RoleFromContext(ctx)

	// Assert
	if err != nil {
		t.Fatalf("RoleFromContext: unexpected error: %v", err)
	}
	if result != expectedRole {
		t.Errorf("RoleFromContext = %q, want %q", result, expectedRole)
	}
}

func TestRoleFromContext_MissingValue(t *testing.T) {
	// Arrange
	ctx := context.Background()

	// Act
	_, err := RoleFromContext(ctx)

	// Assert
	if err == nil {
		t.Fatal("expected error for missing role in context")
	}
}

func TestRoleFromContext_EmptyRole(t *testing.T) {
	// Arrange: empty role string is still a valid value
	ctx := context.WithValue(context.Background(), CtxRoleKey, "")

	// Act
	result, err := RoleFromContext(ctx)

	// Assert
	if err != nil {
		t.Fatalf("RoleFromContext with empty role: %v", err)
	}
	if result != "" {
		t.Errorf("RoleFromContext = %q, want empty", result)
	}
}

func TestRoleFromContext_WrongType(t *testing.T) {
	// Arrange: role stored as wrong type (number instead of string)
	ctx := context.WithValue(context.Background(), CtxRoleKey, 42)

	// Act
	_, err := RoleFromContext(ctx)

	// Assert
	if err == nil {
		t.Fatal("expected error for non-string role in context")
	}
}

func TestRoleFromContext_NilContext(t *testing.T) {
	// Arrange: nil context

	// Act
	_, err := RoleFromContext(nil)

	// Assert
	if err == nil {
		t.Fatal("expected error for RoleFromContext(nil)")
	}
}

func TestGenerateToken_RolePersistsAcrossExpiry(t *testing.T) {
	// Arrange: verify role is preserved in long-lived tokens
	userID := uuid.New()
	secret := "persist-role-test-key-32-bytes!!"
	role := "admin"

	// Act
	token, err := GenerateToken(userID, "admin@test.com", "Admin", role, "personal", "", "", secret, 720)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}

	claims, err := ParseToken(token, secret)
	if err != nil {
		t.Fatalf("ParseToken: %v", err)
	}

	// Assert
	if claims.Role != role {
		t.Errorf("Role = %q, want %q after long expiry", claims.Role, role)
	}

	expectedExpiry := time.Now().Add(720 * time.Hour)
	diff := expectedExpiry.Sub(claims.ExpiresAt.Time)
	if diff < -10*time.Second || diff > 10*time.Second {
		t.Errorf("ExpiresAt diff too large: %v", diff)
	}
}
