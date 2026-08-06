// Package totp implements RFC 6238 TOTP using HMAC-SHA1.
package totp

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"math"
	"net/url"
	"time"
)

// GenerateSecret creates a cryptographically random TOTP secret (base32-encoded).
func GenerateSecret() (string, error) {
	bytes := make([]byte, 20)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("totp: generate secret: %w", err)
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(bytes), nil
}

// GenerateKeyURI builds an otpauth:// URI for QR code generation.
func GenerateKeyURI(secret, email, issuer string) string {
	params := url.Values{}
	params.Set("secret", secret)
	params.Set("issuer", issuer)
	params.Set("algorithm", "SHA1")
	params.Set("digits", "6")
	params.Set("period", "30")
	return fmt.Sprintf("otpauth://totp/%s:%s?%s",
		url.PathEscape(issuer), url.PathEscape(email), params.Encode())
}

// Validate checks a TOTP code against a secret within a ±window time step.
func Validate(secret string, code string, window int) (bool, error) {
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secret)
	if err != nil {
		return false, fmt.Errorf("totp: decode secret: %w", err)
	}
	step := time.Now().UTC().Unix() / 30
	for i := int64(-window); i <= int64(window); i++ {
		if computeTOTP(key, step+i) == code {
			return true, nil
		}
	}
	return false, nil
}

func computeTOTP(key []byte, counter int64) string {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, uint64(counter))
	mac := hmac.New(sha1.New, key)
	mac.Write(buf)
	hash := mac.Sum(nil)
	offset := hash[len(hash)-1] & 0x0f
	bin := int64(binary.BigEndian.Uint32(hash[offset:offset+4])) & 0x7fffffff
	return fmt.Sprintf("%06d", bin%int64(math.Pow10(6)))
}
