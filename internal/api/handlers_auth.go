package api

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"

	"github.com/brymastr/questrade-ynab/internal/auth"
	"github.com/brymastr/questrade-ynab/internal/db"
)

// handleQuestradeConnect exchanges a manually-generated Questrade refresh token
// for an access token. Questrade personal apps do not support the interactive
// OAuth authorize/redirect flow, so the user pastes the refresh token issued by
// the Questrade App Hub instead.
func handleQuestradeConnect(store *db.Store) http.HandlerFunc {
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

		// Get or create user from existing session
		userID, _ := auth.GetSession(r)
		if userID == "" {
			userID, err = store.CreateUser()
			if err != nil {
				writeError(w, http.StatusInternalServerError, "create user failed")
				return
			}
			if err := auth.SetSession(w, userID); err != nil {
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

func handleYNABLogin(store *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Must already be authenticated (Questrade connected first)
		if _, err := auth.GetSession(r); err != nil {
			http.Redirect(w, r, os.Getenv("APP_URL"), http.StatusFound)
			return
		}
		redirectURL := auth.YNABAuthURL(w)
		http.Redirect(w, r, redirectURL, http.StatusFound)
	}
}

func handleYNABCallback(store *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, err := auth.GetSession(r)
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

func handleLogout(w http.ResponseWriter, r *http.Request) {
	auth.ClearSession(w)
	writeJSON(w, http.StatusOK, map[string]string{"status": "logged out"})
}

func handleMe(store *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := userIDFromCtx(r)
		hasQT, _ := store.HasToken(userID, "questrade")
		hasYNAB, _ := store.HasToken(userID, "ynab")
		writeJSON(w, http.StatusOK, map[string]any{
			"user_id":              userID,
			"questrade_connected": hasQT,
			"ynab_connected":      hasYNAB,
		})
	}
}
