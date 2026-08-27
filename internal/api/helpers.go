package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/brymastr/questrade-ynab/internal/auth"
	"github.com/brymastr/questrade-ynab/internal/db"
	"github.com/brymastr/questrade-ynab/internal/questrade"
	"github.com/brymastr/questrade-ynab/internal/ynab"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// questradeClient builds a Questrade client from stored tokens, refreshing if the
// token looks locally expired, and persists rotated tokens back to the DB.
func questradeClient(store db.Store, userID string) (*questrade.Client, error) {
	tok, err := store.GetToken(userID, "questrade")
	if err != nil || tok == nil {
		return nil, fmt.Errorf("questrade not connected")
	}

	c := questrade.NewClientFromTokens(tok.RefreshToken, tok.AccessToken, tok.APIServer, tok.ExpiresAt)
	if c.IsTokenValid() {
		return c, nil
	}
	return refreshQuestradeToken(store, userID, tok.RefreshToken)
}

// refreshQuestradeToken exchanges the given refresh token for a new access token,
// persists the rotated tokens, and returns a client using them.
func refreshQuestradeToken(store db.Store, userID, refreshToken string) (*questrade.Client, error) {
	res, err := auth.RefreshQuestradeToken(refreshToken)
	if err != nil {
		return nil, fmt.Errorf("questrade token refresh: %w", err)
	}
	_ = store.UpsertToken(db.OAuthToken{
		UserID:       userID,
		Provider:     "questrade",
		AccessToken:  res.AccessToken,
		RefreshToken: res.RefreshToken,
		ExpiresAt:    res.ExpiresAt,
		APIServer:    res.APIServer,
	})
	return questrade.NewClientFromTokens(res.RefreshToken, res.AccessToken, res.APIServer, res.ExpiresAt), nil
}

// withQuestradeClient runs fn with a Questrade client. If Questrade rejects the
// access token with a 401 (which a purely local expiry check can't detect), it
// forces a token refresh and retries fn once.
func withQuestradeClient(store db.Store, userID string, fn func(*questrade.Client) error) error {
	c, err := questradeClient(store, userID)
	if err != nil {
		return err
	}
	err = fn(c)
	if !errors.Is(err, questrade.ErrUnauthorized) {
		return err
	}

	// Read the latest (possibly just-rotated) refresh token before retrying.
	tok, gerr := store.GetToken(userID, "questrade")
	if gerr != nil || tok == nil {
		return err
	}
	c, rerr := refreshQuestradeToken(store, userID, tok.RefreshToken)
	if rerr != nil {
		return rerr
	}
	return fn(c)
}

// ynabClient builds a YNAB client from stored tokens, refreshing if needed.
func ynabClient(store db.Store, userID, budgetID string) (*ynab.Client, error) {
	tok, err := store.GetToken(userID, "ynab")
	if err != nil || tok == nil {
		return nil, fmt.Errorf("ynab not connected")
	}

	if time.Now().After(tok.ExpiresAt) {
		res, err := auth.RefreshYNABToken(tok.RefreshToken)
		if err != nil {
			return nil, fmt.Errorf("ynab token refresh: %w", err)
		}
		_ = store.UpsertToken(db.OAuthToken{
			UserID:       userID,
			Provider:     "ynab",
			AccessToken:  res.AccessToken,
			RefreshToken: res.RefreshToken,
			ExpiresAt:    res.ExpiresAt,
		})
		tok.AccessToken = res.AccessToken
	}
	return ynab.NewClient(tok.AccessToken, budgetID), nil
}

// ynabClientForBudgets builds a budget-agnostic YNAB client.
func ynabClientForBudgets(store db.Store, userID string) (*ynab.Client, error) {
	return ynabClient(store, userID, "")
}
