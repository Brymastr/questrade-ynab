package api

import (
	"encoding/json"
	"net/http"

	"github.com/brymastr/questrade-ynab/internal/db"
	"github.com/brymastr/questrade-ynab/internal/match"
	"github.com/brymastr/questrade-ynab/internal/questrade"
	"github.com/go-chi/chi/v5"
)

func handleGetMappings(store db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := userIDFromCtx(r)
		mappings, err := store.GetMappings(userID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if mappings == nil {
			mappings = []db.Mapping{}
		}
		writeJSON(w, http.StatusOK, mappings)
	}
}

func handlePutMappings(store db.Store) http.HandlerFunc {
	type mappingInput struct {
		QuestradeAccountNumber string `json:"questrade_account_number"`
		YNABBudgetID           string `json:"ynab_budget_id"`
		YNABAccountID          string `json:"ynab_account_id"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		userID := userIDFromCtx(r)

		var inputs []mappingInput
		if err := json.NewDecoder(r.Body).Decode(&inputs); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}

		var mappings []db.Mapping
		for _, inp := range inputs {
			if inp.QuestradeAccountNumber == "" || inp.YNABBudgetID == "" || inp.YNABAccountID == "" {
				writeError(w, http.StatusBadRequest, "all fields required per mapping")
				return
			}
			mappings = append(mappings, db.Mapping{
				UserID:                 userID,
				QuestradeAccountNumber: inp.QuestradeAccountNumber,
				YNABBudgetID:           inp.YNABBudgetID,
				YNABAccountID:          inp.YNABAccountID,
			})
		}

		if err := store.ReplaceMappings(userID, mappings); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}

		saved, _ := store.GetMappings(userID)
		writeJSON(w, http.StatusOK, saved)
	}
}

func handleDeleteMapping(store db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := userIDFromCtx(r)
		mappingID := chi.URLParam(r, "id")
		if err := store.DeleteMapping(userID, mappingID); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// handleSuggestMappings guesses a YNAB account for each Questrade account from
// names alone. It never writes anything; the client shows the suggestions and
// the user confirms them through PUT /api/mappings.
func handleSuggestMappings(store db.Store) http.HandlerFunc {
	type suggestionResponse struct {
		match.Suggestion
		YNABBudgetID string `json:"ynab_budget_id"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		userID := userIDFromCtx(r)
		budgetID := r.URL.Query().Get("budget_id")
		if budgetID == "" {
			writeError(w, http.StatusBadRequest, "budget_id query param required")
			return
		}

		var qts []match.QuestradeAccount
		err := withQuestradeClient(store, userID, func(qc *questrade.Client) error {
			accounts, e := qc.GetAccounts()
			if e != nil {
				return e
			}
			for _, a := range accounts {
				qts = append(qts, match.QuestradeAccount{Number: a.Number, Type: a.Type})
			}
			return nil
		})
		if err != nil {
			writeError(w, http.StatusBadGateway, "failed to fetch questrade accounts: "+err.Error())
			return
		}

		yc, err := ynabClient(store, userID, budgetID)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		accounts, err := yc.GetAccounts()
		if err != nil {
			writeError(w, http.StatusBadGateway, "failed to fetch ynab accounts: "+err.Error())
			return
		}
		var ynabs []match.YNABAccount
		for _, a := range accounts {
			if a.Closed {
				continue
			}
			ynabs = append(ynabs, match.YNABAccount{ID: a.ID, Name: a.Name, Type: a.Type})
		}

		out := []suggestionResponse{}
		for _, s := range match.Suggest(qts, ynabs) {
			out = append(out, suggestionResponse{Suggestion: s, YNABBudgetID: budgetID})
		}
		writeJSON(w, http.StatusOK, out)
	}
}
