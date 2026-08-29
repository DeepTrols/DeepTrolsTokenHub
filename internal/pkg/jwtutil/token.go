package jwtutil

import (
	"context"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// Claims represents the JWT claims used by the console API.
type Claims struct {
	jwt.RegisteredClaims
	Email      string `json:"email"`
	Name       string `json:"name"`
	Role       string `json:"role,omitempty"`
	UserType   string `json:"user_type,omitempty"`
	TenantID   string `json:"tenant_id,omitempty"`
	TenantRole string `json:"tenant_role,omitempty"`
}

// ContextKey is used for context value keys to avoid collisions.
type ContextKey string

const (
	// CtxUserIDKey is the context key for the user ID string (UUID).
	CtxUserIDKey ContextKey = "user_id"
	// CtxEmailKey is the context key for the user email.
	CtxEmailKey ContextKey = "email"
	// CtxUserNameKey is the context key for the user display name.
	CtxUserNameKey ContextKey = "user_name"
	// CtxRoleKey is the context key for the user role.
	CtxRoleKey ContextKey = "user_role"
	// CtxUserTypeKey is the context key for the user type (personal/enterprise).
	CtxUserTypeKey ContextKey = "user_type"
	// CtxTenantIDKey is the context key for the enterprise tenant ID.
	CtxTenantIDKey ContextKey = "tenant_id"
	// CtxTenantRoleKey is the context key for the tenant-level role.
	CtxTenantRoleKey ContextKey = "tenant_role"
)

// GenerateToken creates a signed JWT token string using HS256.
// userID is stored in the Subject claim. email, name, and role are stored in custom claims.
func GenerateToken(userID uuid.UUID, email, name, role, userType, tenantID, tenantRole, secret string, expiryHours int) (string, error) {
	now := time.Now()
	expiry := now.Add(time.Duration(expiryHours) * time.Hour)

	claims := &Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID.String(),
			ID:        uuid.NewString(), // per-login nonce: tokens are unique even within the same second
			ExpiresAt: jwt.NewNumericDate(expiry),
			IssuedAt:  jwt.NewNumericDate(now),
		},
		Email:      email,
		Name:       name,
		Role:       role,
		UserType:   userType,
		TenantID:   tenantID,
		TenantRole: tenantRole,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, err := token.SignedString([]byte(secret))
	if err != nil {
		return "", fmt.Errorf("jwtutil: failed to sign token: %w", err)
	}
	return tokenStr, nil
}

// ParseToken validates and parses a JWT token string.
// It verifies the HMAC signature and expiry.
func ParseToken(tokenString, secret string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{},
		func(token *jwt.Token) (interface{}, error) {
			// Validate signing method
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("jwtutil: unexpected signing method: %v", token.Header["alg"])
			}
			return []byte(secret), nil
		},
	)
	if err != nil {
		return nil, fmt.Errorf("jwtutil: failed to parse token: %w", err)
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("jwtutil: invalid token claims")
	}

	return claims, nil
}

// UserIDFromContext extracts the user UUID from context.
// Returns an error if the value is missing or cannot be parsed as a UUID.
func UserIDFromContext(ctx context.Context) (uuid.UUID, error) {
	if ctx == nil {
		return uuid.Nil, fmt.Errorf("jwtutil: context is nil")
	}
	val := ctx.Value(CtxUserIDKey)
	if val == nil {
		return uuid.Nil, fmt.Errorf("jwtutil: user_id not found in context")
	}
	valStr, ok := val.(string)
	if !ok {
		return uuid.Nil, fmt.Errorf("jwtutil: user_id in context is not a string")
	}
	id, err := uuid.Parse(valStr)
	if err != nil {
		return uuid.Nil, fmt.Errorf("jwtutil: failed to parse user_id as UUID: %w", err)
	}
	return id, nil
}

// EmailFromContext extracts the user email from context.
// Returns an error if the value is missing.
func EmailFromContext(ctx context.Context) (string, error) {
	if ctx == nil {
		return "", fmt.Errorf("jwtutil: context is nil")
	}
	val := ctx.Value(CtxEmailKey)
	if val == nil {
		return "", fmt.Errorf("jwtutil: email not found in context")
	}
	email, ok := val.(string)
	if !ok {
		return "", fmt.Errorf("jwtutil: email in context is not a string")
	}
	return email, nil
}

// RoleFromContext extracts the user role from context.
// Returns an error if the value is missing or has the wrong type.
func RoleFromContext(ctx context.Context) (string, error) {
	if ctx == nil {
		return "", fmt.Errorf("jwtutil: context is nil")
	}
	val := ctx.Value(CtxRoleKey)
	if val == nil {
		return "", fmt.Errorf("jwtutil: role not found in context")
	}
	role, ok := val.(string)
	if !ok {
		return "", fmt.Errorf("jwtutil: role in context is not a string")
	}
	return role, nil
}

// UserTypeFromContext extracts the user type from context.
// Returns "personal" if not found.
func UserTypeFromContext(ctx context.Context) (string, error) {
	if ctx == nil {
		return "", fmt.Errorf("jwtutil: context is nil")
	}
	val := ctx.Value(CtxUserTypeKey)
	if val == nil {
		return "", fmt.Errorf("jwtutil: user_type not found in context")
	}
	userType, ok := val.(string)
	if !ok {
		return "", fmt.Errorf("jwtutil: user_type in context is not a string")
	}
	return userType, nil
}

// TenantIDFromContext extracts the tenant UUID from context.
// Returns an empty UUID if the user is not in a tenant.
func TenantIDFromContext(ctx context.Context) (uuid.UUID, error) {
	if ctx == nil {
		return uuid.Nil, fmt.Errorf("jwtutil: context is nil")
	}
	val := ctx.Value(CtxTenantIDKey)
	if val == nil {
		return uuid.Nil, fmt.Errorf("jwtutil: tenant_id not found in context")
	}
	valStr, ok := val.(string)
	if !ok {
		return uuid.Nil, fmt.Errorf("jwtutil: tenant_id in context is not a string")
	}
	if valStr == "" {
		return uuid.Nil, nil
	}
	id, err := uuid.Parse(valStr)
	if err != nil {
		return uuid.Nil, fmt.Errorf("jwtutil: failed to parse tenant_id as UUID: %w", err)
	}
	return id, nil
}

// TenantRoleFromContext extracts the tenant role from context.
// Returns an empty string if the user has no tenant role.
func TenantRoleFromContext(ctx context.Context) (string, error) {
	if ctx == nil {
		return "", fmt.Errorf("jwtutil: context is nil")
	}
	val := ctx.Value(CtxTenantRoleKey)
	if val == nil {
		return "", fmt.Errorf("jwtutil: tenant_role not found in context")
	}
	role, ok := val.(string)
	if !ok {
		return "", fmt.Errorf("jwtutil: tenant_role in context is not a string")
	}
	return role, nil
}
