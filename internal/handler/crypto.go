package handler

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"os"
)

// getSSHEncryptionKey returns the 32-byte AES key for SSH private key encryption.
// Priority: 1) OCI_SSH_KEY_ENCRYPTION_KEY env var, 2) persisted key in DB config table,
// 3) generate new random key and persist to DB.
func (s *Server) getSSHEncryptionKey() ([]byte, error) {
	// 1. Try env var first (explicit override, backward compat)
	if envKey := os.Getenv("OCI_SSH_KEY_ENCRYPTION_KEY"); envKey != "" {
		if decoded, err := base64.StdEncoding.DecodeString(envKey); err == nil && len(decoded) == 32 {
			return decoded, nil
		}
		log.Printf("[crypto] OCI_SSH_KEY_ENCRYPTION_KEY invalid (need 32 base64-decoded bytes); falling back to DB")
	}

	// 2. Try persisted key in DB config table
	if dbKey, err := s.store.GetConfig("ssh_key_encryption_key"); err == nil && dbKey != "" {
		if decoded, err := base64.StdEncoding.DecodeString(dbKey); err == nil && len(decoded) == 32 {
			return decoded, nil
		}
		log.Printf("[crypto] persisted ssh_key_encryption_key invalid; regenerating")
	}

	// 3. Generate new random key and persist to DB
	newKey := make([]byte, 32)
	if _, err := rand.Read(newKey); err != nil {
		return nil, fmt.Errorf("generate SSH key encryption key: %w", err)
	}
	encoded := base64.StdEncoding.EncodeToString(newKey)
	if err := s.store.SetConfig("ssh_key_encryption_key", encoded); err != nil {
		return nil, fmt.Errorf("persist SSH key encryption key: %w", err)
	}
	log.Println("[crypto] generated and persisted new SSH key encryption key in DB")
	return newKey, nil
}

// encryptSSHPrivateKey encrypts plaintext with AES-256-GCM.
// Returns base64-encoded salt(16) + nonce(12) + ciphertext||tag, or an error.
// On any crypto failure, returns ("", error) — never falls back to plaintext.
// Note: salt(16) is prepended for format compatibility with password-based
// KDF tools, even though the key is used directly. Not used in key derivation.
func encryptSSHPrivateKey(key, plaintext []byte) (string, error) {
	salt := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return "", fmt.Errorf("encrypt SSH: read salt: %w", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("encrypt SSH: new cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("encrypt SSH: new GCM: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("encrypt SSH: read nonce: %w", err)
	}
	out := make([]byte, 0, len(salt)+len(nonce)+len(plaintext)+16)
	out = append(out, salt...)
	out = append(out, nonce...)
	out = gcm.Seal(out, nonce, plaintext, nil)
	return base64.StdEncoding.EncodeToString(out), nil
}

// decryptSSHPrivateKey decrypts data produced by encryptSSHPrivateKey.
func decryptSSHPrivateKey(key []byte, encoded string) ([]byte, error) {
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	if len(data) < 28 {
		return nil, fmt.Errorf("too short")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonceSize := gcm.NonceSize()
	nonce := data[16 : 16+nonceSize]
	ciphertext := data[16+nonceSize:]
	return gcm.Open(nil, nonce, ciphertext, nil)
}
