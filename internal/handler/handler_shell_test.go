package handler

import (
	"crypto/rsa"
	"strings"
	"testing"

	gossh "golang.org/x/crypto/ssh"
)

// TestNewConsoleRSAKey: OCI instance console connections accept ssh-rsa keys
// only, so the throwaway console key must be RSA-2048 with an ssh-rsa public
// key string.
func TestNewConsoleRSAKey(t *testing.T) {
	signer, pub, err := newConsoleRSAKey()
	if err != nil {
		t.Fatalf("newConsoleRSAKey: %v", err)
	}
	if !strings.HasPrefix(pub, "ssh-rsa ") {
		t.Fatalf("public key does not start with ssh-rsa: %.40q", pub)
	}

	ck, ok := signer.PublicKey().(gossh.CryptoPublicKey)
	if !ok {
		t.Fatal("console signer is not crypto-backed")
	}
	rsaPub, ok := ck.CryptoPublicKey().(*rsa.PublicKey)
	if !ok || rsaPub.N.BitLen() != 2048 {
		t.Fatalf("console key must be RSA-2048, got %T bits=%d", ck.CryptoPublicKey(), rsaPubBits(rsaPub, ok))
	}
}

func rsaPubBits(p *rsa.PublicKey, ok bool) int {
	if !ok || p == nil {
		return 0
	}
	return p.N.BitLen()
}

// TestUserCandidates: an explicit username is used alone; otherwise the
// conventional list covers OCI platform images plus Debian-style imports.
func TestUserCandidates(t *testing.T) {
	if got := userCandidates("debian"); len(got) != 1 || got[0] != "debian" {
		t.Fatalf("explicit user = %v, want [debian]", got)
	}
	def := userCandidates("")
	want := []string{"opc", "root", "ubuntu", "debian", "admin"}
	if len(def) != len(want) {
		t.Fatalf("default users = %v, want %v", def, want)
	}
	for i := range want {
		if def[i] != want[i] {
			t.Fatalf("default users = %v, want %v", def, want)
		}
	}
}
