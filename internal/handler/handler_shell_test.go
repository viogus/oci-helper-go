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

// TestParseConsoleConnectionString: the proxy hop must dial the instance OCID
// from the outer ssh destination — not "localhost" — or every proxy dial
// fails with "ssh: rejected: connect failed".
func TestParseConsoleConnectionString(t *testing.T) {
	s := `ssh -i /tmp/k.pem -o ProxyCommand='ssh -W %h:%p -p 443 ocid1.instanceconsoleconnection.oc1.ap-chuncheon-1.an4w4@instance-console.ap-chuncheon-1.oci.oraclecloud.com' debian@ocid1.instance.oc1.ap-chuncheon-1.an4w4ljrqyycjticz`
	info, err := parseConsoleConnectionString(s)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if info.ProxyPort != 443 {
		t.Errorf("ProxyPort = %d, want 443", info.ProxyPort)
	}
	if info.ProxyUser != "ocid1.instanceconsoleconnection.oc1.ap-chuncheon-1.an4w4" {
		t.Errorf("ProxyUser = %q", info.ProxyUser)
	}
	if info.ProxyHost != "instance-console.ap-chuncheon-1.oci.oraclecloud.com" {
		t.Errorf("ProxyHost = %q", info.ProxyHost)
	}
	if info.TargetHost != "ocid1.instance.oc1.ap-chuncheon-1.an4w4ljrqyycjticz" {
		t.Errorf("TargetHost = %q, want the instance OCID", info.TargetHost)
	}
	if info.TargetPort != 22 {
		t.Errorf("TargetPort = %d, want 22", info.TargetPort)
	}

	// Outer destination without a user@ prefix.
	s2 := `ssh -o ProxyCommand="ssh -W %h:%p -p 443 ocid1.console.x@instance-console.us-ashburn-1.oci.oraclecloud.com" ocid1.instance.oc1.us-ashburn-1.abc`
	info2, err := parseConsoleConnectionString(s2)
	if err != nil {
		t.Fatalf("parse2: %v", err)
	}
	if info2.TargetHost != "ocid1.instance.oc1.us-ashburn-1.abc" {
		t.Errorf("TargetHost = %q", info2.TargetHost)
	}

	// Legacy string with no outer destination: keep the localhost fallback.
	s3 := `ssh -o ProxyCommand='ssh -W %h:%p -p 443 u@h.example.com'`
	info3, err := parseConsoleConnectionString(s3)
	if err != nil {
		t.Fatalf("parse3: %v", err)
	}
	if info3.TargetHost != "localhost" || info3.TargetPort != 22 {
		t.Errorf("fallback target = %s:%d, want localhost:22", info3.TargetHost, info3.TargetPort)
	}
}
