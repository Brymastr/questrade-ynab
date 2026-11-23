package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const cookieName = "session"

// SessionLifetime is how long a login lasts. Logging in again costs the user
// a fresh Questrade refresh token, so it's long; logout revokes it server-side.
const SessionLifetime = 30 * 24 * time.Hour

// SessionStore persists sessions so they can be revoked. Only a hash of the
// session ID is stored; the cookie holds the ID itself inside a signed JWT.
type SessionStore interface {
	CreateSession(idHash, userID string, expiresAt time.Time) error
	// SessionUser returns the session's user, or "" if it doesn't exist or expired.
	SessionUser(idHash string) (string, error)
	DeleteSession(idHash string) error
}

type Claims struct {
	UserID    string `json:"user_id"`
	SessionID string `json:"sid"`
	jwt.RegisteredClaims
}

func jwtSecret() []byte {
	s := os.Getenv("JWT_SECRET")
	if s == "" {
		panic("JWT_SECRET env var is required")
	}
	return []byte(s)
}

func hashSessionID(sid string) string {
	sum := sha256.Sum256([]byte(sid))
	return hex.EncodeToString(sum[:])
}

// SetSession starts a new server-side session for userID and writes it as an
// HTTP-only cookie (a signed JWT carrying the user and session IDs).
func SetSession(w http.ResponseWriter, store SessionStore, userID string) error {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return err
	}
	sid := base64.RawURLEncoding.EncodeToString(raw)
	expires := time.Now().Add(SessionLifetime)
	if err := store.CreateSession(hashSessionID(sid), userID, expires); err != nil {
		return fmt.Errorf("create session: %w", err)
	}

	claims := Claims{
		UserID:    userID,
		SessionID: sid,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expires),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(jwtSecret())
	if err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    signed,
		Path:     "/",
		HttpOnly: true,
		Secure:   os.Getenv("COOKIE_SECURE") != "false",
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(SessionLifetime.Seconds()),
	})
	return nil
}

func parseCookie(r *http.Request) (*Claims, error) {
	cookie, err := r.Cookie(cookieName)
	if err != nil {
		return nil, errors.New("no session cookie")
	}
	token, err := jwt.ParseWithClaims(cookie.Value, &Claims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return jwtSecret(), nil
	})
	if err != nil || !token.Valid {
		return nil, errors.New("invalid session")
	}
	claims, ok := token.Claims.(*Claims)
	if !ok || claims.SessionID == "" { // pre-revocation cookies carry no sid
		return nil, errors.New("invalid session")
	}
	return claims, nil
}

// GetSession validates the session cookie and checks the session still exists
// server-side (not logged out or expired), returning the userID.
func GetSession(r *http.Request, store SessionStore) (string, error) {
	claims, err := parseCookie(r)
	if err != nil {
		return "", err
	}
	userID, err := store.SessionUser(hashSessionID(claims.SessionID))
	if err != nil {
		return "", fmt.Errorf("look up session: %w", err)
	}
	if userID == "" || userID != claims.UserID {
		return "", errors.New("session revoked or expired")
	}
	return userID, nil
}

// ClearSession revokes the current session server-side and removes the cookie.
func ClearSession(w http.ResponseWriter, r *http.Request, store SessionStore) error {
	var err error
	if claims, perr := parseCookie(r); perr == nil {
		err = store.DeleteSession(hashSessionID(claims.SessionID))
	}
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   os.Getenv("COOKIE_SECURE") != "false",
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
	return err
}
