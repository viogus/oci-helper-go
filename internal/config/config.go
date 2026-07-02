// Package config loads the application configuration from environment variables.
//
package config

import (
	"crypto/rand"
	"encoding/base64"
	"log"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	Port          string
	Username      string
	Password      string
	MFA           bool
	MFASecret     string
	GoogleOAuth   GoogleOAuthConfig
	DBPath        string
	KeysDir       string
	LogLevel      string
	LogFile       string
	SecureCookies bool
}

type GoogleOAuthConfig struct {
	Enabled    bool
	ClientID   string
	ClientSecret string
	RedirectURL  string
}

func Load() *Config {
	c := &Config{
		Port:          envOr("PORT", "8818"),
		Username:      envOr("OCI_USERNAME", "admin"),
		Password:      envOr("OCI_PASSWORD", ""),
		DBPath:        envOr("OCI_DB_PATH", "/app/oci-helper/oci-helper.db"),
		KeysDir:       envOr("OCI_KEYS_DIR", "/app/oci-helper/keys"),
		LogLevel:      envOr("OCI_LOG_LEVEL", "info"),
		LogFile:       envOr("OCI_LOG_FILE", "/app/oci-helper/oci-helper.log"),
		SecureCookies: envOr("OCI_SECURE_COOKIES", "true") == "true",
	}

	if c.Password == "" {
		pw := randStr(16)
		c.Password = pw
		// Write full password to a file with restricted permissions for admin retrieval.
		// Only log the first 4 characters — never the full password.
		pwFile := filepath.Join(filepath.Dir(c.DBPath), ".admin_password")
		if err := os.WriteFile(pwFile, []byte(pw+"\n"), 0600); err != nil {
			log.Printf("warn: cannot write admin password file: %v", err)
		} else {
			log.Printf("[oci-helper] OCI_PASSWORD not set; generated password written to %s (first 4 chars: %s...)", pwFile, pw[:4])
		}
	}

	if v := os.Getenv("OCI_MFA"); strings.ToLower(v) == "true" {
		c.MFA = true
		c.MFASecret = os.Getenv("OCI_MFA_SECRET")
	}

	if v := os.Getenv("GOOGLE_CLIENT_ID"); v != "" {
		c.GoogleOAuth = GoogleOAuthConfig{
			Enabled:     true,
			ClientID:     v,
			ClientSecret: os.Getenv("GOOGLE_CLIENT_SECRET"),
			RedirectURL:  os.Getenv("GOOGLE_REDIRECT_URL"),
		}
	}

	return c
}

func envOr(k, fallback string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return fallback
}

func randStr(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		log.Fatal("config: crypto/rand.Read failed: " + err.Error())
	}
	return base64.RawURLEncoding.EncodeToString(b)[:n]
}
