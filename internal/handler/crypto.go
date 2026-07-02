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
	"sync"
)

var (
	sshKeyEncryptionKey []byte
	sshKeyOnce          sync.Once
)

// getSSHEncryptionKey loads or generates a 32-byte AES key for SSH private key encryption.
// Uses OCI_SSH_KEY_ENCRYPTION_KEY env var if set; otherwise generates a unique key per process
// from random bytes (keys persist only for this process lifetime).
func getSSHEncryptionKey() []byte {
	sshKeyOnce.Do(func() {
		if envKey := os.Getenv("OCI_SSH_KEY_ENCRYPTION_KEY"); envKey != "" {
			if decoded, err := base64.StdEncoding.DecodeString(envKey); err == nil && len(decoded) == 32 {
				sshKeyEncryptionKey = decoded
				log.Println("[crypto] using OCI_SSH_KEY_ENCRYPTION_KEY")
				return
			}
			log.Printf("[crypto] OCI_SSH_KEY_ENCRYPTION_KEY invalid (need 32 base64-decoded bytes); generating random key")
		}
		sshKeyEncryptionKey = make([]byte, 32)
		if _, err := rand.Read(sshKeyEncryptionKey); err != nil {
			log.Fatalf("[crypto] failed to generate SSH key encryption key: %v", err)
		}
		log.Println("[crypto] using auto-generated SSH key encryption key (per-process)")
	})
	return sshKeyEncryptionKey
}

// encryptSSHPrivateKey encrypts plaintext with AES-256-GCM.
// Returns base64-encoded salt(16) + nonce(12) + ciphertext||tag, or an error.
// On any crypto failure, returns ("", error) — never falls back to plaintext.
func encryptSSHPrivateKey(plaintext []byte) (string, error) {
	key := getSSHEncryptionKey()
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
func decryptSSHPrivateKey(encoded string) ([]byte, error) {
	key := getSSHEncryptionKey()
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
