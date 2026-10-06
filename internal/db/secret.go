package db

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"strings"
)

// secretPrefix tags values encrypted by encryptSecret. Legacy plaintext rows
// (written before at-rest encryption existed) have no prefix and are returned
// verbatim by decryptSecret, so upgrades are seamless.
const secretPrefix = "enc1:"

// SetSecretKey installs the AES-256 key used to encrypt sensitive columns at
// rest. A nil/empty or wrong-length key disables encryption (tests and
// deployments without an explicit key keep storing plaintext).
func (s *Store) SetSecretKey(key []byte) {
	if len(key) == 32 {
		s.secretKey = key
	}
}

// encryptSecret encrypts plain with AES-256-GCM, returning a tagged base64
// string. Empty input and a missing key pass through unchanged.
func (s *Store) encryptSecret(plain string) (string, error) {
	if plain == "" || len(s.secretKey) == 0 {
		return plain, nil
	}
	block, err := aes.NewCipher(s.secretKey)
	if err != nil {
		return "", fmt.Errorf("secret cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("secret gcm: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("secret nonce: %w", err)
	}
	sealed := gcm.Seal(nonce, nonce, []byte(plain), nil)
	return secretPrefix + base64.RawStdEncoding.EncodeToString(sealed), nil
}

// decryptSecret reverses encryptSecret. Values without the tag, or that fail
// to decrypt, are returned unchanged so legacy plaintext remains readable.
func (s *Store) decryptSecret(stored string) string {
	if stored == "" || len(s.secretKey) == 0 || !strings.HasPrefix(stored, secretPrefix) {
		return stored
	}
	raw, err := base64.RawStdEncoding.DecodeString(strings.TrimPrefix(stored, secretPrefix))
	if err != nil {
		return stored
	}
	block, err := aes.NewCipher(s.secretKey)
	if err != nil {
		return stored
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return stored
	}
	ns := gcm.NonceSize()
	if len(raw) < ns {
		return stored
	}
	plain, err := gcm.Open(nil, raw[:ns], raw[ns:], nil)
	if err != nil {
		return stored
	}
	return string(plain)
}
