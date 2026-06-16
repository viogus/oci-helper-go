package config

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"strings"
)

type Config struct {
	Port       string
	Username   string
	Password   string
	MFA        bool
	MFASecret  string
	GoogleOAuth GoogleOAuthConfig
	DBPath     string
	KeysDir    string
	LogLevel   string
	LogFile    string
}

type GoogleOAuthConfig struct {
	Enabled    bool
	ClientID   string
	ClientSecret string
	RedirectURL  string
}

func Load() *Config {
	c := &Config{
		Port:     envOr("PORT", "8818"),
		Username: envOr("OCI_USERNAME", "admin"),
		Password: envOr("OCI_PASSWORD", ""),
		DBPath:   envOr("OCI_DB_PATH", "/app/oci-helper/oci-helper.db"),
		KeysDir:  envOr("OCI_KEYS_DIR", "/app/oci-helper/keys"),
		LogLevel: envOr("OCI_LOG_LEVEL", "info"),
		LogFile:  envOr("OCI_LOG_FILE", "/app/oci-helper/oci-helper.log"),
	}

	if c.Password == "" {
		pw := randStr(16)
		c.Password = pw
		fmt.Fprintf(os.Stderr, "[oci-helper] OCI_PASSWORD not set; generated: %s\n", pw)
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
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)[:n]
}
