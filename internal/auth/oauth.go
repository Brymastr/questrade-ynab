package auth

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	questradeTokenURL = "https://login.questrade.com/oauth2/token"
	ynabAuthURL       = "https://app.ynab.com/oauth/authorize"
	ynabTokenURL      = "https://api.ynab.com/oauth/token"
	stateCookieName   = "oauth_state"
)

// OAuthResult holds the tokens returned after a successful OAuth code exchange.
type OAuthResult struct {
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
	APIServer    string // Questrade only
}

// --- state cookie (CSRF protection) ---

func setStateCookie(w http.ResponseWriter, state string) {
	http.SetCookie(w, &http.Cookie{
		Name:     stateCookieName,
		Value:    state,
		Path:     "/",
		HttpOnly: true,
		Secure:   os.Getenv("COOKIE_SECURE") != "false",
		SameSite: http.SameSiteLaxMode,
		MaxAge:   600, // 10 minutes
	})
}

func validateState(r *http.Request, state string) bool {
	cookie, err := r.Cookie(stateCookieName)
	if err != nil {
		return false
	}
	return cookie.Value == state && state != ""
}

func clearStateCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:   stateCookieName,
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})
}

func randomState() string {
	b := make([]byte, 16)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// --- Questrade OAuth ---
//
// Questrade personal apps do not support the interactive authorize/redirect
// flow — the App Hub issues a refresh token directly. RefreshQuestradeToken
// exchanges that token for a short-lived access token and API server. Questrade
// rotates the refresh token on every exchange, so callers must persist the new
// one returned in OAuthResult.

// RefreshQuestradeToken exchanges a refresh token for a new access token.
func RefreshQuestradeToken(refreshToken string) (*OAuthResult, error) {
	data := url.Values{}
	data.Set("grant_type", "refresh_token")
	data.Set("refresh_token", refreshToken)
	data.Set("client_id", os.Getenv("QUESTRADE_CLIENT_ID"))
	data.Set("client_secret", os.Getenv("QUESTRADE_CLIENT_SECRET"))

	resp, err := http.PostForm(questradeTokenURL, data)
	if err != nil {
		return nil, fmt.Errorf("questrade refresh: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("questrade refresh failed: status %d body %s", resp.StatusCode, body)
	}

	var tr struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
		APIServer    string `json:"api_server"`
	}
	if err := json.Unmarshal(body, &tr); err != nil {
		return nil, fmt.Errorf("parse refresh response: %w", err)
	}

	refreshed := tr.RefreshToken
	if refreshed == "" {
		refreshed = refreshToken // some providers don't rotate
	}
	return &OAuthResult{
		AccessToken:  tr.AccessToken,
		RefreshToken: refreshed,
		ExpiresAt:    time.Now().Add(time.Duration(tr.ExpiresIn) * time.Second),
		APIServer:    tr.APIServer,
	}, nil
}

// --- YNAB OAuth ---

func YNABAuthURL(w http.ResponseWriter) string {
	state := randomState()
	setStateCookie(w, state)

	params := url.Values{}
	params.Set("client_id", os.Getenv("YNAB_CLIENT_ID"))
	params.Set("response_type", "code")
	params.Set("redirect_uri", os.Getenv("YNAB_REDIRECT_URI"))
	params.Set("state", state)
	return ynabAuthURL + "?" + params.Encode()
}

func ExchangeYNABCode(r *http.Request, w http.ResponseWriter, code, state string) (*OAuthResult, error) {
	if !validateState(r, state) {
		return nil, fmt.Errorf("invalid OAuth state")
	}
	clearStateCookie(w)

	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("code", code)
	data.Set("redirect_uri", os.Getenv("YNAB_REDIRECT_URI"))

	req, err := http.NewRequest("POST", ynabTokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(os.Getenv("YNAB_CLIENT_ID"), os.Getenv("YNAB_CLIENT_SECRET"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ynab token exchange: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ynab token exchange failed: status %d body %s", resp.StatusCode, body)
	}

	var tr struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &tr); err != nil {
		return nil, fmt.Errorf("parse ynab token response: %w", err)
	}
	return &OAuthResult{
		AccessToken:  tr.AccessToken,
		RefreshToken: tr.RefreshToken,
		ExpiresAt:    time.Now().Add(time.Duration(tr.ExpiresIn) * time.Second),
	}, nil
}

// RefreshYNABToken exchanges a YNAB refresh token for a new access token.
func RefreshYNABToken(refreshToken string) (*OAuthResult, error) {
	data := url.Values{}
	data.Set("grant_type", "refresh_token")
	data.Set("refresh_token", refreshToken)

	req, err := http.NewRequest("POST", ynabTokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(os.Getenv("YNAB_CLIENT_ID"), os.Getenv("YNAB_CLIENT_SECRET"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ynab refresh: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ynab refresh failed: status %d body %s", resp.StatusCode, body)
	}

	var tr struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &tr); err != nil {
		return nil, fmt.Errorf("parse ynab refresh response: %w", err)
	}

	refreshed := tr.RefreshToken
	if refreshed == "" {
		refreshed = refreshToken
	}
	return &OAuthResult{
		AccessToken:  tr.AccessToken,
		RefreshToken: refreshed,
		ExpiresAt:    time.Now().Add(time.Duration(tr.ExpiresIn) * time.Second),
	}, nil
}
