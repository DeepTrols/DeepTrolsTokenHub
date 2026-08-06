package keyhash

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

// Hash returns HMAC-SHA256 hex digest of plaintext using secret as key.
func Hash(plaintext, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(plaintext))
	return hex.EncodeToString(mac.Sum(nil))
}
