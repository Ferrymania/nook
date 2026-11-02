package service

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"go.uber.org/zap"

	"github.com/Ferrymania/nook/internal/helper"
)

// encryptedTestAccountConfig creates stored account fixtures with real encrypted
// credentials. Test clients keep receiving the same plaintext tokens/passwords.
func encryptedTestAccountConfig(t *testing.T, raw string) string {
	t.Helper()
	var values map[string]string
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		t.Fatal("invalid account fixture JSON")
	}
	cipher := NewCryptoService("test-secret", zap.NewNop())
	for _, key := range StrmAccountSecretKeys {
		if value, ok := values[key]; ok {
			values[key] = cipher.Encrypt(value)
		}
	}
	encoded, err := json.Marshal(values)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func TestCryptoRoundtrip(t *testing.T) {
	t.Parallel()

	c := NewCryptoService("super-secret-key-1234567890", zap.NewNop())
	cases := []string{
		"",
		"a",
		"hello world",
		"sk-1234567890abcdef1234567890abcdef1234567890abcdef",
	}
	for _, plain := range cases {
		t.Run(plain, func(t *testing.T) {
			cipher := c.Encrypt(plain)
			if plain == "" {
				if cipher != "" {
					t.Fatalf("empty plaintext should round-trip empty, got %q", cipher)
				}
			} else if cipher == plain {
				t.Fatalf("expected ciphertext to differ from plaintext")
			}
			if plain != "" && !strings.HasPrefix(cipher, "enc:v1:") {
				t.Fatalf("expected enc:v1: prefix, got %q", cipher)
			}
			plain2, err := c.Decrypt(cipher)
			if err != nil {
				t.Fatal(err)
			}
			if plain2 != plain {
				t.Fatalf("decrypt mismatch: got %q, want %q", plain2, plain)
			}
		})
	}
}

func TestCryptoNoSecret(t *testing.T) {
	t.Parallel()

	c := NewCryptoService("", zap.NewNop())
	if c.Encrypt("x") != "x" {
		t.Fatal("empty-secret crypto should be a pass-through")
	}
	stored := NewCryptoService("key", zap.NewNop()).Encrypt("x")
	if value, err := c.Decrypt(stored); value != "" || !errors.Is(err, ErrCryptoUnavailable) {
		t.Fatalf("empty-secret decrypt = %q, %v; want no value and unavailable error", value, err)
	}
}

func TestCryptoRejectsInvalidStoredSecrets(t *testing.T) {
	t.Parallel()

	c := NewCryptoService("k", zap.NewNop())
	for _, tc := range []struct {
		name  string
		value string
		want  error
	}{
		{"plaintext", "legacy-plain", helper.ErrSecretNotEncrypted},
		{"invalid base64", encPrefix + "!invalid!", helper.ErrSecretCiphertextInvalid},
		{"truncated ciphertext", encPrefix + "YQ==", helper.ErrSecretCiphertextInvalid},
		{"wrong key", NewCryptoService("other-key", zap.NewNop()).Encrypt("test-token"), helper.ErrSecretCiphertextInvalid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			plain, err := c.Decrypt(tc.value)
			if plain != "" || !errors.Is(err, tc.want) {
				t.Fatalf("invalid credential returned a value or wrong error: %v", err)
			}
			if strings.Contains(err.Error(), tc.value) {
				t.Fatal("decrypt error exposed the stored credential")
			}
		})
	}
}

func TestMaskAPIKey(t *testing.T) {
	t.Parallel()

	cases := []struct{ in, want string }{
		{"", "****"},
		{"abc", "****"},
		{"abcdefgh", "abcd****efgh"},
		{"sk-1234567890abcdef", "sk-1****cdef"},
	}
	for _, tc := range cases {
		got := MaskAPIKey(tc.in)
		if got != tc.want {
			t.Errorf("MaskAPIKey(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
