package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
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
		return subtle.ConstantTimeCompare([]byte(pw), []byte(s.mfaSecret)) == 1
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
	cookie := &http.Cookie{
		Name:     sessionCookie,
		Value:    base64.RawURLEncoding.EncodeToString(data),
		Path:     "/",
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(sessionTTL.Seconds()),
	}
	http.SetCookie(w, cookie)
	return true
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
	data, err := base64.RawURLEncoding.DecodeString(cookie.Value)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return false
	}
	var sess Session
	if err := json.Unmarshal(data, &sess); err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return false
	}
	if time.Since(sess.CreatedAt) > sessionTTL {
		http.Error(w, "Session expired", http.StatusUnauthorized)
		return false
	}
	return true
}

// GenerateMFA creates a new TOTP secret
func GenerateMFA() string {
	b := make([]byte, 20)
	rand.Read(b)
	return strings.ToUpper(hex.EncodeToString(b))
}
