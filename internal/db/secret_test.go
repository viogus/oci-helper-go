package db

import (
	"strings"
	"testing"
)

func testKey() []byte {
	k := make([]byte, 32)
	for i := range k {
		k[i] = byte(i + 1)
	}
	return k
}

func TestSecretEncryptionRoundTrip(t *testing.T) {
	store, err := New(":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	// Without a key, values pass through unchanged.
	if enc, err := store.encryptSecret("plain"); err != nil || enc != "plain" {
		t.Fatalf("no-key encrypt = (%q, %v), want (plain, nil)", enc, err)
	}

	store.SetSecretKey(testKey())

	enc, err := store.encryptSecret("S3cret")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if enc == "S3cret" || !strings.HasPrefix(enc, "enc1:") {
		t.Fatalf("value not encrypted: %q", enc)
	}
	if got := store.decryptSecret(enc); got != "S3cret" {
		t.Fatalf("round trip = %q, want S3cret", got)
	}

	// Legacy plaintext (no tag) is returned verbatim.
	if got := store.decryptSecret("legacyplain"); got != "legacyplain" {
		t.Fatalf("legacy decrypt = %q, want legacyplain", got)
	}

	// A different key cannot decrypt; the raw value is returned (no panic).
	other := testKey()
	other[0] ^= 0xff
	store.SetSecretKey(other)
	if got := store.decryptSecret(enc); got != enc {
		t.Fatalf("wrong-key decrypt = %q, want raw ciphertext", got)
	}
}

func TestCreateTaskPasswordEncryptedAtRest(t *testing.T) {
	store, err := New(":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	store.SetSecretKey(testKey())

	ten := &Tenant{Name: "t", Region: "r", UserOCID: "u", TenancyOCID: "ten", Fingerprint: "fp"}
	if err := store.CreateTenant(ten); err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	list, _ := store.ListTenants()
	if len(list) == 0 {
		t.Fatal("no tenant after create")
	}

	task := &CreateTask{TenantID: list[0].ID, RootPassword: "RootPw123", IntervalSeconds: 60, CreateNumbers: 1}
	if err := store.CreateCreateTask(task); err != nil {
		t.Fatalf("create task: %v", err)
	}
	got, err := store.GetCreateTask(task.ID)
	if err != nil || got == nil {
		t.Fatalf("get task: %v", err)
	}
	if got.RootPassword != "RootPw123" {
		t.Fatalf("decrypted password = %q, want RootPw123", got.RootPassword)
	}

	var raw string
	if err := store.DB().QueryRow(`SELECT root_password FROM create_tasks WHERE id=?`, task.ID).Scan(&raw); err != nil {
		t.Fatalf("raw select: %v", err)
	}
	if raw == "RootPw123" || !strings.HasPrefix(raw, "enc1:") {
		t.Fatalf("password stored unencrypted: %q", raw)
	}
}
