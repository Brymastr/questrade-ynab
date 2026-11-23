package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/brymastr/questrade-ynab/internal/auth"
	"github.com/brymastr/questrade-ynab/internal/db"
	"github.com/brymastr/questrade-ynab/internal/questrade"
)

// questradeUserAllowed applies ALLOWED_QUESTRADE_USER_IDS, a comma-separated
// allowlist. Unset or empty means anyone can connect.
func questradeUserAllowed(qtUserID string) bool {
	list := strings.TrimSpace(os.Getenv("ALLOWED_QUESTRADE_USER_IDS"))
	if list == "" {
		return true
	}
	for _, id := range strings.Split(list, ",") {
		if strings.TrimSpace(id) == qtUserID {
			return true
		}
	}
	return false
}

// resolveQuestradeUser returns the app user for a Questrade login:
//   - the user already linked to it, whatever the current session says;
//   - else the session's user, if that user isn't linked to another login yet
//     (adopts accounts created before users were linked);
//   - else a new user, linked to it.
func resolveQuestradeUser(store db.Store, qtUserID, sessionUserID string) (string, error) {
	if id, err := store.UserByQuestradeID(qtUserID); err != nil || id != "" {
		return id, err
	}
	if sessionUserID != "" {
		if ok, _ := store.UserExists(sessionUserID); ok {
			err := store.LinkQuestradeUser(sessionUserID, qtUserID)
			if err == nil {
				return sessionUserID, nil
			}
			if !errors.Is(err, db.ErrQuestradeLinked) {
				return "", err
			}
		}
	}
	id, err := store.CreateUser()
	if err != nil {
		return "", err
	}
	if err := store.LinkQuestradeUser(id, qtUserID); err != nil {
		// Lost a race with a concurrent connect for the same login; use theirs.
		if errors.Is(err, db.ErrQuestradeLinked) {
			if existing, gerr := store.UserByQuestradeID(qtUserID); gerr == nil && existing != "" {
				return existing, nil
			}
		}
		return "", err
	}
	return id, nil
}

// handleQuestradeConnect exchanges a manually-generated Questrade refresh token
// for an access token. Questrade personal apps do not support the interactive
// OAuth authorize/redirect flow, so the user pastes the refresh token issued by
// the Questrade App Hub instead.
func handleQuestradeConnect(store db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			RefreshToken string `json:"refresh_token"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		body.RefreshToken = strings.TrimSpace(body.RefreshToken)
		if body.RefreshToken == "" {
			writeError(w, http.StatusBadRequest, "refresh_token is required")
			return
		}

		result, err := auth.RefreshQuestradeToken(body.RefreshToken)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid Questrade refresh token: "+err.Error())
			return
		}

		// Identify the person by their Questrade user ID, not the token (which
		// rotates) or the cookie (which can be lost).
		qc := questrade.NewClientFromTokens(result.RefreshToken, result.AccessToken, result.APIServer, result.ExpiresAt)
		qtID, err := qc.GetUserID()
		if err != nil {
			writeError(w, http.StatusBadGateway, "couldn't read your Questrade profile: "+err.Error())
			return
		}
		qtUserID := strconv.Itoa(qtID)
		if !questradeUserAllowed(qtUserID) {
			writeError(w, http.StatusForbidden, "This app isn't open to new users yet.")
			return
		}

		sessionUserID, _ := auth.GetSession(r, store)
		userID, err := resolveQuestradeUser(store, qtUserID, sessionUserID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "resolve user failed")
			return
		}
		if userID != sessionUserID {
			if err := auth.SetSession(w, store, userID); err != nil {
				writeError(w, http.StatusInternalServerError, "set session failed")
				return
			}
		}

		if err := store.UpsertToken(db.OAuthToken{
			UserID:       userID,
			Provider:     "questrade",
			AccessToken:  result.AccessToken,
			RefreshToken: result.RefreshToken,
			ExpiresAt:    result.ExpiresAt,
			APIServer:    result.APIServer,
		}); err != nil {
			writeError(w, http.StatusInternalServerError, "save token failed")
			return
		}

		hasYNAB, _ := store.HasToken(userID, "ynab")
		writeJSON(w, http.StatusOK, map[string]any{
			"user_id":             userID,
			"questrade_connected": true,
			"ynab_connected":      hasYNAB,
		})
	}
}

func handleYNABLogin(store db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Must already be authenticated (Questrade connected first)
		if _, err := auth.GetSession(r, store); err != nil {
			http.Redirect(w, r, os.Getenv("APP_URL"), http.StatusFound)
			return
		}
		redirectURL := auth.YNABAuthURL(w)
		http.Redirect(w, r, redirectURL, http.StatusFound)
	}
}

func handleYNABCallback(store db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, err := auth.GetSession(r, store)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "not authenticated")
			return
		}

		code := r.URL.Query().Get("code")
		state := r.URL.Query().Get("state")
		if code == "" {
			writeError(w, http.StatusBadRequest, "missing code")
			return
		}

		result, err := auth.ExchangeYNABCode(r, w, code, state)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}

		if err := store.UpsertToken(db.OAuthToken{
			UserID:       userID,
			Provider:     "ynab",
			AccessToken:  result.AccessToken,
			RefreshToken: result.RefreshToken,
			ExpiresAt:    result.ExpiresAt,
		}); err != nil {
			writeError(w, http.StatusInternalServerError, "save token failed")
			return
		}

		http.Redirect(w, r, os.Getenv("APP_URL")+"/#/mappings", http.StatusFound)
	}
}

func handleLogout(store db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := auth.ClearSession(w, r, store); err != nil {
			writeError(w, http.StatusInternalServerError, "logout failed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "logged out"})
	}
}

func handleMe(store db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := userIDFromCtx(r)
		hasQT, _ := store.HasToken(userID, "questrade")
		hasYNAB, _ := store.HasToken(userID, "ynab")
		writeJSON(w, http.StatusOK, map[string]any{
			"user_id":             userID,
			"questrade_connected": hasQT,
			"ynab_connected":      hasYNAB,
		})
	}
}
