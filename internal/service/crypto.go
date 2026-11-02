// Package service — AES-GCM crypto helper for at-rest secrets.
//
// Sensitive API keys and passwords are encrypted with AES-256-GCM using the JWT secret.
// A database backup alone cannot decrypt them.
package service

import (
	"strings"

	"go.uber.org/zap"

	"github.com/Ferrymania/nook/internal/helper"
)

// encPrefix identifies the stored ciphertext format shared with helper.
const encPrefix = helper.EncPrefix

// CryptoService wraps an AES-GCM cipher derived from a stable per-install
// secret (the JWT secret).
//
// The cipher itself lives in helper.SecretCipher; this type keeps the
// service-layer logging and API.
type CryptoService struct {
	log    *zap.Logger
	cipher *helper.SecretCipher
}

// NewCryptoService derives a 256-bit key from the given secret via
// SHA-256 and constructs an AES-GCM AEAD. Empty secrets yield a service
// which cannot decrypt nonempty stored values.
func NewCryptoService(secret string, log *zap.Logger) *CryptoService {
	return &CryptoService{log: log, cipher: helper.NewSecretCipher(secret)}
}

// Encrypt returns the base64-encoded ciphertext (with prefix) for plain.
// Empty inputs round-trip unchanged.
func (c *CryptoService) Encrypt(plain string) string {
	return c.cipher.Encrypt(plain)
}

// Decrypt returns plaintext only for authenticated ciphertext, or an empty value.
func (c *CryptoService) Decrypt(value string) (string, error) {
	return c.cipher.Decrypt(value)
}

// IsEncrypted returns true if value carries the encrypted prefix.
func (c *CryptoService) IsEncrypted(value string) bool {
	return c.cipher.IsEncrypted(value)
}

// MaskAPIKey returns "abcd****wxyz" so the key can be displayed in the
// admin UI without leaking it. Inputs shorter than 8 chars become "****".
func MaskAPIKey(plain string) string {
	plain = strings.TrimSpace(plain)
	if len(plain) < 8 {
		return "****"
	}
	return plain[:4] + "****" + plain[len(plain)-4:]
}

// ErrCryptoUnavailable is returned when callers expect crypto and the
// service is degraded (empty secret, init failure).
var ErrCryptoUnavailable = helper.ErrSecretCipherUnavailable
