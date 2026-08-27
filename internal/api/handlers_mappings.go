package api

import (
	"encoding/json"
	"net/http"

	"github.com/brymastr/questrade-ynab/internal/db"
	"github.com/go-chi/chi/v5"
)

func handleGetMappings(store *db.Store) http.HandlerFunc {
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

func handlePutMappings(store *db.Store) http.HandlerFunc {
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

func handleDeleteMapping(store *db.Store) http.HandlerFunc {
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
