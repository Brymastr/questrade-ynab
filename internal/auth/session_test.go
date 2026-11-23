package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type memSessions map[string]struct {
	user    string
	expires time.Time
}

func (m memSessions) CreateSession(h, u string, exp time.Time) error {
	m[h] = struct {
		user    string
		expires time.Time
	}{u, exp}
	return nil
}
func (m memSessions) SessionUser(h string) (string, error) {
	s, ok := m[h]
	if !ok || !time.Now().Before(s.expires) {
		return "", nil
	}
	return s.user, nil
}
func (m memSessions) DeleteSession(h string) error { delete(m, h); return nil }

// requestWith returns a request carrying the cookie set on rec.
func requestWith(rec *httptest.ResponseRecorder) *http.Request {
	r := httptest.NewRequest("GET", "/", nil)
	for _, c := range rec.Result().Cookies() {
		r.AddCookie(c)
	}
	return r
}

func TestSessionLifecycle(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret")
	store := memSessions{}

	rec := httptest.NewRecorder()
	if err := SetSession(rec, store, "alice"); err != nil {
		t.Fatal(err)
	}
	if len(store) != 1 {
		t.Fatalf("want 1 stored session, got %d", len(store))
	}
	for h := range store {
		if len(h) != 64 {
			t.Errorf("stored key %q isn't a sha256 hash", h)
		}
	}
	req := requestWith(rec)
	if u, err := GetSession(req, store); err != nil || u != "alice" {
		t.Fatalf("GetSession = %q, %v", u, err)
	}

	// Logout revokes server-side: the same cookie no longer works.
	if err := ClearSession(httptest.NewRecorder(), req, store); err != nil {
		t.Fatal(err)
	}
	if _, err := GetSession(req, store); err == nil {
		t.Error("cookie still valid after logout")
	}
}

func TestSessionRejectsLegacyAndForged(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret")
	store := memSessions{}

	sign := func(c Claims) *http.Request {
		tok, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString([]byte("test-secret"))
		r := httptest.NewRequest("GET", "/", nil)
		r.AddCookie(&http.Cookie{Name: cookieName, Value: tok})
		return r
	}
	exp := jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))}

	// Validly signed but from before sessions existed (no sid).
	if _, err := GetSession(sign(Claims{UserID: "alice", RegisteredClaims: exp}), store); err == nil {
		t.Error("accepted a cookie without a session ID")
	}
	// Validly signed with a session ID that was never created.
	if _, err := GetSession(sign(Claims{UserID: "alice", SessionID: "made-up", RegisteredClaims: exp}), store); err == nil {
		t.Error("accepted an unknown session ID")
	}
	// A real session presented with someone else's user ID.
	rec := httptest.NewRecorder()
	SetSession(rec, store, "alice")
	claims, _ := parseCookie(requestWith(rec))
	claims.UserID = "mallory"
	if _, err := GetSession(sign(*claims), store); err == nil {
		t.Error("accepted a session under a different user ID")
	}
}
