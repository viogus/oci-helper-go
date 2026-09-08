package handler

import (
	"bytes"
	"encoding/base64"
	"sync"
	"testing"
)

// TestSSHEncryptionKeyPersistsAcrossRestart guards the 6eaaf89 regression:
// the key must be persisted so a process restart (new Server instance over
// the same DB) can still decrypt private keys written by the old process.
func TestSSHEncryptionKeyPersistsAcrossRestart(t *testing.T) {
	s1, store, _, cleanup := setupTestServer(t)
	defer cleanup()

	key1, err := s1.getSSHEncryptionKey()
	if err != nil {
		t.Fatalf("first key: %v", err)
	}
	if len(key1) != 32 {
		t.Fatalf("key length = %d, want 32", len(key1))
	}

	// The key must have been persisted to the DB config table.
	persisted, err := store.GetConfig("ssh_key_encryption_key")
	if err != nil || persisted == "" {
		t.Fatalf("key not persisted to DB: err=%v value=%q", err, persisted)
	}

	// Simulate a restart: a fresh Server instance sharing the same store must
	// resolve the identical key (from the DB) instead of generating a new one.
	s2 := &Server{cfg: s1.cfg, store: store}
	key2, err := s2.getSSHEncryptionKey()
	if err != nil {
		t.Fatalf("second key: %v", err)
	}
	if !bytes.Equal(key1, key2) {
		t.Fatal("key changed across restart — stored private keys become undecryptable")
	}

	// Round-trip: data encrypted under process-1's key decrypts under
	// process-2's key.
	enc, err := encryptSSHPrivateKey(key1, []byte("-----BEGIN PRIVATE KEY-----\ntest\n-----END PRIVATE KEY-----"))
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	dec, err := decryptSSHPrivateKey(key2, enc)
	if err != nil {
		t.Fatalf("decrypt across restart: %v", err)
	}
	if string(dec) == "" || !bytes.Contains(dec, []byte("PRIVATE KEY")) {
		t.Fatalf("round-trip mismatch: %q", dec)
	}
}

// TestSSHEncryptionKeyConcurrentCalls: concurrent first calls must all return
// the same cached key and persist exactly one consistent value.
func TestSSHEncryptionKeyConcurrentCalls(t *testing.T) {
	s, _, _, cleanup := setupTestServer(t)
	defer cleanup()

	const n = 16
	keys := make([][]byte, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			k, err := s.getSSHEncryptionKey()
			if err != nil {
				t.Errorf("get key %d: %v", idx, err)
				return
			}
			keys[idx] = k
		}(i)
	}
	wg.Wait()
	for i := 1; i < n; i++ {
		if !bytes.Equal(keys[0], keys[i]) {
			t.Fatalf("concurrent calls returned different keys (0 vs %d)", i)
		}
	}
}

// TestSSHEncryptionKeyEnvOverride: OCI_SSH_KEY_ENCRYPTION_KEY wins over the
// persisted DB key.
func TestSSHEncryptionKeyEnvOverride(t *testing.T) {
	s1, store, _, cleanup := setupTestServer(t)
	defer cleanup()

	// Seed a persisted key so the env var must override it.
	if _, err := s1.getSSHEncryptionKey(); err != nil {
		t.Fatalf("seed key: %v", err)
	}

	envKey := make([]byte, 32)
	for i := range envKey {
		envKey[i] = byte(i)
	}
	cfg2 := *s1.cfg
	cfg2.SSHEncryptionKey = base64.StdEncoding.EncodeToString(envKey)
	s2 := &Server{cfg: &cfg2, store: store}

	got, err := s2.getSSHEncryptionKey()
	if err != nil {
		t.Fatalf("env key: %v", err)
	}
	if !bytes.Equal(got, envKey) {
		t.Fatal("OCI_SSH_KEY_ENCRYPTION_KEY was not honoured")
	}
}
