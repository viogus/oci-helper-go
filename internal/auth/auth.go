package auth

import (
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
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

type Session struct {
	User      string    `json:"user"`
	CreatedAt time.Time `json:"createdAt"`
}

const sessionCookie = "oci_helper_session"
const sessionTTL = 24 * time.Hour

type Service struct {
	username     string
	passwordHash []byte
	sessionKey   []byte
	mfaSecret    string
	mfaEnabled   bool
}

func New(username, password, mfaSecret string, mfaEnabled bool) *Service {
	s := &Service{
		username:   username,
		mfaSecret:  mfaSecret,
		mfaEnabled: mfaEnabled,
	}
	if password != "" {
		hash, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		s.passwordHash = hash
	}
	// generate session signing key
	sk := make([]byte, 32)
	rand.Read(sk)
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
	sess := Session{User: user, CreatedAt: time.Now()}
	data, _ := json.Marshal(sess)
	signed := sign(data, s.sessionKey)
	cookie := &http.Cookie{
		Name:     sessionCookie,
		Value:    signed,
		Path:     "/",
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(sessionTTL.Seconds()),
	}
	http.SetCookie(w, cookie)
	return true
}

// CreateSession generates a signed session cookie value for the given user.
func (s *Service) CreateSession(user string) string {
	sess := Session{User: user, CreatedAt: time.Now()}
	data, _ := json.Marshal(sess)
	return sign(data, s.sessionKey)
}

func (s *Service) Logout(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:   sessionCookie,
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})
}

func (s *Service) Authenticate(w http.ResponseWriter, r *http.Request) bool {
	cookie, err := r.Cookie(sessionCookie)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return false
	}
	data, err := unsign(cookie.Value, s.sessionKey)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return false
	}
	var sess Session
	if err := json.Unmarshal(data, &sess); err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return false
	}
	if time.Since(sess.CreatedAt) >= sessionTTL {
		http.Error(w, "Session expired", http.StatusUnauthorized)
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

// GenerateMFA creates a new TOTP secret (base32)
func GenerateMFA() string {
	b := make([]byte, 20)
	rand.Read(b)
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
