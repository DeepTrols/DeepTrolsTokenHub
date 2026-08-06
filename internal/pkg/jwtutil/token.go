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
	Email string `json:"email"`
	Name  string `json:"name"`
	Role  string `json:"role,omitempty"`
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
)

// GenerateToken creates a signed JWT token string using HS256.
// userID is stored in the Subject claim. email, name, and role are stored in custom claims.
func GenerateToken(userID uuid.UUID, email, name, role, secret string, expiryHours int) (string, error) {
	now := time.Now()
	expiry := now.Add(time.Duration(expiryHours) * time.Hour)

	claims := &Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID.String(),
			ExpiresAt: jwt.NewNumericDate(expiry),
			IssuedAt:  jwt.NewNumericDate(now),
		},
		Email: email,
		Name:  name,
		Role:  role,
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
