// Package auth provides session-based authentication with bcrypt password validation,
//
package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base32"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/crypto/bcrypt"
)

type Session struct {
	User      string    `json:"user"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"createdAt"`
	Version   int64     `json:"v"`
}

const sessionCookie = "oci_helper_session"
const sessionTTL = 24 * time.Hour

type Service struct {
	username       string
	passwordHash   []byte
	sessionKey     []byte
	sessionVersion int64
	mfaSecret      string
	mfaEnabled     bool
	secureCookies  bool
}

func New(username, password, mfaSecret string, mfaEnabled bool, secureCookies bool) *Service {
	s := &Service{
		username:      username,
		mfaSecret:     mfaSecret,
		mfaEnabled:    mfaEnabled,
		secureCookies: secureCookies,
	}
	if password != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			panic("auth: bcrypt.GenerateFromPassword failed: " + err.Error())
		}
		s.passwordHash = hash
	}
	// generate session signing key
	sk := make([]byte, 32)
	if _, err := rand.Read(sk); err != nil {
		panic("auth: crypto/rand.Read failed for session key: " + err.Error())
	}
	s.sessionKey = sk
	return s
}

func (s *Service) ValidatePassword(pw string) bool {
	if len(s.passwordHash) == 0 {
		return false
	}
	return bcrypt.CompareHashAndPassword(s.passwordHash, []byte(pw)) == nil
}

func (s *Service) Login(w http.ResponseWriter, r *http.Request) bool {
	user, pass, ok := r.BasicAuth()
	if !ok {
		w.Header().Set("WWW-Authenticate", `Basic realm="oci-helper"`)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return false
	}
	if subtle.ConstantTimeCompare([]byte(user), []byte(s.username)) != 1 || !s.ValidatePassword(pass) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return false
	}
	sess := Session{User: user, Role: "admin", CreatedAt: time.Now(), Version: s.sessionVersion}
	data, err := json.Marshal(sess)
	if err != nil {
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return false
	}
	signed := sign(data, s.sessionKey)
	encrypted, err := encryptSigned([]byte(signed), s.sessionKey)
	if err != nil {
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return false
	}
	cookie := &http.Cookie{
		Name:     sessionCookie,
		Value:    base64.RawURLEncoding.EncodeToString(encrypted),
		Path:     "/",
		HttpOnly: true,
		Secure:   s.secureCookies && (r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(sessionTTL.Seconds()),
	}
	http.SetCookie(w, cookie)
	return true
}

// CreateSession generates a signed session cookie value for the given user and role.
func (s *Service) CreateSession(user, role string) string {
	if role == "" {
		role = "user"
	}
	sess := Session{User: user, Role: role, CreatedAt: time.Now(), Version: s.sessionVersion}
	data, err := json.Marshal(sess)
	if err != nil {
		return ""
	}
	signed := sign(data, s.sessionKey)
	encrypted, err := encryptSigned([]byte(signed), s.sessionKey)
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(encrypted)
}

func (s *Service) Logout(w http.ResponseWriter) {
	atomic.AddInt64(&s.sessionVersion, 1)
	http.SetCookie(w, &http.Cookie{
		Name:   sessionCookie,
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})
}

// GetSession extracts and validates the session from the request cookie.
// Returns nil if no valid session exists.
func (s *Service) GetSession(r *http.Request) *Session {
	cookie, err := r.Cookie(sessionCookie)
	if err != nil {
		return nil
	}
	encrypted, err := base64.RawURLEncoding.DecodeString(cookie.Value)
	if err != nil {
		return nil
	}
	signed, err := decryptSigned(encrypted, s.sessionKey)
	if err != nil {
		return nil
	}
	data, err := unsign(string(signed), s.sessionKey)
	if err != nil {
		return nil
	}
	var sess Session
	if err := json.Unmarshal(data, &sess); err != nil {
		return nil
	}
	if sess.Version != atomic.LoadInt64(&s.sessionVersion) {
		return nil
	}
	if time.Since(sess.CreatedAt) >= sessionTTL {
		return nil
	}
	return &sess
}

func (s *Service) Authenticate(w http.ResponseWriter, r *http.Request) bool {
	sess := s.GetSession(r)
	if sess == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return false
	}
	return true
}

func sign(data []byte, key []byte) string {
	mac := hmac.New(sha256.New, key)
	mac.Write(data)
	sig := mac.Sum(nil)
	return base64.RawURLEncoding.EncodeToString(data) + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func unsign(token string, key []byte) ([]byte, error) {
	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid token")
	}
	data, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, err
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, err
	}
	mac := hmac.New(sha256.New, key)
	mac.Write(data)
	expected := mac.Sum(nil)
	if !hmac.Equal(sig, expected) {
		return nil, fmt.Errorf("invalid signature")
	}
	return data, nil
}

// encryptSigned encrypts plaintext with AES-256-GCM using key.
// Returns salt(16) + nonce(12) + ciphertext||tag.
// NOTE: The cookie format has changed. All existing sessions are invalidated on deploy.
func encryptSigned(plaintext, key []byte) ([]byte, error) {
	salt := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	out := make([]byte, 0, len(salt)+len(nonce)+len(plaintext)+16)
	out = append(out, salt...)
	out = append(out, nonce...)
	return gcm.Seal(out, nonce, plaintext, nil), nil
}

// decryptSigned decrypts data produced by encryptSigned.
func decryptSigned(data, key []byte) ([]byte, error) {
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

// GenerateMFA creates a new TOTP secret (base32)
func GenerateMFA() string {
	b := make([]byte, 20)
	if _, err := rand.Read(b); err != nil {
		panic("auth: crypto/rand.Read failed for MFA secret: " + err.Error())
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b)
}

// ValidateTOTP verifies a TOTP code against a secret
func ValidateTOTP(secret string, code string) bool {
	if len(code) != 6 {
		return false
	}
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(secret))
	if err != nil {
		return false
	}
	// check current and adjacent time steps (30s window, ±1 step)
	now := time.Now().Unix()
	expectedCode := []byte(code)
	for _, step := range []int64{now / 30, now/30 - 1, now/30 + 1} {
		generated := totpAt(key, step)
		if subtle.ConstantTimeCompare([]byte(generated), expectedCode) == 1 {
			return true
		}
	}
	return false
}

func totpAt(key []byte, step int64) string {
	mac := hmac.New(sha1.New, key)
	binary.Write(mac, binary.BigEndian, step)
	hash := mac.Sum(nil)
	offset := hash[len(hash)-1] & 0xf
	binary := int32(hash[offset]&0x7f)<<24 | int32(hash[offset+1])<<16 | int32(hash[offset+2])<<8 | int32(hash[offset+3])
	return fmt.Sprintf("%06d", binary%1000000)
}

// TOTPURI generates an otpauth:// URI for QR code setup
func TOTPURI(secret, label, issuer string) string {
	p := url.Values{}
	p.Set("secret", secret)
	p.Set("issuer", issuer)
	p.Set("algorithm", "SHA1")
	p.Set("digits", "6")
	p.Set("period", "30")
	return fmt.Sprintf("otpauth://totp/%s:%s?%s", url.PathEscape(issuer), url.PathEscape(label), p.Encode())
}
