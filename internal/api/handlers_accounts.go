package api

import (
	"net/http"

	"github.com/brymastr/questrade-ynab/internal/db"
	"github.com/brymastr/questrade-ynab/internal/questrade"
)

func handleQuestradeAccounts(store *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := userIDFromCtx(r)

		var accounts []questrade.Account
		err := withQuestradeClient(store, userID, func(qc *questrade.Client) error {
			a, e := qc.GetAccounts()
			if e != nil {
				return e
			}
			accounts = a
			return nil
		})
		if err != nil {
			writeError(w, http.StatusBadGateway, "failed to fetch questrade accounts: "+err.Error())
			return
		}

		type accountResponse struct {
			Number  string  `json:"number"`
			Type    string  `json:"type"`
			Balance float64 `json:"balance"`
		}
		var out []accountResponse
		for _, a := range accounts {
			var balance float64
			if a.Balances != nil && len(a.Balances.CombinedBalances) > 0 {
				balance = a.Balances.CombinedBalances[0].TotalEquity
			}
			out = append(out, accountResponse{
				Number:  a.Number,
				Type:    a.Type,
				Balance: balance,
			})
		}
		writeJSON(w, http.StatusOK, out)
	}
}

func handleYNABBudgets(store *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := userIDFromCtx(r)
		yc, err := ynabClientForBudgets(store, userID)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}

		budgets, err := yc.GetBudgets()
		if err != nil {
			writeError(w, http.StatusBadGateway, "failed to fetch ynab budgets: "+err.Error())
			return
		}
		writeJSON(w, http.StatusOK, budgets)
	}
}

func handleYNABAccounts(store *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := userIDFromCtx(r)
		budgetID := r.URL.Query().Get("budget_id")
		if budgetID == "" {
			writeError(w, http.StatusBadRequest, "budget_id query param required")
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

		// Only return open accounts
		type accountResponse struct {
			ID      string  `json:"id"`
			Name    string  `json:"name"`
			Type    string  `json:"type"`
			Balance float64 `json:"balance"`
		}
		var out []accountResponse
		for _, a := range accounts {
			if a.Closed {
				continue
			}
			out = append(out, accountResponse{
				ID:      a.ID,
				Name:    a.Name,
				Type:    a.Type,
				Balance: float64(a.Balance) / 1000,
			})
		}
		writeJSON(w, http.StatusOK, out)
	}
}
