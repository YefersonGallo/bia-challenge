package httpapi

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// Auth issues and verifies HMAC-signed tokens for the demo user.
// It is intentionally small: the challenge needs a login screen, not an IdP.
type Auth struct {
	Secret   []byte
	User     string
	Password string
	TTL      time.Duration
	Now      func() time.Time
}

type claims struct {
	Sub string `json:"sub"`
	Exp int64  `json:"exp"`
}

var errBadToken = errors.New("invalid token")

func (a Auth) now() time.Time {
	if a.Now != nil {
		return a.Now()
	}
	return time.Now()
}

func (a Auth) sign(payload string) string {
	m := hmac.New(sha256.New, a.Secret)
	m.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

// Login checks the credentials and returns a token.
func (a Auth) Login(user, password string) (string, time.Time, bool) {
	okUser := subtle.ConstantTimeCompare([]byte(strings.ToLower(user)), []byte(strings.ToLower(a.User))) == 1
	okPass := subtle.ConstantTimeCompare([]byte(password), []byte(a.Password)) == 1
	if !okUser || !okPass {
		return "", time.Time{}, false
	}
	exp := a.now().Add(a.TTL)
	body, _ := json.Marshal(claims{Sub: a.User, Exp: exp.Unix()})
	payload := base64.RawURLEncoding.EncodeToString(body)
	return payload + "." + a.sign(payload), exp, true
}

// Verify validates a token and returns its subject.
func (a Auth) Verify(token string) (string, error) {
	payload, sig, ok := strings.Cut(token, ".")
	if !ok || !hmac.Equal([]byte(sig), []byte(a.sign(payload))) {
		return "", errBadToken
	}
	raw, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return "", errBadToken
	}
	var c claims
	if err := json.Unmarshal(raw, &c); err != nil || a.now().Unix() > c.Exp {
		return "", errBadToken
	}
	return c.Sub, nil
}
